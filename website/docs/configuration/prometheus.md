---
sidebar_position: 7
---

# Prometheus

In order to use the dashboards provided in the repository, a minor
[`metric_relabel_configs`](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#metric_relabel_configs)
must be provided for all the target groups that have NVIDIA GPUs where
`dcgm-exporter` is exporting metrics of the GPUs to Prometheus.

The following shows an example scrape configs where the target nodes
contains NVIDIA GPUs:

```yaml
scrape_configs:
  - job_name: "gpu-node-group"
    metric_relabel_configs:
      - source_labels: [UUID,GPU_I_ID]
        separator: '/'
        target_label: gpuuuid
      - regex: UUID
        action: labeldrop
      - regex: modelName
        action: labeldrop
    static_configs:
      - targets: ["http://gpu-0:9400", "http://gpu-1:9400", ...]
```

The `metric_relabel_configs` is merges labels `UUID` and `GPU_I_ID` which are
the UUID and MIG instance ID of GPU, respectively and sets it to `gpuuuid`
which is compatible with CEEMS exporter. Moreover the config also drops unused
`UUID` and `modelName` labels to reduce storage and cardinality.
