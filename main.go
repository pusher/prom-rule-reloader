// Copyright 2016 The prometheus-operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/spf13/cobra"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	informersv1 "k8s.io/client-go/informers"
	"k8s.io/client-go/informers/internalinterfaces"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
)

var (
	cfgFile        string
	cfgSubstFile   string
	ruleDir        string
	ruleSelector   string
	reloadURLFlag  string
	reloadInterval time.Duration
)

var tweakOptions internalinterfaces.TweakListOptionsFunc = func(o *metav1.ListOptions) {
	o.LabelSelector = ruleSelector
}

func main() {
	cmd := newCommand()

	cmd.PersistentFlags().StringVar(&cfgFile, "config-file", "", "config file watched by the reloader")
	cmd.PersistentFlags().StringVar(&cfgSubstFile, "config-envsubst-file", "", "output file for environment variable substituted config file")
	cmd.PersistentFlags().StringVar(&ruleDir, "rule-dir", "/etc/rules", "rule directory for the loaded rules")
	cmd.PersistentFlags().StringVar(&ruleSelector, "rule-selector", "app=prometheus,component=rules", "label selector for prometheus rules")
	cmd.PersistentFlags().StringVar(&reloadURLFlag, "reload-url", "http://127.0.0.1:9090/-/reload", "reload URL to trigger Prometheus reload on")
	cmd.PersistentFlags().DurationVar(&reloadInterval, "reload-interval", 10*time.Second, "interval between reloading rules")

	cmd.Flags().AddGoFlagSet(flag.CommandLine)

	// Log to stderr by default and fix usage message accordingly
	logToStdErr := cmd.Flags().Lookup("logtostderr")
	logToStdErr.DefValue = "true"
	cmd.Flags().Set("logtostderr", "true")

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "prom-rule-reloader",
		Short: "Prom Rule Reloader aggregates configmaps into prometheus rules",
		Long:  `Prom Rule Reloader aggregates configmaps into prometheus rules`,
		RunE: func(cmd *cobra.Command, args []string) error {
			glog.V(2).Infof("Starting prom-rule-reloader")

			if err := os.MkdirAll(ruleDir, 0777); err != nil {
				return err
			}

			config, err := rest.InClusterConfig()
			if err != nil {
				return err
			}

			client, err := kubernetes.NewForConfig(config)
			if err != nil {
				return err
			}

			ctx := context.Background()
			tick := time.NewTicker(reloadInterval)
			defer tick.Stop()

			glog.V(4).Infof("Polling every %s", reloadInterval.String())

			sharedInformer := informersv1.NewFilteredSharedInformerFactory(client, reloadInterval, "", tweakOptions)
			go sharedInformer.Core().V1().ConfigMaps().Informer().Run(nil)
			cmLister := sharedInformer.Core().V1().ConfigMaps().Lister()

			rfet := newRuleFetcher(client, cmLister, ruleDir)

			for {
				select {
				case <-tick.C:
					if err := rfet.Refresh(ctx); err != nil {
						glog.Errorf("failed to update rules: %v", err)
					}
				case <-ctx.Done():
					return nil
				}
			}
		},
	}
	return rootCmd
}

type ruleFetcher struct {
	client   *kubernetes.Clientset
	cmLister corev1.ConfigMapLister
	outDir   string

	lastHash           map[string]map[string][sha256.Size]byte
	lastConfigMapCount int
}

func newRuleFetcher(client *kubernetes.Clientset, cmLister corev1.ConfigMapLister, outDir string) *ruleFetcher {
	return &ruleFetcher{
		client:   client,
		cmLister: cmLister,
		outDir:   outDir,
		lastHash: make(map[string]map[string][sha256.Size]byte),
	}
}

func (rf *ruleFetcher) Refresh(ctx context.Context) error {
	selector := labels.NewSelector()
	cms, err := rf.cmLister.List(selector)
	if err != nil {
		return err
	}
	glog.V(4).Infof("Found %d configmaps.", len(cms))

	changed, err := rf.configMapsChanged(cms)
	if err != nil {
		return fmt.Errorf("couldn't determine if configmaps changed: %v", err)
	}
	if !changed {
		glog.V(3).Infof("Config unchanged")
		return nil
	}

	glog.V(2).Infof("Config changed. Updating rules.")
	if err = rf.refresh(ctx, cms); err != nil {
		return fmt.Errorf("couldn't refresh rules: %v", err)
	}

	resp, err := http.Post(reloadURLFlag, "", nil)
	if err != nil {
		return fmt.Errorf("error reloading prometheus: %v", err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("couldn't reload prometheus: status %d", resp.StatusCode)
	}

	glog.V(2).Infof("Reloaded prometheus config.")

	err = rf.updateLastHash(cms)
	if err != nil {
		return fmt.Errorf("couldn't update last hash: %v", err)
	}
	return nil
}

func (rf *ruleFetcher) refresh(ctx context.Context, cms []*v1.ConfigMap) error {
	tmpdir := rf.outDir + ".tmp"

	if err := os.MkdirAll(tmpdir, 0777); err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)

	for _, cm := range cms {
		glog.V(4).Infof("Processing %s/%s", cm.ObjectMeta.Namespace, cm.ObjectMeta.Name)
		for fn, content := range cm.Data {
			if _, errs := rulefmt.Parse([]byte(content)); len(errs) > 0 {
				glog.Errorf("Skipping invalid rule file: %s/%s/%s: %v", cm.ObjectMeta.Namespace, cm.ObjectMeta.Name, fn, errs)
				continue
			}

			fp := filepath.Join(tmpdir, fmt.Sprintf("%s_%s_%s", cm.ObjectMeta.Namespace, cm.ObjectMeta.Name, fn))
			glog.V(6).Infof("Writing file %s", fp)
			if err := ioutil.WriteFile(fp, []byte(content), 0666); err != nil {
				return err
			}
		}
	}

	glog.V(6).Infof("Overwriting existing rules")
	if err := os.RemoveAll(rf.outDir); err != nil {
		return err
	}
	return os.Rename(tmpdir, rf.outDir)
}

func (rf *ruleFetcher) configMapsChanged(cms []*v1.ConfigMap) (bool, error) {
	if rf.lastConfigMapCount != len(cms) {
		// A configmap has been added or removed
		return true, nil
	}
	for _, cm := range cms {
		lastHash, ok := rf.lastHash[cm.Namespace][cm.Name]
		if !ok {
			// ConfigMap is new
			return true, nil
		}

		b, err := json.Marshal(cm)
		if err != nil {
			return false, fmt.Errorf("couldn't marshal configmap: %v", err)
		}

		h := sha256.Sum256(b)
		if lastHash != h {
			// ConfigMap has changed
			return true, nil
		}
	}
	return false, nil
}

func (rf *ruleFetcher) updateLastHash(cms []*v1.ConfigMap) error {
	for _, cm := range cms {
		b, err := json.Marshal(cm)
		if err != nil {
			return fmt.Errorf("couldn't marshal configmap: %v", err)
		}
		h := sha256.Sum256(b)

		if _, ok := rf.lastHash[cm.Namespace]; !ok {
			rf.lastHash[cm.Namespace] = make(map[string][sha256.Size]byte)
		}

		rf.lastHash[cm.Namespace][cm.Name] = h
	}
	rf.lastConfigMapCount = len(cms)
	return nil
}
