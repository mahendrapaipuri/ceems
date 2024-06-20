#!/bin/bash

# Set repository information
REPO_OWNER="mahendrapaipuri"
REPO_NAME="ceems"
RELEASES_URL="https://github.com/$REPO_OWNER/$REPO_NAME/releases"
API_URL="https://api.github.com/repos/$REPO_OWNER/$REPO_NAME/releases"

# Fetch all release information
ALL_RELEASES=$(curl -s $API_URL)

# Check if a valid release list is found
if [[ $ALL_RELEASES == *"Not Found"* ]]; then
    echo "Releases not found for the $REPO_NAME repository."
    exit 1
fi

# Get latest release
LATEST_RELEASE_TAG=$(echo "$ALL_RELEASES" | grep -Eo '"tag_name": "v[^"]*' | sed -E 's/"tag_name": "//' | sed -E 's/^.//' | head -n 1)

# If no apps are specified, install all ceems apps
test -z "$APPS" && APPS="ceems_exporter ceems_api_server ceems_lb"

# If not version is specified, install latest version
test -z "$VERSION" && VERSION="${LATEST_RELEASE_TAG}"

# If no prefix is specified, install at /usr/local
test -z "$PREFIX" && PREFIX="/usr/local"

# Name of the package
FILE_BASENAME="ceems"

# Make tmp dir for extraction
TMP_DIR="$(mktemp -d)"
# shellcheck disable=SC2064 # intentionally expands here
trap "rm -rf \"$TMP_DIR\"" EXIT INT TERM

OS="$(uname -s  | awk '{print tolower($0)}')"
ARCH="$(uname -m)"
test "$ARCH" = "x86_64" && ARCH="amd64"
test "$ARCH" = "aarch64" && ARCH="arm64"
FILE_NAME="${FILE_BASENAME}-${VERSION}.${OS}-${ARCH}"
TAR_FILE="${FILE_NAME}.tar.gz"

# Download and verify binaries
(
	cd "$TMP_DIR"
	echo "Downloading CEEMS $VERSION..."
	curl -sfLO "$RELEASES_URL/download/v$VERSION/$TAR_FILE"
	curl -sfLO "$RELEASES_URL/download/v$VERSION/sha256sums.txt"
	echo "Verifying checksums..."
	sha256sum --ignore-missing --quiet --check sha256sums.txt
)

# Extract binaries
tar -xf "$TMP_DIR/$TAR_FILE" -C "$TMP_DIR"
chmod +x -R "$TMP_DIR/$FILE_NAME"

# Install binaries and config files
(
    for APP in $APPS
    do
        CFG_DIR="$PREFIX/etc/$APP"
        BIN_DIR="$PREFIX/bin"
        mkdir -p "$CFG_DIR" "$BIN_DIR"
        cp -r "$TMP_DIR/$FILE_NAME/$APP" "$BIN_DIR/$APP"
        cp -r "$TMP_DIR/$FILE_NAME/web-config.yml" "$CFG_DIR"
        test "$APP" = "ceems_api_server" && cp -r "$TMP_DIR/$FILE_NAME/ceems_api_server.yml" "$CFG_DIR"
        test "$APP" = "ceems_lb" && cp -r "$TMP_DIR/$FILE_NAME/ceems_lb.yml" "$CFG_DIR"
    done
)
