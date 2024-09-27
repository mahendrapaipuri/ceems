---
sidebar_position: 7
---

# Configuration Reference

The following reference applies to configuration files of CEEMS API server, CEEMS LB and
web configuration. CEEMS uses Prometheus' [client config](https://github.com/prometheus/common/tree/main/config)
to configure HTTP clients. Thus, most of the configuration that is used to configure
HTTP clients resemble that of Prometheus'. The configuration reference has also been
inspired from Prometheus docs.

The file is written in [YAML format](https://en.wikipedia.org/wiki/YAML),
defined by the scheme described below.
Brackets indicate that a parameter is optional. For non-list parameters the
value is set to the specified default.

Generic placeholders are defined as follows:

* `<boolean>`: a boolean that can take the values `true` or `false`
* `<duration>`: a duration matching the regular expression `((([0-9]+)y)?(([0-9]+)w)?(([0-9]+)d)?(([0-9]+)h)?(([0-9]+)m)?(([0-9]+)s)?(([0-9]+)ms)?|0)`, e.g. `1d`, `1h30m`, `5m`, `10s`
* `<filename>`: a valid path in the current working directory
* `<float>`: a floating-point number
* `<host>`: a valid string consisting of a hostname or IP followed by an optional port number
* `<int>`: an integer value
* `<path>`: a valid URL path
* `<scheme>`: a string that can take the values `http` or `https`
* `<secret>`: a regular string that is a secret, such as a password
* `<string>`: a regular string
* `<size>`: a size in bytes, e.g. `512MB`. A unit is required. Supported units: B, KB, MB, GB, TB, PB, EB.
* `<idname>`: a string matching the regular expression `[a-zA-Z_-][a-zA-Z0-9_-]*`. Any other unsupported
character in the source label should be converted to an underscore
* `<managername>`: a string that identifies resource manager. Currently accepted values are `slurm`.
* `<updatername>`: a string that identifies updater type. Currently accepted values are `tsdb`.
* `<promql_query>`: a valid PromQL query string.
* `<lbstrategy>`: a valid load balancing strategy. Currently accepted values are `round-robin`, `least-connection` and `resource-based`.

The other placeholders are specified separately.

## `<web_client_config>`

A `web_client_config` allows configuring HTTP clients.

```yaml
# Web URL of the Grafana instance
#
url: <host>

# Sets the `Authorization` header on every API request with the
# configured username and password.
# password and password_file are mutually exclusive.
#
basic_auth:
  [ username: <string> ]
  [ password: <secret> ]
  [ password_file: <string> ]

# Sets the `Authorization` header on every API request with
# the configured credentials.
#
authorization:
  # Sets the authentication type of the request.
  [ type: <string> | default: Bearer ]
  # Sets the credentials of the request. It is mutually exclusive with
  # `credentials_file`.
  [ credentials: <secret> ]
  # Sets the credentials of the request with the credentials read from the
  # configured file. It is mutually exclusive with `credentials`.
  [ credentials_file: <filename> ]

# Optional OAuth 2.0 configuration.
# Cannot be used at the same time as basic_auth or authorization.
#
oauth2: 
  [ <oauth2> ]

# Configure whether scrape requests follow HTTP 3xx redirects.
[ follow_redirects: <boolean> | default = true ]

# Whether to enable HTTP2.
[ enable_http2: <boolean> | default: true ]

# Configures the API request's TLS settings.
#
tls_config:
  [ <tls_config> ]

# List of headers that will be passed in the API requests to the server.
#
http_headers:
  [ <http_headers_config> ]
```

## `<oauth2>`

OAuth 2.0 authentication using the client credentials grant type. Prometheus fetches an
access token from the specified endpoint with the given client access and secret keys.

```yaml
client_id: <string>
[ client_secret: <secret> ]

# Read the client secret from a file.
# It is mutually exclusive with `client_secret`.
[ client_secret_file: <filename> ]

# Scopes for the token request.
scopes:
  [ - <string> ... ]

# The URL to fetch the token from.
token_url: <string>

# Optional parameters to append to the token URL.
endpoint_params:
  [ <string>: <string> ... ]

# Configures the token request's TLS settings.
tls_config:
  [ <tls_config> ]

# Optional proxy URL.
[ proxy_url: <string> ]
# Comma-separated string that can contain IPs, CIDR notation, domain names
# that should be excluded from proxying. IP and domain names can
# contain port numbers.
[ no_proxy: <string> ]
# Use proxy URL indicated by environment variables (HTTP_PROXY, https_proxy, HTTPs_PROXY, https_proxy, and no_proxy)
[ proxy_from_environment: <boolean> | default: false ]
# Specifies headers to send to proxies during CONNECT requests.
[ proxy_connect_header:
  [ <string>: [<secret>, ...] ] ]
```

## `<tls_config>`

A `tls_config` allows configuring TLS connections.

```yaml
# CA certificate to validate API server certificate with. At most one of ca and ca_file is allowed.
[ ca: <string> ]
[ ca_file: <filename> ]

# Certificate and key for client cert authentication to the server.
# At most one of cert and cert_file is allowed.
# At most one of key and key_file is allowed.
[ cert: <string> ]
[ cert_file: <filename> ]
[ key: <secret> ]
[ key_file: <filename> ]

# ServerName extension to indicate the name of the server.
# https://tools.ietf.org/html/rfc4366#section-3.1
[ server_name: <string> ]

# Disable validation of the server certificate.
[ insecure_skip_verify: <boolean> ]

# Minimum acceptable TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS
# 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3).
# If unset, Prometheus will use Go default minimum version, which is TLS 1.2.
# See MinVersion in https://pkg.go.dev/crypto/tls#Config.
[ min_version: <string> ]
# Maximum acceptable TLS version. Accepted values: TLS10 (TLS 1.0), TLS11 (TLS
# 1.1), TLS12 (TLS 1.2), TLS13 (TLS 1.3).
# If unset, Prometheus will use Go default maximum version, which is TLS 1.3.
# See MaxVersion in https://pkg.go.dev/crypto/tls#Config.
[ max_version: <string> ]
```

## `<http_headers_config>`

A `http_headers_config` allows configuring HTTP headers in requests.

```yaml
# Authentication related headers may be configured in this section. Header name
# must be configured as key and header value supports three different types of 
# headers: values, secrets and files.
#
# The difference between values and secrets is that secret will be redacted
# in server logs where as values will be emitted in the logs.
#
# Values are regular headers with values, secrets are headers that pass secret
# information like tokens and files pass the file content in the headers.
#
# Example:
# http_headers:
#   one:
#     values: [value1a, value1b, value1c]
#   two:
#     values: [value2a]
#     secrets: [value2b, value2c]
#   three:
#     files: [testdata/headers-file-a, testdata/headers-file-b, testdata/headers-file-c]
#
[ <string>: 
    values: 
      [- <string> ... ] 
    secrets: 
      [- <secret> ... ]
    files:
      [- <filename> ... ] ... ]
```
