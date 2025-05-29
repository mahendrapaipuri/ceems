---
sidebar_position: 3
---

# CEEMS Load Balancer

## Background

The motivation behind creating the CEEMS load balancer component is that neither Prometheus TSDB
nor Grafana Pyroscope enforce any sort of access control over their metrics/profiles querying.
This means once a user has been given the permissions to query a Prometheus TSDB/Grafana
Pyroscope server, they can query _any_ metrics/profiles stored in those servers.

Generally, it is not necessary to expose the TSDB/Pyroscope server to end users directly, and it is done
using Grafana as a Prometheus/Pyroscope data source. Dashboards that are exposed to end users
need to have query access on the underlying
data source that the dashboard uses. Although a regular user with
[`Viewer`](https://grafana.com/docs/grafana/latest/administration/roles-and-permissions/access-control/#basic-roles)
role cannot add more panels to an existing dashboard, in order to _view_ the metrics, the
user has effectively `query` permissions on the data source.

This effectively means that the user can make _any_ query to the underlying data source (_e.g.,_
Prometheus) using the browser cookie that is set by Grafana auth. The consequence is that
the user can query the metrics of _any_ user or _any_ compute unit. A straightforward
solution to this problem is to create a Prometheus instance for each project/namespace.
However, this is not a scalable solution when there are thousands of projects/namespaces
in a given deployment.

This can pose a few issues in multi-tenant systems like HPC and cloud computing platforms.
Ideally, we do not want one user to be able to access the compute unit metrics of
other users. The CEEMS load balancer component has been created to address this issue.

The CEEMS Load Balancer addresses this issue by acting as a gatekeeper to introspect the
query before deciding whether to proxy the request to TSDB/Pyroscope or not. This means when a user
makes a TSDB/Pyroscope query for a given compute unit, the CEEMS load balancer will check if the user
owns that compute unit by verifying with the CEEMS API server.

## Objectives

The main objectives of the CEEMS load balancer are twofold:

- To provide access control on the TSDB/Pyroscope so that compute units of each project/namespace
are only accessible to the members of that project/namespace
- To provide basic load balancing for replicated TSDB/Pyroscope instances

Thus, the CEEMS load balancer can be configured as a Prometheus and Pyroscope data source in Grafana, and
the load balancer will take care of routing traffic to backend TSDB/Pyroscope instances while
enforcing access control.

## Access Control

Besides providing access control to the metrics, the CEEMS load balancer blacklists most of the
management API of the backend TSDB/Pyroscope servers. This ensures that end users will not
be able to access API resources that return configuration details, build time/runtime information,
_etc_. The CEEMS load balancer only allows a handful of API resources for TSDB/Pyroscope that are
strictly necessary to make queries. Currently, the allowed resources are as follows:

### TSDB

- `/api/v1/labels`
- `/api/v1/series`
- `/api/v1/<label_name>/values`
- `/api/v1/query`
- `/api/v1/query_range`

More details on each of these resources can be found in the
[Prometheus Documentation](https://prometheus.io/docs/prometheus/latest/querying/api/#http-api).

### Pyroscope

- `/querier.v1.QuerierService/SelectMergeStacktraces`
- `/querier.v1.QuerierService/LabelNames`
- `/querier.v1.QuerierService/LabelValues`

The API documentation for Pyroscope is very minimal. These are the minimum resources
needed to pull profiling data from the Pyroscope server.

## Load Balancing

The CEEMS load balancer supports classic load balancing strategies like round-robin and least
connection methods. Let's take a look at this strategy in detail.

:::warning[WARNING]

The resource-based load balancing strategy is not supported anymore and starting from
version `0.9.0`, it is removed. We recommend to use
[Prometheus' remote read](https://prometheus.io/docs/prometheus/latest/querying/remote_read_api/)
feature to achieve the same functionality of resource-based load balancing which is more
performant and reliable.

:::

Taking Prometheus TSDB as an example, Prometheus advises using the local file system to store
the data. This ensures performance and data integrity. However, storing data on local
disk is not fault tolerant unless data is replicated elsewhere. There are cloud-native
projects like [Thanos](https://thanos.io/) and [Cortex](https://cortexmetrics.io/) to
address this issue. This load balancer is meant
to provide the basic functionality proposed by Thanos, Cortex, _etc_.

The core idea is to replicate the Prometheus data using
[Prometheus' remote write](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#remote_write)
functionality onto a remote storage which
is fault-tolerant. In this we can have following different scenarios.

In the first scenario, both TSDB using local storage and remote storage have same retention
period and we achieve fault tolerance of data by remote write feature. In this case
CEEMS LB loading balancing techniques (round-robin or least connection) can be used
to distribute the requests between two instances of TSDB.

In a different scenario, we have two TSDBs with the following characteristics:

- TSDB using local disk: faster query performance with limited storage space
- TSDB using remote storage: slower query performance with bigger storage space

The TSDB using local disk ("hot" instance) will have a shorter retention period, and the
one using remote storage ("cold" instance) can have a longer retention. By enabling
[Prometheus' remote read](https://prometheus.io/docs/prometheus/latest/querying/remote_read_api/)
feature, the hot instance of TSDB can fetch the data from cold instance when it does not
find it in its own database. In this case as well we cab use
CEEMS LB loading balancing techniques (round-robin or least connection)
to distribute the requests between two instances of TSDB.

## Multi-Cluster Support

A single deployment of the CEEMS load balancer is capable of load balancing traffic between
different replicated TSDB/Pyroscope instances of multiple clusters. Imagine there are two different
clusters, one for SLURM and one for OpenStack, in a DC. Slurm cluster has two dedicated
TSDB/Pyroscope instances with data replicated between them, and the same applies to the OpenStack cluster.
Thus, in total, there are four TSDB/Pyroscope instances: two for the SLURM cluster and two for the
OpenStack cluster. A single instance of the CEEMS load balancer can route the traffic
between these four different TSDB/Pyroscope instances by targeting the correct cluster.

However, in production with heavy traffic, a single instance of the CEEMS load balancer
might not be an optimal solution. In that case, it is possible to deploy a dedicated
CEEMS load balancer for each cluster.

More details on the configuration of multi-clusters can be found in the [Configuration](../configuration/ceems-lb.md)
section, and some example scenarios are discussed in the [Advanced](../advanced/multi-cluster.md)
section.
