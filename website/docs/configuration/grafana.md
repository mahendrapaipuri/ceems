---
sidebar_position: 8
---

# Grafana

This document shows the necessary configuration for Grafana server and its
datasources

## Grafana Server

The only configuration that needs to be added to the Grafana server in the
`/etc/grafana/grafana.ini` file is the following:

```ini
[dataproxy]
send_user_header = true
```

or the same can be done by setting `GF_DATAPROXY_SEND_USER_HEADER=true` environment
variable on Grafana server.

## Grafana Datasources

When using CEEMS LB to provide access control and loading balancing for
TSDB servers, the Prometheus datasource on the Grafana must be configured
slightly differently than using a regular native Prometheus server. As
discussed in [CEEMS LB Configuration](./ceems-lb.md#matching-backendsid-with-clustersid),
a custom header must be added to the queries to CEEMS LB server.

For instance, if CEEMS API server and CEEMS LB has following configuration:

```yaml
ceems_api_server:
  data:
    path: /var/lib/ceems
    update_interval: 15m

  admin:
    users:
      # Add admin user of Grafana here to the list of admins
      - admin
  
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
      tsdb_urls: 
        - http://tsdb-0
        - http://tsdb-0-replica

    - id: os-0
      tsdb_urls: 
        - http://tsdb-1
        - http://tsdb-1-replica
```

it is clear that there are two different clusters, `slurm-0` and `os-0`
and each cluster has its own TSDB server `tsdb-0` and `tsdb-1`, respectively.
In Grafana, a Prometheus datasource for each cluster must be configured to
present the metrics of each cluster separately. Thus, the following
provisioning config can be used to configure datasources of each cluster

```yaml
datasources:
  - name: SLURM-TSDB
    type: prometheus
    access: proxy
    url: http://ceems-lb:9030
    basicAuth: true
    basicAuthUser: <ceems_lb_basic_auth_user>
    # Notice we are setting custom header X-Ceems-Cluster-Id to `slurm-0`.
    # IT IS IMPORTANT TO HAVE IT
    jsonData:
      httpHeaderName1: X-Ceems-Cluster-Id
    secureJsonData:
      basicAuthPassword: <ceems_lb_basic_auth_password>
      httpHeaderValue1: slurm-0

  - name: OS-TSDB
    type: prometheus
    access: proxy
    url: http://ceems-lb:9030
    basicAuth: true
    basicAuthUser: <ceems_lb_basic_auth_user>
    # Notice we are setting custom header X-Ceems-Cluster-Id to `slurm-0`.
    # IT IS IMPORTANT TO HAVE IT
    jsonData:
      httpHeaderName1: X-Ceems-Cluster-Id
    secureJsonData:
      basicAuthPassword: <ceems_lb_basic_auth_password>
      httpHeaderValue1: os-0
```

Internally, CEEMS LB will check the header `X-Ceems-Cluster-Id` and forwards the request
to the correct backends group based on the provided cluster ID. This ensures
that we can use a single instance of CEEMS LB to load balance across multiple
clusters.

:::important[IMPORTANT]

Even if there is only one cluster and one TSDB instance for that cluster, we need
to configure the datasource on Grafana with custom header as explained above if we wish to use
CEEMS LB. This is the only way for the CEEMS LB to know which cluster to target.

:::
