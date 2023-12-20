#!/usr/bin/env bash

set -euf -o pipefail

cd "$(dirname $0)/.."

port="$((10000 + (RANDOM % 10000)))"
tmpdir=$(mktemp -d /tmp/batchjob_exporter_e2e_test.XXXXXX)

skip_re="^(go_|batchjob_exporter_build_info|batchjob_scrape_collector_duration_seconds|process_|batchjob_textfile_mtime_seconds|batchjob_time_(zone|seconds)|batchjob_network_(receive|transmit)_(bytes|packets)_total)"

arch="$(uname -m)"

package="exporter"; keep=0; update=0; verbose=0
while getopts 'hp:kuv' opt
do
  case "$opt" in
    p)
      package=$OPTARG
      ;;
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
      echo "Usage: $0 [-p] [-k] [-u] [-v]"
      echo "  -p: package to test [options: exporter, stats]"
      echo "  -k: keep temporary files and leave batchjob_exporter running"
      echo "  -u: update fixtures"
      echo "  -v: verbose output"
      exit 1
      ;;
  esac
done

if [ "${package}" = "exporter" ]
then
  cgroups_mode=$([ $(stat -fc %T /sys/fs/cgroup/) = "cgroup2fs" ] && echo "unified" || ( [ -e /sys/fs/cgroup/unified/ ] && echo "hybrid" || echo "legacy"))
  # cgroups_mode="legacy"
  echo "cgroups mode detected: ${cgroups_mode}"

  case "${cgroups_mode}" in
    legacy|hybrid) fixture='pkg/collector/fixtures/e2e-test-cgroupsv1-output.txt' ;;
    *) fixture='pkg/collector/fixtures/e2e-test-cgroupsv2-output.txt' ;;
  esac

  logfile="${tmpdir}/batchjob_exporter.log"
  fixture_output="${tmpdir}/e2e-test-exporter-output.txt"
  pidfile="${tmpdir}/batchjob_exporter.pid"
elif [ "${package}" = "stats" ] 
then
  fixture='pkg/jobstats/fixtures/e2e-test-stats-server-output.txt'
  logfile="${tmpdir}/batchjob_stats_server.log"
  fixture_output="${tmpdir}/e2e-test-stats-server-output.txt"
  pidfile="${tmpdir}/batchjob_stats_server.pid"
fi

finish() {
  if [ $? -ne 0 -o ${verbose} -ne 0 ]
  then
    cat << EOF >&2
LOG =====================
$(cat "${logfile}")
=========================
EOF
  fi

  if [ ${update} -ne 0 ]
  then
    cp "${fixture_output}" "${fixture}"
  fi

  if [ ${keep} -eq 0 ]
  then
    kill -9 "$(cat ${pidfile})"
    # This silences the "Killed" message
    set +e
    wait "$(cat ${pidfile})" > /dev/null 2>&1
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

if [ "${package}" = "exporter" ] 
then
  if [ ! -x ./bin/batchjob_exporter ]
  then
      echo './bin/batchjob_exporter not found. Consider running `go build` first.' >&2
      exit 1
  fi

  PATH=$PWD/pkg/collector/fixtures:$PATH ./bin/batchjob_exporter \
    --path.sysfs="pkg/collector/fixtures/sys" \
    --path.cgroupfs="pkg/collector/fixtures/sys/fs/cgroup" \
    --collector.slurm.create.unique.jobids \
    --collector.slurm.job.props.path="pkg/collector/fixtures/slurmjobprops" \
    --collector.ipmi.dcmi.cmd="pkg/collector/fixtures/ipmi-dcmi-wrapper.sh" \
    --collector.nvidia_gpu \
    --collector.nvidia.gpu.job.map.path="pkg/collector/fixtures/gpujobmap" \
    --web.listen-address "127.0.0.1:${port}" \
    --log.level="debug" > "${logfile}" 2>&1 &

  echo $! > "${pidfile}"

  sleep 1

  get "127.0.0.1:${port}/metrics" | grep -E -v "${skip_re}" > "${fixture_output}"
elif [ "${package}" = "stats" ] 
then
  if [ ! -x ./bin/batchjob_stats_server ]
  then
      echo './bin/batchjob_stats_server not found. Consider running `go build` first.' >&2
      exit 1
  fi

  ./bin/batchjob_stats_server \
    --slurm.sacct.path="pkg/jobstats/fixtures/sacct" \
    --data.path="${tmpdir}" \
    --web.listen-address="127.0.0.1:${port}" \
    --log.level="debug" > "${logfile}" 2>&1 &

  echo $! > "${pidfile}"

  sleep 2

  get -H "X-Grafana-User: usr" "127.0.0.1:${port}/api/jobs?from=2023-02-20&to=2023-02-25&account=acc1" > "${fixture_output}"
fi

diff -u \
    "${fixture}" \
    "${fixture_output}"
