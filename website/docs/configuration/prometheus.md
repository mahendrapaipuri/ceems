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
  # Scrape job containing NVIDIA DCGM exporter targets
  - job: <job-name>
    metric_relabel_configs:
      - source_labels:
          - modelName
          - UUID
        target_label: gpuuuid
        regex: NVIDIA(.*);(.*)
        replacement: $2
        action: replace
      - source_labels:
          - modelName
          - GPU_I_ID
        target_label: gpuiid
        regex: NVIDIA(.*);(.*)
        replacement: $2
        action: replace
      - regex: UUID
        action: labeldrop
      - regex: GPU_I_ID
        action: labeldrop

  # Scrape job containing AMD SMI exporter targets
  - job: <job-name>
    metric_relabel_configs:
      - source_labels:
          - gpu_power
        target_label: index
        regex: (.*)
        replacement: $1
        action: replace
      - source_labels:
          - index
          - gpu_use_percent
        target_label: index
        regex: ;(.+)
        replacement: $1
        action: replace
      - source_labels:
          - index
          - gpu_memory_use_percent
        target_label: index
        regex: ;(.+)
        replacement: $1
        action: replace
      - regex: gpu_power
        action: labeldrop
      - regex: gpu_use_percent
        action: labeldrop
      - regex: gpu_memory_use_percent
        action: labeldrop
```

The `metric_relabel_configs` renames `UUID` and `GPU_I_ID` which are
the UUID and MIG instance ID of NVIDIA GPU, respectively and sets it to `gpuuuid` and
`gpuiid` which are compatible with CEEMS exporter. Moreover the config also drops unused
`UUID` and `GPU_I_ID` labels to reduce storage.

Similarly, for AMD SMI exporter targets, `metric_relabel_configs` `gpu_power`,
`gpu_use_percent` and `gpu_memory_use_percent` labels, which provides GPU index,
to `index` that is compatible with CEEMS exporter.
