#!/usr/bin/env bash

set -euf -o pipefail

cd "$(dirname $0)/.."

port="$((10000 + (RANDOM % 10000)))"
tmpdir=$(mktemp -d /tmp/ceems_e2e_test.XXXXXX)

skip_re="^(go_|ceems_exporter_build_info|ceems_scrape_collector_duration_seconds|process_|ceems_textfile_mtime_seconds|ceems_time_(zone|seconds)|ceems_network_(receive|transmit)_(bytes|packets)_total)"

arch="$(uname -m)"

api_version="v1"

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
    fixture='pkg/collector/testdata/output/exporter/e2e-test-cgroupsv1-output.txt'
  elif [ "${scenario}" = "exporter-cgroups-v1-memory-subsystem" ]
  then
    cgroups_mode="legacy"
    desc="Cgroups V1 with memory subsystem"
    fixture='pkg/collector/testdata/output/exporter/e2e-test-cgroupsv1-memory-subsystem-output.txt'
  elif [ "${scenario}" = "exporter-cgroups-v2-nvidia-ipmiutil" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2 with nVIDIA GPU and ipmiutil"
    fixture='pkg/collector/testdata/output/exporter/e2e-test-cgroupsv2-nvidia-ipmiutil-output.txt'
  elif [ "${scenario}" = "exporter-cgroups-v2-nvidia-gpu-reordering" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2 with nVIDIA GPU reordering"
    fixture='pkg/collector/testdata/output/exporter/e2e-test-cgroupsv2-nvidia-gpu-reordering.txt'
  elif [ "${scenario}" = "exporter-cgroups-v2-amd-ipmitool" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2 with AMD GPU and ipmitool"
    fixture='pkg/collector/testdata/output/exporter/e2e-test-cgroupsv2-amd-ipmitool-output.txt'
  elif [ "${scenario}" = "exporter-cgroups-v2-nogpu" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2 when there are no GPUs"
    fixture='pkg/collector/testdata/output/exporter/e2e-test-cgroupsv2-nogpu-output.txt'
  elif [ "${scenario}" = "exporter-cgroups-v2-procfs" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2 using /proc for fetching job properties"
    fixture='pkg/collector/testdata/output/exporter/e2e-test-cgroupsv2-procfs-output.txt'
  elif [ "${scenario}" = "exporter-cgroups-v2-all-metrics" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2 enabling all available cgroups metrics"
    fixture='pkg/collector/testdata/output/exporter/e2e-test-cgroupsv2-all-metrics-output.txt'
  elif [ "${scenario}" = "exporter-cgroups-v1-libvirt" ]
  then
    cgroups_mode="legacy"
    desc="Cgroups V1 with libvirt"
    fixture='pkg/collector/testdata/output/exporter/e2e-test-cgroupsv1-libvirt-output.txt'
  elif [ "${scenario}" = "exporter-cgroups-v2-libvirt" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2 with libvirt"
    fixture='pkg/collector/testdata/output/exporter/e2e-test-cgroupsv2-libvirt-output.txt'
   elif [ "${scenario}" = "exporter-cgroups-v2-libvirt-nonsystemd-layout" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2 with libvirt with non-systemd cgroup layout"
    fixture='pkg/collector/testdata/output/exporter/e2e-test-cgroupsv2-libvirt-nonsystemd-layout-output.txt'
  elif [ "${scenario}" = "exporter-cgroups-v1-k8s" ]
  then
    cgroups_mode="legacy"
    desc="Cgroups V1 with k8s"
    fixture='pkg/collector/testdata/output/exporter/e2e-test-cgroupsv1-k8s-output.txt'
  elif [ "${scenario}" = "exporter-cgroups-v2-k8s" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2 with k8s"
    fixture='pkg/collector/testdata/output/exporter/e2e-test-cgroupsv2-k8s-output.txt'
  elif [ "${scenario}" = "exporter-cgroups-v2-k8s-nogpu" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2 with k8s and no GPUs"
    fixture='pkg/collector/testdata/output/exporter/e2e-test-cgroupsv2-k8s-nogpu-output.txt'
  fi

  logfile="${tmpdir}/ceems_exporter.log"
  fixture_output="${tmpdir}/e2e-test-exporter-output.txt"
  pidfile="${tmpdir}/ceems_exporter.pid"
elif [[ "${scenario}" =~ ^"discoverer" ]] 
then

  if [ "${scenario}" = "discoverer-cgroups-v2-slurm" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2 discoverer for Slurm"
    fixture='pkg/collector/testdata/output/discoverer/e2e-test-discoverer-cgroupsv2-slurm-output.txt'
  elif [ "${scenario}" = "discoverer-cgroups-v1-slurm" ]
  then
    cgroups_mode="legacy"
    desc="Cgroups V1 discoverer for Slurm"
    fixture='pkg/collector/testdata/output/discoverer/e2e-test-discoverer-cgroupsv1-slurm-output.txt'
  elif [ "${scenario}" = "discoverer-cgroups-v2-k8s" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2 discoverer for k8s"
    fixture='pkg/collector/testdata/output/discoverer/e2e-test-discoverer-cgroupsv2-k8s-output.txt'
  elif [ "${scenario}" = "discoverer-cgroups-v1-k8s" ]
  then
    cgroups_mode="legacy"
    desc="Cgroups V1 discoverer for k8s"
    fixture='pkg/collector/testdata/output/discoverer/e2e-test-discoverer-cgroupsv1-k8s-output.txt'
  fi

  logfile="${tmpdir}/ceems_exporter.log"
  fixture_output="${tmpdir}/e2e-test-exporter-output.txt"
  pidfile="${tmpdir}/ceems_exporter.pid"
elif [[ "${scenario}" =~ ^"api" ]] 
then

  if [ "${scenario}" = "api-project-query" ]
  then
    desc="/projects end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-server-project-query.txt'
  elif [ "${scenario}" = "api-project-empty-query" ]
  then
    desc="/projects end point test with user query a project that they are not part of"
    fixture='pkg/api/testdata/output/e2e-test-api-server-project-empty-query.txt'
  elif [ "${scenario}" = "api-project-admin-query" ]
  then
    desc="/projects/admin end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-server-project-admin-query.txt'
  elif [ "${scenario}" = "api-project-query-k8s" ]
  then
    desc="/projects end point test with k8s"
    fixture='pkg/api/testdata/output/e2e-test-api-server-project-query-k8s.txt'
  elif [ "${scenario}" = "api-user-query" ]
  then
    desc="/users end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-server-user-query.txt'
  elif [ "${scenario}" = "api-user-admin-query" ]
  then
    desc="/users/admin end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-server-user-admin-query.txt'
  elif [ "${scenario}" = "api-user-admin-all-query" ]
  then
    desc="/users/admin end point test that queries all users"
    fixture='pkg/api/testdata/output/e2e-test-api-server-user-admin-all-query.txt'
  elif [ "${scenario}" = "api-user-query-k8s" ]
  then
    desc="/users end point test with k8s"
    fixture='pkg/api/testdata/output/e2e-test-api-server-user-query-k8s.txt'
  elif [ "${scenario}" = "api-cluster-admin-query" ]
  then
    desc="/clusters/admin end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-server-cluster-admin-query.txt'
  elif [ "${scenario}" = "api-uuid-query" ]
  then
    desc="/units end point test with uuid query param"
    fixture='pkg/api/testdata/output/e2e-test-api-server-uuid-query.txt'
   elif [ "${scenario}" = "api-units-invalid-query" ]
  then
    desc="/units end point test with invalid field query"
    fixture='pkg/api/testdata/output/e2e-test-api-server-units-invalid-query.txt'
  elif [ "${scenario}" = "api-running-query" ]
  then
    desc="/units end point test with running query param"
    fixture='pkg/api/testdata/output/e2e-test-api-server-running-query.txt'
  elif [ "${scenario}" = "api-units-query-k8s" ]
  then
    desc="/units end point test with k8s"
    fixture='pkg/api/testdata/output/e2e-test-api-server-query-k8s.txt'
  elif [ "${scenario}" = "api-admin-query" ]
  then
    desc="/units/admin end point test for admin query"
    fixture='pkg/api/testdata/output/e2e-test-api-server-admin-query.txt'
  elif [ "${scenario}" = "api-admin-query-all" ]
  then
    desc="/units/admin end point test for admin query for all jobs"
    fixture='pkg/api/testdata/output/e2e-test-api-server-admin-query-all.txt'
  elif [ "${scenario}" = "api-admin-query-all-selected-fields" ]
  then
    desc="/units/admin end point test for admin query for all jobs with selected fields"
    fixture='pkg/api/testdata/output/e2e-test-api-server-admin-query-all-selected-fields.txt'
  elif [ "${scenario}" = "api-admin-denied-query" ]
  then
    desc="/units/admin end point test for denied request"
    fixture='pkg/api/testdata/output/e2e-test-api-server-admin--denied-query.txt'
  elif [ "${scenario}" = "api-current-usage-query" ]
  then
    desc="/usage/current end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-server-current-usage-query.txt'
  elif [ "${scenario}" = "api-current-usage-experimental-query" ]
  then
    desc="/usage/current end point test with experimental aggregation"
    fixture='pkg/api/testdata/output/e2e-test-api-server-current-usage-experimental-query.txt'
  elif [ "${scenario}" = "api-global-usage-query" ]
  then
    desc="/usage/global end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-server-global-usage-query.txt'
  elif [ "${scenario}" = "api-current-usage-admin-query" ]
  then
    desc="/usage/current/admin end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-server-current-usage-admin-query.txt'
  elif [ "${scenario}" = "api-current-usage-admin-experimental-query" ]
  then
    desc="/usage/current/admin end point test with experimental aggregation"
    fixture='pkg/api/testdata/output/e2e-test-api-server-current-usage-admin-experimental-query.txt'
  elif [ "${scenario}" = "api-current-usage-query-k8s" ]
  then
    desc="/usage/current end point test with k8s"
    fixture='pkg/api/testdata/output/e2e-test-api-server-current-usage-query-k8s.txt'
  elif [ "${scenario}" = "api-global-usage-admin-query" ]
  then
    desc="/usage/global/admin end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-server-global-usage-admin-query.txt'
  elif [ "${scenario}" = "api-current-usage-admin-denied-query" ]
  then
    desc="/usage/current/admin end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-server-current-usage-admin-denied-query.txt'
  elif [ "${scenario}" = "api-current-stats-admin-query" ]
  then
    desc="/stats/current/admin end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-server-current-stats-admin-query.txt'
  elif [ "${scenario}" = "api-global-stats-admin-query" ]
  then
    desc="/stats/global/admin end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-server-global-stats-admin-query.txt'
  elif [ "${scenario}" = "api-global-usage-query-k8s" ]
  then
    desc="/usage/global end point test with k8s"
    fixture='pkg/api/testdata/output/e2e-test-api-server-global-usage-query-k8s.txt'
  elif [ "${scenario}" = "api-verify-pass-query" ]
  then
    desc="/units/verify end point test with pass request"
    fixture='pkg/api/testdata/output/e2e-test-api-verify-pass-query.txt'
  elif [ "${scenario}" = "api-verify-fail-query" ]
  then
    desc="/units/verify end point test with fail request"
    fixture='pkg/api/testdata/output/e2e-test-api-verify-fail-query.txt'
  elif [ "${scenario}" = "api-demo-units-query" ]
  then
    desc="/demo/units end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-demo-units-query.txt'
  elif [ "${scenario}" = "api-demo-usage-query" ]
  then
    desc="/demo/usage end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-demo-usage-query.txt'
  elif [ "${scenario}" = "api-cors-preflight" ]
  then
    desc="/api/units preflight end point test"
    fixture='pkg/api/testdata/output/e2e-test-api-preflight-query.txt'
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
  elif [ "${scenario}" = "lb-basic-tsdb-only" ]
  then
    desc="basic e2e load balancer test with only TSDB backends"
    fixture='pkg/lb/testdata/output/e2e-test-lb-server-tsdb-only.txt'
  elif [ "${scenario}" = "lb-basic-pyro-only" ]
  then
    desc="basic e2e load balancer test with only Pyroscope backends"
    fixture='pkg/lb/testdata/output/e2e-test-lb-server-pyro-only.txt'
  elif [ "${scenario}" = "lb-basic-tsdb-pyro" ]
  then
    desc="basic e2e load balancer test with mix of TSDB and Pyroscope backends"
    fixture='pkg/lb/testdata/output/e2e-test-lb-server-tsdb-pyro-mix.txt'
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
    desc="basic e2e load balancer test with auth configured for backend"
    fixture='pkg/lb/testdata/output/e2e-test-lb-auth-server.txt'
  fi

  logfile="${tmpdir}/ceems_lb.log"
  fixture_output="${tmpdir}/e2e-test-lb-output.txt"
  pidfile="${tmpdir}/ceems_lb.pid"
elif [[ "${scenario}" =~ ^"redfish" ]] 
then

  if [ "${scenario}" = "redfish-proxy-frontend-plain-backend-plain" ]
  then
    desc="Redfish proxy with both frontend and backend running without TLS"
    fixture='cmd/redfish_proxy/testdata/output/e2e-test-redfish-proxy-plain-plain-output.txt'
  elif [ "${scenario}" = "redfish-proxy-frontend-tls-backend-plain" ]
  then
    desc="Redfish proxy with TLS frontend and backend running without TLS"
    fixture='cmd/redfish_proxy/testdata/output/e2e-test-redfish-proxy-tls-plain-output.txt'
  elif [ "${scenario}" = "redfish-proxy-frontend-plain-backend-tls" ]
  then
    desc="Redfish proxy with frontend non-TLS and backend running with TLS"
    fixture='cmd/redfish_proxy/testdata/output/e2e-test-redfish-proxy-plain-tls-output.txt'
  elif [ "${scenario}" = "redfish-proxy-frontend-tls-backend-tls" ]
  then
    desc="Redfish proxy with both frontend and backend running with TLS"
    fixture='cmd/redfish_proxy/testdata/output/e2e-test-redfish-proxy-tls-tls-output.txt'
  elif [ "${scenario}" = "redfish-proxy-targetless-frontend-plain-backend-plain" ]
  then
    desc="Redfish proxy with both frontend and backend running without TLS and without targets in config"
    fixture='cmd/redfish_proxy/testdata/output/e2e-test-redfish-proxy-targetless-plain-plain-output.txt'
  fi

  logfile="${tmpdir}/redfish_proxy.log"
  fixture_output="${tmpdir}/e2e-test-redfish-proxy-output.txt"
  pidfile="${tmpdir}/redfish_proxy.pid"
elif [[ "${scenario}" =~ ^"cacct" ]] 
then

  if [ "${scenario}" = "cacct-default-format" ]
  then
    desc="cacct with default format"
    fixture='cmd/cacct/testdata/output/e2e-test-cacct-default-format.txt'
  elif [ "${scenario}" = "cacct-long-format" ]
  then
    desc="cacct with long format"
    fixture='cmd/cacct/testdata/output/e2e-test-cacct-long-format.txt'
  elif [ "${scenario}" = "cacct-custom-format" ]
  then
    desc="cacct with custom format"
    fixture='cmd/cacct/testdata/output/e2e-test-cacct-custom-format.txt'
  elif [ "${scenario}" = "cacct-admin-user" ]
  then
    desc="cacct by admin user"
    fixture='cmd/cacct/testdata/output/e2e-test-cacct-admin-user.txt'
  elif [ "${scenario}" = "cacct-forbid-query" ]
  then
    desc="cacct by normal user attempt to admin query"
    fixture='cmd/cacct/testdata/output/e2e-test-cacct-forbid-query.txt'
  elif [ "${scenario}" = "cacct-tsdata" ]
  then
    desc="cacct to dump time series data"
    fixture='cmd/cacct/testdata/output/e2e-test-cacct-tsdata.txt'
  elif [ "${scenario}" = "cacct-tsdata-fail" ]
  then
    desc="cacct to dump time series data failed"
    fixture='cmd/cacct/testdata/output/e2e-test-cacct-tsdata-fail.txt'
  fi

  logfile="${tmpdir}/cacct.log"
  fixture_output="${tmpdir}/e2e-test-cacct-output.txt"
  pidfile="${tmpdir}/cacct.pid"
elif [[ "${scenario}" =~ ^"tool" ]]
then

  if [ "${scenario}" = "tool-recording-rules" ]
  then
    desc="ceems tool to generate recording rules"
    fixture='cmd/ceems_tool/testdata/output/e2e-test-recording-rules-output.txt'
  elif [ "${scenario}" = "tool-relabel-configs" ]
  then
    desc="ceems tool to generate relabel config"
    fixture='cmd/ceems_tool/testdata/output/e2e-test-relabel-config-output.txt'
  elif [ "${scenario}" = "tool-web-config" ]
  then
    desc="ceems tool to generate web config"
    fixture='cmd/ceems_tool/testdata/output/e2e-test-web-config-output.txt'
  fi

  logfile="${tmpdir}/ceems_tool.log"
  fixture_output="${tmpdir}/e2e-test-ceems-tool-output.txt"
  pidfile="${tmpdir}/ceems_tool.pid"

elif [[ "${scenario}" =~ ^"k8s-admission-controller" ]]
then

  if [ "${scenario}" = "k8s-admission-controller-basic" ]
  then
    desc="ceems k8s admission controller admission basic endpoints tests"
    fixture='cmd/ceems_k8s_admission_controller/testdata/output/e2e-test-admission-basic-response-output.txt'
  elif [ "${scenario}" = "k8s-admission-controller-advanced" ]
  then
    desc="ceems k8s admission controller admission advanced endpoints tests"
    fixture='cmd/ceems_k8s_admission_controller/testdata/output/e2e-test-admission-advanced-response-output.txt'
  fi

  logfile="${tmpdir}/ceems_k8s_admission_controller.log"
  fixture_output="${tmpdir}/e2e-test-ceems-k8s-admission-controller-output.txt"
  pidfile="${tmpdir}/ceems_k8s_admission_controller.pid"

fi

# Current time stamp
timestamp=$(date +%s)

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
    curl -k -s "$@"
  elif command -v wget > /dev/null 2>&1
  then
    wget -O - "$@"
  else
    echo "Neither curl nor wget found"
    exit 1
  fi
}

waitport() {
  timeout 5 bash -c "while ! curl -s "http://localhost:${1}" > /dev/null; do sleep 0.1; done";
  sleep 1
}

if [[ "${scenario}" =~ ^"exporter" ]] || [[ "${scenario}" =~ ^"discoverer" ]]
then
  if [ ! -x ./bin/ceems_exporter ]
  then
      echo './bin/ceems_exporter not found. Consider running `go build` first.' >&2
      exit 1
  fi

  export PATH="${GOBIN:-}:${PATH}"
  export CEEMS_KUBELET_SOCKET_DIR="${tmpdir}/kubelet"
  ./bin/mock_servers redfish k8s-api k8s-kubelet-socket >> "${logfile}" 2>&1 &
  MOCK_SERVERS_PID=$!

  waitport "5000"
  waitport "9080"

  if [ "${scenario}" = "exporter-cgroups-v1" ] 
  then
      REDFISH_HOST=localhost REDFISH_PORT=5000 ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.cgroups.force-version="v1" \
        --collector.slurm \
        --collector.slurm.gres-config-file="pkg/collector/testdata/gres.conf" \
        --collector.gpu.type="nvidia" \
        --collector.gpu.nvidia-smi-path="pkg/collector/testdata/nvidia-smi" \
        --collector.ipmi_dcmi \
        --collector.ipmi_dcmi.test-mode \
        --collector.ipmi_dcmi.cmd="pkg/collector/testdata/ipmi/freeipmi/ipmi-dcmi" \
        --collector.redfish \
        --collector.redfish.web-config="pkg/collector/testdata/redfish/config.yml" \
        --collector.redfish.config.file.expand-env-vars \
        --collector.cray_pm_counters \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:${port}" \
        --web.disable-exporter-metrics \
        --log.level="debug" > "${logfile}" 2>&1 &
      
  elif [ "${scenario}" = "exporter-cgroups-v1-memory-subsystem" ] 
  then
      ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.cgroups.force-version="v1" \
        --collector.cgroups.active-subsystem="memory" \
        --collector.slurm \
        --collector.slurm.gres-config-file="pkg/collector/testdata/gres.conf" \
        --collector.gpu.type="nvidia" \
        --collector.gpu.nvidia-smi-path="pkg/collector/testdata/nvidia-smi" \
        --collector.ipmi_dcmi \
        --collector.ipmi_dcmi.cmd="pkg/collector/testdata/ipmi/freeipmi/ipmi-dcmi" \
        --collector.ipmi_dcmi.test-mode \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:${port}" \
        --web.disable-exporter-metrics \
        --log.level="debug" > "${logfile}" 2>&1 &

  elif [ "${scenario}" = "exporter-cgroups-v2-nvidia-ipmiutil" ] 
  then
      REDFISH_HOST=localhost REDFISH_PORT=5000 PATH="${PWD}/pkg/collector/testdata/ipmi/ipmiutils:${PATH}" ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.cgroups.force-version="v2" \
        --collector.slurm \
        --collector.slurm.gres-config-file="pkg/collector/testdata/gres.conf" \
        --collector.gpu.type="nvidia" \
        --collector.gpu.nvidia-smi-path="pkg/collector/testdata/nvidia-smi" \
        --collector.rdma.stats \
        --collector.rdma.cmd="pkg/collector/testdata/rdma" \
        --collector.empty-hostname-label \
        --collector.ipmi_dcmi \
        --collector.ipmi_dcmi.test-mode \
        --collector.redfish \
        --collector.redfish.config.file="pkg/collector/testdata/redfish/config.yml" \
        --collector.redfish.config.file.expand-env-vars \
        --collector.netdev \
        --collector.netdev.device-include="eth0" \
        --web.listen-address "127.0.0.1:${port}" \
        --web.disable-exporter-metrics \
        --log.level="debug" > "${logfile}" 2>&1 &

  elif [ "${scenario}" = "exporter-cgroups-v2-nvidia-gpu-reordering" ] 
  then
      PATH="${PWD}/pkg/collector/testdata/ipmi/ipmiutils:${PATH}" ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.cgroups.force-version="v2" \
        --collector.slurm \
        --collector.slurm.gpu-order-map="0:0,1:1,2:4,3:5,4:2.1,5:2.5,6:2.13,7:3.1,8:3.5,9:3.6,10:6,11:7" \
        --collector.gpu.type="nvidia" \
        --collector.gpu.nvidia-smi-path="pkg/collector/testdata/nvidia-smi" \
        --collector.rdma.stats \
        --collector.rdma.cmd="pkg/collector/testdata/rdma" \
        --collector.empty-hostname-label \
        --collector.ipmi_dcmi \
        --collector.ipmi_dcmi.test-mode \
        --collector.cray_pm_counters \
        --web.listen-address "127.0.0.1:${port}" \
        --web.disable-exporter-metrics \
        --log.level="debug" > "${logfile}" 2>&1 &

  elif [ "${scenario}" = "exporter-cgroups-v2-amd-ipmitool" ] 
  then
      REDFISH_HOST=localhost REDFISH_PORT=5000 PATH="${PWD}/pkg/collector/testdata/ipmi/openipmi:${PATH}" ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.cgroups.force-version="v2" \
        --collector.slurm \
        --collector.gpu.type="amd" \
        --collector.gpu.rocm-smi-path="pkg/collector/testdata/rocm-smi" \
        --collector.empty-hostname-label \
        --collector.hwmon \
        --collector.ipmi_dcmi \
        --collector.ipmi_dcmi.test-mode \
        --collector.redfish \
        --collector.redfish.config.file="pkg/collector/testdata/redfish/config.yml" \
        --collector.redfish.config.file.expand-env-vars \
        --web.listen-address "127.0.0.1:${port}" \
        --web.disable-exporter-metrics \
        --log.level="debug" > "${logfile}" 2>&1 &

  elif [ "${scenario}" = "exporter-cgroups-v2-nogpu" ] 
  then
      PATH="${PWD}/pkg/collector/testdata/ipmi/capmc:${PATH}" ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.cgroups.force-version="v2" \
        --collector.slurm \
        --collector.gpu.type="nogpu" \
        --collector.empty-hostname-label \
        --collector.ipmi_dcmi \
        --collector.ipmi_dcmi.test-mode \
        --collector.infiniband \
        --web.listen-address "127.0.0.1:${port}" \
        --web.disable-exporter-metrics \
        --log.level="debug" > "${logfile}" 2>&1 &

  elif [ "${scenario}" = "exporter-cgroups-v2-procfs" ] 
  then
      ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.cgroups.force-version="v2" \
        --collector.slurm \
        --collector.gpu.type="nvidia" \
        --collector.gpu.nvidia-smi-path="pkg/collector/testdata/nvidia-smi" \
        --collector.ipmi_dcmi \
        --collector.ipmi.dcmi.cmd="pkg/collector/testdata/ipmi/ipmiutils/ipmiutil" \
        --collector.ipmi_dcmi.test-mode \
        --collector.cray_pm_counters \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:${port}" \
        --web.disable-exporter-metrics \
        --log.level="debug" > "${logfile}" 2>&1 &
  
  elif [ "${scenario}" = "exporter-cgroups-v2-all-metrics" ] 
  then
      REDFISH_HOST=localhost REDFISH_PORT=5000 ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.cgroups.force-version="v2" \
        --collector.slurm \
        --collector.gpu.type="amd" \
        --collector.gpu.rocm-smi-path="pkg/collector/testdata/rocm-smi" \
        --collector.slurm.swap.memory.metrics \
        --collector.slurm.psi.metrics \
        --collector.ipmi_dcmi \
        --collector.ipmi.dcmi.cmd="pkg/collector/testdata/ipmi/capmc/capmc" \
        --collector.ipmi_dcmi.test-mode \
        --collector.redfish \
        --collector.redfish.config.file="pkg/collector/testdata/redfish/config.yml" \
        --collector.redfish.config.file.expand-env-vars \
        --collector.cray_pm_counters \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:${port}" \
        --web.disable-exporter-metrics \
        --log.level="debug" > "${logfile}" 2>&1 &
  elif [ "${scenario}" = "exporter-cgroups-v1-libvirt" ] 
  then
      ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.cgroups.force-version="v1" \
        --collector.libvirt \
        --collector.gpu.type="nvidia" \
        --collector.gpu.nvidia-smi-path="pkg/collector/testdata/nvidia-smi" \
        --collector.libvirt.xml-dir="pkg/collector/testdata/qemu" \
        --collector.libvirt.swap-memory-metrics \
        --collector.libvirt.psi-metrics \
        --collector.libvirt.blkio-metrics \
        --collector.ipmi_dcmi \
        --collector.ipmi.dcmi.cmd="pkg/collector/testdata/ipmi/capmc/capmc" \
        --collector.ipmi_dcmi.test-mode \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:${port}" \
        --web.disable-exporter-metrics \
        --log.level="debug" > "${logfile}" 2>&1 &
  elif [ "${scenario}" = "exporter-cgroups-v2-libvirt" ] 
  then
      REDFISH_HOST=localhost REDFISH_PORT=5000 ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.cgroups.force-version="v2" \
        --collector.libvirt \
        --collector.gpu.type="nvidia" \
        --collector.gpu.nvidia-smi-path="pkg/collector/testdata/nvidia-smi" \
        --collector.libvirt.xml-dir="pkg/collector/testdata/qemu" \
        --collector.libvirt.swap-memory-metrics \
        --collector.libvirt.psi-metrics \
        --collector.libvirt.blkio-metrics \
        --collector.ipmi_dcmi \
        --collector.ipmi.dcmi.cmd="pkg/collector/testdata/ipmi/capmc/capmc" \
        --collector.ipmi_dcmi.test-mode \
        --collector.redfish \
        --collector.redfish.config.file="pkg/collector/testdata/redfish/config.yml" \
        --collector.redfish.config.file.expand-env-vars \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:${port}" \
        --web.disable-exporter-metrics \
        --log.level="debug" > "${logfile}" 2>&1 &
  elif [ "${scenario}" = "exporter-cgroups-v2-libvirt-nonsystemd-layout" ] 
  then
      REDFISH_HOST=localhost REDFISH_PORT=5000 ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup_alt" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.cgroups.force-version="v2" \
        --collector.libvirt \
        --collector.gpu.type="nvidia" \
        --collector.gpu.nvidia-smi-path="pkg/collector/testdata/nvidia-smi" \
        --collector.libvirt.xml-dir="pkg/collector/testdata/qemu" \
        --collector.libvirt.swap-memory-metrics \
        --collector.libvirt.psi-metrics \
        --collector.libvirt.blkio-metrics \
        --collector.ipmi_dcmi \
        --collector.ipmi.dcmi.cmd="pkg/collector/testdata/ipmi/capmc/capmc" \
        --collector.ipmi_dcmi.test-mode \
        --collector.redfish \
        --collector.redfish.config.file="pkg/collector/testdata/redfish/config.yml" \
        --collector.redfish.config.file.expand-env-vars \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:${port}" \
        --web.disable-exporter-metrics \
        --log.level="debug" > "${logfile}" 2>&1 &
  elif [ "${scenario}" = "exporter-cgroups-v1-k8s" ] 
  then
      ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.cgroups.force-version="v1" \
        --collector.k8s \
        --collector.k8s.kube-config-file="pkg/collector/testdata/k8s/kubeconfig.yml" \
        --collector.k8s.kubelet-socket-file="${CEEMS_KUBELET_SOCKET_DIR}/nvidia/kubelet.sock" \
        --collector.gpu.type="nvidia" \
        --collector.gpu.nvidia-smi-path="pkg/collector/testdata/nvidia-smi" \
        --collector.ipmi_dcmi \
        --collector.ipmi.dcmi.cmd="pkg/collector/testdata/ipmi/capmc/capmc" \
        --collector.ipmi_dcmi.test-mode \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:${port}" \
        --web.disable-exporter-metrics \
        --log.level="debug" > "${logfile}" 2>&1 &
  elif [ "${scenario}" = "exporter-cgroups-v2-k8s" ] 
  then
      REDFISH_HOST=localhost REDFISH_PORT=5000 ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.cgroups.force-version="v2" \
        --collector.k8s \
        --collector.k8s.kube-config-file="pkg/collector/testdata/k8s/kubeconfig.yml" \
        --collector.k8s.kubelet-socket-file="${CEEMS_KUBELET_SOCKET_DIR}/amd/kubelet.sock" \
        --collector.gpu.type="amd" \
        --collector.gpu.amd-smi-path="pkg/collector/testdata/amd-smi" \
        --collector.ipmi_dcmi \
        --collector.ipmi.dcmi.cmd="pkg/collector/testdata/ipmi/capmc/capmc" \
        --collector.ipmi_dcmi.test-mode \
        --collector.redfish \
        --collector.redfish.config.file="pkg/collector/testdata/redfish/config.yml" \
        --collector.redfish.config.file.expand-env-vars \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:${port}" \
        --web.disable-exporter-metrics \
        --log.level="debug" > "${logfile}" 2>&1 &
  elif [ "${scenario}" = "exporter-cgroups-v2-k8s-nogpu" ] 
  then
      ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.cgroups.force-version="v2" \
        --collector.gpu.type="nogpu" \
        --collector.k8s \
        --collector.k8s.kube-config-file="pkg/collector/testdata/k8s/kubeconfig.yml" \
        --collector.ipmi_dcmi \
        --collector.ipmi.dcmi.cmd="pkg/collector/testdata/ipmi/capmc/capmc" \
        --collector.ipmi_dcmi.test-mode \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:${port}" \
        --web.disable-exporter-metrics \
        --log.level="debug" > "${logfile}" 2>&1 &
  elif [ "${scenario}" = "discoverer-cgroups-v2-slurm" ] 
  then
      ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --discoverer.alloy-targets \
        --collector.cgroups.force-version="v2" \
        --collector.slurm \
        --collector.gpu.type="nogpu" \
        --collector.ipmi_dcmi \
        --collector.ipmi.dcmi.cmd="pkg/collector/testdata/ipmi/capmc/capmc" \
        --collector.ipmi_dcmi.test-mode \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:${port}" \
        --web.disable-exporter-metrics \
        --log.level="debug" > "${logfile}" 2>&1 &
  elif [ "${scenario}" = "discoverer-cgroups-v1-slurm" ] 
  then
      REDFISH_HOST=localhost REDFISH_PORT=5000 ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --discoverer.alloy-targets \
        --collector.slurm \
        --collector.gpu.type="nogpu" \
        --collector.cgroups.force-version="v1" \
        --collector.ipmi_dcmi \
        --collector.ipmi.dcmi.cmd="pkg/collector/testdata/ipmi/capmc/capmc" \
        --collector.ipmi_dcmi.test-mode \
        --collector.redfish \
        --collector.redfish.config.file="pkg/collector/testdata/redfish/config.yml" \
        --collector.redfish.config.file.expand-env-vars \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:${port}" \
        --web.disable-exporter-metrics \
        --log.level="debug" > "${logfile}" 2>&1 &
  elif [ "${scenario}" = "discoverer-cgroups-v2-k8s" ] 
  then
      ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --discoverer.alloy-targets \
        --collector.cgroups.force-version="v2" \
        --collector.gpu.type="nogpu" \
        --collector.k8s \
        --collector.k8s.kube-config-file="pkg/collector/testdata/k8s/kubeconfig.yml" \
        --collector.k8s.kubelet-socket-file="${CEEMS_KUBELET_SOCKET_DIR}/amd/kubelet.sock" \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:${port}" \
        --web.disable-exporter-metrics \
        --log.level="debug" > "${logfile}" 2>&1 &
  elif [ "${scenario}" = "discoverer-cgroups-v1-k8s" ] 
  then
      REDFISH_HOST=localhost REDFISH_PORT=5000 ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --discoverer.alloy-targets \
        --collector.gpu.type="nogpu" \
        --collector.k8s \
        --collector.k8s.kube-config-file="pkg/collector/testdata/k8s/kubeconfig.yml" \
        --collector.k8s.kubelet-socket-file="${CEEMS_KUBELET_SOCKET_DIR}/amd/kubelet.sock" \
        --collector.cgroups.force-version="v1" \
        --collector.redfish \
        --collector.redfish.config.file="pkg/collector/testdata/redfish/config.yml" \
        --collector.redfish.config.file.expand-env-vars \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:${port}" \
        --web.disable-exporter-metrics \
        --log.level="debug" > "${logfile}" 2>&1 &
  fi
  CEEMS_EXPORTER_PID=$!

  echo "${MOCK_SERVERS_PID} ${CEEMS_EXPORTER_PID}" > "${pidfile}"

  # sleep 1
  waitport "${port}"

  if [[ "${scenario}" =~ ^"discoverer" ]]
  then
    get "127.0.0.1:${port}/alloy-targets" | grep -E -v "${skip_re}" > "${fixture_output}"
  else
    get "127.0.0.1:${port}/metrics" | grep -E -v "${skip_re}" > "${fixture_output}"
  fi
elif [[ "${scenario}" =~ ^"api" ]] 
then
  if [ ! -x ./bin/ceems_api_server ]
  then
      echo './bin/ceems_api_server not found. Consider running `go build` first.' >&2
      exit 1
  fi

  export PATH="${GOBIN:-}:${PATH}"
  ./bin/mock_servers prom os-compute os-identity k8s-api >> "${logfile}" 2>&1 &
  MOCK_SERVERS_PID=$!

  waitport "9090"
  waitport "8080"
  waitport "7070"
  waitport "9080"

  # Copy config file to tmpdir
  cp pkg/api/testdata/config.yml "${tmpdir}/config.yml"

  # Replace strings in the config file
  sed -i -e "s,TO_REPLACE,${tmpdir},g" "${tmpdir}/config.yml"

  ./bin/ceems_api_server \
    --storage.data.skip.delete.old.units \
    --test.disable.checks \
    --web.listen-address="127.0.0.1:${port}" \
    --config.file="${tmpdir}/config.yml" \
    --log.level="debug" >> "${logfile}" 2>&1 &
  CEEMS_API_PID=$!

  echo "${MOCK_SERVERS_PID} ${CEEMS_API_PID}" > "${pidfile}"

  sleep 2
  waitport "${port}"

  # Usage from and to timestamps
  usage_from=$(date +%s --date='86400 seconds ago')
  usage_to=$(date +%s --date='1800 seconds')
  timezone="Europe%2FAthens"

  if [ "${scenario}" = "api-project-query" ]
  then
    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/${api_version}/projects?project=acc1" > "${fixture_output}"
  elif [ "${scenario}" = "api-project-empty-query" ]
  then
    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/${api_version}/projects?project=acc3" > "${fixture_output}"
  elif [ "${scenario}" = "api-project-admin-query" ]
  then
    get -H "X-Grafana-User: grafana" "127.0.0.1:${port}/api/${api_version}/projects/admin?project=test-project-3" > "${fixture_output}"
  elif [ "${scenario}" = "api-project-query-k8s" ]
  then
    get -H "X-Grafana-User: rb1" "127.0.0.1:${port}/api/${api_version}/projects?project=ns2" > "${fixture_output}"
  elif [ "${scenario}" = "api-user-query" ]
  then
    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/${api_version}/users" > "${fixture_output}"
  elif [ "${scenario}" = "api-user-admin-query" ]
  then
    get -H "X-Grafana-User: grafana" "127.0.0.1:${port}/api/${api_version}/users/admin?user=usr1" > "${fixture_output}"
  elif [ "${scenario}" = "api-user-admin-all-query" ]
  then
    get -H "X-Grafana-User: grafana" "127.0.0.1:${port}/api/${api_version}/users/admin" > "${fixture_output}"
  elif [ "${scenario}" = "api-user-query-k8s" ]
  then
    get -H "X-Grafana-User: rb1" "127.0.0.1:${port}/api/${api_version}/users" > "${fixture_output}"
  elif [ "${scenario}" = "api-cluster-admin-query" ]
  then
    get -H "X-Grafana-User: ceems-int-svc" "127.0.0.1:${port}/api/${api_version}/clusters/admin" > "${fixture_output}"
  elif [ "${scenario}" = "api-uuid-query" ]
  then
    get -H "X-Grafana-User: usr2" "127.0.0.1:${port}/api/${api_version}/units?uuid=1481508&project=acc2&cluster_id=slurm-0" > "${fixture_output}"
  elif [ "${scenario}" = "api-units-invalid-query" ]
  then
    get -H "X-Grafana-User: usr2" "127.0.0.1:${port}/api/${api_version}/units?cluster_id=slurm-0&from=1676934000&to=1677538800&field=uuiid" > "${fixture_output}"
  elif [ "${scenario}" = "api-running-query" ]
  then
    get -H "X-Grafana-User: test-user-1" "127.0.0.1:${port}/api/${api_version}/units?running&cluster_id=os-1&field=uuid&field=state&field=started_at&field=allocation&field=tags&timezone=${timezone}" > "${fixture_output}"
  elif [ "${scenario}" = "api-units-query-k8s" ]
  then
    get -H "X-Grafana-User: kusr2" "127.0.0.1:${port}/api/${api_version}/units?cluster_id=k8s-1&project=ns2&to=1751883060" > "${fixture_output}"
  elif [ "${scenario}" = "api-admin-query" ]
  then
    get -H "X-Grafana-User: grafana" "127.0.0.1:${port}/api/${api_version}/units/admin?user=usr3&cluster_id=slurm-0&project=acc3&from=1676934000&to=1677538800" > "${fixture_output}"
  elif [ "${scenario}" = "api-admin-query-all" ]
  then
    get -H "X-Grafana-User: grafana" "127.0.0.1:${port}/api/${api_version}/units/admin?cluster_id=slurm-1&from=1676934000&to=1677538800" > "${fixture_output}"
  elif [ "${scenario}" = "api-admin-query-all-selected-fields" ]
  then
    get -H "X-Grafana-User: grafana" "127.0.0.1:${port}/api/${api_version}/units/admin?cluster_id=os-0&running&from=1729000800&to=1729002300&field=uuid&field=started_at&field=ended_at&field=foo" > "${fixture_output}"
  elif [ "${scenario}" = "api-admin-denied-query" ]
  then
    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/${api_version}/units/admin" > "${fixture_output}"
  elif [ "${scenario}" = "api-current-usage-query" ]
  then
    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/${api_version}/usage/current?cluster_id=slurm-1&from=${usage_from}&to=${usage_to}&__terminated" > "${fixture_output}"
  elif [ "${scenario}" = "api-current-usage-experimental-query" ]
  then
    get -H "X-Grafana-User: test-user-4" "127.0.0.1:${port}/api/${api_version}/usage/current?cluster_id=os-1&from=${usage_from}&to=${usage_to}&experimental" > "${fixture_output}"
  elif [ "${scenario}" = "api-current-usage-admin-query" ]
  then
    get -H "X-Grafana-User: grafana" "127.0.0.1:${port}/api/${api_version}/usage/current/admin?cluster_id=slurm-1&user=usr15&user=usr3&from=${usage_from}&to=${usage_to}&__terminated" > "${fixture_output}"
  elif [ "${scenario}" = "api-current-usage-admin-experimental-query" ]
  then
    get -H "X-Grafana-User: grafana" "127.0.0.1:${port}/api/${api_version}/usage/current/admin?cluster_id=slurm-1&user=usr15&user=usr4&cluster_id=os-1&user=test-user-4&from=${usage_from}&to=${usage_to}&experimental" > "${fixture_output}"
  elif [ "${scenario}" = "api-current-usage-query-k8s" ]
  then
    get -H "X-Grafana-User: kusr2" "127.0.0.1:${port}/api/${api_version}/usage/current?cluster_id=k8s-1&from=${usage_from}&to=${usage_to}&__terminated" > "${fixture_output}"
  elif [ "${scenario}" = "api-current-usage-admin-denied-query" ]
  then
    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/${api_version}/usage/global/admin?cluster_id=slurm-1&user=usr2" > "${fixture_output}"
  elif [ "${scenario}" = "api-global-usage-query" ]
  then
    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/${api_version}/usage/global?cluster_id=slurm-0&field=username&field=project&field=num_units" > "${fixture_output}"
  elif [ "${scenario}" = "api-global-usage-admin-query" ]
  then
    get -H "X-Grafana-User: grafana" "127.0.0.1:${port}/api/${api_version}/usage/global/admin?cluster_id=slurm-0&field=username&field=project&field=num_units" > "${fixture_output}"
  elif [ "${scenario}" = "api-global-usage-query-k8s" ]
  then
    get -H "X-Grafana-User: kusr1" "127.0.0.1:${port}/api/${api_version}/usage/global?cluster_id=slurm-0&field=username&field=project&field=num_units" > "${fixture_output}"
  elif [ "${scenario}" = "api-current-stats-admin-query" ]
  then
    get -H "X-Grafana-User: grafana" "127.0.0.1:${port}/api/${api_version}/stats/current/admin?cluster_id=os-1&from=1728994800&to=1729005000" > "${fixture_output}"
  elif [ "${scenario}" = "api-global-stats-admin-query" ]
  then
    get -H "X-Grafana-User: grafana" "127.0.0.1:${port}/api/${api_version}/stats/global/admin" > "${fixture_output}"
  elif [ "${scenario}" = "api-verify-pass-query" ]
  then
    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/${api_version}/units/verify?cluster_id=slurm-0&uuid=1479763&uuid=1479765" > "${fixture_output}"
  elif [ "${scenario}" = "api-verify-fail-query" ]
  then
    get -H "X-Grafana-User: usr2" "127.0.0.1:${port}/api/${api_version}/units/verify?cluster_id=slurm-1&uuid=1479763&uuid=11508" > "${fixture_output}"
  elif [ "${scenario}" = "api-demo-units-query" ]
  then
    get -s -o /dev/null -w "%{http_code}" "127.0.0.1:${port}/api/${api_version}/demo/units" > "${fixture_output}"
  elif [ "${scenario}" = "api-demo-usage-query" ]
  then
    get -s -o /dev/null -w "%{http_code}" "127.0.0.1:${port}/api/${api_version}/demo/usage" > "${fixture_output}"
  elif [ "${scenario}" = "api-cors-preflight" ]
  then
    get -I -X OPTIONS -H "Access-Control-Request-Method: GET" -H "Access-Control-Request-Headers: x-grafana-user" -H "Origin: https://reqbin.com" "127.0.0.1:${port}/api/${api_version}/units" > "${fixture_output}"
    # Remove Date line from output
    sed -i '/^Date/d' "${fixture_output}"
  fi

elif [[ "${scenario}" =~ ^"lb" ]] 
then
  if [ ! -x ./bin/ceems_lb ]
  then
      echo './bin/ceems_lb not found. Consider running `go build` first.' >&2
      exit 1
  fi

  port2=$((port + 10))

  if [[ "${scenario}" = "lb-basic" ]] 
  then
    ./bin/mock_servers prom pyro >> "${logfile}" 2>&1 &
    MOCK_SERVERS_PID=$!

    waitport "4040"
    waitport "9090"

    ./bin/ceems_lb \
      --config.file pkg/lb/testdata/config-db.yml \
      --web.listen-address="127.0.0.1:${port}" \
      --web.listen-address="127.0.0.1:${port2}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    LB_PID=$!

    echo "${MOCK_SERVERS_PID} ${LB_PID}" > "${pidfile}"

    waitport "${port}"
    waitport "${port2}"

    get -H "X-Grafana-User: usr1" -H "X-Ceems-Cluster-Id: slurm-0" "127.0.0.1:${port}/api/v1/status/config" > "${fixture_output}"
    get -H "X-Grafana-User: usr1" -H "X-Ceems-Cluster-Id: slurm-0" "127.0.0.1:${port2}/api/v1/status/config" >> "${fixture_output}"

  elif [[ "${scenario}" = "lb-basic-tsdb-only" ]] 
  then
    ./bin/mock_servers prom >> "${logfile}" 2>&1 &
    MOCK_SERVERS_PID=$!

    waitport "9090"

    ./bin/ceems_lb \
      --config.file pkg/lb/testdata/config-tsdb-only.yml \
      --web.listen-address="127.0.0.1:${port}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    LB_PID=$!

    echo "${MOCK_SERVERS_PID} ${LB_PID}" > "${pidfile}"

    waitport "${port}"

    get -H "X-Grafana-User: grafana" -H "X-Ceems-Cluster-Id: slurm-0" "127.0.0.1:${port}/api/v1/status/config" > "${fixture_output}"

  elif [[ "${scenario}" = "lb-basic-pyro-only" ]] 
  then
    export PATH="${GOBIN:-}:${PATH}"
    ./bin/mock_servers pyro >> "${logfile}" 2>&1 &
    MOCK_SERVERS_PID=$!

    waitport "4040"

    ./bin/ceems_lb \
      --config.file pkg/lb/testdata/config-pyro-only.yml \
      --web.listen-address="127.0.0.1:${port}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    LB_PID=$!

    echo "${MOCK_SERVERS_PID} ${LB_PID}" > "${pidfile}"

    waitport "${port}"

    get -H "X-Grafana-User: usr1" -H "X-Ceems-Cluster-Id: slurm-0" "127.0.0.1:${port}/api/v1/status/config" > "${fixture_output}"

  elif [[ "${scenario}" = "lb-basic-tsdb-pyro" ]] 
  then
    ./bin/mock_servers prom pyro >> "${logfile}" 2>&1 &
    MOCK_SERVERS_PID=$!

    waitport "9090"
    waitport "4040"

    ./bin/ceems_lb \
      --config.file pkg/lb/testdata/config-tsdb-pyro.yml \
      --web.listen-address="127.0.0.1:${port}" \
      --web.listen-address="127.0.0.1:${port2}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    LB_PID=$!

    echo "${MOCK_SERVERS_PID} ${LB_PID}" > "${pidfile}"

    waitport "${port}"
    waitport "${port2}"

    get -H "X-Grafana-User: usr1" -H "X-Ceems-Cluster-Id: slurm-0" "127.0.0.1:${port}/api/v1/status/config" > "${fixture_output}"
    get -H "X-Grafana-User: usr1" -H "X-Ceems-Cluster-Id: slurm-0" "127.0.0.1:${port2}/api/v1/status/config" >> "${fixture_output}"
    get -H "X-Grafana-User: usr1" -H "X-Ceems-Cluster-Id: slurm-1" "127.0.0.1:${port2}/api/v1/status/config" >> "${fixture_output}"

  elif [[ "${scenario}" = "lb-forbid-user-query-db" ]] 
  then
    ./bin/mock_servers prom pyro >> "${logfile}" 2>&1 &
    MOCK_SERVERS_PID=$!

    waitport "4040"
    waitport "9090"

    ./bin/ceems_lb \
      --config.file pkg/lb/testdata/config-db.yml \
      --web.listen-address="127.0.0.1:${port}" \
      --web.listen-address="127.0.0.1:${port2}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    LB_PID=$!

    echo "${MOCK_SERVERS_PID} ${LB_PID}" > "${pidfile}"

    waitport "${port}"
    waitport "${port2}"

    get -H "X-Grafana-User: usr1" -H "X-Ceems-Cluster-Id: slurm-1" "127.0.0.1:${port}/api/v1/query?query=avg_cpu_usage\{uuid=\"1481510\"\}&time=1713032179.506" > "${fixture_output}"
    ./bin/pyro_requestor -url "http://localhost:${port2}/querier.v1.QuerierService/SelectMergeStacktraces" -username usr1 -cluster-id slurm-1 -uuid 1481510 -start 1713032179000 >> "${fixture_output}"

  elif [[ "${scenario}" = "lb-allow-user-query-db" ]] 
  then
    ./bin/mock_servers prom pyro >> "${logfile}" 2>&1 &
    MOCK_SERVERS_PID=$!

    waitport "4040"
    waitport "9090"

    ./bin/ceems_lb \
      --config.file pkg/lb/testdata/config-db.yml \
      --web.listen-address="127.0.0.1:${port}" \
      --web.listen-address="127.0.0.1:${port2}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    LB_PID=$!

    echo "${MOCK_SERVERS_PID} ${LB_PID}" > "${pidfile}"

    waitport "${port}"
    waitport "${port2}"

    get -H "X-Grafana-User: usr1" -H "X-Ceems-Cluster-Id: slurm-0" "127.0.0.1:${port}/api/v1/query?query=avg_cpu_usage\{uuid=\"1479763\"\}&time=1645450627" > "${fixture_output}"
    ./bin/pyro_requestor -url "http://localhost:${port2}/querier.v1.QuerierService/SelectMergeStacktraces" -username usr1 -cluster-id slurm-0 -uuid 1479763 -start 1645450627000 >> "${fixture_output}"

  elif [[ "${scenario}" = "lb-forbid-user-query-api" ]] 
  then
    ./bin/mock_servers prom pyro os-compute os-identity k8s-api >> "${logfile}" 2>&1 &
    MOCK_SERVERS_PID=$!

    waitport "9090"
    waitport "4040"
    waitport "8080"
    waitport "7070"
    waitport "9080"

    # Copy config file to tmpdir
    cp pkg/api/testdata/config.yml "${tmpdir}/config.yml"

    # Replace strings in the config file
    sed -i -e "s,TO_REPLACE,${tmpdir},g" "${tmpdir}/config.yml"

    ./bin/ceems_api_server \
      --storage.data.skip.delete.old.units \
      --test.disable.checks \
      --config.file="${tmpdir}/config.yml" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    CEEMS_API_PID=$!

    waitport "9020"

    ./bin/ceems_lb \
      --config.file pkg/lb/testdata/config-api.yml \
      --web.listen-address="127.0.0.1:${port}" \
      --web.listen-address="127.0.0.1:${port2}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    LB_PID=$!

    echo "${MOCK_SERVERS_PID} ${CEEMS_API_PID} ${LB_PID}" > "${pidfile}"

    waitport "${port}"
    waitport "${port2}"

    get -H "X-Grafana-User: usr1" -H "X-Ceems-Cluster-Id: slurm-1" "127.0.0.1:${port}/api/v1/query?query=avg_cpu_usage\{uuid=\"1481510\"\}&time=1676990946" > "${fixture_output}"
    ./bin/pyro_requestor -url "http://localhost:${port2}/querier.v1.QuerierService/SelectMergeStacktraces" -username usr1 -cluster-id slurm-1 -uuid 1481510 -start 1676990946000 >> "${fixture_output}"

  elif [[ "${scenario}" = "lb-allow-user-query-api" ]] 
  then
    ./bin/mock_servers prom pyro os-compute os-identity k8s-api >> "${logfile}" 2>&1 &
    MOCK_SERVERS_PID=$!

    waitport "4040"
    waitport "8080"
    waitport "7070"
    waitport "9090"
    waitport "9080"

    # Copy config file to tmpdir
    cp pkg/api/testdata/config.yml "${tmpdir}/config.yml"

    # Replace strings in the config file
    sed -i -e "s,TO_REPLACE,${tmpdir},g" "${tmpdir}/config.yml"

    ./bin/ceems_api_server \
      --storage.data.skip.delete.old.units \
      --test.disable.checks \
      --config.file="${tmpdir}/config.yml" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    CEEMS_API_PID=$!

    waitport "9020"

    ./bin/ceems_lb \
      --config.file pkg/lb/testdata/config-api.yml \
      --web.listen-address="127.0.0.1:${port}" \
      --web.listen-address="127.0.0.1:${port2}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    LB_PID=$!

    echo "${MOCK_SERVERS_PID} ${CEEMS_API_PID} ${LB_PID}" > "${pidfile}"

    waitport "${port}"
    waitport "${port2}"

    get -H "X-Grafana-User: usr1" -H "X-Ceems-Cluster-Id: slurm-0" "127.0.0.1:${port}/api/v1/query?query=avg_cpu_usage\{uuid=\"1479763\"\}&time=1645450627" > "${fixture_output}"
    ./bin/pyro_requestor -url "http://localhost:${port2}/querier.v1.QuerierService/SelectMergeStacktraces" -username usr1 -cluster-id slurm-0 -uuid 1479763 -start 1645450627000 >> "${fixture_output}"

  elif [[ "${scenario}" = "lb-allow-admin-query" ]] 
  then
    ./bin/mock_servers prom pyro >> "${logfile}" 2>&1 &
    MOCK_SERVERS_PID=$!

    waitport "4040"
    waitport "9090"

    ./bin/ceems_lb \
      --config.file pkg/lb/testdata/config-db.yml \
      --web.listen-address="127.0.0.1:${port}" \
      --web.listen-address="127.0.0.1:${port2}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    LB_PID=$!

    echo "${MOCK_SERVERS_PID} ${LB_PID}" > "${pidfile}"

    waitport "${port}"
    waitport "${port2}"

    get -H "X-Grafana-User: grafana" -H "X-Ceems-Cluster-Id: slurm-1" -H "Content-Type: application/x-www-form-urlencoded" -X POST -d "query=avg_cpu_usage{uuid=\"1479765\"}" "127.0.0.1:${port}/api/v1/query" > "${fixture_output}"
    ./bin/pyro_requestor -url "http://localhost:${port2}/querier.v1.QuerierService/SelectMergeStacktraces" -username grafana -cluster-id slurm-1 -uuid 1479765 -start 1645450627000 >> "${fixture_output}"

  elif [[ "${scenario}" = "lb-auth" ]] 
  then
    export PATH="${GOBIN:-}:${PATH}"
    prometheus \
      --config.file pkg/lb/testdata/prometheus.yml \
      --web.config.file pkg/lb/testdata/web-config.yml \
      --storage.tsdb.retention.time 10y \
      --storage.tsdb.path "${tmpdir}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    PROMETHEUS_PID=$!

    waitport "9090"

    ./bin/ceems_lb \
      --config.file pkg/lb/testdata/config-with-auth.yml \
      --web.config.file pkg/lb/testdata/web-config-ceems.yml \
      --web.listen-address="127.0.0.1:${port}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    LB_PID=$!

    echo "${PROMETHEUS_PID} ${LB_PID}" > "${pidfile}"

    waitport "${port}"

    get -H "X-Grafana-User: usr1" -H "X-Ceems-Cluster-Id: slurm-0" "http://ceems:password@127.0.0.1:${port}/api/v1/query?query=avg_cpu_usage\{uuid=\"1479763\"\}&time=1645450627" > "${fixture_output}"
  fi
elif [[ "${scenario}" =~ ^"redfish" ]]
then
  if [ ! -x ./bin/redfish_proxy ]
  then
        echo './bin/redfish_proxy not found. Consider running `go build` first.' >&2
        exit 1
  fi

  if [ "${scenario}" = "redfish-proxy-frontend-plain-backend-plain" ]
  then
    export PATH="${GOBIN:-}:${PATH}"
    ./bin/mock_servers redfish-targets-plain >> "${logfile}" 2>&1 &
    MOCK_SERVERS_PID=$!

    ./bin/redfish_proxy \
      --web.listen-address="127.0.0.1:${port}" \
      --config.file="cmd/redfish_proxy/testdata/config-plain.yml" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    REDFISH_PROXY_PID=$!

    echo "${MOCK_SERVERS_PID} ${REDFISH_PROXY_PID}" > "${pidfile}"

    sleep 5
    waitport "${port}"
    for i in {0..9}
    do
      get -H "X-Real-IP: 192.168.1.${i}" "127.0.0.1:${port}/redfish/v1/" >> "${fixture_output}"
    done
    get -H "X-Redfish-Url: http://localhost:5000" "127.0.0.1:${port}/redfish/v1/" >> "${fixture_output}"

  elif [ "${scenario}" = "redfish-proxy-frontend-tls-backend-plain" ]
  then
    export PATH="${GOBIN:-}:${PATH}"
    ./bin/mock_servers redfish-targets-plain >> "${logfile}" 2>&1 &
    MOCK_SERVERS_PID=$!

    ./bin/redfish_proxy \
      --web.listen-address="127.0.0.1:${port}" \
      --config.file="cmd/redfish_proxy/testdata/config-plain.yml" \
      --web.config.file="cmd/redfish_proxy/testdata/web-config.yml" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    REDFISH_PROXY_PID=$!

    echo "${MOCK_SERVERS_PID} ${REDFISH_PROXY_PID}" > "${pidfile}"

    sleep 5
    waitport "${port}"
    for i in {0..9}
    do
      get -H "X-Real-IP: 192.168.1.${i}" "https://admin:admin@127.0.0.1:${port}/redfish/v1/" >> "${fixture_output}"
    done
    get -H "X-Redfish-Url: http://localhost:5000" "https://admin:admin@127.0.0.1:${port}/redfish/v1/" >> "${fixture_output}"

    # Request without basic auth credentials which should give us unauthorized response
    get -H "X-Redfish-Url: http://localhost:5000" "https://127.0.0.1:${port}/redfish/v1/" >> "${fixture_output}"

  elif [ "${scenario}" = "redfish-proxy-frontend-plain-backend-tls" ]
  then
    export PATH="${GOBIN:-}:${PATH}"
    ./bin/mock_servers redfish-targets-tls >> "${logfile}" 2>&1 &
    MOCK_SERVERS_PID=$!

    ./bin/redfish_proxy \
      --web.listen-address="127.0.0.1:${port}" \
      --config.file="cmd/redfish_proxy/testdata/config-tls.yml" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    REDFISH_PROXY_PID=$!

    echo "${MOCK_SERVERS_PID} ${REDFISH_PROXY_PID}" > "${pidfile}"

    sleep 5
    waitport "${port}"
    for i in {0..9}
    do
      get -H "X-Real-IP: 192.168.1.${i}" "127.0.0.1:${port}/redfish/v1/" >> "${fixture_output}"
    done
    get -H "X-Redfish-Url: https://localhost:5005" "127.0.0.1:${port}/redfish/v1/" >> "${fixture_output}"

  elif [ "${scenario}" = "redfish-proxy-frontend-tls-backend-tls" ]
  then
    export PATH="${GOBIN:-}:${PATH}"
    ./bin/mock_servers redfish-targets-tls >> "${logfile}" 2>&1 &
    MOCK_SERVERS_PID=$!

    ./bin/redfish_proxy \
      --web.listen-address="127.0.0.1:${port}" \
      --config.file="cmd/redfish_proxy/testdata/config-tls.yml" \
      --web.config.file="cmd/redfish_proxy/testdata/web-config.yml" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    REDFISH_PROXY_PID=$!

    echo "${MOCK_SERVERS_PID} ${REDFISH_PROXY_PID}" > "${pidfile}"

    sleep 5
    waitport "${port}"
    for i in {0..9}
    do
      get -H "X-Real-IP: 192.168.1.${i}" "https://admin:admin@127.0.0.1:${port}/redfish/v1/" >> "${fixture_output}"
    done
    get -H "X-Redfish-Url: https://localhost:5005" "https://admin:admin@127.0.0.1:${port}/redfish/v1/" >> "${fixture_output}"

  elif [ "${scenario}" = "redfish-proxy-targetless-frontend-plain-backend-plain" ]
  then
    export PATH="${GOBIN:-}:${PATH}"
    ./bin/mock_servers redfish-targets-plain >> "${logfile}" 2>&1 &
    MOCK_SERVERS_PID=$!

    ./bin/redfish_proxy \
      --web.listen-address="127.0.0.1:${port}" \
      --log.level="debug" >> "${logfile}" 2>&1 &
    REDFISH_PROXY_PID=$!

    echo "${MOCK_SERVERS_PID} ${REDFISH_PROXY_PID}" > "${pidfile}"

    sleep 5
    waitport "${port}"
    get -H "X-Redfish-Url: http://localhost:5000" "http://127.0.0.1:${port}/redfish/v1/" >> "${fixture_output}"
  fi

elif [[ "${scenario}" =~ ^"cacct" ]]
then
  if [ ! -x ./bin/cacct ]
  then
        echo './bin/cacct not found. Consider running `go build` first.' >&2
        exit 1
  fi

  ./bin/mock_servers prom os-compute os-identity k8s-api >> "${logfile}" 2>&1 &
  MOCK_SERVERS_PID=$!

  waitport "9090"
  waitport "8080"
  waitport "7070"
  waitport "9080"

  # Copy config file to tmpdir
  cp pkg/api/testdata/config.yml "${tmpdir}/config.yml"

  # Replace strings in the config file
  sed -i -e "s,TO_REPLACE,${tmpdir},g" "${tmpdir}/config.yml"

  ./bin/ceems_api_server \
    --storage.data.skip.delete.old.units \
    --test.disable.checks \
    --config.file="${tmpdir}/config.yml" \
    --web.config.file="cmd/cacct/testdata/web-config.yml" \
    --log.level="debug" >> "${logfile}" 2>&1 &
  CEEMS_API_PID=$!

  waitport "9020"

  echo "${CEEMS_API_PID} ${MOCK_SERVERS_PID}" > "${pidfile}"

  if [ "${scenario}" = "cacct-default-format" ]
  then
    ./bin/cacct --current-user=usr1 --config-path="cmd/cacct/testdata" --starttime="2022-02-20" --endtime="2022-03-20" > "${fixture_output}" 2>&1
  elif [ "${scenario}" = "cacct-long-format" ]
  then
    ./bin/cacct --current-user=usr1 --config-path="cmd/cacct/testdata" --starttime="2022-02-20" --endtime="2022-03-20" --long > "${fixture_output}" 2>&1
  elif [ "${scenario}" = "cacct-custom-format" ]
  then
    ./bin/cacct --current-user=usr1 --config-path="cmd/cacct/testdata" --starttime="2022-02-20" --endtime="2022-03-20" --format="jobid,account,cpuusage" > "${fixture_output}" 2>&1
  elif [ "${scenario}" = "cacct-admin-user" ]
  then
    ./bin/cacct --current-user=grafana --config-path="cmd/cacct/testdata" --starttime="2022-02-20" --endtime="2022-03-20" --user=usr1,usr2 > "${fixture_output}" 2>&1
  elif [ "${scenario}" = "cacct-forbid-query" ]
  then
    ./bin/cacct --current-user=usr3 --config-path="cmd/cacct/testdata" --starttime="2022-02-20" --endtime="2022-03-20" --user=usr1,usr2 > "${fixture_output}" 2>&1 || true
  elif [ "${scenario}" = "cacct-tsdata" ]
  then
    ./bin/cacct --current-user=usr1 --config-path="cmd/cacct/testdata" --job="147973" --ts --ts.out-dir="${tmpdir}/ts" > "${fixture_output}" 2>&1
    cat "${tmpdir}/ts/metadata.json" >> "${fixture_output}"
    cat "${tmpdir}/ts/554b56cadf9dea4b.csv" >> "${fixture_output}"
    # Remove line that says time series data has been saved in the output file as it will contain directory name which changes on every run
    sed -i '/^time series data saved to directory/d' "${fixture_output}"
  elif [ "${scenario}" = "cacct-tsdata-fail" ]
  then
    ./bin/cacct --current-user=usr1 --config-path="cmd/cacct/testdata" --starttime="2022-02-20" --endtime="2022-03-20" --ts --ts.out-dir="${tmpdir}/ts" > "${fixture_output}" 2>&1 || true
  fi

elif [[ "${scenario}" =~ ^"tool" ]]
then
  if [ ! -x ./bin/ceems_tool ]
  then
      echo './bin/ceems_tool not found. Consider running `go build` first.' >&2
      exit 1
  fi

  export PATH="${GOBIN:-}:${PATH}"

  if [ "${scenario}" = "tool-recording-rules" ] 
  then
      ./bin/mock_servers redfish > /dev/null 2>&1 &
      MOCK_REDFISH_PID=$!

      waitport "5000"

      ./bin/mock_exporters test-mode dcgm amd-smi amd-device-metrics > /dev/null 2>&1 &
      MOCK_SERVERS_PID=$!

      waitport "9400"
      waitport "9500"
      waitport "9600"

      # IPMI and RAPL available
      ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.cgroups.force-version="v1" \
        --collector.slurm \
        --collector.gpu.type="nogpu" \
        --collector.ipmi_dcmi \
        --collector.ipmi_dcmi.test-mode \
        --collector.ipmi_dcmi.cmd="pkg/collector/testdata/ipmi/freeipmi/ipmi-dcmi" \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:9010" \
        --web.disable-exporter-metrics \
        --log.level="debug" > /dev/null 2>&1 &
      MOCK_EXPORTER1_PID=$!

      # Only RAPL available
      ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.cgroups.force-version="v1" \
        --collector.slurm \
        --collector.gpu.type="nogpu" \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:9011" \
        --web.disable-exporter-metrics \
        --log.level="debug" > /dev/null 2>&1 &
      MOCK_EXPORTER2_PID=$!

      # Only Redfish available with SINGLE CHASSIS
      REDFISH_HOST=localhost REDFISH_PORT=5000 ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.cgroups.force-version="v1" \
        --collector.slurm \
        --collector.gpu.type="nogpu" \
        --collector.redfish \
        --collector.redfish.config.file="pkg/collector/testdata/redfish/config.yml" \
        --collector.redfish.config.file.expand-env-vars \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:9012" \
        --web.disable-exporter-metrics \
        --log.level="debug" > /dev/null 2>&1 &
      MOCK_EXPORTER3_PID=$!

      # Redfish and RAPL available
      REDFISH_HOST=localhost REDFISH_PORT=5000 ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.cgroups.force-version="v1" \
        --collector.slurm \
        --collector.gpu.type="nvidia" \
        --collector.gpu.nvidia-smi-path="pkg/collector/testdata/nvidia-smi" \
        --collector.redfish \
        --collector.redfish.config.file="pkg/collector/testdata/redfish/config.yml" \
        --collector.redfish.config.file.expand-env-vars \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:9013" \
        --web.disable-exporter-metrics \
        --log.level="debug" > /dev/null 2>&1 &
      MOCK_EXPORTER4_PID=$!

      # Cray available
      ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.cgroups.force-version="v1" \
        --collector.slurm \
        --collector.gpu.type="amd" \
        --collector.gpu.rocm-smi-path="pkg/collector/testdata/rocm-smi" \
        --collector.cray_pm_counters \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:9014" \
        --web.disable-exporter-metrics \
        --log.level="debug" > /dev/null 2>&1 &
      MOCK_EXPORTER5_PID=$!

      # Only IPMI available (No RAPL). IPMI includes GPU power
      ./bin/ceems_exporter \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --collector.cgroups.force-version="v1" \
        --collector.slurm \
        --collector.gpu.type="nvidia" \
        --collector.gpu.nvidia-smi-path="pkg/collector/testdata/nvidia-smi" \
        --collector.ipmi_dcmi \
        --collector.ipmi_dcmi.test-mode \
        --collector.ipmi_dcmi.cmd="pkg/collector/testdata/ipmi/openipmi/ipmitool" \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:9015" \
        --web.disable-exporter-metrics \
        --log.level="debug" > /dev/null 2>&1 &
      MOCK_EXPORTER6_PID=$!

      # Emissions target
      ./bin/ceems_exporter \
        --collector.disable-defaults \
        --collector.emissions \
        --collector.emissions.provider=owid \
        --collector.empty-hostname-label \
        --web.listen-address "127.0.0.1:9016" \
        --web.disable-exporter-metrics \
        --log.level="debug" > /dev/null 2>&1 &
      MOCK_EXPORTER7_PID=$!

      waitport "9010"
      waitport "9011"
      waitport "9012"
      waitport "9013"
      waitport "9014"
      waitport "9015"
      waitport "9016"

      prometheus \
        --config.file cmd/ceems_tool/testdata/prometheus.yml \
        --storage.tsdb.retention.time 10y \
        --storage.tsdb.path "${tmpdir}/tsdb" \
        --log.level="debug" >> "${logfile}" 2>&1 &
      PROMETHEUS_PID=$!

      # Time stamp when scrapping
      scrapestart=$(date +%s)

      waitport "9090"

      # Sleep a while for Prometheus to scrape metrics
      sleep 30

      echo "0" | ./bin/ceems_tool tsdb create-recording-rules --country-code=FR --output-dir "${tmpdir}/rules" >> "${logfile}" 2>&1

      # Add content of each recording file to fixture_output
      find "${tmpdir}/rules" -type f -print0 | sort -z | while IFS= read -r -d $'\0' file; do
          # Check generated rules
          promtool check rules "${file}" >> "${logfile}" 2>&1

          echo $(basename "${file}") >> "${fixture_output}"
          cat "$file" >> "${fixture_output}"
      done

      # Rules without emissions target
      echo "1" | ./bin/ceems_tool tsdb create-recording-rules --country-code=FR --disable-providers --output-dir "${tmpdir}/rules" >> "${logfile}" 2>&1

      # Add content of each recording file to fixture_output
      find "${tmpdir}/rules" -type f -print0 | sort -z | while IFS= read -r -d $'\0' file; do
          # Check generated rules
          promtool check rules "${file}" >> "${logfile}" 2>&1

          echo $(basename "${file}") >> "${fixture_output}"
          cat "$file" >> "${fixture_output}"
      done

      # Rules with static emission factor
      echo "1" | ./bin/ceems_tool tsdb create-recording-rules --emission-factor=50 --output-dir "${tmpdir}/rules" >> "${logfile}" 2>&1

      # Add content of each recording file to fixture_output
      find "${tmpdir}/rules" -type f -print0 | sort -z | while IFS= read -r -d $'\0' file; do
        # Check generated rules
          promtool check rules "${file}" >> "${logfile}" 2>&1

          echo $(basename "${file}") >> "${fixture_output}"
          cat "$file" >> "${fixture_output}"
      done

      # Current time stamp
      scrapeend=$(date +%s)

      # Generate metrics from these recording rules
      promtool tsdb create-blocks-from rules --start="${scrapestart}" --end="${scrapeend}" --eval-interval=500ms --output-dir="${tmpdir}/tsdb" "${tmpdir}/rules/cpu-only-ipmi.rules" >> "${logfile}" 2>&1

      # Kill and restart Prom to sync data
      kill -9 "${PROMETHEUS_PID}"

      prometheus \
        --config.file cmd/ceems_tool/testdata/prometheus.yml \
        --storage.tsdb.retention.time 10y \
        --storage.tsdb.path "${tmpdir}/tsdb" \
        --log.level="debug" >> "${logfile}" 2>&1 &
      PROMETHEUS_PID=$!

      echo "${PROMETHEUS_PID} ${MOCK_REDFISH_PID} ${MOCK_SERVERS_PID} ${MOCK_EXPORTER1_PID} ${MOCK_EXPORTER2_PID} ${MOCK_EXPORTER3_PID} ${MOCK_EXPORTER4_PID} ${MOCK_EXPORTER5_PID} ${MOCK_EXPORTER6_PID} ${MOCK_EXPORTER7_PID}" > "${pidfile}"
      
      # Sleep 10 seconds for data to sync
      sleep 10

      # Generate TSDB updater queries
      ./bin/ceems_tool tsdb create-ceems-tsdb-updater-queries >> "${fixture_output}" 2>&1
  elif [ "${scenario}" = "tool-relabel-configs" ] 
  then
      ./bin/mock_exporters test-mode dcgm amd-smi amd-device-metrics >> "${logfile}" 2>&1 &
      MOCK_SERVERS_PID=$!

      waitport "9400"
      waitport "9500"
      waitport "9600"

      prometheus \
        --config.file cmd/ceems_tool/testdata/prometheus.yml \
        --storage.tsdb.retention.time 10y \
        --storage.tsdb.path "${tmpdir}/tsdb" \
        --log.level="debug" >> "${logfile}" 2>&1 &
      PROMETHEUS_PID=$!

      waitport "9090"

      # Sleep a while for Prometheus to scrape metrics
      sleep 20

      echo "${PROMETHEUS_PID} ${MOCK_SERVERS_PID}" > "${pidfile}"

      ./bin/ceems_tool tsdb create-relabel-configs > "${fixture_output}" 2>&1

  elif [ "${scenario}" = "tool-web-config" ] 
  then
      ./bin/ceems_tool config create-web-config --tls --tls.host example.com --output-dir "${tmpdir}/config" > "${logfile}" 2>&1

      # Check if config is valid by starting a CEEMS exporter
      ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/testdata/sys" \
        --path.cgroupfs="pkg/collector/testdata/sys/fs/cgroup" \
        --path.procfs="pkg/collector/testdata/proc" \
        --web.listen-address "127.0.0.1:${port}" \
        --web.config.file="${tmpdir}/config/web-config.yml" \
        --web.disable-exporter-metrics \
        --collector.empty-hostname-label \
        --log.level="debug" >> "${logfile}" 2>&1 &
      EXPORTER_PID=$!

      waitport "${port}"

      echo "${EXPORTER_PID}" > "${pidfile}"

      # Find basic auth password from logfile
      logtext=$(cat "${logfile}")
      regex='plain text password for basic auth is (.*)
store (.*)'

      [[ $logtext =~ $regex ]]
      secret=${BASH_REMATCH[1]}
      
      get "https://ceems:${secret}@127.0.0.1:${port}/metrics" | grep -E -v "${skip_re}" > "${fixture_output}"
  fi

elif [[ "${scenario}" =~ ^"k8s-admission-controller" ]]
then
  if [ ! -x ./bin/ceems_k8s_admission_controller ]
  then
      echo './bin/ceems_k8s_admission_controller not found. Consider running `go build` first.' >&2
      exit 1
  fi

  export PATH="${GOBIN:-}:${PATH}"

  ./bin/ceems_k8s_admission_controller \
    --log.level=debug \
    --web.listen-address "127.0.0.1:${port}" >> "${logfile}" 2>&1 &
  K8S_ADMISSION_CONTROLLER_PID=$!

  waitport "${port}"

  echo "${K8S_ADMISSION_CONTROLLER_PID}" > "${pidfile}"

  if [ "${scenario}" = "k8s-admission-controller-basic" ] 
  then
      for endpoint in "validate" "mutate"; do
        for request in "create-request-without-annotations.json" "create-request-with-annotations.json" "update-request-without-annotations.json" "update-request-with-annotations.json"; do
          for version in "v1" "v1beta1"; do
            vrequest=$(echo "${request}" | sed "s/request/${version}request/g")
            get -X POST -H "Content-Type: application/json" -d @cmd/ceems_k8s_admission_controller/testdata/requests/${vrequest} "http://127.0.0.1:${port}/ceems-admission-controller/${endpoint}" >> "${fixture_output}"
          done
        done
      done

  elif [ "${scenario}" = "k8s-admission-controller-advanced" ] 
  then
      for request in "deployment-request-with-annotations-sa.json" "deployment-request-with-annotations.json" "deployment-request-with-template-annotations-sa.json" "deployment-request-with-template-annotations.json" "deployment-request-without-annotations-sa.json" "deployment-request-without-annotations.json"; do
        get -X POST -H "Content-Type: application/json" -d @cmd/ceems_k8s_admission_controller/testdata/requests/${request} "http://127.0.0.1:${port}/ceems-admission-controller/mutate" >> "${fixture_output}"
      done
  fi
  
fi

# Make classic diff and if it fails attempt to compare JSON
diff -u "${fixture}" "${fixture_output}" || ./scripts/compare_json "${fixture}" "${fixture_output}"
