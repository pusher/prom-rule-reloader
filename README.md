# Prometheus Rule Reloader

Prometheus Rule Reloader aims to aggregate Prometheus recording and alerting rules from multiple Kubernetes ConfigMaps
into a single configuration file for Prometheus.


## What problem does it solve?

Prometheus requires all recording and alerting rules to be specified in "rule_files" it parses.
When running on Kubernetes, these are usually stored in ConfigMaps inside the Prometheus namespace which are mounted into the Prometheus Pods at runtime.

If a Prometheus instance is meant to scrape targets across multiple namespaces, owned by multiple teams, this poses a problem if those owners wish
to manage their own rules.
They will either need to have access to the Prometheus namespace and edit the ConfigMaps directly or ask the team managing Prometheus to make the changes for them.

In order to work around this, Prometheus Rule Reloader can aggregate multiple ConfigMaps from multiple namespaces into a single Prometheus configuration.
This allows namespace owners to manage their Prometheus rules alongside their workloads inside the namespace they usually manage.
It also enables them to use whatever tooling they normally use to manage their deployments.


## How does it work?

Prometheus Rule Reloader is meant to run as a sidecar container inside the Prometheus Pod, sharing its filesystem.
It will find all ConfigMaps across the cluster carrying a particular set of labels (e.g. `app=prometheus,component=rules`).

Each entry in the ConfigMap so identified will be parsed using the Prometheus rule parser to ensure it represents a valid rules file.
All valid entries are then written to the shared filesystem (e.g. to `/etc/rules`).
If rule changes are detected, the Reloader will trigger a Prometheus config reload to make them take effect.

Prometheus needs to be configured to scan this location for rules files using the `rule_files` directive of the [config file](https://prometheus.io/docs/prometheus/latest/configuration/configuration/).


## Usage

### Prometheus Configuration

In order for Prometheus to pick up the rules written by the Reloader, add a `rule_files:` entry to the configuration that points to a location within the Pod filesystem (e.g. `/etc/rules`):

```
    rule_files:
      - /etc/rules/rules/*.rules
```

In your Prometheus PodSpec add an `emptyDir` volume and mount it at that location, then add the Reloader as a separate container:

```
[...]
  spec:
    containers:
    - image: prom/prometheus:v2.2.1
      args:
        [...]
      volumeMounts:
      - mountPath: /etc/rules
        name: rules-volume
      [...]
    - image: quay.io/pusher/prom-rule-reloader:v0.1.2
      args:
      - --rule-dir=/etc/rules/rules
      - --rule-selector=app=prometheus,component=rules
      - -v=2
      [...]
      volumeMounts:
      - mountPath: /etc/rules
        name: rules-volume
    volumes:
    - emptyDir: {}
      name: rules-volume
[...]

```


## Command Line Options

```
  --rule-dir          The directory to which aggregated rules files will be written (default: /etc/rules)
  --rule-selector     The labelSelector used to identify ConfigMaps that should be aggregated (default: app=prometheus,component=rules)
  --reload-url        Prometheus URL to trigger a reload (default: http://127.0.0.1:9090/-/reload)
  --reload-interval   The time interval between polls for ConfigMap changes
```


# Contributing

Pull Requests are welcome. For major changes, please open an issue first to discuss what you would like to change.

Please make sure to update tests as appropriate

# License
[Apache 2](./LICENSE)
