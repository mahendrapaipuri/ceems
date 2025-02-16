---
sidebar_position: 2
---

# CEEMS API Server

## Basic usage

CEEMS API server uses a simple model where core and web configurations are set by
two different files. Once these files are available, running API server can be done
as follows:

```bash
ceems_api_server --config.file=/path/core/config/file --web.config.file=/path/to/web/config/file
```

Default interface and port can be changed using `--web.listen-address` CLI argument.

```bash
ceems_api_server --web.listen-address="localhost:8020"
```

:::tip[TIP]

All the available command line options are listed in
[CEEMS API Server CLI docs](../cli/ceems-api-server.md).

:::

All the endpoints of CEEMS API server are discussed in detail in a dedicated
[API documentation](/ceems/api).

## Access control

CEEMS API server is not meant to expose to end users directly as it does not provide
a user centric authentication and authorization. It is only meant to use either as
Grafana JSON datasource or using custom CLI script that control the authentication
and setting headers for making requests.

### Using with Grafana

CEEMS API server depends on Grafana's user header to identify user that is making the
request. The following configuration on Grafana are prerequisites for CEEMS API server
to work properly:

- The setting [`send_user_header`](https://grafana.com/docs/grafana/latest/setup-grafana/configure-grafana/#send_user_header)
must be set to `true` for Grafana to set `X-Grafana-User` in the requests.

- When configuring the TSDB data source using either vanilla Prometheus or
[CEEMS Load balancer](../components//ceems-lb.md), we need to set `access` mode to `proxy`.
This means Grafana will make the requests to TSDB from the backend server and sets the user header
on the backend server. This prevents the users spoofing the user header `X-Grafana-User`.

Thus, the current user on Grafana is identified by header `X-Grafana-User` and the CEEMS
API server will serve the information about user's and their projects compute units and
usage statistics

:::important[IMPORTANT]

For this approach to work, the identity provider that is used by the resource manager
and Grafana must be the same one or if different identity providers are being used,
the usernames must match for the same user.

:::

CEEMS API server can be used as
[Infinity DataSource](https://grafana.com/grafana/plugins/yesoreyeram-infinity-datasource/)
in Grafana to fetch data and expose it to end users. We recommend the users to consult
the documentation of above stated Grafana plugins on how to configure CEEMS API server
as a datasource.

### Using custom CLI scripts

Operators can develop custom CLI scripts that use the CEEMS API server for making requests
and output the usage statistics to the users. CEEMS API server supports basic authentication
and it is discussed in [Configuration section](../configuration/basic-auth.md). The custom
script can use the basic auth and set the appropriate user header `X-Grafana-User` based
on the user who is executing the script to make requests to the server.

## Admin users

CEEMS API server supports admin users with privileged access. These users can
be configured using [Configuration file](../configuration/ceems-api-server.md). Besides, it is
possible to synchronize users from a given Grafana team periodically. This lets the
operators to create a dedicated admin team for CEEMS API server and add users dynamically
to this team. CEEMS API server will synchronize the members of the team and access
admin privileges for accessing CEEMS API server's DB.

:::important[IMPORTANT]

Admin users are also identified by the user header `X-Grafana-User` and if the current
user name is in the list of admin users, that user will be able to access admin
end points.

:::

Admin users have dedicated endpoints that end with `/admin` to make requests to CEEMS
API server. For instance, if an admin wants to query a list of compute units of a user
`foo`, the request must be made to `http://localhost:9020/api/v1/units/admin?user=foo`
assuming CEEMS API server is running with default settings.
