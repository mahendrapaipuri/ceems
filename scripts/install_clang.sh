#!/bin/bash
set -exo pipefail

# Install clang stable version dependencies
apt-get update && apt-get install -y --no-install-recommends  \
    wget lsb-release wget software-properties-common gnupg    \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Install clang 18
bash -c "$(wget -O - https://apt.llvm.org/llvm.sh)"

# Create necessary symlinks
ln -vsnf /usr/lib/llvm-18/bin/clang /usr/bin/clang
ln -vsnf /usr/lib/llvm-18/bin/llc /usr/bin/llc
