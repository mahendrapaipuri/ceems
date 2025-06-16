---
sidebar_position: 5
---

# Redfish Proxy

## Flags

| Flag                   | Environment Variable               | Description                                                                                                                                                 | Default  |
|------------------------|------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------|----------|
| `--help`               |                                    | Show context-sensitive help (also try --help-long and --help-man).                                                                                          |          |
| `--version`            |                                    | Show application version.                                                                                                                                   |          |
| `--log.format`         |                                    | Output format of log messages. One of: [logfmt, json]                                                                                                       | `logfmt` |
| `--log.level`          |                                    | Only log messages with the given severity or above. One of: [debug, info, warn, error]                                                                      | `info`   |
| `--web.systemd-socket` |                                    | Use systemd socket activation listeners instead of port listeners (Linux only).                                                                             | `false`  |
| `--runtime.gomaxprocs` | `GOMAXPROCS`                       | The target number of CPUs Go will run on                                                                                                                    | 1        |
| `--web.config.file`    | `REDFISH_PROXY_WEB_CONFIG_FILE` | Path to configuration file that can enable TLS or authentication. [Docs](https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md) |          |
| `--web.listen-address` |                                    | Addresses on which to expose proxy server and web interface.                                                                                                  | `:5000`  |
| `--config.file`        | `REDFISH_PROXY_CONFIG_FILE`     | Configuration file of redfish proxy file                                                                                                                 |   |
| `--web.debug-server`                       |                                  | Enable /debug/pprof profiling endpoints                                                                                                                                                                                                                                                                                                                                     | `false`          |
