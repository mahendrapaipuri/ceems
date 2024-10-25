---
sidebar_position: 3
---

# CEEMS Load Balancer

## Background

The motivation behind creating CEEMS load balancer component is that Prometheus TSDB
do not enforce any sort of access control over its metrics querying. This means once
a user has been given the permissions to query a Prometheus TSDB, they can query _any_
metrics stored in the TSDB.

Generally, it is not necessary to expose TSDB to end users directly and it is done
using Grafana as Prometheus datasource. Dashboards that are exposed to the end users
need to have query access on the underlying
datasource that the dashboard uses. Although a regular user with
[`Viewer`](https://grafana.com/docs/grafana/latest/administration/roles-and-permissions/access-control/#basic-roles)
role cannot add more panels to an existing dashboard, in order to _view_ the metrics the
user has effectively `query` permissions on the datasource.

This effectively means, the user can make _any_ query to the underlying datasource, _e.g.,_
Prometheus, using the browser cookie that is set by Grafana auth. The consequence is that
the user can query the metrics of _any_ user or _any_ compute unit. Straight forward
solutions to this problem is to create a Prometheus instance for each project/namespace.
However, this is not a scalable solution when they are thousands of projects/namespaces
exist.

This can pose few issues in multi tenant systems like HPC and cloud computing platforms.
Ideally, we do not want one user to be able to access the compute unit metrics of
other users. CEEMS load balancer component has been created to address this issue.

CEEMS Load Balancer addresses this issue by acting as a gate keeper to introspect the
query before deciding whether to proxy the request to TSDB or not. It means when a user
makes a TSDB query for a given compute unit, CEEMS load balancer will check if the user
owns that compute unit by verifying with CEEMS API server.

## Objectives

The main objectives of the CEEMS load balancer are two-fold:

- To provide access control on the TSDB so that compute units of each project/namespace
are only accessible to the members of that project/namespace
- To provide basic load balancing for replicated TSDB instances.

Thus, CEEMS load balancer can be configured as Prometheus data source in Grafana and
the load balancer will take care of routing traffic to backend TSDB instances and at
the same time enforcing access control.

## Load balancing

CEEMS load balancer supports classic load balancing strategies like round-robin and least
connection methods. Besides these two, it supports resource based strategy that is
based on retention time. Let's take a look at this strategy in-detail.

Taking Prometheus TSDB as an example, Prometheus advises to use local file system to store
the data. This ensure performance and data integrity. However, storing data on local
disk is not fault tolerant unless data is replicated elsewhere. There are cloud native
projects like [Thanos](https://thanos.io/), [Cortex](https://cortexmetrics.io/) to
address this issue. This load balancer is meant
to provide the basic functionality proposed by Thanos, Cortex, _etc_.

The core idea is to replicate the Prometheus data using
[Prometheus' remote write](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#remote_write)
functionality onto a remote storage which
is fault tolerant and have higher storage capacity but with a degraded query performance.
In this scenario, we have two TSDBs with following characteristics:

- TSDB using local disk: faster query performance with limited storage space
- TSDB using remote storage: slower query performance with bigger storage space

TSDB using local disk ("hot" instance) will have shorter retention period and the
one using remote storage ("cold" instance)
can have longer retention. CEEMS load balancer is capable of introspecting the query and
then routing the request to either "hot" or "cold" instances of TSDB.

## Multi cluster support

A single deployment of CEEMS load balancer is capable of loading balancing traffic between
different replicated TSDB instances of multiple clusters. Imagine there are two different
clusters, one for SLURM and one for Openstack, in a DC. Slurm cluster has two dedicated
TSDB instances where data is replicated between them and the same for Openstack cluster.
Thus, in total, there are four TSDB instances, two for SLURM cluster and two for
Openstack cluster. A single instance of CEEMS load balancer can route the traffic
between these four different TSDB instances by targeting the correct cluster.

However, in the production with heavy traffic a single instance of CEEMS load balancer
might not be a optimal solution. In that case, it is however possible to deploy a dedicated
CEEMS load balancer for each cluster.

More details on how to configuration of multi-clusters can be found in [Configuration](../configuration/ceems-lb.md)
section and some example scenarios are discussed in [Advanced](../advanced/multi-cluster.md)
section.
