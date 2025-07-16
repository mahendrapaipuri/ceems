---
sidebar_position: 3
---

# CEEMS Load Balancer

## Basic Usage

The CEEMS load balancer can be started with its core and web configuration files as follows:

```bash
ceems_lb --config.file=/path/to/core/config/file --web.config.file=/path/to/web/config/file
```

If there are environment variables used in `/path/core/config/file`, they can be expanded using

```bash
ceems_lb --config.file=/path/core/config/file --config.file.expand-env-vars --web.config.file=/path/to/web/config/file
```

:::tip[TIP]

In order to escape `$` in the config file when CLI flag `--config.file.expand-env-vars` is used,
use `$$`.

:::

This will start the CEEMS load balancer at the default port `9030`, listening on all interfaces.
To change the default port and host, the `--web.listen-address` CLI argument must be passed to
the binary:

```bash
ceems_lb --web.listen-address="localhost:8030"
```

:::tip[TIP]

All available command-line options are listed in the
[CEEMS LB CLI documentation](../cli/ceems-lb.md).

:::

The CEEMS LB supports both TSDB and Pyroscope backends and starts a load balancing server
for each of TSDB and Pyroscope when both backends are configured. To control the
address of each instance of CEEMS LB, `--web.listen-address` can be repeated. The first
configured address will be used for TSDB and the second one for Pyroscope. For example, when
the CEEMS LB is launched as follows:

```bash
ceems_lb --web.listen-address="localhost:7030" --web.listen-address=":10030"
```

the TSDB load balancer will be reachable at `localhost:7030` while the Pyroscope load
balancer will be available at `localhost:10030`.

## Access Control

The CEEMS load balancer is capable of providing basic access control for
TSDB using the CEEMS API server. For this to work, the CEEMS load balancer configuration file must
include configuration related to the CEEMS API server as discussed in the [Configuration](../configuration/ceems-lb.md)
section. If the CEEMS load balancer has access to the data directory of the CEEMS API server,
the load balancer will query the database directly to enforce access control.

If the data directory of the CEEMS API server is not accessible to the CEEMS load balancer, it
is possible to configure the client configuration of the CEEMS API server in the CEEMS load
balancer, and the load balancer will make API requests to the API server to determine the ownership
details of a given compute unit before enforcing access control.

:::important[IMPORTANT]

As described in the [CEEMS API Server](./ceems-api-server.md#access-control) documentation, Grafana must
be configured to send the user header in the requests to the data source for access control to
work.

:::

## Using with Grafana

As discussed in the [CEEMS Load Balancer](../components/ceems-lb.md) section, it is
possible for a single instance of the CEEMS load balancer to support multiple clusters at
the same time. Let's take a sample CEEMS load balancer configuration file as follows:

```yaml
ceems_lb:
  strategy: round-robin
  backends:
    - id: slurm-one
      tsdb_urls:
        - http://slurm-one-tsdb-one:9090
        - http://slurm-one-tsdb-two:9090
    - id: slurm-two
      tsdb_urls:
        - http://slurm-two-tsdb-one:9090
        - http://slurm-two-tsdb-two:9090

ceems_api_server:
  data:
    path: /var/lib/ceems
```

It is clear from the configuration that there are two different SLURM clusters, namely `slurm-one`
and `slurm-two`. Each cluster has its own dedicated set of TSDB instances.

Conventionally, operators configure two different data sources on Grafana, one for each
cluster. In the current case, the frontend load balancer of both clusters is the CEEMS
load balancer, and it is a single instance. Then the question arises: How do we
target the correct cluster when configuring the data source?

This is done using a custom header. When configuring the data source for cluster
`slurm-one`, a custom header `X-Ceems-Cluster-Id` must be configured to `slurm-one`,
and a similar configuration must be done for `slurm-two`. The CEEMS load balancer will look up
this custom header and load balance the traffic between the TSDB instances
of the correct target cluster.

:::important[IMPORTANT]

Even if there is only one cluster, it is necessary to add the custom header to the TSDB
Grafana data source configuration.

:::

Thus, the difference in configuring the Prometheus data source on Grafana compared to vanilla
Prometheus TSDB and CEEMS load balancer is the addition of this custom header in the data source
configuration.

## Admin Users

The CEEMS load balancer supports admin users with privileged access to TSDB. Users that
are configured as admin users in the CEEMS API server will have admin privileges in the CEEMS
load balancer as well. The CEEMS load balancer will allow admin users to query data of
_any_ compute unit. It is not possible to have admin privileges on the CEEMS load balancer
without having admin privileges on the CEEMS API server.
