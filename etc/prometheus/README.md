# Prometheus config

The [rules](https://github.com/mahendrapaipuri/ceems/tree/main/etc/prometheus/rules)
directory contain sample recording rules files that can be used to estimate the
energy and emissions time series metrics of each compute unit derived from different
sources.

## Rules

The following recording rules must be modified appropriately according to the target
cluster environment and added to the Prometheus deployment to estimate power usage
of individual compute units.

### `cpu-only-nodes.rules`

The rules defined in this file are meant to be used for group of nodes that do not
have any GPUs. The rules make the following assumptions:

- Total server energy is reported by IPMI DCMI
- RAPL counters are available for both CPU and DRAM packages

The provided rules estimate the power usage of individual compute units based on
compute unit CPU and DRAM usage and total node's CPU and DRAM usage. More details
are provided in the comments of the rules file.

### `cpu-gpu-nodes.rules`

The rules defined in this file are meant to be used for group of nodes that have
GPUs (NVIDIA or AMD). The rules make the following assumptions:

- Total server energy is reported by IPMI DCMI and it **contains** power usage
by GPUs as well
- RAPL counters are available for both CPU and DRAM packages

As power usage reported by IPMI DCMI contains both CPU and GPU, we need to remove
power usage by GPU to get the power usage by CPU alone. To do so, we leverage the
power usage reported by [NVIDIA DCGM exporter](https://github.com/NVIDIA/dcgm-exporter).

### `gpu.rules`

The rules defined in this file are meant to be used for group of nodes that have
GPUs (NVIDIA or AMD). The rules compute few derived metrics from metrics reported
by [NVIDIA DCGM exporter](https://github.com/NVIDIA/dcgm-exporter) that are
relevant to monitor overall cluster status.

## Installing rules

The rules files must be modified appropriately by using correct job names and installed
to Prometheus deployment. For instance, imagine a target cluster can be grouped as follows:

- `cpu-partition-1`: A group of nodes with only CPUs
- `cpu-partition-2`: Another group of nodes with only CPUs
- `v100-partition-1`: A group of nodes with V100 GPUs
- `a100-partition-1`: A group of nodes with A100 GPUs

And operators defined a prometheus job for each group using the same names as used above.
CEEMS exporter must be deployed on all the nodes and
[NVIDIA DCGM exporter](https://github.com/NVIDIA/dcgm-exporter) on groups `v100-partition-1`
and `a100-partition-1`. Assume DCGM targets are placed in Prometheus job with `dcgm-` as suffix
to the group name. For example, DCGM targets on group `v100-partition-1` will be in a job
`dcgm-v100-partition-1`. Moreover imagine that the IPMI DCMI reports only CPU power usage
for the group `v100-partition-1` where as it reports both CPU and GPU for the group
`a100-partition-1`. In this case, rules files can be generated as follows:

```bash
# Create a folder to keep all created rules files
mkdir -p cluster_rules

# Create rules files for cpu-partition-1 and cpu-partition-2
sed 's/<sample-cpu>/<cpu-partition-1>/g' cpu-only-nodes.rules > cluster_rules/cpu-partition-1.rules
sed 's/<sample-cpu>/<cpu-partition-2>/g' cpu-only-nodes.rules > cluster_rules/cpu-partition-2.rules

# Create rules files for v100-partition-1
sed 's/<sample-gpu>/<v100-partition-1>/g' cpu-only-nodes.rules > cluster_rules/v100-partition-1.rules
sed 's/<sample-dcgm>/<dcgm-v100-partition-1>/g' gpu.rules > cluster_rules/dcgm-v100-partition-1.rules

# Create rules files for a100-partition-1
sed -e 's/<sample-gpu>/<a100-partition-1>/g' -e 's/<sample-dcgm>/<dcgm-a100-partition-1>/g' cpu-gpu-nodes.rules > cluster_rules/a100-partition-1.rules
sed 's/<sample-dcgm>/<dcgm-a100-partition-1>/g' gpu.rules > cluster_rules/dcgm-a100-partition-1.rules
```

The generated rules can be verified using [`promtool`](https://prometheus.io/docs/prometheus/latest/command-line/promtool/)

```bash
find cluster_rules -name "*.rules" | xargs -I {} promtool check rules {}
```

Finally, all the rules files must be placed under the folder provided to `rules_files` key
in Prometheus [config](https://prometheus.io/docs/prometheus/latest/configuration/configuration/).

Once the rules have been installed, restart/reload Prometheus.
