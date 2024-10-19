---
sidebar_position: 8
---

# Grafana

When using CEEMS LB to provide access control and loading balancing for
TSDB servers, the Prometheus datasource on the Grafana must be configured
slightly differently than using a regular native Prometheus server. As
discussed in [CEEMS LB Configuration](./ceems-lb.md#matching-backendsid-with-clustersid),
a path parameter corresponding to the cluster must be appended to CEEMS LB server URL.

For instance, if CEEMS API server and CEEMS LB has following configuration:

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
    # Notice the path parameter `slurm-0` at the end.
    # IT IS IMPORTANT TO HAVE IT
    url: http://ceems-lb:9030/slurm-0
    basicAuth: true
    basicAuthUser: <ceems_lb_basic_auth_user>
    secureJsonData:
      basicAuthPassword: <ceems_lb_basic_auth_password>

  - name: OS-TSDB
    type: prometheus
    access: proxy
    # Notice the path parameter `os-0` at the end.
    # IT IS IMPORTANT TO HAVE IT
    url: http://ceems-lb:9030/os-0
    basicAuth: true
    basicAuthUser: <ceems_lb_basic_auth_user>
    secureJsonData:
      basicAuthPassword: <ceems_lb_basic_auth_password>
```

Internally, CEEMS LB will strip the path parameter and forwards the request
to the correct backends group based on the provided path parameter. This ensures
that we can use a single instance of CEEMS LB to load balance across multiple
clusters.

:::important[IMPORTANT]

Even if there is only one cluster and one TSDB instance for that cluster, we need
to configure the datasource on Grafana as explained above if we wish to use
CEEMS LB. This is the only way for the CEEMS LB to know which cluster to target.

:::
