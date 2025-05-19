#!/bin/sh

# When we enable kube-rbac-proxy, we are obliged to run ceems apps
# on localhost. This means kubelet cannot access them and thus
# liveness probes will fail.
# 
# To address this issue, we ship this little script with the image
# and use command method to test for container liveness. This ensures
# that the liveness will always work irrespective of using kube-rbac-proxy
# or not.
#
# The script takes two arguments:
#  - App name (eg ceems_exporter, ceems_api_server)
#  - Port at which app is running
#
# One of these two must be provided for the liveness check. If not, check will
# be skipped with a warning message.
#
# The script always assumes that the app is running without TLS as kube-rbac-proxy
# will handle TLS when enabled

app=""; port=""; verbose=0
while getopts ":a:p:v" opt; do
  case $opt in
    a) app="$OPTARG"
    ;;
    p) port="$OPTARG"
    ;;
    v)
      verbose=1
      set -x
      ;;
    *)
      echo "Usage: $0 [-a] [-p] [-v]"
      echo "  -a: app name to test [options: ceems_exporter, ceems_api_server, ceems_lb, redfish_proxy]"
      echo "  -p: port at which app is running"
      echo "  -v: verbose output"
      exit 1
      ;;
  esac
done

# Check if both arguments are set
if [ -z "${app}" ] && [ -z "${port}" ]; then
    echo "At least one of -a or -p arguments must be provided"
    echo "App name and port are not set. Skipping liveness check..."
    exit 0
fi

# Check if the main process is running
if ! [ -z "${app}" ]; then
    if ! pgrep "${app}" > /dev/null; then
        echo "${app} is not running"
        exit 1
    fi
fi

# Check if HTTP server is responding
if ! [ -z "${port}" ]; then
    if ! curl -sf "http://localhost:${port}/health" > /dev/null; then
        echo "${app} at port ${port} is not responding"
        exit 1
    fi
fi

echo "Liveness check passed"
exit 0 
