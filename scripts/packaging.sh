#!/usr/bin/env bash

CEEMS_COMPONENTS="ceems_exporter ceems_api_server ceems_lb cacct redfish_proxy"
CEEMS_SERVICES="ceems_exporter ceems_api_server ceems_lb redfish_proxy"

version="0.0.0"; build=0; test=0;
while getopts 'hv:btd' opt
do
  case "$opt" in
    v)
      version=$OPTARG
      ;;
    b)
      build=1
      ;;
    t)
      test=1
      ;;
    d)
      verbose=1
      set -x
      ;;
    *)
      echo "Usage: $0 [-v] [-b] [-t] [-d]"
      echo "  -v: package version"
      echo "  -b: build packages"
      echo "  -t: test packages"
      echo "  -d: verbose output"
      exit 1
      ;;
  esac
done

build () {
	# Ensure target directory exists
	mkdir -p .tarballs

	# Build packages
	# Use a simple for loop instead of matrix strategy as building packages
	# is a very rapid process and we pay more price by repeating all the steps
	# if using a matrix strategy
	for arch in amd64 arm64; do
	for packager in rpm deb; do
			for app in ${CEEMS_COMPONENTS}; do
					GOARCH=${arch} CEEMS_VERSION=${version} nfpm pkg --config build/package/${app}/nfpm.yml --packager ${packager} --target .tarballs/${app}-${version}-linux-${arch}.${packager}
			done 
	done 
	done
}

test () {
	# Install all CEEMS components
	for app in ${CEEMS_COMPONENTS}; do
			GOARCH=amd64 sudo apt-get install ./.tarballs/${app}-${version}-linux-amd64.deb
	done 

	# Test systemd service of each CEEMS component
	for app in ${CEEMS_SERVICES}; do
			systemctl is-active --quiet "${app}.service" && echo "${app}" is running
	done

	# Test if cacct has setgid bit on it
	[ -g "/usr/local/bin/cacct" ] && printf "cacct has setgid set\n"
}

if [ ${build} -ne 0 ]
  then
		echo ">> building RPM/DEB packages"
		build
fi

if [ ${test} -ne 0 ]
  then
		echo ">> testing RPM/DEB packages"
    test
fi
