---
sidebar_position: 5
---

# CEEMS Tool

## Flags

| Flag                   | Description                                                                                                                                                 |
|------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `--help`               |  Show context-sensitive help (also try --help-long and --help-man).                                                                                          |
| `--version`            | Show application version.                                                                                                                                   |

## Commands

| Command  | Description                      |
|----------|----------------------------------|
| `check`  | Check the resources for validity |
| `config` | Configuration related tooling    |
| `tsdb`   | TSDB related commands            |

### `ceems_tool check`

Check the resources for validity.

#### `ceems_tool check api-healthy`

Check if the CEEMS API server is healthy.

| Flag                 | Description                                                                   | Default                 |
|----------------------|-------------------------------------------------------------------------------|-------------------------|
| `--http.config.file` | HTTP client configuration file for ceems_tool to connect to CEEMS API server. |                         |
| `--url`              | The URL for the CEEMS API server.                                             | `http://localhost:9020` |

#### `ceems_tool check lb-healthy`

Check if the CEEMS LB server is healthy.

| Flag                 | Description                                                                   | Default                 |
|----------------------|-------------------------------------------------------------------------------|-------------------------|
| `--http.config.file` | HTTP client configuration file for ceems_tool to connect to CEEMS LB server. |                         |
| `--url`              | The URL for the CEEMS LB server.                                             | `http://localhost:9030` |

#### `ceems_tool check web-config`

Check if the web config files are valid or not.

| Argument                 | Description                                                                   |
|----------------------|-------------------------------------------------------------------------------|
| `web-config-files` | Web configuration files. |

#### `ceems_config create-web-config`

Create web config file for CEEMS components.

| Flag             | Description                                               | Default  |
|------------------|-----------------------------------------------------------|----------|
| `--basic-auth`   | Create web config file with basic auth.                   | `true`   |
| `--tls`          | Create web config file with self signed TLS certificates. | `false`  |
| `--tls.host`     | Hostnames and/or IPs to generate a certificate for.       |          |
| `--tls.validity` | Validity for TLS certificates.                            | 1 year   |
| `--output-dir`   | Output directory to place config files.                   | `config` |

### `ceems_tool tsdb create-recording-rules`

Create Prometheus recording rules.

| Flag                 | Description                                                                                 | Default                 |
|----------------------|---------------------------------------------------------------------------------------------|-------------------------|
| `--http.config.file` | HTTP client configuration file for ceems_tool to connect to Prometheus server.              | `true`                  |
| `--url`              | The URL for the Prometheus server.                                                          | `http://localhost:9090` |
| `--start`            | The time to start querying for metrics. Must be a RFC3339 formatted date or Unix timestamp. | current time - 3 hr     |
| `--end`              | The time to end querying for metrics. Must be a RFC3339 formatted date or Unix timestamp.   | current time            |
| `--pue`              | Power Usage Effectiveness (PUE) value to use in power estimation rules.                     | 1                       |
| `--emission-factor`  | Static emission factor in gCO2/kWh value to use in equivalent emission estimation rules.    | 0                       |
| `--country-code`     | ISO-2 code of the country to use in emissions estimation rules.                             |                         |
| `--eval-interval`    | Evaluation interval for the rules. If not set, default will be used.                        |                         |
| `--output-dir`       | Output directory to place config files.                                                     | `rules`                 |

### `ceems_tool tsdb create-relabel-configs`

Create Prometheus relabel configs.

| Flag                 | Description                                                                                 | Default                 |
|----------------------|---------------------------------------------------------------------------------------------|-------------------------|
| `--http.config.file` | HTTP client configuration file for ceems_tool to connect to Prometheus server.              | `true`                  |
| `--url`              | The URL for the Prometheus server.                                                          | `http://localhost:9090` |
| `--start`            | The time to start querying for metrics. Must be a RFC3339 formatted date or Unix timestamp. | current time - 3 hr     |
| `--end`              | The time to end querying for metrics. Must be a RFC3339 formatted date or Unix timestamp.   | current time            |

### `ceems_tool tsdb create-ceems-tsdb-updater-queries`

Create CEEMS API TSDB updater queries.

| Flag                 | Description                                                                                 | Default                 |
|----------------------|---------------------------------------------------------------------------------------------|-------------------------|
| `--http.config.file` | HTTP client configuration file for ceems_tool to connect to Prometheus server.              | `true`                  |
| `--url`              | The URL for the Prometheus server.                                                          | `http://localhost:9090` |
| `--start`            | The time to start querying for metrics. Must be a RFC3339 formatted date or Unix timestamp. | current time - 3 hr     |
| `--end`              | The time to end querying for metrics. Must be a RFC3339 formatted date or Unix timestamp.   | current time            |
