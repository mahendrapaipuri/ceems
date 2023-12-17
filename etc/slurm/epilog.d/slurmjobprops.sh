#!/bin/bash

DEST=/run/slurmjobprops
[ -e $DEST ] || mkdir -m 755 $DEST
rm -rf $DEST/$SLURM_JOB_ID
exit 0 
