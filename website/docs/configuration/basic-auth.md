---
sidebar_position: 1
---

# Web security

## Authentication

All the CEEMS components support basic authentication and it is the only authentication 
method supported. The rationale is that the none of the CEEMS components are meant to 
be exposed to the end users directly. They are all system level services that are 
consumed by other services like Grafana to expose the data to the users. Thus, to 
keep the components simple and maintainable, only basic authentication is supported.

CEEMS uses [Prometheus exporter toolkit](https://github.com/prometheus/exporter-toolkit)
for basic authentication and TLS support. Basic auth can be configured using a configuration 
file that can be passed to each component using CLI argument. A 
[sample configuration file](https://github.com/mahendrapaipuri/ceems/blob/setup_docs/build/config/common/web-config.yml) 
is provided in the repository for the reference. 

A sample basic auth configuration can be set as follows:

```yaml
basic_auth_users:
  <username>: <hashed_password>
```

where `<username>` is the username of basic auth user and `<hashed_password>` is 
the basic auth password that must be hashed with `bcrypt`. An example to generate 
such hashed passwords is:

```bash
htpasswd -nBC 10 "" | tr -d ':\n'
```

This command will prompt the user to input the password and outputs the hashed password.
Multiple basic auth users can be configured using username, hashed password pair for 
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
file. Besides, there are more advanced options available for TLS and they are explained
in [comments in the sample file](https://github.com/mahendrapaipuri/ceems/blob/setup_docs/build/config/common/web-config.yml).
