package main

import (
	"crypto/sha256"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCheckConfigChanged(t *testing.T) {
	rf := newRuleFetcher(nil, nil, "")

	test1 := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test1",
			Namespace: "test_namespace_1",
		},
	}

	test2 := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test2",
			Namespace: "test_namespace_1",
		},
	}

	test3 := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test3",
			Namespace: "test_namespace_3",
		},
	}

	cms := []*corev1.ConfigMap{&test1}
	changed, _ := rf.configMapsChanged(cms)
	assert.True(t, changed, "Configmap should be changed on new config.")
	rf.updateLastHash(cms)
	changed, _ = rf.configMapsChanged(cms)
	assert.False(t, changed, "Config should be unchanged.")

	cms = []*corev1.ConfigMap{&test2, &test1}
	changed, _ = rf.configMapsChanged(cms)
	assert.True(t, changed, "Configmap should be changed on new config.")
	rf.updateLastHash(cms)
	changed, _ = rf.configMapsChanged(cms)
	assert.False(t, changed, "Config should be unchanged.")

	cms = []*corev1.ConfigMap{&test3, &test1}
	changed, _ = rf.configMapsChanged(cms)
	assert.True(t, changed, "Configmap should be changed on new config.")
	rf.updateLastHash(cms)
	changed, _ = rf.configMapsChanged(cms)
	assert.False(t, changed, "Config should be unchanged.")
}

func TestUpdateLastHash(t *testing.T) {
	rf := newRuleFetcher(nil, nil, "")

	test1 := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test1",
			Namespace: "test_namespace_1",
		},
	}

	test1changed := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test1",
			Namespace: "test_namespace_1",
		},
		Data: map[string]string{
			"some_key": "some_value",
		},
	}

	test2 := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test2",
			Namespace: "test_namespace_1",
		},
	}

	test3 := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test3",
			Namespace: "test_namespace_3",
		},
	}

	cms := []*corev1.ConfigMap{&test1}
	rf.updateLastHash(cms)
	assert.Equal(t, 1, rf.lastConfigMapCount, "lastConfigMapCount should be 1")
	assert.NotEmpty(t, rf.lastHash[test1.Namespace], "test1 namespace should not be empty")
	test1bytes, _ := json.Marshal(test1)
	assert.Equal(t, sha256.Sum256(test1bytes), rf.lastHash[test1.Namespace][test1.Name], "Test 1 shasum incorrect")

	cms = []*corev1.ConfigMap{&test2, &test1}
	rf.updateLastHash(cms)
	assert.Equal(t, 2, rf.lastConfigMapCount, "lastConfigMapCount should be 1")
	assert.NotEmpty(t, rf.lastHash[test2.Namespace], "test1 namespace should not be empty")
	test2bytes, _ := json.Marshal(test2)
	assert.Equal(t, sha256.Sum256(test2bytes), rf.lastHash[test2.Namespace][test2.Name], "Test 2 shasum incorrect")

	cms = []*corev1.ConfigMap{&test3, &test1}
	rf.updateLastHash(cms)
	assert.Equal(t, 2, rf.lastConfigMapCount, "lastConfigMapCount should be 1")
	assert.NotEmpty(t, rf.lastHash[test3.Namespace], "test1 namespace should not be empty")
	test3bytes, _ := json.Marshal(test3)
	assert.Equal(t, sha256.Sum256(test3bytes), rf.lastHash[test3.Namespace][test3.Name], "Test 1 shasum incorrect")

	cms = []*corev1.ConfigMap{&test3, &test1changed}
	rf.updateLastHash(cms)
	assert.Equal(t, 2, rf.lastConfigMapCount, "lastConfigMapCount should be 1")
	test1changedbytes, _ := json.Marshal(test1changed)
	assert.Equal(t, sha256.Sum256(test1changedbytes), rf.lastHash[test1.Namespace][test1.Name], "Test 1 changed shasum incorrect")
}
