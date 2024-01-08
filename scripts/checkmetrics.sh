#!/usr/bin/env bash

if [[ ( -z "$1" ) || ( -z "$2" ) ]]; then
    echo "usage: ./checkmetrics.sh /usr/bin/promtool output_dir"
    exit 1
fi

# Ignore known issues in auto-generated and network specific collectors.
search_dir="$2"
for entry in "$search_dir"/*
do
  lint=$($1 check metrics < "$entry" 2>&1 | grep -v -E "^batchjob_slurm_job_(memory_fail_count|memsw_fail_count)|batchjob_meminfo_")

  if [[ -n $lint ]]; then
      echo -e "Some Prometheus metrics do not follow best practices:\n"
      echo "$lint"

      exit 1
  fi
done

