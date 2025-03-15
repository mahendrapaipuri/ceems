#!/bin/bash
set -exo pipefail

# This script only works for Ubuntu derivatives and it is meant to be
# used in CI to install clang in golang builder containers.
# Works on Ubuntu 24
LLVM_DIR=''

find_llvm() {
    for dir in /usr/lib/*/  # Location where llvm will be installation
		do
				dir=${dir%*/}  # remove the trailing "/"
				if [[ "$dir" == *"llvm"* ]]; then
					LLVM_DIR="${dir}"
					break
				fi
		done
}

create_symlinks() {
    echo "Creating symlinks"
		find_llvm
    $SUDO ln -vsnf "${LLVM_DIR}/bin/clang" /usr/bin/clang
    $SUDO ln -vsnf "${LLVM_DIR}/bin/llc" /usr/bin/llc
}

# Setup sudo prefix
SUDO=''
if (( $EUID != 0 )); then
    SUDO='sudo'
fi

# Check if clang exists. If it exists, we need to ensure that it
# is at least of version >= 18
if [ -x "$(command -v clang)" ]; then
    clang_major_version=$(clang -v 2>&1 | grep version | grep -o "[0-9]\+\.[0-9]\+\.[0-9]\+" | cut -d "." -f1)
    if (( ${clang_major_version} >= 18 )); then
        echo "clang >=18 already installed. Skipping installation...."
        create_symlinks
        exit 0
    fi
fi

# Install clang stable version dependencies
$SUDO apt-get update && $SUDO apt-get install -y --no-install-recommends  \
    wget lsb-release wget software-properties-common gnupg    \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Install clang 19
$SUDO bash -c "$(wget -O - https://apt.llvm.org/llvm.sh)"

# Create necessary symlinks
create_symlinks
