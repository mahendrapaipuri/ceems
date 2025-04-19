---
sidebar_position: 4
---

# CEEMS Load Balancer

The CEEMS load balancer supports TSDB and Pyroscope servers. When both TSDB and Pyroscope backend servers are configured, the CEEMS LB will launch two different web servers listening at different ports - one for TSDB and one for Pyroscope.

## CEEMS Load Balancer Configuration

The CEEMS Load Balancer configuration has one main section and two optional sections. A basic configuration skeleton is as follows:

```yaml
# CEEMS Load Balancer configuration skeleton

ceems_lb: <CEEMS LB CONFIG>

# Optional section
ceems_api_server: <CEEMS API SERVER CONFIG>

# Optional section
clusters: <CLUSTERS CONFIG>
```

The CEEMS LB uses the same configuration sections for `ceems_api_server` and `clusters`, so it is **possible to merge config files** of the CEEMS API server and CEEMS LB. Each component will read the necessary configuration from the same file.

A valid sample configuration file can be found in the [repository](https://github.com/@ceemsOrg@/@ceemsRepo@/blob/main/build/config/ceems_lb/ceems_lb.yml).

A sample CEEMS LB config file is shown below:

```yaml
ceems_lb:
  strategy: round-robin
  backends:
    - id: slurm-0
      tsdb: 
        - web:
            url: http://localhost:9090
      pyroscope:
        - web:
            url: http://localhost:4040

    - id: slurm-1
      tsdb: 
        - web:
            url: http://localhost:9090

    - id: slurm-2
      pyroscope: 
        - web:
            url: http://localhost:4040
```

- `strategy`: Load balancing strategy. Besides the classical `round-robin` and `least-connection` strategies, a custom `round-robin` strategy is supported. In the `round-robin` strategy, the query will be proxied to the TSDB instance that has the data based on the time period in the query.
- `backends`: A list of objects describing each TSDB backend.
  - `backends[0].id`: It is **important** that the `id` in the backend must be the same `id` used in the [Clusters Configuration](./ceems-api-server.md#clusters-configuration). This is how the CEEMS LB will know which cluster to target.
  - `backends[0].tsdb`: A list of TSDB servers that scrape metrics from the cluster identified by `id`.
    - `backends[0].tsdb.web`: Client HTTP configuration of the TSDB
    - `backends[0].tsdb.filter_labels`: A list of labels to filter before sending the response to the client. Useful to filter hypervisor or compute node specific information for OpenStack and k8s clusters.
  - `backends[0].pyroscope`: A list of Pyroscope servers that store profiling data from the cluster identified by `id`.
    - `backends[0].pyroscope.web`: Client HTTP configuration of Pyroscope

:::warning[WARNING]

The `round-robin` strategy is only supported for TSDB and when used along with Pyroscope, the load balancing strategy for Pyroscope servers will be defaulted to `least-connection`.

The CEEMS LB is meant to be deployed in the same DMZ as the TSDB servers and hence, it does not support TLS for the backends.

:::

### CEEMS Load Balancer CLI configuration

By default, the CEEMS LB servers listen at ports `9030` and `9040` when both TSDB and Pyroscope backend servers are configured. If intended to use custom ports, the CLI flag `--web.listen-address` must be repeated to set up ports for TSDB and Pyroscope backends. For instance, for the sample config shown above, the CLI arguments to launch LB servers at custom ports will be:

```bash
ceems_lb --config.file config.yml --web.listen-address ":8000" --web.listen-address ":9000"
```

This will launch the TSDB load balancer listening at port `8000` and the Pyroscope load balancer listening at port `9000`.

:::important[IMPORTANT]

When both TSDB and Pyroscope backend servers are configured, the first listen address is attributed to TSDB and the second one to Pyroscope.

:::

### Matching `backends.id` with `clusters.id`

#### Using custom header

This is the tricky part of the configuration which can be better explained with an example. Consider we are running the CEEMS API server with the following configuration:

```yaml
ceems_api_server:
  data:
    path: /var/lib/ceems
    update_interval: 15m

clusters:
  - id: slurm-0
    manager: slurm
    updaters:
      - tsdb-0
    cli: 
      <omitted for brevity>

  - id: slurm-1
    manager: slurm
    updaters:
      - tsdb-1
    cli: 
      <omitted for brevity>

updaters:
  - id: tsdb-0
    updater: tsdb
    web:
      url: http://tsdb-0
    extra_config:
        <omitted for brevity>
  
  - id: tsdb-1
    updater: tsdb
    web:
      url: http://tsdb-1
    extra_config:
        <omitted for brevity>
```

Here we are monitoring two SLURM clusters: `slurm-0` and `slurm-1`. There are two different TSDB servers `tsdb-0` and `tsdb-1` where `tsdb-0` is scraping metrics from `slurm-0` and `tsdb-1` scraping metrics from only `slurm-1`. Assuming `tsdb-0` is replicating data onto `tsdb-0-replica` and `tsdb-1` onto `tsdb-1-replica`, we need to use the following config for `ceems_lb`:

```yaml
ceems_lb:
  strategy: round-robin
  backends:
    - id: slurm-0
      tsdb: 
        - web:
            url: http://tsdb-0
        - web:
            url: http://tsdb-0-replica

    - id: slurm-1
      tsdb: 
        - web:
            url: http://tsdb-1
        - web:
            url: http://tsdb-1-replica
```

As metrics data of `slurm-0` only exists in either `tsdb-0` or `tsdb-0-replica`, we need to set `backends.id` to `slurm-0` for these TSDB backends.

Effectively we will use the CEEMS LB as a Prometheus datasource in
Grafana and while doing so, we need to target the correct cluster.
This is done using a custom header `X-Ceems-Cluster-Id`. When
configuring the datasource in Grafana, we need to add `X-Ceems-Cluster-Id`
to the custom headers section and set the value to the cluster ID.

For instance, for the `slurm-0` cluster, the provisioned datasource config for Grafana will look as follows:

```yaml
- name: CEEMS-TSDB-LB
  type: prometheus
  access: proxy
  url: http://localhost:9030
  basicAuth: true
  basicAuthUser: ceems
  jsonData:
    prometheusVersion: 2.51
    prometheusType: Prometheus
    timeInterval: 30s
    incrementalQuerying: true
    cacheLevel: Medium
    httpHeaderName1: X-Ceems-Cluster-Id
  secureJsonData:
    basicAuthPassword: <ceems_lb_basic_auth_password>
    httpHeaderValue1: slurm-0
```

assuming the CEEMS LB is running at port 9030 on the same host as Grafana. Similarly, for Pyroscope, the provisioned config must look like:

```yaml
- name: CEEMS-Pyro-LB
  type: pyroscope
  access: proxy
  url: http://localhost:9040
  basicAuth: true
  basicAuthUser: ceems
  jsonData:
    httpHeaderName1: X-Ceems-Cluster-Id
  secureJsonData:
    basicAuthPassword: <ceems_lb_basic_auth_password>
    httpHeaderValue1: slurm-0
```

Notice that we set the header and value in `jsonData` and `secureJsonData`, respectively. This ensures that the datasource will send the header with every request to the CEEMS LB, and then the LB will redirect the query request to the correct backend. This allows a single instance of CEEMS to load balance across different clusters.

#### Using query label

If for any reason, the above strategy does not work for a given deployment, it is also possible to identify target clusters using query labels. However, for this strategy to work, it is needed to inject labels to Prometheus metrics. For example, in the above case, using [static_config](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#static_config), we can set a custom label as follows:

```yaml
- job_name: ceems                        
  static_configs:                     
  - targets:                          
    - compute-0:9100          
    labels:                           
      ceems_id: slurm-0
```

The CEEMS LB will read the value of the `ceems_id` label and then redirect the query to the appropriate backend.

:::important[IMPORTANT]

If both custom header and label `ceems_id` are present in the request to the CEEMS LB, the query label will take precedence.

:::

Similarly, for setting up this label on profiling data in Pyroscope, it is necessary to use the `external_labels` config parameter for Grafana Alloy when exporting profiles to the Pyroscope server. A sample config for Grafana Alloy that pushes profiling data can be as follows:

```river
pyroscope.write "monitoring" {
  endpoint {
    url = "http://pyroscope:4040"
  }

  external_labels = {
    "ceems_id" = "slurm-0",
  }
}
```

## CEEMS API Server Configuration

This is an optional configuration that, when provided, will enforce access control for the backend TSDBs. A sample config file is given below:

```yaml
ceems_api_server:
  web:
    url: http://localhost:9020
```

- `web.url`: Address at which the CEEMS API server is running. The CEEMS LB will make a request to the CEEMS API server to verify the ownership of the compute unit before proxying the request to TSDB. All the possible configuration parameters for `web` can be found in the [Web Client Configuration Reference](./config-reference.md#web_client_config).

If both the CEEMS API server and CEEMS LB have access to the CEEMS data path, it is possible to use the `ceems_api_server.db.path` as well to query the DB directly instead of making an API request. This will have much lower latency and higher performance.

## Clusters Configuration

The same configuration as discussed in the [CEEMS API Server's Cluster Configuration](./ceems-api-server.md#clusters-configuration) can be provided as an optional configuration to verify the `backends` configuration. This is not mandatory and if not provided, the CEEMS LB will verify the backend `ids` by making an API request to the CEEMS API server.

## Example configuration files

As it is clear from the above sections, there is a lot of common configuration between the CEEMS API server and CEEMS LB. Thus, when possible, it is advised to merge two configurations in one file.

Taking one of the [examples](./ceems-api-server.md#examples) in the CEEMS API server section, we can add the CEEMS LB config as follows:

```yaml
ceems_api_server:
  data:
    path: /var/lib/ceems
    update_interval: 15m

  admin:
    users:
      - adm1
  
  web:
    url: http://localhost:9020

clusters:
  - id: slurm-0
    manager: slurm
    updaters:
      - tsdb-0
    cli: 
      <omitted for brevity>

  - id: os-0
    manager: openstack
    updaters:
      - tsdb-1
    web: 
      <omitted for brevity>

updaters:
  - id: tsdb-0
    updater: tsdb
    web:
      url: http://tsdb-0
    extra_config:
      <omitted for brevity>
  
  - id: tsdb-1
    updater: tsdb
    web:
      url: http://tsdb-1
    extra_config:
      <omitted for brevity>

ceems_lb:
  strategy: round-robin
  backends:
    - id: slurm-0
      tsdb: 
        - web:
            url: http://tsdb-0
        - web:
            url: http://tsdb-0-replica

    - id: os-0
      tsdb: 
        - web:
            url: http://tsdb-1
        - web:
            url: http://tsdb-1-replica
```

This config assumes `tsdb-0` is replicating data to `tsdb-0-replica`, `tsdb-1` to `tsdb-1-replica`, and the CEEMS API server is running on port `9020` on the same host as the CEEMS LB.
