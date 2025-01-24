---
sidebar_position: 2
---

# HPC Platforms

Following shows a reference deployment of CEEMS alongside Prometheus and Grafana on
typical HPC platforms.

## Using Thanos

![CEEMS with Thanos](/img/deployment/with_thanos.png)

### Takeaways

- Thanos is used here for replication and long term storage of TSDB data
- [Litestream](https://litestream.io/) is used to replicate and create snapshots of
CEEMS API server SQLite DB.
- [Minio](https://min.io/) is used to store TSDB data on typical parallel file systems
like Lustre, Spectrum Scale, BGFS, _etc_ of HPC platforms.
- CEEMS load balancer is used to enforce access control on TSDB backends.

## Using Prometheus' remote write

![CEEMS with Prometheus' remote write](/img/deployment/with_remote_write.jpg)

### Takeaways

- A second instance of Prometheus with its remote write protocol is used to replicate
the data and for long term storage.
- [Litestream](https://litestream.io/) is used to replicate and create snapshots of
CEEMS API server SQLite DB.
- CEEMS load balancer is used to enforce access control on TSDB backends.
