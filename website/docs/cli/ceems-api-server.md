---
sidebar_position: 2
---

# CEEMS API Server

## Flags

| Flag                   | Environment Variable               | Description                                                                                                                                                 | Default  |
|------------------------|------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------|----------|
| `--help`               |                                    | Show context-sensitive help (also try --help-long and --help-man).                                                                                          |          |
| `--version`            |                                    | Show application version.                                                                                                                                   |          |
| `--log.format`         |                                    | Output format of log messages. One of: [logfmt, json]                                                                                                       | `logfmt` |
| `--log.level`          |                                    | Only log messages with the given severity or above. One of: [debug, info, warn, error]                                                                      | `info`   |
| `--web.systemd-socket` |                                    | Use systemd socket activation listeners instead of port listeners (Linux only).                                                                             | `false`  |
| `--runtime.gomaxprocs` | `GOMAXPROCS`                       | The target number of CPUs Go will run on                                                                                                                    | 1        |
| `--web.config.file`    | `CEEMS_API_SERVER_WEB_CONFIG_FILE` | Path to configuration file that can enable TLS or authentication. [Docs](https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md) |          |
| `--web.listen-address` |                                    | Addresses on which to expose API server and web interface.                                                                                                  | `:9020`  |
| `--config.file`        | `CEEMS_API_SERVER_CONFIG_FILE`     | Path to CEEMS API server configuration file                                                                                                                 | `false`  |
| `--web.debug-server`                       |                                  | Enable /debug/pprof profiling endpoints                                                                                                                                                                                                                                                                                                                                     | `false`          |
