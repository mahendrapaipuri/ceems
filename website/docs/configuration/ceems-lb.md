---
sidebar_position: 4
---

# CEEMS Load Balancer

CEEMS Load Balancer configuration has one main section and two optional
section. A basic skeleton of the configuration is as follows:

```yaml
# CEEMS Load Balancer configuration skeleton

ceems_lb: <CEEMS LB CONFIG>

# Optional section
ceems_api_server: <CEEMS API SERVER CONFIG>

# Optional section
clusters: <CLUSTERS CONFIG>
```

CEEMS LB uses the same configuration section of `ceems_api_server` and
`clusters` and hence, it is **possible to merge config files** of CEEMS
API server and CEEMS LB. Each component will read the necessary config
from the same file.

A valid sample
configuration file can be found in the
[repo](https://github.com/mahendrapaipuri/ceems/blob/main/build/config/ceems_lb/ceems_lb.yml).

## CEEMS Load Balancer Configuration

A sample CEEMS LB config file is shown below:

```yaml

ceems_lb:
  strategy: resource-based
  backends:
    - id: slurm-0
      tsdb_urls: 
        - http://localhost:9090

    - id: slurm-1
      tsdb_urls: 
        - http://localhost:9090
```

- `strategy`: Load balancing strategy. Besides classical `round-robin` and
`least-connection` strategies, a custom `resource-based` strategy is supported.
In the  `resource-based` strategy, the query will be proxied to the TSDB instance
that has the data based on the time period in the query.
- `backends`: A list of objects describing each TSDB backend.
  - `backends.id`: It is **important**
     that the `id` in the backend must be the same `id` used in the
     [Clusters Configuration](./ceems-api-server.md#clusters-configuration). This
     is how CEEMS LB will know which cluster to target.
  - `backends.tsdb_urls`: A list of TSDB servers that scrape metrics from this
     cluster identified by `id`.

:::warning[WARNING]

CEEMS LB is meant to deploy in the same DMZ as the TSDB servers and hence, it
does not support TLS for the backends.

:::

### Matching `backends.id` with `clusters.id`

This is the tricky part of the configuration which can be better explained with
an example. Consider we are running CEEMS API server with the following
configuration:

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

Here are we monitoring two SLURM clusters: `slurm-0`and `slurm-1`.
There are two different TSDB servers `tsdb-0`
and `tsdb-1` where `tsdb-0` is scrapping metrics from `slurm-0`
and `tsdb-1` scrapping metrics from only `slurm-1`. Assuming
`tsdb-0` is replicating data onto `tsdb-0-replica` and `tsdb-1`
onto `tsdb-1-replica`, we need to use the following config for
`ceems_lb`

```yaml
ceems_lb:
  strategy: resource-based
  backends:
    - id: slurm-0
      tsdb_urls: 
        - http://tsdb-0
        - http://tsdb-0-replica

    - id: slurm-1
      tsdb_urls: 
        - http://tsdb-1
        - http://tsdb-1-replica

```

As metrics data of `slurm-0` only exists in either `tsdb-0` or
`tsdb-0-replica`, we need to set `backends.id` to `slurm-0` for
these TSDB backends.

Effectively we will use CEEMS LB as a Prometheus datasource in
Grafana and while doing so, we need to target correct cluster
using path parameter. For instance, for `slurm-0` cluster the
datasource URL must be configured as `http://ceems-lb:9030/slurm-0`
assuming `ceems_lb` is running on port `9030`. Now, CEEMS LB will
know which cluster to target (in this case `slurm-0`), strips
the path parameter `slurm-0` from the path and proxies the request
to one of the configured backends. This allows a single instance
of CEEMS to load balance across different clusters.

## CEEMS API Server Configuration

This is an optional config when provided will enforce access
control for the backend TSDBs. A sample config file is given
below:

```yaml
ceems_api_server:
  web:
    url: http://localhost:9020
```

- `web.url`: Address at which CEEMS API server is running. CEEMS LB
will make a request to CEEMS API request to verify the ownership of
the comput unit before proxying request to TSDB. All the possible
configuration parameters for `web` can be found in
[Web Client Configuration Reference](./config-reference.md#web_client_config).

If both CEEMS API server and CEEMS LB has access to CEEMS data path,
it is possible to use the `ceems_api_server.db.path` as well to
query the DB directly instead of making an API request. This will have
much lower latency and higher performance.

## Clusters Configuration

Same configuration as discussed in
[CEEMS API Server's Cluster Configuration](./ceems-api-server.md#clusters-configuration)
can be provided as an optional configuration to verify the `backends` configuration.
This is not mandatory and if not provided, CEEMS LB will verify the backend
`ids` by making an API request to CEEMS API server.

## Example configuration files

As it is clear from above sections, there is a lot of common configuration
between CEEMS API server and CEEMS LB. Thus, when it is possible, it is
advised to merge two configurations in one file.

Taking one of the [examples](./ceems-api-server.md#examples) in CEEMS API
server section, we can add CEEMS LB config as follows:

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
    requests_limit: 30

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
  strategy: resource-based
  backends:
    - id: slurm-0
      tsdb_urls: 
        - http://tsdb-0
        - http://tsdb-0-replica

    - id: os-0
      tsdb_urls: 
        - http://tsdb-1
        - http://tsdb-1-replica
```

This config assumes `tsdb-0` is replicating data to `tsdb-0-replica`,
`tsdb-1` to `tsbd-1-replica` and CEEMS API server is running on
port `9020` on the same host as CEEMS LB.
