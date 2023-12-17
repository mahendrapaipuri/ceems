#!/bin/bash

# Need to use this path in --collector.nvidia.gpu.job.map.path flag for batchjob_exporter
DEST=/run/gpujobmap
[ -e $DEST ] || mkdir -m 755 $DEST

# CUDA_VISIBLE_DEVICES in epilog will be "actual" GPU indices and once job starts
# CUDA will reset the indices to always start from 0. Thus inside a job, CUDA_VISIBLE_DEVICES
# will always start with 0 but during epilog script execution it can be any ordinal index
# based on how SLURM allocated the GPUs
# Ref: https://slurm.schedmd.com/prolog_epilog.html
for i in ${GPU_DEVICE_ORDINAL//,/ } ${CUDA_VISIBLE_DEVICES//,/ }; do
  rm -rf $DEST/$i
done
exit 0 
