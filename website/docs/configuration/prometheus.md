---
sidebar_position: 6
---

# Prometheus

In order to use the dashboards provided in the repository, minor
[`metric_relabel_configs`](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#metric_relabel_configs)
configuration must be provided for all target groups that have NVIDIA GPUs where
the `dcgm-exporter` exports GPU metrics to Prometheus.

:::tip[TIP]

[`ceems_tool`](../usage/ceems-tool.md) can be used to generate the relabel configs based on your
current Prometheus targets.

:::

The following example shows scrape configurations where the target nodes contain NVIDIA GPUs:

```yaml
scrape_configs:
  # Scrape job containing NVIDIA DCGM exporter targets
  - job_name: <job-name>
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
  # https://github.com/amd/amd_smi_exporter
  - job_name: <job-name>
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

  # Scrape job containing AMD device metrics exporter targets
  # https://github.com/ROCm/device-metrics-exporter/
  - job_name: <job-name>
    metric_relabel_configs:
      - source_labels:
          - gpu_id
        target_label: index
        regex: (.*)
        replacement: $1
        action: replace
      - source_labels:
          - gpu_partition_id
        target_label: gpuiid
        regex: (.*)
        replacement: $1
        action: replace
      - regex: gpu_id
        action: labeldrop
      - regex: gpu_partition_id
        action: labeldrop
```

The `metric_relabel_configs` section renames the `UUID` and `GPU_I_ID` labels
(which represent the UUID and MIG instance ID of the NVIDIA GPU, respectively) to
`gpuuuid` and `gpuiid`, making them compatible with the CEEMS exporter. Moreover,
the configuration also drops the unused `UUID` and `GPU_I_ID` labels to reduce
storage usage.

Similarly, for [AMD SMI exporter](https://github.com/amd/amd_smi_exporter) targets,
the `metric_relabel_configs` section
extracts the GPU index from the `gpu_power`, `gpu_use_percent`, and
`gpu_memory_use_percent` labels and maps it to the `index` label, which is
compatible with the CEEMS exporter. In the case of
[AMD Device Metrics Exporter](https://github.com/ROCm/device-metrics-exporter/),
the GPU index is exported as `gpu_id` and GPU partition index is exported by
`gpu_partition_id` which are replaced by `index` and `gpuiid`.
