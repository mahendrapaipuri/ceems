#!/bin/bash

# Need to use this path in --collector.nvidia.gpu.job.map.path flag for ceems_exporter
DEST=/run/gpujobmap
[ -e $DEST ] || mkdir -m 755 $DEST

# CUDA_VISIBLE_DEVICES in prolog will be "actual" GPU indices and once job starts
# CUDA will reset the indices to always start from 0. Thus inside a job, CUDA_VISIBLE_DEVICES
# will always start with 0 but during prolog script execution it can be any ordinal index
# based on how SLURM allocated the GPUs
# Ref: https://slurm.schedmd.com/prolog_epilog.html
for i in ${GPU_DEVICE_ORDINAL//,/ } ${CUDA_VISIBLE_DEVICES//,/ }; do
  echo $SLURM_JOB_ID > $DEST/$i
done
exit 0 
