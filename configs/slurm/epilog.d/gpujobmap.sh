#!/bin/bash

# Need to use this path in --collector.nvidia.gpu.job.map.path flag for batchjob_exporter
DEST=/run/gpujobmap
[ -e $DEST ] || mkdir -m 755 $DEST

# Ensure to remove the file with jobid once the job finishes
for i in ${GPU_DEVICE_ORDINAL//,/ } ${CUDA_VISIBLE_DEVICES//,/ }; do
  rm -rf $DEST/$i
done
exit 0 
