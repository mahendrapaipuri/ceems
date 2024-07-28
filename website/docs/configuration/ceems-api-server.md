---
sidebar_position: 3
---

# CEEMS API Server

The following shows the reference for CEEMS API server config. A valid sample configuration 
file can be found in the [repo](https://github.com/mahendrapaipuri/ceems/blob/main/build/config/ceems_api_server/ceems_api_server.yml)

```yaml
# Configuration file to configure CEEMS API server
#
# This config file has following sections:
#  - `ceems_api_server`: Core configuration of CEEMS API server
#  - `clusters`: Configuration of clusters that are being monitored
#  - `updaters`: Configuration of updaters
#
---
# CEEMS API Server and data config
ceems_api_server:
  # Data related configuration of the CEEMS API server. This config concerns with the 
  # locations where data will be saved, frequency of data updates, etc.
  #
  data:
    [ <data_config> ]

  # HTTP web admin related config for CEEMS API server
  #
  admin:
    [ <admin_config> ]

  # HTTP web related config for CEEMS API server.
  #
  web:
    # Maximum allowable query range, ie, the difference between `from` and `to` query 
    # parameters. 
    #
    # This can be used to restrict the query range made by the users to avoid OOM errors
    # when handling too much data.
    #
    # Default value `0s` means no restrictions are imposed on the query.
    #
    # Units Supported: y, w, d, h, m, s, ms.
    #
    [ max_query: <duration> | default: 0s ]

    # Number of requests allowed in ONE MINUTE per client identified by Real IP address.
    # Request headers `True-Client-IP`, `X-Real-IP` and `X-Forwarded-For` are looked up
    # to get the real IP address.
    #
    # This is to effectively impose a rate limit for the entire CEEMS server irrespective
    # of URL path. We advise to set it to a value based on your needs to avoid DoS/DDoS
    # attacks.
    #
    # Rate limiting is done using the Sliding Window Counter pattern inspired by 
    # CloudFlare https://blog.cloudflare.com/counting-things-a-lot-of-different-things/
    #
    # Default value `0` means no rate limiting is applied.
    #
    [ requests_limit: <int> | default: 0 ]

    # It will be used to prefix all HTTP endpoints served by CEEMS API server. 
    # For example, if CEEMS API server is served via a reverse proxy. 
    # 
    # Default is '/'
    #
    [ route_prefix: <path> | default: / ]

# A list of clusters from which CEEMS API server will fetch the compute units.
# 
# Each cluster must provide an unique `id`. The `id` will enable CEEMS to identify 
# different clusters in multi-cluster setup. This `id` must be consistent throughout 
# all the CEEMS components.
# 
clusters:
  [ - <cluster_config> ... ]
  
    
# A list of Updaters that will be used to update the compute unit metrics. This update 
# step can be used to update the aggregate metrics of each compute unit in real time
# or to add complementary information to the compute units from on-premise third 
# party services.
#
# Currently only TSDB updater is supported. The compute unit aggregate metrics can be
# updated from TSDB (Prometheus/VM) instances.
#
updaters:
  [ - <updater_config> ... ]
  
```

## `<data_config>`

A `data_config` allows configuring the DB settings of CEEMS API server.

```yaml
# Path at which CEEMS API server data will be stored.
# If relative path is used, it will be resolved based on the current working directory.
#
[ path: <filename> | default = data ]

# Units data will be fetched at this interval. CEEMS will pull the units from the 
# underlying resource manager at this frequency into its own DB.
#
# Units Supported: y, w, d, h, m, s, ms.
#
[ update_interval: <duration> | default = 15m ]

# The duration to retain the data in the DB. Units older than this duration will be
# purged from the DB. 
#
# In the case of global usage stats, if the last activity on a given project/user 
# combination is older than this period, those stats will be purged from the DB.
#
# Units Supported: y, w, d, h, m, s, ms.
#
[ retention_period: <duration> | default = 30d ]

# CEEMS API server is capable of creating DB backups using SQLite backup API. Created
# DB backups will be saved to this path. NOTE that for huge DBs, this backup can take 
# a considerable amount of time. 
#
# Use a different disk device than `ceems_api_server.data.path` to achieve 
# fault tolerance.
#
# If the path is empty, no backups will be created.
#
[ backup_path: <filename> ]

# The interval at which DB back ups will be created. 
#
# Minimum allowable interval is `1d`, ie, 1 day.
#
# Units Supported: y, w, d, h, m, s, ms.
#
[ backup_interval: <duration> | default = 1d ]

```

## `<admin_config>`

A `admin_config` allows configuring the admin users of CEEMS API server.

```yaml
# List of users that will have admin privileges for accessing CEEMS API server
#
# These users will have full access to DB and can query stats of any user/project.
#
# In addition, it is possible to pull users from Grafana teams and add them to 
# admin users. Check `grafana` configuration on how to fetch users from Grafana.
#
users:
    [ - <string> ... ]

# Besides setting a static list of admin users using `ceems_api_server.web.admin_users`,
# it is possible to pull the users from a given Grafana instance and update the admin users
# list of CEEMS API server. This allows operators to add new admins to CEEMS API server
# without having to restart `ceems_api_server`. 
#
# Typically, one or several Grafana team(s) can be created dedicated to CEEMS admins and 
# CEEMS API server will fetch the Grafana team members at the same frequency as compute 
# units.
#
# The web config of Grafana can be set in the following section:
#
grafana:
  [ <grafana_config> ]
```

## `<grafana_config>`

A `grafana_config` allows configuring the Grafana client config to fetch members of 
Grafana teams to be added to admin users of CEEMS API server.

```yaml
# Web URL of the Grafana instance
#
url: <host>

# List of IDs of the Grafana teams from which the members will be synchronized 
# with CEEMS admin users
#
teams_ids:
  - <string> ...

# Sets the `Authorization` header on every API request with the
# configured username and password.
# password and password_file are mutually exclusive.
#
basic_auth:
  [ username: <string> ]
  [ password: <secret> ]
  [ password_file: <string> ]

# Sets the `Authorization` header on every API request with
# the configured credentials.
#
authorization:
  # Sets the authentication type of the request.
  [ type: <string> | default: Bearer ]
  # Sets the credentials of the request. It is mutually exclusive with
  # `credentials_file`.
  [ credentials: <secret> ]
  # Sets the credentials of the request with the credentials read from the
  # configured file. It is mutually exclusive with `credentials`.
  [ credentials_file: <filename> ]

# Optional OAuth 2.0 configuration.
# Cannot be used at the same time as basic_auth or authorization.
#
oauth2: 
  [ <oauth2> ]

# Configure whether scrape requests follow HTTP 3xx redirects.
[ follow_redirects: <boolean> | default = true ]

# Whether to enable HTTP2.
[ enable_http2: <boolean> | default: true ]

# Configures the API request's TLS settings.
#
tls_config:
  [ <tls_config> ]

# List of headers that will be passed in the API requests to the server.
# Authentication related headers may be configured in this section. Header name
# must be configured as key and header value supports three different types of 
# headers: values, secrets and files.
#
# The difference between values and secrets is that secret will be redacted
# in server logs where as values will be emitted in the logs.
#
# Values are regular headers with values, secrets are headers that pass secret
# information like tokens and files pass the file content in the headers.
#
http_headers:
  [ <http_headers_config> ]
```

## `<cluster_config>`

A `cluster_config` allows configuring the cluster of CEEMS API server.

```yaml
# Identifier of the cluster. Must be unique for each cluster
#
# Use an id that end users can identify, for instance, name of the cluster.
#
id: <idname>

# Resource manager of the cluster. Currently only `slurm` is supported. In future,
# `openstack` will be supported
#
manager: <managername>

# List of updater IDs to run on the compute units of current cluster. The updaters
# will be run in the same order as provided in the list.
#
# ID of each updater is set in the `updaters` section of the config. If an unknown
# ID is provided here, it will be ignored during the update step.
#
updaters:
  [- <idname> ... ]

# CLI tool configuration.
# 
# If the resource manager supports fetching compute units data from a CLI tool,
# this section can be used to configure the tool. This can be mainly used to configure
# SLURM CLI utility tools that can be used to fetch job data.
#
# When SLURM resource manager is configured to fetch job data using `sacct` command,
# execution mode of the command will be decided as follows:
#
#  - If the current user running `ceems_api_server` is `root` or `slurm` user, `sacct`
#    command will be executed natively as that user.
#
#  - If above check fails, `sacct` command will be attempted to execute as `slurm` user.
#    If the `ceems_api_server` process have enough privileges setup using Linux capabilities
#    in the systemd unit file, this will succeed and `sacct` will be always executed 
#    as `slurm` user.
#
#  - If above check fails as well, we attempt to execute `sacct` with `sudo` prefix. If
#    the current user running `ceems_api_server` is in the list of sudoers, this check
#    will pass and `sacct` will be always executed as `sudo sacct <args>` to fetch jobs.
#
# If none of the above checks, pass, `sacct` will be executed as the current user 
# which might not give job data of _all_ users in the cluster.
#
# If the operators are unsure which method to use, there is a default systemd
# unit file provided in the repo that uses Linux capabilities. Use that file as 
# starting point and modify the CLI args accordingly
#
# If no `cli` and no `web` config is found, `ceems_api_server` will check
# if CLI utilities like `sacct` exist on `PATH` and if found, will use them.
#
# Systemd Unit File:
# https://github.com/mahendrapaipuri/ceems/blob/main/build/package/ceems_api_server/ceems_api_server.service
#
cli:
  # Path to the binaries of the CLI utilities.
  #
  [ path: <filename> ]

  # An object of environment variables that will be injected while executing the 
  # CLI utilities to fetch compute unit data. 
  #
  # This is handy when executing CLI tools like `keystone` for openstack or `kubectl` 
  # for k8s needs to source admin credentials. Those credentials can be set manually
  # here in this section. 
  #
  environment_variables: 
    [ <string>: <string> ... ]

# If the resource manager supports API server, configure the REST API
# server details here.
#
# When configured, REST API server is always preferred over CLI utilities for 
# fetching compute units
#
# Most of the web configuration has been inspired from Prometheus `scrape_config`
# and its utility functions are used to create HTTP client using the configuration
# set below.
# 
web:
  # Web client config of resource manager's cluster
  #
  [ <web_client_config> ]

# Any other configuration needed to reach API server of the resource manager
# can be configured in this section.
#
# Currently this section is used for both SLURM and Openstack resource managers
# to configure API versions
#
# For example, for SLURM if your API endpoints are of form `/slurm/v0.0.40/diag`, 
# the version is `v0.0.40`.
# Docs: https://slurm.schedmd.com/rest_api.html
# SLURM's REST API version can be set as `slurm: v0.0.40`
#
# In the case of Openstack, we need to fetch from different sources like identity,
# compute and they use different versioning of API. They can be configured using
# this section as well
#
# Example:
#
# slurm: v0.0.40  # SLURM
# identity: v3  # Openstack
# compute: v2.1  # Openstack
#
extra_config:
  [ <string>: <string> ... ]
```

## `<updater_config>`

A `updater_config` allows configuring updaters of CEEMS API server.

```yaml
# Identifier of the updater. Must be unique for each updater
#
# This identifier should be used in the `updaters` section inside each 
# `clusters` config to update the compute units of that resource manager with a
# given updater.
#
id: <idname>

# Updater kind. Currently only `tsdb` is supported.
#
updater: <updatername>

# Web Config of the updater.
#
web:
  # Web client config of updater instance
  #
  [ <web_client_config> ]

# Any other configuration needed for the updater instance can be configured 
# in this section.
# Currently this section is used for `tsdb` updater to configure the queries that
# will be used to aggregate the compute unit metrics.
#
extra_config:
  # Query batch size when making TSDB queries.
  # CEEMS making queries in batches in order to avoid OOM errors on TSDB.
  # This parameter can be used to configure the number of compute units queried
  # in a single query. 
  #
  # Set this value based on your `--query.max-samples` parameter set to TSDB and 
  # scrape interval. For instance, at a given time, if there are 80k compute units
  # running, TSDB is scrapping at a rate of 5sec and CEEMS is updating for every 
  # 60 min. In this case, a given metric for 20k compute units will have 
  # 80,000 * (60 * 60) / 5 = 57600000 samples in the query. The default value 
  # used by Prometheus for `--query.max-samples` is 50000000 which is less than
  # what we got in the calculation in the example. Thus, we need to make multiple
  # queries by batching the compute units. In the current example, using a batch
  # size of 40k should work, however, we recommend using much lesser batch sizes
  # to protect TSDB from over consuming the memory.
  #
  # Default value is 1000 and it should work in most of the cases
  #
  [ query_batch_size: <int>  | default: 1000 ]

  # Compute units that have total life time less than this value will be deleted from 
  # TSDB to reduce number of labels and cardinality
  #
  # Default value `0s` means no compute units will be purged.
  #
  # Units Supported: y, w, d, h, m, s, ms.
  #
  [ cutoff_duration: <duration> | default: 0s ]

  # List of labels to delete from TSDB. These labels should be valid matchers for TSDB
  # More information of delete API of Prometheus https://prometheus.io/docs/prometheus/latest/querying/api/#delete-series
  #
  # TSDB must be started with --web.enable-admin-api flag for this to work
  #
  labels_to_drop:
    [ - <string> ... ]

  # Define queries that are used to estimate aggregate metrics of each compute unit
  # These queries will be passed to golang's text/template package to build them
  # Available template variables
  # - UUIDs -> UUIDs string delimited by "|", eg, 123|345|567
  # - ScrapeInterval -> Scrape interval of TSDB in time.Duration format eg 15s, 1m
  # - ScrapeIntervalMilli -> Scrape interval of TSDB in milli seconds eg 15000, 60000
  # - EvaluationInterval -> Evaluation interval of TSDB in time.Duration format eg 15s, 1m
  # - EvaluationIntervalMilli -> Evaluation interval of TSDB in milli seconds eg 15s, 1m
  # - RateInterval -> Rate interval in time.Duration format. It is estimated based on Scrape interval as 4*scrape_interval
  # - Range -> Duration of interval where aggregation is being made in time.Duration format
  #
  queries:
    [ <queries_config> ]
```

## `<queries_config>`

A `queries_config` allows configuring PromQL queries for TSDB updater of CEEMS API server.

```yaml
#
# It is possible to define multiple "sub-metrics" for each parent metric.
# For instance, for the case of `total_cpu_energy_usage_kwh`, we wish to store
# energy usage from different sources like RAPL, IPMI, we can do so using following
# config:
#
# total_cpu_energy_usage_kwh:
#   rapl_total: <TSDB query to get energy usage from RAPL for the unit>
#   ipmi_total: <TSDB query to get energy usage from IPMI for the unit>
#
# With the above configuration, the server response from API server will contain
# energy usage from both RAPL and IPMI using the same keys as we used in the 
# sub query. For instance, an example response can be:
#
# `{"total_cpu_energy_usage_kwh": {"rapl_total": 100, "ipmi_total": 120}}`
#
# This approach will let the operators to define the metrics freely according to
# their deployments. This will also allow to fetch metrics from third party 
# DBs outside of CEEMS components without hassle.
#
# The placeholder queries shown below should work out-of-the-box with CEEMS 
# exporter and operators are free to deploy more exporters of their own and use
# the metrics from them to estimate aggregated metrics of each compute unit
#
# Average CPU utilisation
#
# Example of valid query:
#
# global:
#   avg_over_time(
#     avg by (uuid) (
#       (
#         rate(ceems_compute_unit_cpu_user_seconds_total{uuid=~"{{.UUIDs}}"}[{{.RateInterval}}])
#         +
#         rate(ceems_compute_unit_cpu_system_seconds_total{uuid=~"{{.UUIDs}}"}[{{.RateInterval}}])
#       )
#       /
#       ceems_compute_unit_cpus{uuid=~"{{.UUIDs}}"}
#     )[{{.Range}}:]
#   ) * 100
avg_cpu_usage:
  [ <string>: <promql_query> ... ]
  

# Average CPU Memory utilisation
#
# Example of valid query:
#
# global:
#   avg_over_time(
#     avg by (uuid) (
#       ceems_compute_unit_memory_used_bytes{uuid=~"{{.UUIDs}}"}
#       /
#       ceems_compute_unit_memory_total_bytes{uuid=~"{{.UUIDs}}"}
#     )[{{.Range}}:]
#   ) * 100
avg_cpu_mem_usage:
  [ <string>: <promql_query> ... ]
  

# Total CPU energy usage in kWh
#
# Example of valid query:
#
# total:
#   sum_over_time(
#     sum by (uuid) (
#       unit:ceems_compute_unit_cpu_energy_usage:sum{uuid=~"{{.UUIDs}}"} * {{.ScrapeIntervalMilli}} / 3.6e9
#     )[{{.Range}}:{{.ScrapeInterval}}]
#   )
total_cpu_energy_usage_kwh:
  [ <string>: <promql_query> ... ]
  

# Total CPU emissions in gms
#
# Example of valid query:
#
# rte_total:
#   sum_over_time(
#     sum by (uuid) (
#       label_replace(
#         unit:ceems_compute_unit_cpu_energy_usage:sum{uuid=~"{{.UUIDs}}"} * {{.ScrapeIntervalMilli}} / 3.6e9,
#         "common_label",
#         "mock",
#         "hostname",
#         "(.*)"
#       )
#       * on (common_label) group_left ()
#       label_replace(
#         ceems_emissions_gCo2_kWh{provider="rte",country_code="fr"},
#         "common_label",
#         "mock",
#         "hostname",
#         "(.*)"
#       )
#     )[{{.Range}}:{{.ScrapeInterval}}]
#   )
# emaps_total:
#   sum_over_time(
#     sum by (uuid) (
#       label_replace(
#         unit:ceems_compute_unit_cpu_energy_usage:sum{uuid=~"{{.UUIDs}}"} * {{.ScrapeIntervalMilli}} / 3.6e9,
#         "common_label",
#         "mock",
#         "hostname",
#         "(.*)"
#       )
#       * on (common_label) group_left ()
#       label_replace(
#         ceems_emissions_gCo2_kWh{provider="emaps",country_code="fr"},
#         "common_label",
#         "mock",
#         "hostname",
#         "(.*)"
#       )
#     )[{{.Range}}:{{.ScrapeInterval}}]
#   )
total_cpu_emissions_gms:
  [ <string>: <promql_query> ... ]
  

# Average GPU utilization
#
# Example of valid query:
#
# global:
#   avg_over_time(
#     avg by (uuid) (
#       DCGM_FI_DEV_GPU_UTIL
#       * on (gpuuuid) group_right ()
#       ceems_compute_unit_gpu_index_flag{uuid=~"{{.UUIDs}}"}
#     )[{{.Range}}:{{.ScrapeInterval}}]
#   )
avg_gpu_usage:
  [ <string>: <promql_query> ... ]
  

# Average GPU memory utilization
#
# Example of valid query:
#
# global:
#   avg_over_time(
#     avg by (uuid) (
#       DCGM_FI_DEV_MEM_COPY_UTIL
#       * on (gpuuuid) group_right ()
#       ceems_compute_unit_gpu_index_flag{uuid=~"{{.UUIDs}}"}
#     )[{{.Range}}:{{.ScrapeInterval}}]
#   )
avg_gpu_mem_usage:
  [ <string>: <promql_query> ... ]
  

# Total GPU energy usage in kWh
#
# Example of valid query:
#
# total:
#   sum_over_time(
#     sum by (uuid) (
#       instance:DCGM_FI_DEV_POWER_USAGE:pue_avg * {{.ScrapeIntervalMilli}} / 3.6e9
#       * on (gpuuuid) group_right()
#       ceems_compute_unit_gpu_index_flag{uuid=~"{{.UUIDs}}"}
#     )[{{.Range}}:{{.ScrapeInterval}}]
#   )
total_gpu_energy_usage_kwh:
  [ <string>: <promql_query> ... ]
  

# Total GPU emissions in gms
#
# Example of valid query:
#
# rte_total:
#   sum_over_time(
#     sum by (uuid) (
#       label_replace(
#         instance:DCGM_FI_DEV_POWER_USAGE:pue_avg * {{.ScrapeIntervalMilli}} / 3.6e+09
#         * on (gpuuuid) group_right ()
#         ceems_compute_unit_gpu_index_flag{uuid=~"{{.UUIDs}}"},
#         "common_label",
#         "mock",
#         "instance",
#         "(.*)"
#       )
#       * on (common_label) group_left ()
#       label_replace(
#         ceems_emissions_gCo2_kWh{provider="rte",country_code="fr"},
#         "common_label",
#         "mock",
#         "instance",
#         "(.*)"
#       )
#     )[{{.Range}}:{{.ScrapeInterval}}]
#   )
# emaps_total:
#   sum_over_time(
#     sum by (uuid) (
#       label_replace(
#         instance:DCGM_FI_DEV_POWER_USAGE:pue_avg * {{.ScrapeIntervalMilli}} / 3.6e+09
#         * on (gpuuuid) group_right ()
#         ceems_compute_unit_gpu_index_flag{uuid=~"{{.UUIDs}}"},
#         "common_label",
#         "mock",
#         "instance",
#         "(.*)"
#       )
#       * on (common_label) group_left ()
#       label_replace(
#         ceems_emissions_gCo2_kWh{provider="emaps",country_code="fr"},
#         "common_label",
#         "mock",
#         "instance",
#         "(.*)"
#       )
#     )[{{.Range}}:{{.ScrapeInterval}}]
#   )
total_gpu_emissions_gms:
  [ <string>: <promql_query> ... ]
  

# Total IO write in GB stats
#
# Currently CEEMS exporter do not scrape this metric. Operators can configure
# this metric from third party exporters, if and when available
#
total_io_write_stats:
  [ <string>: <promql_query> ... ]

# Total IO read in GB stats
#
# Currently CEEMS exporter do not scrape this metric. Operators can configure
# this metric from third party exporters, if and when available
#
total_io_read_stats:
  [ <string>: <promql_query> ... ]

# Total ingress traffic stats
#
# Currently CEEMS exporter do not scrape this metric. Operators can configure
# this metric from third party exporters, if and when available
#
total_ingress_stats:
  [ <string>: <promql_query> ... ]

# Total outgress traffic stats
#
# Currently CEEMS exporter do not scrape this metric. Operators can configure
# this metric from third party exporters, if and when available
#
total_outgress_stats:
  [ <string>: <promql_query> ... ]
```
