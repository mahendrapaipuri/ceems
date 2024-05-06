---
sidebar_position: 3
---

# CEEMS Load Balancer

`ceems_lb` is a basic load balancer meant to provide basic access control for
TSDB so that one user cannot access metrics of another user.

## Objectives

The main objectives of the CEEMS load balancer is two-fold:

- To provide access control on the TSDB so that compute units of each project/namespace
are only accessible to the members of that project/namespace
- To provide basic load balancing for replicated TSDB instances.

### Access control

CEEMS load balancer is capable of providing basic access control policies of 
TSDB using CEEMS API server. Let's look into the problem first using a typical scenario
where Grafana is used for exposing the dashboards to the end users.

Dashboards that are exposed to the end users need to have query access on the underlying 
datasource that the dashboard uses. Although a regular user with 
[`Viewer`](https://grafana.com/docs/grafana/latest/administration/roles-and-permissions/access-control/#basic-roles) 
role cannot add more panels to an existing dashboard, in order to _view_ the metrics the 
user has effectively `query` permissions on the datasource. In the current context, the 
datasource is a TSDB like Prometheus. 

This effectively means, the user can make _any_ query to the underlying datasource, _e.g.,_ 
Prometheus, using the browser cookie that is set by Grafana auth. The consequence is that 
the user can query the metrics of _any_ user or _any_ compute unit. Straight forward 
solutions to this problem is to create a Prometheus instance for each project/namespace. 
However, this is not a scable solution when they are thousands of projects/namespaces 
exist. 

CEEMS Load Balancer addresses this issue by acting as a gate keeper to introspect the 
query before deciding whether to proxy the request to TSDB or not. It means when a user 
makes a TSDB query for a given compute unit identified by a `uuid`, CEEMS load balancer 
will check if the user owns that compute unit by verfiying with CEEMS API server.

:::note[NOTE]

As described in [CEEMS API Server](./ceems-api-server.md#access-control), the current 
user is identified using header `X-Grafana-User`.

:::

CEEMS Load Balancer can interact with CEEMS API server either by making a API request 
to verify the compute unit ownership or directly by querying the CEEMS API server's DB 
if they both deployed on the same host.

### Load balancing

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

## For Admins/Operators

CEEMS load balancer supports admin users with privileged access to TSDB. These users can 
be configured using [configuration file](../configuration/ceems-lb.md). Besides, it is 
possible to synchornize users from a given Grafana team periodically. This lets the 
operators to create a dedicated admin team for CEEMS API server and add users dynamically 
to this team. CEEMS load balancer will synchronize the members of the team and access 
admin privileges for accessing TSDB for querying data of _any_ compute unit.

:::important[IMPORTANT]

Admin users are also identified by the user header `X-Grafana-User` and if the current 
user name is in the list of admin users, that user will be able to access admin 
end points.

:::
