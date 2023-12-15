# Batch job metrics monitoring stack

This repository contains a Prometheus exporter to export job metrics of batch jobs 
like SLURM, PBS, LSF, _etc_ and other utility tools that can be used to deploy a 
fully functional monitoring stack on a HPC platform.

## Design objectives

### CPU, memory and IO metrics

The main design objective of this stack is to gathering job metrics _via_ cgroups and 
avoid using batch scheduler native tools like `sacct` for SLURM. The rationale is that 
on huge HPC platforms (nodes > 2000), that churn few thousands of jobs at a given 
time, gathering time series job metrics from a tool like `sacct`, say every 10 sec, can 
put a lot of stress on DB which can negatively impact the performance of batch scheduler.

The idea we are leveraging here is that every resource manager has to resort to cgroups 
on Linux to manage the quota on CPU, memory and IO. Each resource manager does it 
differently but the take away here is that the accounting information is readily 
available in the cgroups. By walking through the cgroups file system, we can gather the 
job metrics that map them to a particular job as resource manager tends to create 
cgroups for each job with some sort of job identifier attached to it.

This is a distributed approach where exporter will run on each compute node and walk 
through cgroup file system whenever Prometheus make a scrape request and pull the data 
to Prometheus. As reading cgroups file system is relatively cheap, there is a very 
little overhead running this daemon service.  

### Energy consumption

In an age where green computing is becoming more and more important, it is essential to
expose the energy consumed by the batch jobs to the users to make them more aware. 
Most of energy measurement tools are based on 
[RAPL](https://www.kernel.org/doc/html/next/power/powercap/powercap.html) which reports 
mostly CPU and memory consumption. It does not report consumption from other peripherals 
like PCIe, network, disk, _etc_. 

To address this, the current exporter will expose IPMI power statistics in addition to 
RAPL metrics. IPMI measurements are generally made at the node level which includes 
consumption by _most_ of the components. However, the implementations are vendor 
dependent and it is desirable to validate with them before reading too much into the 
numbers. In any case, this is the only complete metric we can get our hands on without 
needing to install any additional hardware like Wattmeters. 

This monitoring power consumption can be split into consumption of individual batch jobs
by using relative CPU times used by batch job. Although, this is not an exact 
estimation of power consumed by the batch job, it stays a very good approximation.

### Emissions

The exporter is capable of exporting emission factor data which can be used in 
conjunction energy consumption to estimate equivalent CO2 emissions. Currently, for 
France, a _real_ time emission factor will be used that is based on 
[RTE eCO2 mix data](https://www.rte-france.com/en/eco2mix/co2-emissions). For other 
countries, a constant average based on historic data will be used. This historic data 
is gathered from [CodeCarbon's DB](https://raw.githubusercontent.com/mlco2/codecarbon/master/codecarbon/data/private_infra/global_energy_mix.json).

### GPU metrics

Currently, only nVIDIA GPUs are supported. This exporter leverages 
[DCGM exporter](https://github.com/NVIDIA/dcgm-exporter/tree/main) to get GPU metrics of
each job. The current exporter only exposes the GPU index to job mapping which will be 
used in Grafana dashboard to show GPU metrics of each job.

## Repository contents

This monorepo contains three main utils that are essential for the batch job monitoring 
stack.

- `batchjob_exporter`: This is the Prometheus exporter that exposes individual job 
metrics, RAPL energy, IPMI power consumption, emission factor and GPU to batch job 
mapping.

- `batchjob_stats_db`: As batch jobs are ephemeral, we need at least start and stop 
times of each job to be able to limit the time window in Grafana dashboard while 
browsing through metrics. This binary will pull these job statistics from batch scheduler 
at configured interval of time and keeps in a local DB based on SQLite3.

- `batchjob_stats_server`: This is a simple API server that exposes accounts and jobs 
information of users by looking at the SQLite3 DB populated by `batchjob_stats_db`. 
This server will be used as 
[JSON API DataSource](https://grafana.github.io/grafana-json-datasource/installation/) 
in Grafana to construct dashboards for users.
