#!/usr/bin/env bash

set -euf -o pipefail

cd "$(dirname $0)/.."

port="$((10000 + (RANDOM % 10000)))"
tmpdir=$(mktemp -d /tmp/batchjob_exporter_e2e_test.XXXXXX)

skip_re="^(go_|batchjob_exporter_build_info|batchjob_scrape_collector_duration_seconds|process_|batchjob_textfile_mtime_seconds|batchjob_time_(zone|seconds)|batchjob_network_(receive|transmit)_(bytes|packets)_total)"

arch="$(uname -m)"

cgroups_mode=$([ $(stat -fc %T /sys/fs/cgroup/) = "cgroup2fs" ] && echo "unified" || ( [ -e /sys/fs/cgroup/unified/ ] && echo "hybrid" || echo "legacy"))
# cgroups_mode="legacy"
echo "cgroups mode detected is ${cgroups_mode}"

case "${cgroups_mode}" in
  legacy|hybrid) fixture='collector/fixtures/e2e-test-cgroupsv1-output.txt' ;;
  *) fixture='collector/fixtures/e2e-test-cgroupsv2-output.txt' ;;
esac

keep=0; update=0; verbose=0
while getopts 'hkuv' opt
do
  case "$opt" in
    k)
      keep=1
      ;;
    u)
      update=1
      ;;
    v)
      verbose=1
      set -x
      ;;
    *)
      echo "Usage: $0 [-k] [-u] [-v]"
      echo "  -k: keep temporary files and leave batchjob_exporter running"
      echo "  -u: update fixture"
      echo "  -v: verbose output"
      exit 1
      ;;
  esac
done

if [ ! -x ./bin/batchjob_exporter ]
then
    echo './bin/batchjob_exporter not found. Consider running `go build` first.' >&2
    exit 1
fi

PATH=$PWD/collector/fixtures:$PATH ./bin/batchjob_exporter \
  --path.sysfs="collector/fixtures/sys" \
  --path.cgroupfs="collector/fixtures/sys/fs/cgroup" \
  --collector.slurm.unique.jobid \
  --collector.slurm.job.stat.path="collector/fixtures/slurmjobstat" \
  --collector.ipmi.dcmi.wrapper.path="collector/fixtures/ipmi-dcmi-wrapper.sh" \
  --collector.nvidia_gpu \
  --collector.nvidia.gpu.stat.path="collector/fixtures/gpustat" \
  --web.listen-address "127.0.0.1:${port}" \
  --log.level="debug" > "${tmpdir}/batchjob_exporter.log" 2>&1 &

echo $! > "${tmpdir}/batchjob_exporter.pid"

finish() {
  if [ $? -ne 0 -o ${verbose} -ne 0 ]
  then
    cat << EOF >&2
LOG =====================
$(cat "${tmpdir}/batchjob_exporter.log")
=========================
EOF
  fi

  if [ ${update} -ne 0 ]
  then
    cp "${tmpdir}/e2e-test-output.txt" "${fixture}"
  fi

  if [ ${keep} -eq 0 ]
  then
    kill -9 "$(cat ${tmpdir}/batchjob_exporter.pid)"
    # This silences the "Killed" message
    set +e
    wait "$(cat ${tmpdir}/batchjob_exporter.pid)" > /dev/null 2>&1
    rm -rf "${tmpdir}"
  fi
}

trap finish EXIT

get() {
  if command -v curl > /dev/null 2>&1
  then
    curl -s -f "$@"
  elif command -v wget > /dev/null 2>&1
  then
    wget -O - "$@"
  else
    echo "Neither curl nor wget found"
    exit 1
  fi
}

sleep 1

get "127.0.0.1:${port}/metrics" | grep -E -v "${skip_re}" > "${tmpdir}/e2e-test-output.txt"

diff -u \
  "${fixture}" \
  "${tmpdir}/e2e-test-output.txt" 
