#!/bin/bash

# Need to use this path in --collector.slurm.job.props.path flag for ceems_exporter
DEST=/run/slurmjobprops
[ -e $DEST ] || mkdir -m 755 $DEST

# Important to keep the order as SLURM_JOB_USER SLURM_JOB_ACCOUNT SLURM_JOB_NODELIST
echo $SLURM_JOB_USER $SLURM_JOB_ACCOUNT $SLURM_JOB_NODELIST > $DEST/$SLURM_JOB_ID
exit 0 
