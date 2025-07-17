---
sidebar_position: 3
---

# CEEMS API Server

The CEEMS API server can be configured using a YAML file under the key `ceems_api_server`.
Along with the base configuration of the server, we need to provide the configurations
for clusters that we need to monitor and updaters that will be used to update the
compute unit metrics. These can be configured under keys `clusters` and `updaters`,
respectively. A basic configuration file looks like this:

```yaml
# CEEMS API Server configuration skeleton

ceems_api_server: <CEEMS API SERVER CONFIG>

clusters: <CLUSTERS CONFIG>

updaters: <UPDATERS CONFIG>
```

A complete reference for the CEEMS API server configuration can be found in the [Reference](./config-reference.md)
section. A valid sample configuration
file can be found in the [repository](https://github.com/@ceemsOrg@/@ceemsRepo@/blob/main/build/config/ceems_api_server/ceems_api_server.yml)

## CEEMS API Server Configuration

This section guides on how to configure the CEEMS API server. A sample configuration
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
```

The configuration for `ceems_api_server` has three sections: `data`, `admin` and `web`
for configuring different aspects of the API server. Some explanation about the `data`
config is discussed below:

- `data.path`: Path where all CEEMS-related data will be stored.
- `data.update_interval`: The frequency at which the CEEMS API server will fetch compute units
from the underlying cluster. Do not use too small intervals or high frequency. `15m` is a
sane default and it should work in most production cases.
- `data.retention_period`: The CEEMS API server stores all the metadata of compute units along
with their aggregated metrics in a SQLite relational DB. This config parameter can be used
to set the retention time of the compute unit data in the SQLite database. For example, when
a value of `1y` is used, it means all compute units data in the last one year will be
retained, and older data will be purged.
- `data.backup_path`: It is possible to create backups of the SQLite DB at intervals specified
by `data.backup_interval` to a fault-tolerant storage.

:::warning[WARNING]

As the database grows in size, the time taken to create a backup increases exponentially
and hence, use the backup option only if it is absolutely necessary. A general
recommendation is to use a continuous backup solution like
[litestream](https://litestream.io/) instead of the native backup solution offered
by CEEMS.

:::

The CEEMS API server exposes admin endpoints in its API, and the `admin` section can be used to
configure which users can access those endpoints. More details on admin endpoints can be
found in the [API Docs](https://@ceemsOrg@.github.io/@ceemsRepo@/docs/category/api).

- `admin.users`: A list of statically defined users that will have access to the admin endpoints.
- `admin.grafana`: In addition to a static list of users, the CEEMS API server can pull users from specified Grafana teams to add them to the list of users that will be granted access to the admin endpoints. This
enables dynamic addition of users to the admin users list for CEEMS without having to reconfigure and restart the CEEMS API server. This section allows to provide the client configuration for
Grafana. All possible client configuration options can be found in the
[Config Reference](./config-reference.md#grafana-config).

<!-- Finally, the section `web` can be used to configured HTTP server of CEEMS API server.

- `web.max_query`: Maximum allowable query period. Configure this value appropriately
based on the needs as queries with too longer period can put considerable amount of
pressure on DB queries.
- `web.requests_limit`: Maximum number of requests per minute per client identified by
remote IP address.
- `web.route_prefix`: All the CEEMS API end points will be prefixed by this value. It
is useful when serving CEEMS API server behind a reverse proxy at a given path. -->

## Clusters Configuration

A sample clusters configuration section is shown below:

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
  
  - id: k8s-0
    manager: k8s
    updaters:
      - tsdb-0
    extra_config:
      username_annotations:
        - myk8s.io/created-by
        - ceems.io/created-by
      gpu_resource_names:
        - nvidia.com/gpu
        - amd.com/gpu
      users_db_file: /var/run/ceems/users.yaml
```

Essentially, it is a list of objects where each object describes a cluster.

- `id`: A unique identifier for each cluster. The identifier must stay consistent across
CEEMS components, especially for CEEMS LB. More details can be found in the
[Configuring CEEMS LB](./ceems-lb.md) section.
- `manager`: Resource manager kind. Currently, `slurm`, `openstack` and `k8s` are
supported.
- `updaters`: List of updaters to be used to update the aggregate metrics of
compute units. The order is important as compute units are updated in the same order
as specified here. For example, using the current sample file, it is important for the
operators to ensure that metrics updated by `tsdb-0` are not overridden by `tsdb-1`.
More details on updaters can be found in the [Updaters Configuration](#updaters-configuration).
- `cli`: If the resource manager uses CLI tools to fetch compute units, configuration related
to those CLI tools can be provided here. For example, currently the CEEMS API server supports
fetching SLURM jobs only using the `sacct` command, so it is essential to provide the
path to the `bin` folder where the `sacct` command will be found. More options on the CLI section can
be found in the [Cluster Configuration Reference](./config-reference.md#cluster_config).
- `web`: If the resource manager supports fetching compute units using an API, the client
configuration to access API endpoints can be provided here. In this particular example,
the OpenStack cluster's authentication is configured using the `web.http_headers` section.
All available options for the `web` configuration can be found in the
[Web Client Configuration Reference](./config-reference.md#web_client_config).
- `extra_config`: Any extra configuration required by a particular resource manager can be
provided here. Currently, the OpenStack and k8s resource managers uses this section to configure
extra information about API servers, users and projects.

### SLURM specific clusters configuration

As stated before, currently fetching SLURM jobs using the `sacct` command is the only supported
way. If the `sacct` binary is available on the `PATH`, there is no need to provide any specific
configuration. However, if the binary is present in a non-standard location, it is necessary to
provide the path to the binary using the `cli` section of the config. For example, if the absolute
path of `sacct` is `/opt/slurm/bin/sacct`, then we need to configure the `cli` section as follows:

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

The `cli` section also has an `environment_variables` key to provide any environment variables
while executing the `sacct` command in a sub-process. This section takes key-value pairs as values:

```yaml
clusters:
  - id: slurm-0
    manager: slurm
    cli: 
      path: /opt/slurm/bin
      environment_variables:
        ENVVAR_NAME: ENVVAR_VALUE
```

### OpenStack specific clusters configuration

In the case of OpenStack, the `extra_config` section must be used to set up OpenStack's API
and auth configs. The following keys in the `extra_config` section must be provided:

- `api_service_endpoints`: This section must provide the API endpoints for compute and
identity services.
- `auth`: This is the same auth object that needs to be passed to OpenStack's identity
service to get an API token. More details can be found in the [Keystone's API docs](https://docs.openstack.org/api-ref/identity/v3/#authentication-and-token-management).

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

Similarly, the following example shows how to use application credentials:

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
service catalog in the OpenStack cluster. For instance, the CEEMS API server supports
only identity API version `v3` and it adds `v3` to the URL path when making API requests.
However, it expects the configured API URL for compute to contain the API version `v2.1`
as shown in the above config.

:::

:::note[NOTE]

Admin-level privileges must be available for the configured auth object as the CEEMS API server
needs to fetch instances of **all** tenants and projects, and it is only possible
with admin scope.

:::

It is advised to use application credentials instead of the Admin password as it is
possible to scope the usage of application credentials to only compute and
identity services, whereas the admin account will give unrestricted access to all
cluster-level resources. More details on how to create application credentials with
scopes can be found in the [Keystone's docs](https://docs.openstack.org/keystone/latest/user/application_credentials.html).

OpenStack Nova (compute) uses micro versions for the API, and by default, the CEEMS API
server uses the latest supported micro version. If a specific micro version is
desired, it can be configured using the `web.http_headers` section as follows:

```yaml
web: 
  http_headers:
    X-OpenStack-Nova-API-Version:
      values:
        - 2.12
```

A sample full clusters config for OpenStack is shown below:

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

### k8s specific clusters configuration

:::important[IMPORTANT]

Before going into the k8s configuration, we strongly recommend users to read
[Kubernetes Section](./resource-managers.md#kubernetes) in
Resource Managers docs.

:::

In the case of k8s, the `extra_config` section resembles as follows:

```yaml
extra_config:
  kubeconfig_file: /path/to/out-of-cluster/kubeconfig.yaml
  gpu_resource_names:
    - nvidia.com/gpu
    - amd.com/gpu
  username_annotations:
    - ceems.io/created-by
    - exmaple.com/username
  project_annotations:
    - example.com/project
  ns_users_list_file: /path/to/users.yaml
```

where each key is explained below:

- `kubeconfig_file`: Path to the out-of-cluster kube config file to connect to
k8s API. In the k8s context, CEEMS API server app will be deployed as a pod and
hence, CEEMS API server will use in-cluster config. If in a scenario where CEEMS
API server is deployed outside of k8s cluster, this key can be used to configure
the kube config file of the target k8s cluster.
- `gpu_resource_names`: If the k8s cluster supports GPUs, this key take a list
of GPU resource names. By default it uses `[nvidia.com/gpu, amd.com/gpu]`. If
there any special resource names like NVIDIA MIG profiles, they must be explicitly
configured using this parameter.
- `username_annotations`: List of annotations names where username of the pod can be
retrieved. The first non empty string found in this list of annotations will be used
as the username.
- `project_annotations`: Similar to `username_annotations` but to retrieve the name
of the project. If not configured or none of the annotations exist on the pod, namespace
will be used as project in CEEMS DB.
- `ns_users_list_file`: When k8s cluster do not use native RBAC model to separate users
and projects, use this file to define a list of namespaces and the users that have
access to those namespaces. Effectively the user and namespace association described
in this file will be used in CEEMS DB to account for usage statistics and access control.

A sample `ns_users_list_file` is shown below:

```yaml
users:
  ns1:
    - usr1
    - usr2
  ns2:
    - usr2
    - usr3
```

Everytime a new namespace or user is created, this file must be updated so that CEEMS
API server will update its own DB with new associations.

:::note[NOTE]

CEEMS API server pulls users and their associated namespaces from rolebindings when the
cluster uses native RBAC model. In the cases where users and namespaces do not use native
RBAC model, use `ns_users_list_file` to setup users and namespaces

:::

#### Access control

In the cases where end users interact with k8s clusters _via_ services like Argo CD, Kubeflow,
it is not possible to get the real user who is created k8s resources like pods, deployments, _etc_.
In such a scenario, it is not possible to impose "strict" access control by the CEEMS components
as pod ownership cannot be established reliably. In this case, using `ns_users_list_file` to
define users and their namespace associations can mitigate this issue to a certain extent. However,
even with `ns_users_list_file` defined and maintained, usage statistics and access control can only
be imposed at the project/namespace level and not at the user level. All the pods that have been
created by k8s service accounts will be made available to all users in a given project.

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
        # Average CPU utilization
        avg_cpu_usage: 
          global: |
            avg_over_time(avg by (uuid) (unit:ceems_compute_unit_cpu_usage:ratio_rate1m{uuid=~"{{.UUIDs}}"} > 0 < inf)[{{.Range}}:])
```

Similar to `clusters`, `updaters` is also a list of objects where each object
describes an `updater`. Currently, only the **TSDB** updater is supported to update
compute units metrics from PromQL-compliant TSDB servers like Prometheus and Victoria Metrics.

- `id`: A unique identifier for the updater. This identifier must be used in the
`updaters` section of `clusters` as shown in the [Clusters Configuration](#clusters-configuration)
section.
- `updater`: Name of the updater. Currently, only `tsdb` is allowed.
- `web`: Web client configuration of the updater server.
- `extra_config`: The `extra_config` allows to further configure the TSDB.
  - `extra_config.cutoff_duration`: The time series data of compute units that have
    a total elapsed time less than this period will be marked as ignored in the CEEMS API server database.
  - `extra_config.delete_ignored`: The compute units' labels that are marked as ignored
    based on `extra_config.cutoff_duration` will be purged from the TSDB to decrease
    cardinality. This is useful to remove time series data of failed compute units
    or compute units that lasted for a very short duration, and in turn, keep the cardinality of the
    TSDB under check. For this feature to work, Prometheus needs to be started with
    the `--web.enable-admin-api` CLI flag that enables admin API endpoints.
  - `extra_config.queries`: This defines the queries to be made to the TSDB to estimate
    the aggregate metrics of each compute unit. The example config shows the query
    to estimate average CPU usage of the compute unit. All the supported queries can
    be consulted from the [Updaters Configuration Reference](./config-reference.md#updater_config).

## Examples

The following configuration shows a basic config needed to fetch batch jobs from the
SLURM resource manager:

```yaml
ceems_api_server:
  data:
    path: /var/lib/ceems
    update_interval: 15m

  admin:
    users:
      - adm1

clusters:
  - id: slurm-0
    manager: slurm
    cli: 
      path: /usr/bin
```

Both SLURM and OpenStack clusters can be monitored using a single instance of
CEEMS API server with a config as shown below:

```yaml
ceems_api_server:
  data:
    path: /var/lib/ceems
    update_interval: 15m

  admin:
    users:
      - adm1

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
and OpenStack clusters and metrics are scrapped by a Prometheus running
at `https://prometheus.example.com`, we can add the updater config to
the above examples as follows:

```yaml
ceems_api_server:
  data:
    path: /var/lib/ceems
    update_interval: 15m

  admin:
    users:
      - adm1

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
        # Average CPU utilization
        avg_cpu_usage:
          global: |
            avg_over_time(avg by (uuid) (unit:ceems_compute_unit_cpu_usage:ratio_rate1m{uuid=~"{{.UUIDs}}"} > 0 < inf)[{{.Range}}:])
            
        # Average CPU Memory utilization
        avg_cpu_mem_usage:
          global: |
            avg_over_time(avg by (uuid) (unit:ceems_compute_unit_memory_usage:ratio{uuid=~"{{.UUIDs}}"} > 0 < inf)[{{.Range}}:])
            
        # Total CPU energy usage in kWh
        total_cpu_energy_usage_kwh:
          total: |
            sum_over_time(sum by (uuid) (unit:ceems_compute_unit_cpu_energy_usage:sum{uuid=~"{{.UUIDs}}"} > 0 < inf)[{{.Range}}:{{.ScrapeInterval}}]) * {{.ScrapeIntervalMilli}} / 3.6e9
            
        # Total CPU emissions in gms
        total_cpu_emissions_gms:
          rte_total: |
            sum_over_time(sum by (uuid) (unit:ceems_compute_unit_cpu_emissions:sum{uuid=~"{{.UUIDs}}",provider="rte"} > 0 < inf)[{{.Range}}:{{.ScrapeInterval}}]) * {{.ScrapeIntervalMilli}} / 1e3

          emaps_total: |
            sum_over_time(sum by (uuid) (unit:ceems_compute_unit_cpu_emissions:sum{uuid=~"{{.UUIDs}}",provider="emaps"} > 0 < inf)[{{.Range}}:{{.ScrapeInterval}}]) * {{.ScrapeIntervalMilli}} / 1e3
            
          owid_total: |
            sum_over_time(sum by (uuid) (unit:ceems_compute_unit_cpu_emissions:sum{uuid=~"{{.UUIDs}}",provider="owid"} > 0 < inf)[{{.Range}}:{{.ScrapeInterval}}]) * {{.ScrapeIntervalMilli}} / 1e3
            
        # Average GPU utilization
        avg_gpu_usage:
          global: |
            avg_over_time(avg by (uuid) (unit:ceems_compute_unit_gpu_usage:ratio{uuid=~"{{.UUIDs}}"} > 0 < inf)[{{.Range}}:])
            
        # Average GPU memory utilization
        avg_gpu_mem_usage:
          global: |
            avg_over_time(avg by (uuid) (unit:ceems_compute_unit_gpu_memory_usage:ratio{uuid=~"{{.UUIDs}}"} > 0 < inf)[{{.Range}}:])
            
        # Total GPU energy usage in kWh
        total_gpu_energy_usage_kwh:
          total: |
            sum_over_time(sum by (uuid) (unit:ceems_compute_unit_gpu_energy_usage:sum{uuid=~"{{.UUIDs}}"} > 0 < inf)[{{.Range}}:{{.ScrapeInterval}}]) * {{.ScrapeIntervalMilli}} / 3.6e9
            
        # Total GPU emissions in gms
        total_gpu_emissions_gms:
          rte_total: |
            sum_over_time(sum by (uuid) (unit:ceems_compute_unit_gpu_emissions:sum{uuid=~"{{.UUIDs}}",provider="rte"} > 0 < inf)[{{.Range}}:{{.ScrapeInterval}}]) * {{.ScrapeIntervalMilli}} / 1e3
            
          emaps_total: |
            sum_over_time(sum by (uuid) (unit:ceems_compute_unit_gpu_emissions:sum{uuid=~"{{.UUIDs}}",provider="emaps"} > 0 < inf)[{{.Range}}:{{.ScrapeInterval}}]) * {{.ScrapeIntervalMilli}} / 1e3
            
          owid_total: |
            sum_over_time(sum by (uuid) (unit:ceems_compute_unit_gpu_emissions:sum{uuid=~"{{.UUIDs}}",provider="owid"} > 0 < inf)[{{.Range}}:{{.ScrapeInterval}}]) * {{.ScrapeIntervalMilli}} / 1e3         
```

The above configuration assumes that GPU compute nodes possess NVIDIA GPUs and the
[dcgm-exporter](https://github.com/NVIDIA/dcgm-exporter) is running alongside the CEEMS
exporter to export GPU metrics.
