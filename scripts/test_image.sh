#!/bin/bash
set -exo pipefail

docker_image=$1
port=$2
app="${@:3}"

container_id=''

wait_start() {
    for in in {1..10}; do
        if  curl -s -m 5 "http://localhost:${port}" > /dev/null; then
            docker_cleanup
            exit 0
        else
            sleep 1
        fi
    done

    exit 1
}

docker_start() {
    container_id=$(docker run -d -p "${port}":"${port}" "${docker_image}" ${app})
}

docker_cleanup() {
    docker kill "${container_id}"
}

if [[ "$#" -lt 3 ]] ; then
    echo "Usage: $0 quay.io/mahendrapaipuri/ceems-exporter:v0.1.0 9010 ceems_exporter <args>" >&2
    exit 1
fi

docker_start
wait_start
