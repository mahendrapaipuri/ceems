#!/bin/bash
set -exo pipefail

# Change directory
cd pkg/collector/bpf

# List of archs to test
declare -a archs=("386" "amd64" "arm64" "mips" "mipsle" "mips64" "mips64le" "ppc64le" "riscv64")

# Test if we can compile bpf assets for all these archs
for arch in "${archs[@]}"
do
   echo "Compiling bpf assets for $arch"
   make clean
   GOARCH="$arch" make
done

# Clean up all the assets
make clean

# Always end up with default targets
make
