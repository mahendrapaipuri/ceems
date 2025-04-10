---
sidebar_position: 2
---

# CEEMS API Server

## Basic Usage

The CEEMS API server uses a simple model where core and web configurations are set by
two different files. Once these files are available, running the API server can be done
as follows:

```bash
ceems_api_server --config.file=/path/core/config/file --web.config.file=/path/to/web/config/file
```

The default interface and port can be changed using the `--web.listen-address` CLI argument:

```bash
ceems_api_server --web.listen-address="localhost:8020"
```

If the CEEMS API server is running behind a reverse proxy at a prefix `/ceems`, the path prefix can
be configured using:

```bash
ceems_api_server --web.route-prefix=/ceems
```

To limit the maximum query time period, use `--query.max-period`. For example, to set
the maximum allowable query period to 1 year:

```bash
ceems_api_server --query.max-period=1y
```

:::tip[TIP]

All available command-line options are listed in the
[CEEMS API Server CLI documentation](../cli/ceems-api-server.md).

:::

All endpoints of the CEEMS API server are discussed in detail in the dedicated [API documentation](/ceems/api).

## Access Control

The CEEMS API server is not meant to be exposed to end users directly as it does not provide
user-centric authentication and authorization. It is only meant to be used either as
a Grafana JSON data source or using custom CLI scripts that control the authentication
and set headers for making requests.

### Using with Grafana

The CEEMS API server depends on Grafana's user header to identify the user making the
request. The following configuration in Grafana are prerequisites for the CEEMS API server to work properly:

- The setting [`send_user_header`](https://grafana.com/docs/grafana/latest/setup-grafana/configure-grafana/#send_user_header)
must be set to `true` for Grafana to set `X-Grafana-User` in the requests.

- When configuring the TSDB data source using either vanilla Prometheus or
[CEEMS Load Balancer](../components/ceems-lb.md), we need to set the `access` mode to `proxy`.
This means Grafana will make the requests to TSDB from the backend server and set the user header
on the backend server. This prevents users from spoofing the user header `X-Grafana-User`.

Thus, the current user in Grafana is identified by the header `X-Grafana-User`, and the CEEMS
API server will serve information about the user's and their projects' compute units and
usage statistics.

:::important[IMPORTANT]

For this approach to work, the identity provider used by the resource manager
and Grafana must be the same, or if different identity providers are being used,
the usernames must match for the same user.

:::

The CEEMS API server can be used as an
[Infinity DataSource](https://grafana.com/grafana/plugins/yesoreyeram-infinity-datasource/)
in Grafana to fetch data and expose it to end users. We recommend users consult
the documentation of the above-stated Grafana plugins on how to configure the CEEMS API server
as a data source.

### Using Custom CLI Scripts

Operators can develop custom CLI scripts that use the CEEMS API server for making requests
and output the usage statistics to users. The CEEMS API server supports basic authentication,
which is discussed in the [Configuration section](../configuration/basic-auth.md). The custom
script can use basic authentication and set the appropriate user header `X-Grafana-User` based
on the user who is executing the script to make requests to the server.

## Admin Users

The CEEMS API server supports admin users with privileged access. These users can
be configured using the [Configuration file](../configuration/ceems-api-server.md). Additionally, it is
possible to synchronize users from a given Grafana team periodically. This lets
operators create a dedicated admin team for the CEEMS API server and add users dynamically
to this team. The CEEMS API server will synchronize the members of the team and grant
admin privileges for accessing the CEEMS API server's database.

:::important[IMPORTANT]

Admin users are also identified by the user header `X-Grafana-User`, and if the current
username is in the list of admin users, that user will be able to access admin
endpoints.

:::

Admin users have dedicated endpoints that end with `/admin` to make requests to the CEEMS
API server. For instance, if an admin wants to query a list of compute units of a user
`foo`, the request must be made to `http://localhost:9020/api/v1/units/admin?user=foo`,
assuming the CEEMS API server is running with default settings.