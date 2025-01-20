---
sidebar_position: 3
---

# CEEMS API Server

CEEMS API server can be configured using a YAML file under the key `ceems_api_server`.
Along with the base configuration of the server, we need to provide the configurations
of clusters that we need to monitor and updaters that will be used to update the
compute unit metrics. They can be configured under keys `clusters` and `updaters`,
respectively. Thus a bare configuration file looks as below:

```yaml
# CEEMS API Server configuration skeleton

ceems_api_server: <CEEMS API SERVER CONFIG>

clusters: <CLUSTERS CONFIG>

updaters: <UPDATERS CONFIG>
```

A complete reference on CEEMS API server configuration can be found in [Reference](./config-reference.md)
section. A valid sample configuration
file can be found in the [repo](https://github.com/mahendrapaipuri/ceems/blob/main/build/config/ceems_api_server/ceems_api_server.yml)

## CEEMS API Server Configuration

This section guides on how to configure CEEMS API server. A sample configuration
file is shown below:

```yaml
ceems_api_server:
  data:
    path: /path/to/ceems/data
    update_interval: 15m
    retention_period: 1y
    backup_path: /path/to/backup/ceems/data
    backup_interval: 1d

  admin:
    users:
      - adm1
      - adm2
    grafana:
      url: https://grafana.example.com
      teams_ids:
        - 1
      authorization:
        type: Bearer
        credentials: mysupersecretgrafanaservicetoken
  
  web:
    max_query: 30d
    requests_limit: 30
    route_prefix: /ceems/
```

The configuration for `ceems_api_server` has three sections namely, `data`, `admin` and `web`
for configuring different aspects of the API server. Some explanation about the `data`
config is discussed below:

- `data.path`: Path where all CEEMS related data will be stored.
- `data.update_interval`: The frequency at which CEEMS API server will fetch compute units
from the underlying cluster. Do not use too small intervals or high frequency. `15m` is a
sane default and it should work in most of the production cases.
- `data.retention_period`: CEEMS API server stores all the meta data of compute units along
with their aggregated metrics in a SQLite relational DB. This config parameter can be used
to configure the retention time of the compute unit data in the SQLite. For example, when
a value of `1y` is used, it means all the compute units data in the last one year will be
retained and the rest of the units data will be purged.
- `data.backup_path`: It is possible to create backups of SQLite DB at a configured interval
set by `data.backup_interval` onto a fault tolerant storage.

:::warning[WARNING]

As DB grows in size, time taken for creating a backup increases non-linearly
and hence, use the back up option only if it is absolutely needed. A general
advice is to use a continuous backup solution like
[litestream](https://litestream.io/) instead of native backup solution offered
by CEEMS

:::

CEEMS API server exposes admin endpoints in its API and the `admin` section can be used to
configure which users can access those endpoints. More details on admin endpoints can be
consulted from the [API Docs](https://mahendrapaipuri.github.io/ceems/api).

- `admin.users`: A list of statically defined users that will have access to admin endpoints.
- `admin.grafana`: Besides a static list of users, CEEMS API server can pull users from given
Grafana teams to add them to list of users that will be granted access to admin endpoints. This
enables to dynamically add users to admin users list for CEEMS without having to re-configure
and restart CEEMS API server. This section allows to provide the client configuration of
Grafana. All possible client configuration options can be consulted in the
[Config Reference](./config-reference.md#grafana-config).

Finally, the section `web` can be used to configured HTTP server of CEEMS API server.

- `web.max_query`: Maximum allowable query period. Configure this value appropriately
based on the needs as queries with too longer period can put considerable amount of
pressure on DB queries.
- `web.requests_limit`: Maximum number of requests per minute per client identified by
remote IP address.
- `web.route_prefix`: All the CEEMS API end points will be prefixed by this value. It
is useful when serving CEEMS API server behind a reverse proxy at a given path.

## Clusters Configuration

A sample clusters configuration section is shown as below:

```yaml
clusters:
  - id: slurm-0
    manager: slurm
    updaters:
      - tsdb-0
      - tsdb-1
    cli: 
      path: path/to/slurm/bin

  - id: os-0
    manager: openstack
    updaters:
      - tsdb-0
    web: 
      http_headers:
        X-OpenStack-Nova-API-Version:
          values:
            - latest
    extra_config:
      api_service_endpoints:
        compute: https://openstack-nova.example.com/v2.1
        identity: https://openstack-keystone.example.com
      auth:
        methods:
          - password
        password:
          user:
            name: admin
            password: supersecret
```

Essentially it is a list of objects where each object describes a cluster.

- `id`: A unique identifier for each cluster. The identifier must stay consistent across
CEEMS components, especially for CEEMS LB. More details can be found in
[Configuring CEEMS LB](./ceems-lb.md) section.
- `manager`: Resource manager kind. Currently only `slurm` and `openstack` are
supported.
- `updaters`: List of updaters to be used to update the aggregate metrics of the
compute units. The order is important as compute units are updated in the same order
as provided here. For example, using the current sample file, it is important for the
operators to ensure that we do not override the metrics updated by `tsdb-0` by `tsdb-1`.
More details on updaters can be found in [Updaters Configuration](#updaters-configuration).
- `cli`: If the resource manager uses CLI tools to fetch compute units, configuration related
to those CLI tools can be provided here. For example, currently CEEMS API server supports
fetching SLURM jobs only using `sacct` command and hence, it is essential to provide the
path to `bin` folder where `sacct` command will be found. More options on CLI section can
be found in [Cluster Configuration Reference](./config-reference.md#cluster_config).
- `web`: If the resource manager supports fetching compute units using API, the client
configuration to access API endpoints can be provided here. In this particular example,
the Openstack cluster's authentication is configured using `web.http_headers` section.
All available options for the `web` configuration can be found in
[Web Client Configuration Reference](./config-reference.md#web_client_config).
- `extra_config`: Any extra configuration required by a particular resource manager can be
provided here. Currently, Openstack resource manager uses this section to configure the API
URLs for compute and identity servers to fetch compute units, users and projects data.

### SLURM specific clusters configuration

As stated before, currently fetching SLURM jobs using `sacct` command is the only supported
way. If the `sacct` binary is available on `PATH`, there is no need to provide any specific
configuration. However, if the binary is present on non-standard location, it is necessary to
provide the path to the binary using `cli` section of the config. For example, if the absolute
path of `sacct` is `/opt/slurm/bin/sacct`, then we need to configure `cli` section as follows:

```yaml
cli:
  path: /opt/slurm/bin
```

A minimal full cluster configuration would be:

```yaml
clusters:
  - id: slurm-0
    manager: slurm
    cli: 
      path: /opt/slurm/bin
```

The section `cli` also has `environment_variables` key to provide any environment variables
while executing `sacct` command in a sub-process. This section takes key value as values:

```yaml
clusters:
  - id: slurm-0
    manager: slurm
    cli: 
      path: /opt/slurm/bin
      environment_variables:
        ENVVAR_NAME: ENVVAR_VALUE
```

### Openstack specific clusters configuration

In the case of Openstack, `extra_config` section must be used to setup Openstack's API
and auth configs. The following keys in `extra_config` must be provided:

- `api_service_endpoints`: This section must provide the API endpoints for compute and
identity services.
- `auth`: This is the same auth object that needs to be passed to Openstack's identity
service to get an API token. More details can be found in [Keystone's API docs](https://docs.openstack.org/api-ref/identity/v3/#authentication-and-token-management).

An example that provides password auth method is shown below:

```yaml
extra_config:
  api_service_endpoints:
    compute: https://openstack-nova.example.com/v2.1
    identity: https://openstack-keystone.example.com
  auth:
    identity:
      methods:
        - password
      password:
        user:
          name: admin
          password: supersecret
```

Similarly, the following example shows on how to use application credentials:

```yaml
extra_config:
  api_service_endpoints:
    compute: https://openstack-nova.example.com/v2.1
    identity: https://openstack-keystone.example.com
  auth:
    identity:
      methods:
        - application_credential
      application_credential:
        id: 21dced0fd20347869b93710d2b98aae0
        secret: supersecret
```

:::important[IMPORTANT]

It is important to configure the compute and identity API URLs as displayed by the
service catalog in the Openstack cluster. For instance, CEEMS API server supports
only identity API version `v3` and it adds `v3` to URL path when making API requests.
However, it expects the configured API URL for compute contains the API version `v2.1`
as shown in the above config.

:::

:::note[NOTE]

Admin level privileges must be available for configured auth object as CEEMS API server
needs to fetch the instances of **all** tenants and projects and it is only possible
with admin scope.

:::

It is advised to use application credentials instead of Admin password as it is
possible to scope the usage of application credentials to only compute and
identity services whereas admin account will give unrestricted access to all
cluster level resources. More details on how to create application credentials with
scopes can be found in [Keystone's docs](https://docs.openstack.org/keystone/latest/user/application_credentials.html).

Openstack Nova (compute) uses micro versions for API and by default, CEEMS API
server uses the latest supported micro version. If a specific micro version is
desired it can be configured using `web.http_headers` section as follows:

```yaml
web: 
  http_headers:
    X-OpenStack-Nova-API-Version:
      values:
        - 2.12
```

A sample full clusters config for Openstack is shown as below:

```yaml
clusters:
  - id: os-0
    manager: openstack
    web: 
      http_headers:
        X-OpenStack-Nova-API-Version:
          values:
            - latest
    extra_config:
      api_service_endpoints:
        compute: https://openstack-nova.example.com/v2.1
        identity: https://openstack-keystone.example.com
      auth:
        identity:
          methods:
            - password
          password:
            user:
              name: admin
              password: supersecret
```

## Updaters Configuration

A sample updater config is shown below:

```yaml
updaters:
  - id: tsdb-0
    updater: tsdb
    web:
      url: http://localhost:9090
    extra_config:
      cutoff_duration: 5m
      delete_ignored: true
      queries:
        # Average CPU utilisation
        avg_cpu_usage: 
          global: |
            avg_over_time(
              avg by (uuid) (
                (
                  irate(ceems_compute_unit_cpu_user_seconds_total{uuid=~"{{.UUIDs}}"}[{{.RateInterval}}])
                  +
                  irate(ceems_compute_unit_cpu_system_seconds_total{uuid=~"{{.UUIDs}}"}[{{.RateInterval}}])
                )
                /
                ceems_compute_unit_cpus{uuid=~"{{.UUIDs}}"}
              )[{{.Range}}:{{.ScrapeInterval}}]
            ) * 100
```

Similar to `clusters`, `updaters` is also a list of objects where each object
describes an `updater`. Currently only **TSDB** updater is supported to update
compute units metrics from PromQL compliant TSDB server like Prometheus, Victoria
Metrics.

- `id`: A unique identifier for the updater. This identifier must be used in
`updaters` section of `clusters` as shown in [Clusters Configuration](#clusters-configuration)
section.
- `updater`: Name of the updater. Currently only `tsdb` is allowed.
- `web`: Web client configuration of updater server.
- `extra_config`: The `extra_config` allows to further configure TSDB.
  - `extra_config.cutoff_duration`: The time series data of compute units that have
    total elapsed time less than this period will be marked as ignored in CEEMS API
    server DB.
  - `extra_config.delete_ignored`: The compute units' labels that are marked as ignored
    based on `extra_config.cutoff_duration` will be purged from TSDB to decrease
    cardinality. This is useful to remove time series data of failed compute units
    or compute units that lasted very short duration and in-turn keep cardinality of
    TSDB under check. For this feature to work, Prometheus needs to be started with
    `--web.enable-admin-api` CLI flag that enabled admin API endpoints
  - `extra_config.queries`: This defines the queries to be made to TSDB to estimate
    the aggregate metrics of each compute unit. The example config shows the query
    to estimate average CPU usage of the compute unit. All the supported queries can
    be consulted from the [Updaters Configuration Reference](./config-reference.md#updater_config).

## Examples

The following configuration shows a basic config needed to fetch batch jobs from
SLURM resource manager

```yaml
ceems_api_server:
  data:
    path: /var/lib/ceems
    update_interval: 15m

  admin:
    users:
      - adm1
  
  web:
    requests_limit: 30

clusters:
  - id: slurm-0
    manager: slurm
    cli: 
      path: /usr/bin
```

Both SLURM and openstack clusters can be monitored using a single instance of
CEEMS API server using a config as below:

```yaml
ceems_api_server:
  data:
    path: /var/lib/ceems
    update_interval: 15m

  admin:
    users:
      - adm1
  
  web:
    requests_limit: 30

clusters:
  - id: slurm-0
    manager: slurm
    cli: 
      path: /usr/bin

  - id: os-0
    manager: openstack
    web: 
      http_headers:
        X-OpenStack-Nova-API-Version:
          values:
            - latest
    extra_config:
      api_service_endpoints:
        compute: https://openstack-nova.example.com/v2.1
        identity: https://openstack-keystone.example.com
      auth:
        methods:
          - password
        password:
          user:
            name: admin
            password: supersecret
```

Assuming CEEMS exporter is deployed on the compute nodes of both SLURM
and Openstack clusters and metrics are scrapped by a Prometheus running
at `https://prometheus.example.com`, we can add updater config to
the above examples as follows:

```yaml
ceems_api_server:
  data:
    path: /var/lib/ceems
    update_interval: 15m

  admin:
    users:
      - adm1
  
  web:
    requests_limit: 30

clusters:
  - id: slurm-0
    manager: slurm
    updaters:
      - tsdb-0
    cli: 
      path: /usr/bin

  - id: os-0
    manager: openstack
    updaters:
      - tsdb-0
    web: 
      http_headers:
        X-OpenStack-Nova-API-Version:
          values:
            - latest
    extra_config:
      api_service_endpoints:
        compute: https://openstack-nova.example.com/v2.1
        identity: https://openstack-keystone.example.com
      auth:
        methods:
          - password
        password:
          user:
            name: admin
            password: supersecret

updaters:
  - id: tsdb-0
    updater: tsdb
    web:
      url: http://tsdb-0
    extra_config:
      cutoff_duration: 5m
      queries:
        # Average CPU utilisation
        avg_cpu_usage: 
          global: |
            avg_over_time(
              avg by (uuid) (
                (
                  irate(ceems_compute_unit_cpu_user_seconds_total{uuid=~"{{.UUIDs}}"}[{{.RateInterval}}])
                  +
                  irate(ceems_compute_unit_cpu_system_seconds_total{uuid=~"{{.UUIDs}}"}[{{.RateInterval}}])
                )
                /
                ceems_compute_unit_cpus{uuid=~"{{.UUIDs}}"}
              )[{{.Range}}:{{.ScrapeInterval}}]
            ) * 100

        # Avgerage CPU Memory utilisation
        avg_cpu_mem_usage:
          global: |
            avg_over_time(
              avg by (uuid) (
                ceems_compute_unit_memory_used_bytes{uuid=~"{{.UUIDs}}"}
                /
                ceems_compute_unit_memory_total_bytes{uuid=~"{{.UUIDs}}"}
              )[{{.Range}}:{{.ScrapeInterval}}]
            ) * 100

        # Total CPU energy usage in kWh
        total_cpu_energy_usage_kwh:
          total: |
            sum_over_time(
              sum by (uuid) (
                unit:ceems_compute_unit_cpu_energy_usage:sum{uuid=~"{{.UUIDs}}"} * {{.ScrapeIntervalMilli}} / 3.6e9
              )[{{.Range}}:{{.ScrapeInterval}}]
            )

        # Total CPU emissions in gms
        total_cpu_emissions_gms:
          rte_total: |
            sum_over_time(
              sum by (uuid) (
                label_replace(
                  unit:ceems_compute_unit_cpu_energy_usage:sum{uuid=~"{{.UUIDs}}"} * {{.ScrapeIntervalMilli}} / 3.6e9,
                  "common_label",
                  "mock",
                  "hostname",
                  "(.*)"
                )
                * on (common_label) group_left ()
                label_replace(
                  ceems_emissions_gCo2_kWh{provider="rte",country_code="FR"},
                  "common_label",
                  "mock",
                  "hostname",
                  "(.*)"
                )
              )[{{.Range}}:{{.ScrapeInterval}}]
            )

          emaps_total: |
            sum_over_time(
              sum by (uuid) (
                label_replace(
                  unit:ceems_compute_unit_cpu_energy_usage:sum{uuid=~"{{.UUIDs}}"} * {{.ScrapeIntervalMilli}} / 3.6e9,
                  "common_label",
                  "mock",
                  "hostname",
                  "(.*)"
                )
                * on (common_label) group_left ()
                label_replace(
                  ceems_emissions_gCo2_kWh{provider="emaps",country_code="FR"},
                  "common_label",
                  "mock",
                  "hostname",
                  "(.*)"
                )
              )[{{.Range}}:{{.ScrapeInterval}}]
            )

          owid_total: |
            sum_over_time(
              sum by (uuid) (
                label_replace(
                  unit:ceems_compute_unit_cpu_energy_usage:sum{uuid=~"{{.UUIDs}}"} * {{.ScrapeIntervalMilli}} / 3.6e9,
                  "common_label",
                  "mock",
                  "hostname",
                  "(.*)"
                )
                * on (common_label) group_left ()
                label_replace(
                  ceems_emissions_gCo2_kWh{provider="owid",country_code="FR"},
                  "common_label",
                  "mock",
                  "hostname",
                  "(.*)"
                )
              )[{{.Range}}:{{.ScrapeInterval}}]
            )

        # Average GPU utilization
        avg_gpu_usage:
          global: |
            avg_over_time(
              avg by (uuid) (
                DCGM_FI_DEV_GPU_UTIL
                * on (gpuuuid) group_right ()
                ceems_compute_unit_gpu_index_flag{uuid=~"{{.UUIDs}}"}
              )[{{.Range}}:{{.ScrapeInterval}}]
            )

        # Average GPU memory utilization
        avg_gpu_mem_usage:
          global: |
            avg_over_time(
              avg by (uuid) (
                DCGM_FI_DEV_MEM_COPY_UTIL
                * on (gpuuuid) group_right ()
                ceems_compute_unit_gpu_index_flag{uuid=~"{{.UUIDs}}"}
              )[{{.Range}}:{{.ScrapeInterval}}]
            )

        # Total GPU energy usage in kWh
        total_gpu_energy_usage_kwh:
          total: |
            sum_over_time(
              sum by (uuid) (
                instance:DCGM_FI_DEV_POWER_USAGE:pue_avg * {{.ScrapeIntervalMilli}} / 3.6e9
                * on (gpuuuid) group_right()
                ceems_compute_unit_gpu_index_flag{uuid=~"{{.UUIDs}}"}
              )[{{.Range}}:{{.ScrapeInterval}}]
            )

        # Total GPU emissions in gms
        total_gpu_emissions_gms:
          rte_total: |
            sum_over_time(
              sum by (uuid) (
                label_replace(
                  instance:DCGM_FI_DEV_POWER_USAGE:pue_avg * {{.ScrapeIntervalMilli}} / 3.6e+09
                  * on (gpuuuid) group_right ()
                  ceems_compute_unit_gpu_index_flag{uuid=~"{{.UUIDs}}"},
                  "common_label",
                  "mock",
                  "instance",
                  "(.*)"
                )
                * on (common_label) group_left ()
                label_replace(
                  ceems_emissions_gCo2_kWh{provider="rte",country_code="FR"},
                  "common_label",
                  "mock",
                  "instance",
                  "(.*)"
                )
              )[{{.Range}}:{{.ScrapeInterval}}]
            )

          emaps_total: |
            sum_over_time(
              sum by (uuid) (
                label_replace(
                  instance:DCGM_FI_DEV_POWER_USAGE:pue_avg * {{.ScrapeIntervalMilli}} / 3.6e+09
                  * on (gpuuuid) group_right ()
                  ceems_compute_unit_gpu_index_flag{uuid=~"{{.UUIDs}}"},
                  "common_label",
                  "mock",
                  "instance",
                  "(.*)"
                )
                * on (common_label) group_left ()
                label_replace(
                  ceems_emissions_gCo2_kWh{provider="emaps",country_code="FR"},
                  "common_label",
                  "mock",
                  "instance",
                  "(.*)"
                )
              )[{{.Range}}:{{.ScrapeInterval}}]
            )

          owid_total: |
            sum_over_time(
              sum by (uuid) (
                label_replace(
                  instance:DCGM_FI_DEV_POWER_USAGE:pue_avg * {{.ScrapeIntervalMilli}} / 3.6e+09
                  * on (gpuuuid) group_right ()
                  ceems_compute_unit_gpu_index_flag{uuid=~"{{.UUIDs}}"},
                  "common_label",
                  "mock",
                  "instance",
                  "(.*)"
                )
                * on (common_label) group_left ()
                label_replace(
                  ceems_emissions_gCo2_kWh{provider="owid",country_code="FR"},
                  "common_label",
                  "mock",
                  "instance",
                  "(.*)"
                )
              )[{{.Range}}:{{.ScrapeInterval}}]
            )
```

The above configuration assumes that GPU compute nodes possess NVIDIA GPUs and
[dcgm-exporter](https://github.com/NVIDIA/dcgm-exporter) is running along CEEMS
exporter to export metrics of GPUs.
