#!/usr/bin/env bash

if [[ ( -z "$1" ) || ( -z "$2" ) ]]; then
    echo "usage: ./checkmetrics.sh /usr/bin/promtool output_dir"
    exit 1
fi

# Ignore known issues in auto-generated and network specific collectors.
search_dir="$2"
for entry in "$search_dir"/*
do
  lint=$($1 check metrics < "$entry" 2>&1 | grep -v -E "^ceems_compute_unit_(memory_fail_count|memsw_fail_count)|ceems_meminfo_|ceems_cpu_count|ceems_cpu_per_core_count|ceems_compute_unit_gpu_sm_count")

  if [[ -n $lint ]]; then
      echo -e "Some Prometheus metrics do not follow best practices:\n"
      echo "$lint"

      exit 1
  fi
done

