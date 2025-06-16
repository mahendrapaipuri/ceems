---
sidebar_position: 3
---

# CEEMS LB

## Flags

| Flag                   | Environment Variable               | Description                                                                                                                                                 | Default  |
|------------------------|------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------|----------|
| `--help`               |                                    | Show context-sensitive help (also try --help-long and --help-man).                                                                                          |          |
| `--version`            |                                    | Show application version.                                                                                                                                   |          |
| `--log.format`         |                                    | Output format of log messages. One of: [logfmt, json]                                                                                                       | `logfmt` |
| `--log.level`          |                                    | Only log messages with the given severity or above. One of: [debug, info, warn, error]                                                                      | `info`   |
| `--web.systemd-socket` |                                    | Use systemd socket activation listeners instead of port listeners (Linux only).                                                                             | `false`  |
| `--runtime.gomaxprocs` | `GOMAXPROCS`                       | The target number of CPUs Go will run on                                                                                                                    | 1        |
| `--web.config.file`    | `CEEMS_LB_WEB_CONFIG_FILE` | Path to configuration file that can enable TLS or authentication. [Docs](https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md) |          |
| `--web.listen-address` |                                    | Addresses on which to expose load balancer(s). When both TSDB and Pyroscope LBs are configured, it must be repeated to provide two addresses: one for TSDB LB and one for Pyroscope LB. In that case TSDB LB will listen on first address and Pyroscope LB on second address.                                                                                                  | `:9030`  |
| `--config.file`        | `CEEMS_LB_CONFIG_FILE`     | Path to CEEMS LB configuration file                                                                                                                 |   |
| `--config.file.expand-env-vars`        |     |  Any environment variables that are referenced in config file will be expanded. To escape $ use $$                                                                                                                                                                                                        | `false`  |
