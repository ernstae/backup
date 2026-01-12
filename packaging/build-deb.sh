#!/bin/bash
set -e

# Build DEB package for Ubuntu
# Usage: ./packaging/build-deb.sh
# This builds for Ubuntu 24.04 LTS (noble)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
IMAGE="twindb-backup-builder-ubuntu"

# Build the container image if it doesn't exist
if ! podman image exists "$IMAGE" 2>/dev/null; then
    echo "Building container image $IMAGE..."
    podman build -f "$SCRIPT_DIR/docker/Dockerfile.ubuntu" -t "$IMAGE" "$SCRIPT_DIR/docker"
fi

# Get version from git or use default
cd "$PROJECT_ROOT"
VERSION_TAG=$(git describe --tags --exact-match 2>/dev/null || git describe --tags --always 2>/dev/null || echo "v0.1.0")
VERSION_NUMBER=$(echo "$VERSION_TAG" | sed 's/^v//' | sed 's/-.*//')

# Detect architecture if not set
ARCH="${GOARCH:-$(go env GOARCH 2>/dev/null || echo 'amd64')}"
echo "Building DEB package for architecture: $ARCH"

# Run build in container
echo "Building DEB package..."
podman run --rm \
    -v "$PROJECT_ROOT:/build" \
    -e VERSION="$VERSION_NUMBER" \
    -e GOARCH="$ARCH" \
    "$IMAGE" \
    bash -c "
        cd /build
        mkdir -p packaging/dist
        cp -r packaging/deb debian
        chmod +x debian/rules
        # Update changelog with version
        sed -i \"s/^twindb-backup (.*)/twindb-backup (${VERSION_NUMBER}-1) unstable; urgency=medium/\" debian/changelog
        export CGO_ENABLED=0
        export GOOS=linux
        export GOARCH=${ARCH}
        # Build for specific architecture
        # For pure Go binaries (CGO_ENABLED=0), Go handles cross-compilation natively.
        # The -a flag sets the package architecture metadata, but Go builds the binary
        # for the target architecture via GOARCH. Since we're not using C/C++ code,
        # we don't need cross toolchains. However, if the container arch differs from
        # target arch, we're relying on Go's native cross-compilation support.
        # Use -d to skip build dependency checks since we install Go manually from upstream
        dpkg-buildpackage -us -uc -b -d -a${ARCH}
        cp ../twindb-backup_*.deb /build/packaging/dist/ 2>/dev/null || true
        cp ../twindb-backup_*.changes /build/packaging/dist/ 2>/dev/null || true
        cp ../twindb-backup_*.dsc /build/packaging/dist/ 2>/dev/null || true
        cp ../twindb-backup_*.tar.gz /build/packaging/dist/ 2>/dev/null || true
        # Clean up debian directory
        rm -rf debian
    "

echo "DEB package built in packaging/dist/"
