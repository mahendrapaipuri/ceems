#!/bin/bash
set -exo pipefail

# Check if clang exists. If it exists, we need to ensure that it
# is at least of version >= 18
if [ -x "$(command -v clang)" ]; then
    clang_major_version=$(clang -v 2>&1 | grep version | grep -o "[0-9]\+\.[0-9]\+\.[0-9]\+" | cut -d "." -f1)
    if (( ${clang_major_version} >= 18 )); then
        echo "clang >=18 already installed. Skipping installation...."
        exit 0
    fi
fi

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
