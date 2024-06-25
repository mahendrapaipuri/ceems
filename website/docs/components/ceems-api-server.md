---
sidebar_position: 2
---

# CEEMS API server

## Background

CEEMS exporter exports compute unit and node level metrics to Prometheus. But this is 
not enough to be able to query the metrics from Prometheus efficiently. Especially, for 
batch jobs we need at least the timestamps of when the job has started and ended and 
on which compute nodes to efficiently query the metrics. Storing these meta data of 
the compute units in Prometheus is not ideal as they are not time series and using storing meta 
data as labels can increase the cardinality very rapidly. 

At the same time, we would like to show to the end users aggregate metrics of their usage 
which needs to make queries to Prometheus every time they load their dashboards. The 
CEEMS API server has been introduced into the stack to address these limitations. CEEMS 
API server is meant to store and server compute unit meta data, aggregate metrics of 
compute units, users and projects _via_ API end points. This data will be gathered from 
the underlying resource manager and keep it in a local DB based on SQLite. 

:::important[IMPORTANT]

SQLite was chosen for its simplicity and to reduce dependencies. There is no need for
concurrent write access to DB as compute unit data is updated frequently by only one
process.

:::

Effectively, it acts as an abstraction layer for different 
resource managers and it is capable to storing data from different resource managers. 
The advantage of this approach is that it acts a single point of data collection for 
different resource managers of a data center and users will be able to consult their 
usage of their compute units in a unified way.

:::note[NOTE]

If the usernames are identical for different resource managers, _i.e.,_ if a data center 
has SLURM and Openstack clusters and user identities for these two clusters are provided 
by the same Identity Provider (IDP), it is possible for the Operators to use a 
single deployment of CEEMS that uses the same IDP and expose the compute unit metrics 
of both SLURM and Openstack clusters with the same instance of Grafana with 
different dashboards.

:::

## Objectives

CEEMS API server primarily serves for two objectives:

- To store the compute unit information of different resource managers in a unified way.
The information we need is very basic like unique identifier of compute unit, project it
belongs to, owner, current state, when it has started, resources allocated, _etc_.
- To update aggregate metrics of each compute unit by querying the TSDB in realtime. 
This allows the end users to view the usage of their workloads in realtime like CPU, 
energy, emissions, _etc_.
- To keep latest copy of users and their associated projects to enforce access control.

When coupled with 
[JSON API DataSource](https://grafana.github.io/grafana-json-datasource/installation/) or 
[Infinity DataSource](https://grafana.com/grafana/plugins/yesoreyeram-infinity-datasource/)
of Grafana, we can list the compute units of a user 
along with the metadata and aggregate metrics of each unit. The stored metadata in the 
CEEMS API server DB will allow us to construct the URLs for the Grafana dashboards for 
each compute dynamically based on start and end time of each compute unit.

![User job list](/img/dashboards/job_list_user.png)

## Architecture

### Resource Managers

Now that it is clear that CEEMS _can_ support different resource managers, it is time 
to explain how CEEMS actually supports them. CEEMS has its own DB schema that stores 
compute units metrics and meta data. Let's take meta data of each compute unit as an 
example. For example, SLURM exposes meta data of jobs using either `sacct` command or 
SLURM REST API. Openstack does it too using Keystone and Nova API servers so does the 
Kubernetes with its API server. However, all these managers expose these meta data 
in different ways each having their own API spec. 

CEEMS API server must implement each of this resource manager to fetch compute unit 
meta data and store it in CEEMS API server's DB. This is done using factory design
pattern and implemented in an extensible way. That means operators can implement their 
own custom third party resource managers and plug into CEEMS API server. Essentially, 
this translates to implementing two interfaces, one for fetching compute units and one 
for fetching users and projects/namespaces/tenants data from the underlying resource 
manager. 

Currently, CEEMS API server ships SLURM support and soon Openstack support 
will be added. 

### Aggregate metrics

As CEEMS API server must store aggregate metrics of each compute unit, it must query 
some sort of external DB that stores time series metrics of the compute units to 
estimate aggregate metrics. As CEEMS ships an exporter that is capable of exporting 
metrics to a Prometheus TSDB, a straight forward approach is to deploy CEEMS exporter 
on compute nodes and query Prometheus to estimate aggregate metrics.

This is done using a sub-component of CEEMS API server called updater. The job of 
updater is to update the compute units fetched by a given resource manager with the 
aggregate metrics of that compute unit. Like in the case of resource manager, updater 
uses factory design pattern and it is extensible. It is possible to use custom 
third party tools to update the compute units with aggregate metrics. 

Currently, CEEMS API server ships TSDB updater which is capable of estimating aggregate 
metrics using Prometheus TSDB server.

## Multi cluster support

A single deployment of CEEMS API server must be able to fetch and serve aggregate metrics 
of compute units of multiple clusters of either same resource manager or different 
resource managers. This means single CEEMS API server can store and serve metrics data 
of multiple SLURM, Openstack and Kubernetes clusters. 

In the same way, irrespective of each cluster using its own dedicated TSDB or a shared 
TSDB with other clusters, Updater sub component of CEEMS API server is capable of 
estimating aggregate metrics of each compute unit.

More details on how to configuration of multi-clusters can be found in [Configuration](../configuration/ceems-api-server.md) 
section and some example scenarios are discussed in [Advanced](../advanced/multi-cluster.md) 
section.
