#!/usr/bin/env bash

set -euf -o pipefail

cd "$(dirname $0)/.."

port="$((10000 + (RANDOM % 10000)))"
tmpdir=$(mktemp -d /tmp/ceems_e2e_test.XXXXXX)

skip_re="^(go_|ceems_exporter_build_info|ceems_scrape_collector_duration_seconds|process_|ceems_textfile_mtime_seconds|ceems_time_(zone|seconds)|ceems_network_(receive|transmit)_(bytes|packets)_total)"

arch="$(uname -m)"

scenario="exporter-cgroups-v1"; keep=0; update=0; verbose=0
while getopts 'hs:kuv' opt
do
  case "$opt" in
    s)
      scenario=$OPTARG
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
      echo "  -s: scenario to test [options: exporter, api, lb]"
      echo "  -k: keep temporary files and leave ceems_{exporter,server,lb} running"
      echo "  -u: update testdata"
      echo "  -v: verbose output"
      exit 1
      ;;
  esac
done

if [[ "${scenario}" =~ ^"exporter" ]]
then
  # cgroups_mode=$([ $(stat -fc %T /sys/fs/cgroup/) = "cgroup2fs" ] && echo "unified" || ( [ -e /sys/fs/cgroup/unified/ ] && echo "hybrid" || echo "legacy"))
  # cgroups_mode="legacy"

  if [ "${scenario}" = "exporter-cgroups-v1" ]
  then
    cgroups_mode="legacy"
    desc="Cgroups V1"
    fixture='pkg/collector/testdata/output/e2e-test-cgroupsv1-output.txt'
  elif [ "${scenario}" = "exporter-cgroups-v2-nvidia-ipmiutil" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2 with nVIDIA GPU and ipmiutil"
    fixture='pkg/collector/testdata/output/e2e-test-cgroupsv2-nvidia-ipmiutil-output.txt'
   elif [ "${scenario}" = "exporter-cgroups-v2-amd-ipmitool" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2 with AMD GPU and ipmitool"
    fixture='pkg/collector/testdata/output/e2e-test-cgroupsv2-amd-ipmitool-output.txt'
  elif [ "${scenario}" = "exporter-cgroups-v2-nogpu" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2 when there are no GPUs"
    fixture='pkg/collector/testdata/output/e2e-test-cgroupsv2-nogpu-output.txt'
  elif [ "${scenario}" = "exporter-cgroups-v2-procfs" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2 using /proc for fetching job properties"
    fixture='pkg/collector/testdata/output/e2e-test-cgroupsv2-procfs-output.txt'
  elif [ "${scenario}" = "exporter-cgroups-v2-all-metrics" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2 enabling all available cgroups metrics"
    fixture='pkg/collector/testdata/output/e2e-test-cgroupsv2-all-metrics-output.txt'
  fi

  logfile="${tmpdir}/ceems_exporter.log"
  fixture_output="${tmpdir}/e2e-test-exporter-output.txt"
  pidfile="${tmpdir}/ceems_exporter.pid"
elif [[ "${scenario}" =~ ^"api" ]] 
then

  if [ "${scenario}" = "api-project-query" ]
  then
    desc="/api/projects end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-server-project-query.txt'
  elif [ "${scenario}" = "api-uuid-query" ]
  then
    desc="/api/units end point test with uuid query param"
    fixture='pkg/api/testdata/output/e2e-test-api-server-uuid-query.txt'
  elif [ "${scenario}" = "api-running-query" ]
  then
    desc="/api/units end point test with running query param"
    fixture='pkg/api/testdata/output/e2e-test-api-server-running-query.txt'
  elif [ "${scenario}" = "api-admin-query" ]
  then
    desc="/api/units/admin end point test for admin query"
    fixture='pkg/api/testdata/output/e2e-test-api-server-admin-query.txt'
  elif [ "${scenario}" = "api-admin-query-all" ]
  then
    desc="/api/units/admin end point test for admin query for all jobs"
    fixture='pkg/api/testdata/output/e2e-test-api-server-admin-query-all.txt'
  elif [ "${scenario}" = "api-admin-query-all-selected-fields" ]
  then
    desc="/api/units/admin end point test for admin query for all jobs with selected fields"
    fixture='pkg/api/testdata/output/e2e-test-api-server-admin-query-all-selected-fields.txt'
  elif [ "${scenario}" = "api-admin-denied-query" ]
  then
    desc="/api/units/admin end point test for denied request"
    fixture='pkg/api/testdata/output/e2e-test-api-server-admin--denied-query.txt'
  elif [ "${scenario}" = "api-current-usage-query" ]
  then
    desc="/api/usage/current end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-server-current-usage-query.txt'
  elif [ "${scenario}" = "api-global-usage-query" ]
  then
    desc="/api/usage/global end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-server-global-usage-query.txt'
  elif [ "${scenario}" = "api-current-usage-admin-query" ]
  then
    desc="/api/usage/current/admin end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-server-current-usage-admin-query.txt'
  elif [ "${scenario}" = "api-global-usage-admin-query" ]
  then
    desc="/api/usage/global/admin end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-server-global-usage-admin-query.txt'
  elif [ "${scenario}" = "api-current-usage-admin-denied-query" ]
  then
    desc="/api/usage/current/admin end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-server-current-usage-admin-denied-query.txt'
  elif [ "${scenario}" = "api-verify-pass-query" ]
  then
    desc="/api/units/verify end point test with pass request"
    fixture='pkg/api/testdata/output/e2e-test-api-verify-pass-query.txt'
  elif [ "${scenario}" = "api-verify-fail-query" ]
  then
    desc="/api/units/verify end point test with fail request"
    fixture='pkg/api/testdata/output/e2e-test-api-verify-fail-query.txt'
  elif [ "${scenario}" = "api-demo-units-query" ]
  then
    desc="/api/units/demo end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-demo-units-query.txt'
  elif [ "${scenario}" = "api-demo-usage-query" ]
  then
    desc="/api/usage/demo end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-demo-usage-query.txt'
  fi

  logfile="${tmpdir}/ceems_api_server.log"
  fixture_output="${tmpdir}/e2e-test-api-server-output.txt"
  pidfile="${tmpdir}/ceems_api_server.pid"
elif [[ "${scenario}" =~ ^"lb" ]] 
then

  if [ "${scenario}" = "lb-basic" ]
  then
    desc="basic e2e load balancer test"
    fixture='pkg/lb/testdata/output/e2e-test-lb-server.txt'
  elif [ "${scenario}" = "lb-forbid-user-query-db" ]
  then
    desc="e2e load balancer test that forbids user query for backend using DB conn"
    fixture='pkg/lb/testdata/output/e2e-test-lb-forbid-user-query-db.txt'
  elif [ "${scenario}" = "lb-allow-user-query-db" ]
  then
    desc="e2e load balancer test that allow user query for backend using DB conn"
    fixture='pkg/lb/testdata/output/e2e-test-lb-allow-user-query-db.txt'
  elif [ "${scenario}" = "lb-forbid-user-query-api" ]
  then
    desc="e2e load balancer test that forbids user query for backend using API"
    fixture='pkg/lb/testdata/output/e2e-test-lb-forbid-user-query-api.txt'
  elif [ "${scenario}" = "lb-allow-user-query-api" ]
  then
    desc="e2e load balancer test that allow user query for backend using API"
    fixture='pkg/lb/testdata/output/e2e-test-lb-allow-user-query-api.txt'
  elif [ "${scenario}" = "lb-allow-admin-query" ]
  then
    desc="e2e load balancer test that allows admin user query for backend"
    fixture='pkg/lb/testdata/output/e2e-test-lb-allow-admin-query.txt'
  elif [ "${scenario}" = "lb-auth" ]
  then
    desc="basic e2e load balancer test with auth configued for backend"
    fixture='pkg/lb/testdata/output/e2e-test-lb-auth-server.txt'
  fi

  logfile="${tmpdir}/ceems_lb.log"
  fixture_output="${tmpdir}/e2e-test-lb-output.txt"
  pidfile="${tmpdir}/ceems_lb.pid"
fi

echo "using scenario: ${scenario}. Description: ${desc}"

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
    for pid in "$(cat ${pidfile})"
    do
        kill -9 $pid
        # This silences the "Killed" message
        set +e
        wait $pid > /dev/null 2>&1
    done
    rm -rf "${tmpdir}"
  fi
}

trap finish EXIT

get() {
  if command -v curl > /dev/null 2>&1
  then
    curl -s "$@"
  elif command -v wget > /dev/null 2>&1
  then
    wget -O - "$@"
  else
    echo "Neither curl nor wget found"
    exit 1
  fi
}

waitport() {
  timeout 5 bash -c "while ! curl -H 'X-Grafana-User: usr1' -s "http://localhost:${1}" > /dev/null; do sleep 0.1; done";
  sleep 1
}

if [[ "${scenario}" =~ ^"exporter" ]] 
then
  if [ ! -x ./bin/ceems_exporter ]
  then
      echo './bin/ceems_exporter not found. Consider running `go build` first.' >&2
      exit 1
  fi

  if [ "${scenario}" = "exporter-cgroups-v1" ] 
  then
      ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.slurm.create.unique.jobids \
        --collector.slurm.job.props.path="pkg/collector/testdata/slurmjobprops" \
        --collector.slurm.gpu.type="nvidia" \
        --collector.slurm.nvidia.smi.path="pkg/collector/testdata/nvidia-smi" \
        --collector.slurm.force.cgroups.version="v1" \
        --collector.slurm.gpu.job.map.path="pkg/collector/testdata/gpujobmap" \
        --collector.ipmi.dcmi.cmd="pkg/collector/testdata/ipmi/freeipmi/ipmi-dcmi" \
        --collector.empty.hostname.label \
        --web.listen-address "127.0.0.1:${port}" \
        --log.level="debug" > "${logfile}" 2>&1 &

  elif [ "${scenario}" = "exporter-cgroups-v2-nvidia-ipmiutil" ] 
  then
      PATH="${PWD}/pkg/collector/testdata/ipmi/ipmiutils:${PATH}" ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.slurm.job.props.path="pkg/collector/testdata/slurmjobprops" \
        --collector.slurm.gpu.type="nvidia" \
        --collector.slurm.nvidia.smi.path="pkg/collector/testdata/nvidia-smi" \
        --collector.slurm.force.cgroups.version="v2" \
        --collector.slurm.gpu.job.map.path="pkg/collector/testdata/gpujobmap" \
        --collector.empty.hostname.label \
        --web.listen-address "127.0.0.1:${port}" \
        --log.level="debug" > "${logfile}" 2>&1 &

  elif [ "${scenario}" = "exporter-cgroups-v2-amd-ipmitool" ] 
  then
      PATH="${PWD}/pkg/collector/testdata/ipmi/openipmi:${PATH}" ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.slurm.create.unique.jobids \
        --collector.slurm.job.props.path="pkg/collector/testdata/slurmjobprops" \
        --collector.slurm.gpu.type="amd" \
        --collector.slurm.rocm.smi.path="pkg/collector/testdata/rocm-smi" \
        --collector.slurm.force.cgroups.version="v2" \
        --collector.slurm.gpu.job.map.path="pkg/collector/testdata/gpujobmap" \
        --collector.empty.hostname.label \
        --web.listen-address "127.0.0.1:${port}" \
        --log.level="debug" > "${logfile}" 2>&1 &

  elif [ "${scenario}" = "exporter-cgroups-v2-nogpu" ] 
  then
      PATH="${PWD}/pkg/collector/testdata/ipmi/capmc:${PATH}" ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.slurm.create.unique.jobids \
        --collector.slurm.job.props.path="pkg/collector/testdata/slurmjobprops" \
        --collector.slurm.force.cgroups.version="v2" \
        --collector.empty.hostname.label \
        --web.listen-address "127.0.0.1:${port}" \
        --log.level="debug" > "${logfile}" 2>&1 &

  elif [ "${scenario}" = "exporter-cgroups-v2-procfs" ] 
  then
      ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.slurm.create.unique.jobids \
        --collector.slurm.gpu.type="nvidia" \
        --collector.slurm.nvidia.smi.path="pkg/collector/testdata/nvidia-smi" \
        --collector.slurm.force.cgroups.version="v2" \
        --collector.ipmi.dcmi.cmd="pkg/collector/testdata/ipmi/ipmiutils/ipmiutil" \
        --collector.empty.hostname.label \
        --web.listen-address "127.0.0.1:${port}" \
        --log.level="debug" > "${logfile}" 2>&1 &
  
  elif [ "${scenario}" = "exporter-cgroups-v2-all-metrics" ] 
  then
      ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.slurm.create.unique.jobids \
        --collector.slurm.job.props.path="pkg/collector/testdata/slurmjobprops" \
        --collector.slurm.gpu.type="amd" \
        --collector.slurm.rocm.smi.path="pkg/collector/testdata/rocm-smi" \
        --collector.slurm.force.cgroups.version="v2" \
        --collector.slurm.gpu.job.map.path="pkg/collector/testdata/gpujobmap" \
        --collector.slurm.swap.memory.metrics \
        --collector.slurm.psi.metrics \
        --collector.ipmi.dcmi.cmd="pkg/collector/testdata/ipmi/capmc/capmc" \
        --collector.empty.hostname.label \
        --web.listen-address "127.0.0.1:${port}" \
        --log.level="debug" > "${logfile}" 2>&1 &
  fi

  echo $! > "${pidfile}"

  # sleep 1
  waitport "${port}"

  get "127.0.0.1:${port}/metrics" | grep -E -v "${skip_re}" > "${fixture_output}"
elif [[ "${scenario}" =~ ^"api" ]] 
then
  if [ ! -x ./bin/ceems_api_server ]
  then
      echo './bin/ceems_api_server not found. Consider running `go build` first.' >&2
      exit 1
  fi

  export PATH="${GOBIN:-}:${PATH}"
    prometheus \
      --config.file pkg/lb/testdata/prometheus.yml \
      --storage.tsdb.path "${tmpdir}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    PROMETHEUS_PID=$!

    waitport "9090"

  ./bin/ceems_api_server \
    --slurm.sacct.path="pkg/api/testdata/sacct" \
    --resource.manager.slurm \
    --storage.data.path="${tmpdir}" \
    --storage.data.backup.path="${tmpdir}" \
    --storage.data.backup.interval="2s" \
    --storage.data.skip.delete.old.units \
    --test.disable.checks \
    --web.listen-address="127.0.0.1:${port}" \
    --web.admin-users="grafana" \
    --updater.tsdb \
    --tsdb.config.file="pkg/api/testdata/tsdb-config.yml" \
    --log.level="debug" > "${logfile}" 2>&1 &
  CEEMS_API=$!

  echo "${PROMETHEUS_PID} ${CEEMS_API}" > "${pidfile}"

  # sleep 2
  waitport "${port}"

  if [ "${scenario}" = "api-project-query" ]
  then
    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/projects" > "${fixture_output}"
  elif [ "${scenario}" = "api-uuid-query" ]
  then
    get -H "X-Grafana-User: usr2" "127.0.0.1:${port}/api/units?uuid=1481508&project=acc2" > "${fixture_output}"
  elif [ "${scenario}" = "api-running-query" ]
  then
    get -H "X-Grafana-User: usr3" "127.0.0.1:${port}/api/units?running&from=1676934000&to=1677538800" > "${fixture_output}"
  elif [ "${scenario}" = "api-admin-query" ]
  then
    get -H "X-Grafana-User: grafana" -H "X-Dashboard-User: usr3" "127.0.0.1:${port}/api/units?project=acc3&from=1676934000&to=1677538800" > "${fixture_output}"
  elif [ "${scenario}" = "api-admin-query-all" ]
  then
    get -H "X-Grafana-User: grafana" "127.0.0.1:${port}/api/units/admin?from=1676934000&to=1677538800" > "${fixture_output}"
  elif [ "${scenario}" = "api-admin-query-all-selected-fields" ]
  then
    get -H "X-Grafana-User: grafana" "127.0.0.1:${port}/api/units/admin?from=1676934000&to=1677538800&field=uuid&field=started_at&field=ended_at&field=foo" > "${fixture_output}"
  elif [ "${scenario}" = "api-admin-denied-query" ]
  then
    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/units/admin" > "${fixture_output}"
  elif [ "${scenario}" = "api-current-usage-query" ]
  then
    get -H "X-Grafana-User: usr3" "127.0.0.1:${port}/api/usage/current?from=1676934000&to=1677538800" > "${fixture_output}"
  elif [ "${scenario}" = "api-global-usage-query" ]
  then
    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/usage/global" > "${fixture_output}"
  elif [ "${scenario}" = "api-current-usage-admin-query" ]
  then
    get -H "X-Grafana-User: grafana" "127.0.0.1:${port}/api/usage/current/admin?user=usr3&from=1676934000&to=1677538800" > "${fixture_output}"
  elif [ "${scenario}" = "api-global-usage-admin-query" ]
  then
    get -H "X-Grafana-User: grafana" "127.0.0.1:${port}/api/usage/global/admin" > "${fixture_output}"
  elif [ "${scenario}" = "api-current-usage-admin-denied-query" ]
  then
    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/usage/global/admin?user=usr2" > "${fixture_output}"
  elif [ "${scenario}" = "api-verify-pass-query" ]
  then
    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/units/verify?uuid=1479763&uuid=1479765" > "${fixture_output}"
  elif [ "${scenario}" = "api-verify-fail-query" ]
  then
    get -H "X-Grafana-User: usr2" "127.0.0.1:${port}/api/units/verify?uuid=1479763&uuid=11508" > "${fixture_output}"
  elif [ "${scenario}" = "api-demo-units-query" ]
  then
    get -s -o /dev/null -w "%{http_code}" "127.0.0.1:${port}/api/units/demo" > "${fixture_output}"
  elif [ "${scenario}" = "api-demo-usage-query" ]
  then
    get -s -o /dev/null -w "%{http_code}" "127.0.0.1:${port}/api/usage/demo" > "${fixture_output}"
  fi

elif [[ "${scenario}" =~ ^"lb" ]] 
then
  if [[ "${scenario}" = "lb-basic" ]] 
  then
    if [ ! -x ./bin/ceems_lb ]
    then
        echo './bin/ceems_lb not found. Consider running `go build` first.' >&2
        exit 1
    fi

    export PATH="${GOBIN:-}:${PATH}"
    prometheus \
      --config.file pkg/lb/testdata/prometheus.yml \
      --storage.tsdb.path "${tmpdir}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    PROMETHEUS_PID=$!

    waitport "9090"

    ./bin/ceems_lb \
      --config.file pkg/lb/testdata/config-db.yml \
      --web.listen-address="127.0.0.1:${port}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    LB_PID=$!

    echo "${PROMETHEUS_PID} ${LB_PID}" > "${pidfile}"

    waitport "${port}"

    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/v1/status/config" > "${fixture_output}"

  elif [[ "${scenario}" = "lb-forbid-user-query-db" ]] 
  then
    if [ ! -x ./bin/ceems_lb ]
    then
        echo './bin/ceems_lb not found. Consider running `go build` first.' >&2
        exit 1
    fi

    export PATH="${GOBIN:-}:${PATH}"
    prometheus \
      --config.file pkg/lb/testdata/prometheus.yml \
      --storage.tsdb.path "${tmpdir}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    PROMETHEUS_PID=$!

    waitport "9090"

    ./bin/ceems_lb \
      --config.file pkg/lb/testdata/config-db.yml \
      --web.listen-address="127.0.0.1:${port}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    LB_PID=$!

    echo "${PROMETHEUS_PID} ${LB_PID}" > "${pidfile}"

    waitport "${port}"

    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/v1/query?query=foo\{uuid=\"1481510\"\}&time=1713032179.506" > "${fixture_output}"

  elif [[ "${scenario}" = "lb-allow-user-query-db" ]] 
  then
    if [ ! -x ./bin/ceems_lb ]
    then
        echo './bin/ceems_lb not found. Consider running `go build` first.' >&2
        exit 1
    fi

    export PATH="${GOBIN:-}:${PATH}"
    prometheus \
      --config.file pkg/lb/testdata/prometheus.yml \
      --storage.tsdb.path "${tmpdir}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    PROMETHEUS_PID=$!

    waitport "9090"

    ./bin/ceems_lb \
      --config.file pkg/lb/testdata/config-db.yml \
      --web.listen-address="127.0.0.1:${port}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    LB_PID=$!

    echo "${PROMETHEUS_PID} ${LB_PID}" > "${pidfile}"

    waitport "${port}"

    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/v1/query?query=foo\{uuid=\"1479763\"\}&time=1713032179.506" > "${fixture_output}"

  elif [[ "${scenario}" = "lb-forbid-user-query-api" ]] 
  then
    if [ ! -x ./bin/ceems_lb ]
    then
        echo './bin/ceems_lb not found. Consider running `go build` first.' >&2
        exit 1
    fi

    export PATH="${GOBIN:-}:${PATH}"
    prometheus \
      --config.file pkg/lb/testdata/prometheus.yml \
      --storage.tsdb.path "${tmpdir}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    PROMETHEUS_PID=$!

    waitport "9090"

    ./bin/ceems_api_server \
      --storage.data.path="pkg/lb/testdata" \
      --storage.data.skip.delete.old.units \
      --test.disable.checks \
      --web.listen-address="127.0.0.1:9020" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    CEEMS_API_PID=$!

    waitport "9020"

    ./bin/ceems_lb \
      --config.file pkg/lb/testdata/config-api.yml \
      --web.listen-address="127.0.0.1:${port}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    LB_PID=$!

    echo "${PROMETHEUS_PID} ${CEEMS_API_PID} ${LB_PID}" > "${pidfile}"

    waitport "${port}"

    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/v1/query?query=foo\{uuid=\"1481510\"\}&time=1713032179.506" > "${fixture_output}"

  elif [[ "${scenario}" = "lb-allow-user-query-api" ]] 
  then
    if [ ! -x ./bin/ceems_lb ]
    then
        echo './bin/ceems_lb not found. Consider running `go build` first.' >&2
        exit 1
    fi

    export PATH="${GOBIN:-}:${PATH}"
    prometheus \
      --config.file pkg/lb/testdata/prometheus.yml \
      --storage.tsdb.path "${tmpdir}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    PROMETHEUS_PID=$!

    waitport "9090"

    ./bin/ceems_api_server \
      --storage.data.path="pkg/lb/testdata" \
      --storage.data.skip.delete.old.units \
      --test.disable.checks \
      --web.listen-address="127.0.0.1:9020" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    CEEMS_API_PID=$!

    waitport "9020"

    ./bin/ceems_lb \
      --config.file pkg/lb/testdata/config-api.yml \
      --web.listen-address="127.0.0.1:${port}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    LB_PID=$!

    echo "${PROMETHEUS_PID} ${CEEMS_API_PID} ${LB_PID}" > "${pidfile}"

    waitport "${port}"

    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/v1/query?query=foo\{uuid=\"1479763\"\}&time=1713032179.506" > "${fixture_output}"

  elif [[ "${scenario}" = "lb-allow-admin-query" ]] 
  then
    if [ ! -x ./bin/ceems_lb ]
    then
        echo './bin/ceems_lb not found. Consider running `go build` first.' >&2
        exit 1
    fi

    export PATH="${GOBIN:-}:${PATH}"
    prometheus \
      --config.file pkg/lb/testdata/prometheus.yml \
      --storage.tsdb.path "${tmpdir}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    PROMETHEUS_PID=$!

    waitport "9090"

    ./bin/ceems_lb \
      --config.file pkg/lb/testdata/config-db.yml \
      --web.listen-address="127.0.0.1:${port}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    LB_PID=$!

    echo "${PROMETHEUS_PID} ${LB_PID}" > "${pidfile}"

    waitport "${port}"

    get -H "X-Grafana-User: adm1" -H "Content-Type: application/x-www-form-urlencoded" -X POST -d "query=foo{uuid=\"1479765\"}&time=1713032179.506" "127.0.0.1:${port}/api/v1/query" > "${fixture_output}"

  elif [[ "${scenario}" = "lb-auth" ]] 
  then
    if [ ! -x ./bin/ceems_lb ]
    then
        echo './bin/ceems_lb not found. Consider running `go build` first.' >&2
        exit 1
    fi

    export PATH="${GOBIN:-}:${PATH}"
    prometheus \
      --config.file pkg/lb/testdata/prometheus.yml \
      --web.config.file pkg/lb/testdata/web-config.yml \
      --storage.tsdb.path "${tmpdir}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    PROMETHEUS_PID=$!

    waitport "9090"

    ./bin/ceems_lb \
      --config.file pkg/lb/testdata/config-with-auth.yml \
      --web.listen-address="127.0.0.1:${port}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    LB_PID=$!

    echo "${PROMETHEUS_PID} ${LB_PID}" > "${pidfile}"

    waitport "${port}"

    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/v1/status/config" > "${fixture_output}"
  fi
fi

diff -u \
    "${fixture}" \
    "${fixture_output}"
