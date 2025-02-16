---
sidebar_position: 4
---

# CEEMS Tool

CEEMS tool is a utility tool that can be used to test, configure and validate
CEEMS related components. It is distributed along with pre-compiled binaries,
`ceems_api_server` DEB/RPM packages and CEEMS container.

All the available options from the `ceems_tool` can be listed using `--help`

```bash
ceems_tool --help
```

:::tip[TIP]

All the available command line options are listed in
[CEEMS Tool CLI docs](../cli/ceems-tool.md).

:::

## Prometheus Recording Rules

Grafana dashboards provided by CEEMS rely on
[recording rules](https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/)
feature of Prometheus to create new "derived" metrics from raw metrics exported by the
`ceems_exporter`. The advantage of using recording rules is that operators will have unified
view of compute unit metrics irrespective of their infrastructure.

Imagine a situation where a set of nodes in a cluster exposes the power usage with
IPMI DCMI and another set of nodes in the same cluster exposes power usage with Redfish. In this
case we need to write different queries depending on the target node in the Grafana dashboard
which can quickly become complicated and prohibitive. By using recording-rules we can create
new "dervied" metrics with same labels for all sets of nodes by taking into account differences
in their hardware while writing the rules.

Recording rules also helps to speed up the queries. For instance, in a typical case of estimating
power usage of a compute unit, we need to use multiple time series to get the power usage of a given
compute unit. Making such requests involving multiple time series with long durations can slow down
Prometheus response time. By using recording rules, there will be only one time series for each
metric and hence, fetching them would be much faster.

`ceems_tool` can be used to generate recording rules files that will create new series which will
be eventually used in the Grafana dashboards. Once the Prometheus has been configured to scrape the
target nodes, we can use `ceems_tool` as follows:

```bash
ceems_tool tsdb create-recording-rules --url=http://localhost:9090 --country-code=FR
```

:::important[IMPORTANT]

The above command `ceems_tool tsdb create-recording-rules` supports CLI options
`--start` and `--end` to use a start and end times, respectively. Use times where there
is a representative usage of the cluster. The tool will make few queries to auto detect
the metrics available. For instance, if there are no SLURM jobs or Openstack VMs that are
using GPUs between `--start` and `--end` times, the tool will not generate rule files
for GPU related metrics.

:::

The `--url` flag must point to Prometheus server. To include emission factor for appropriate country,
ISO2 country code can be passed to `--country-code` flag. This command will fetch all the CEEMS related
series in the provided Prometheus server and generates rules files in `rules` directory. If operators
would like to use their own custom static emission factor value, it can be passed using
`--emission-factor` CLI flag. However, it is not possible to set both `--country-code` and
`--emission-factor`.

:::note[NOTE]

If [Redfish Collector](../configuration/ceems-exporter.md#redfish-collector) is being used and it has
multiple chassis, the above will ask for user input on which chassis must be included in the
recording rules. Operators must choose the chassis that reports the host power usage.

:::

The generated rules files have comments explaining the rationale behind estimation of power usage
for individual compute units.

:::important[IMPORTANT]

The generated rules files should work in most of the cases. If operators would like to change the
power estimation method, they can modify the generated rules files manually to suit their
infrastructure.

:::

The generated recording rules must be added to Prometheus server config. On a default configuration,
copy these rules files to `/etc/prometheus/rules` and add the following configuration to Prometheus
configuration file:

```yml
rule_files:
  - "/etc/prometheus/ruls/*.rules"
```

By reloading/restarting Prometheus, new series defined in the recording rules files must be evaluated
and added to Prometheus server.

## CEEMS TSDB Updater Queries

Once the generated recording rules are added to Prometheus server, we can leverage the new "derived"
series to estimate the aggregate metrics of compute units that will be stored in CEEMS API server's DB.
[TSDB Updater](../components/ceems-api-server.md#updaters) is responsible for estimating the aggregate
metrics of each compute unit and updating the DB. As a part of
[TSDB Updater Config](../configuration/config-reference.md#updater_config), we need to configure the
queries to estimate aggregate metrics. This can be done using `ceems_tool` as follows:

```bash
ceems_tool tsdb create-ceems-tsdb-updater-queries --url=http://localhost:9090
```

The `--url` flag must point to Prometheus server. This will output the `queries` section of TSDB
updater config which must be added to `ceems_api_server`'s configuration file.
