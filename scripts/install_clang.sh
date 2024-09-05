#!/bin/bash
set -exo pipefail

# Setup sudo prefix
SUDO=''
if (( $EUID != 0 )); then
    SUDO='sudo'
fi

# Install clang stable version dependencies
$SUDO apt-get update && $SUDO apt-get install -y --no-install-recommends  \
    wget lsb-release wget software-properties-common gnupg    \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Install clang 18
$SUDO bash -c "$(wget -O - https://apt.llvm.org/llvm.sh)"

# Create necessary symlinks
$SUDO ln -vsnf /usr/lib/llvm-18/bin/clang /usr/bin/clang
$SUDO ln -vsnf /usr/lib/llvm-18/bin/llc /usr/bin/llc
