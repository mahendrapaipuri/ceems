---
sidebar_position: 1
---

# Web Security

## Authentication

All CEEMS components support basic authentication, and it is the only authentication
method supported. The rationale is that none of the CEEMS components are meant to
be exposed to end users directly. They are all system-level services that are
consumed by other services like Grafana to expose the data to users. Thus, to
keep the components simple and maintainable, only basic authentication is supported.

CEEMS uses the [Prometheus exporter toolkit](https://github.com/prometheus/exporter-toolkit)
for basic authentication and TLS support. Basic auth can be configured using a configuration
file that can be passed to each component using CLI arguments. A
[sample configuration file](https://github.com/@ceemsOrg@/@ceemsRepo@/blob/main/build/config/common/web-config.yml)
is provided in the repository for reference.

:::tip[TIP]

CEEMS provides a tooling application that can generate web configuration files with basic
auth and TLS with self-signed certificates. It can be used to generate a basic web
configuration file for different CEEMS components. More details can be found in the CEEMS Tool
[Usage Docs](../usage/ceems-tool.md) and [CLI Docs](../cli/ceems-tool.md).

:::

A sample basic auth configuration can be set as follows:

```yaml
basic_auth_users:
  <username>: <hashed_password>
```

where `<username>` is the username of the basic auth user and `<hashed_password>` is
the basic auth password that must be hashed with `bcrypt`. An example to generate
such hashed passwords is:

```bash
htpasswd -nBC 10 "" | tr -d ':\n'
```

This command will prompt the user to input the password and output the hashed password.
Multiple basic auth users can be configured using a username and hashed password pair for
each line.

## TLS

All CEEMS components support TLS using the same
[Prometheus exporter toolkit](https://github.com/prometheus/exporter-toolkit). A basic
TLS config can be as follows:

```yaml
# TLS and basic authentication configuration example.
#
# Additionally, a certificate and a key file are needed.
tls_server_config:
  cert_file: server.crt
  key_file: server.key
```

The files `server.crt` and `server.key` must exist in the same folder as the configuration
file. Additionally, there are more advanced options available for TLS, which are explained
in the [comments in the sample file](https://github.com/@ceemsOrg@/@ceemsRepo@/blob/main/build/config/common/web-config.yml).

## Reference

The following shows the reference for the web configuration file:

```yaml
tls_server_config:
  # Certificate for server to use to authenticate to client.
  # Expected to be passed as a PEM encoded sequence of bytes as a string.
  #
  # NOTE: If passing the cert inline, cert_file should not be specified below.
  [ cert: <string> ]

  # Key for server to use to authenticate to client.
  # Expected to be passed as a PEM encoded sequence of bytes as a string.
  #
  # NOTE: If passing the key inline, key_file should not be specified below.
  [ key: <secret> ]

  # CA certificate for client certificate authentication to the server.
  # Expected to be passed as a PEM encoded sequence of bytes as a string.
  #
  # NOTE: If passing the client_ca inline, client_ca_file should not be specified below.
  [ client_ca: <string> ]

  # Certificate and key files for server to use to authenticate to client.
  cert_file: <filename>
  key_file: <filename>

  # Server policy for client authentication. Maps to ClientAuth Policies.
  # For more detail on clientAuth options:
  # https://golang.org/pkg/crypto/tls/#ClientAuthType
  #
  # NOTE: If you want to enable client authentication, you need to use
  # RequireAndVerifyClientCert. Other values are insecure.
  [ client_auth_type: <string> | default = "NoClientCert" ]

  # CA certificate for client certificate authentication to the server.
  [ client_ca_file: <filename> ]

  # Verify that the client certificate has a Subject Alternate Name (SAN)
  # which is an exact match to an entry in this list, else terminate the
  # connection. SAN match can be one or multiple of the following: DNS,
  # IP, e-mail, or URI address from https://pkg.go.dev/crypto/x509#Certificate.
  [ client_allowed_sans:
    [ - <string> ] ]

  # Minimum TLS version that is acceptable.
  [ min_version: <string> | default = "TLS12" ]

  # Maximum TLS version that is acceptable.
  [ max_version: <string> | default = "TLS13" ]

  # List of supported cipher suites for TLS versions up to TLS 1.2. If empty,
  # Go default cipher suites are used. Available cipher suites are documented
  # in the Go documentation:
  # https://golang.org/pkg/crypto/tls/#pkg-constants
  #
  # Note that only the cipher returned by the following function are supported:
  # https://pkg.go.dev/crypto/tls#CipherSuites
  [ cipher_suites:
    [ - <string> ] ]

  # prefer_server_cipher_suites controls whether the server selects the
  # client's most preferred ciphersuite, or the server's most preferred
  # ciphersuite. If true then the server's preference, as expressed in
  # the order of elements in cipher_suites, is used.
  [ prefer_server_cipher_suites: <bool> | default = true ]

  # Elliptic curves that will be used in an ECDHE handshake, in preference
  # order. Available curves are documented in the Go documentation:
  # https://golang.org/pkg/crypto/tls/#CurveID
  [ curve_preferences:
    [ - <string> ] ]

http_server_config:
  # Enable HTTP/2 support. Note that HTTP/2 is only supported with TLS.
  # This cannot be changed on the fly.
  [ http2: <boolean> | default = true ]
  # List of headers that can be added to HTTP responses.
  [ headers:
    # Set the Content-Security-Policy header to HTTP responses.
    # Unset if blank.
    [ Content-Security-Policy: <string> ]
    # Set the X-Frame-Options header to HTTP responses.
    # Unset if blank. Accepted values are deny and sameorigin.
    # https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/X-Frame-Options
    [ X-Frame-Options: <string> ]
    # Set the X-Content-Type-Options header to HTTP responses.
    # Unset if blank. Accepted value is nosniff.
    # https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/X-Content-Type-Options
    [ X-Content-Type-Options: <string> ]
    # Set the X-XSS-Protection header to all responses.
    # Unset if blank.
    # https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/X-XSS-Protection
    [ X-XSS-Protection: <string> ]
    # Set the Strict-Transport-Security header to HTTP responses.
    # Unset if blank.
    # Please make sure that you use this with care as this header might force
    # browsers to load Prometheus and the other applications hosted on the same
    # domain and subdomains over HTTPS.
    # https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Strict-Transport-Security
    [ Strict-Transport-Security: <string> ] ]

# Usernames and hashed passwords that have full access to the web
# server via basic authentication. If empty, no basic authentication is
# required. Passwords are hashed with bcrypt.
basic_auth_users:
  [ <string>: <secret> ... ]
```
