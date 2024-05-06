---
sidebar_position: 2
---

# CEEMS API server

CEEMS API server serves the usage and compute unit details of users _via_ API end points. 
This data will be gathered from the underlying resource manager and 
keep it in a local DB. 

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
of both SLURM and Openstack clusters with the same instance of Grafana in different dashboards.

:::

## Objectives

CEEMS API server primarily serves for two objectives:

- To store the compute unit information of different resource managers in a unified way.
The information we need is very basic like unique identifier of compute unit, project it
belongs to, owner, current state, when it has started, resources allocated, _etc_.
- To update aggregate metrics of each compute unit by querying the TSDB in realtime. 
This allows the end users to view the usage of their workloads in realtime like CPU, 
energy, emissions, _etc_.

When coupled with 
[JSON API DataSource](https://grafana.github.io/grafana-json-datasource/installation/) or 
[Infinity DataSource](https://grafana.com/grafana/plugins/yesoreyeram-infinity-datasource/)
of Grafana, we can list the compute units of a user 
along with the metadata and aggregate metrics of each unit. The stored metadata in the 
CEEMS API server DB will allow us to construct the URLs for the Grafana dashboards for 
each compute dynamically based on start and end time of each compute unit.

![User job list](/img/dashboards/job_list_user.png)

## Access control

CEEMS API server is not meant to expose to end users directly as it does not provide
a user centric authentication and authorization. It is only meant to use either as 
Grafana JSON datasource or using custom CLI script that control the authentication 
and setting headers for making requests.

### Using with Grafana

CEEMS API server depends on Grafana's user header to identify user that is making the 
request. The following configuration on Grafana are prerquisities for CEEMS API server 
to work properly:

- The setting [`send_user_header`](https://grafana.com/docs/grafana/latest/setup-grafana/configure-grafana/#send_user_header) 
must be set to `true` for Grafana to set `X-Grafana-User` in the requests. 

- When configuring the TSDB data source, either vanilla Prometheus or 
[CEEMS Load balancer](./ceems-lb.md), we need to set `access` mode to `proxy`. This means
Grafana will make the requests to TSDB from the backend server and sets the user header 
on the backend server. This prevents the users spoofing the user header `X-Grafana-User`.

Thus, the current user on Grafana is identified by header `X-Grafana-User` and the CEEMS 
will authorise the user to fetch only their own compute units. 

:::important[IMPORTANT]

For this approach to work, the identify provider that is used by the resource manager 
and Grafana must be the same one or if different identity providers are being used, 
the usernames must match for the same user.

:::

This server can be used as 
[JSON API DataSource](https://grafana.github.io/grafana-json-datasource/installation/) or 
[Infinity DataSource](https://grafana.com/grafana/plugins/yesoreyeram-infinity-datasource/)
in Grafana to fetch data and expose it to end users. 

### Using custom CLI scripts

Operators can develop custom CLI scripts that use the CEEMS API server for making requests 
and output the usage statistics to the users. CEEMS API server supports basic authentication 
and it is discussed in [Configuration section](../configuration/basic-auth.md). The custom 
script can use the basic auth and set the appropriate user header `X-Grafana-User` based 
on the user who is executing the script to make requests to the server. 

## For Admins/Operators

CEEMS API server supports admin users with privileged access. These users can 
be configured using [CLI arguments](../usage/ceems-api-server.md). Besides, it is 
possible to synchornize users from a given Grafana team periodically. This lets the 
operators to create a dedicated admin team for CEEMS API server and add userd dynamically 
to this team. CEEMS API server will synchronize the members of the team and access 
admin privileges for accessing CEEMS API server's DB.

:::important[IMPORTANT]

Admin users are also identified by the user header `X-Grafana-User` and if the current 
user name is in the list of admin users, that user will be able to access admin 
end points.

:::
