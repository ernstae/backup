#!/bin/bash
set -e

# Build RPM package for RockyLinux
# Usage: ./packaging/build-rpm.sh <rockylinux-version>
# Example: ./packaging/build-rpm.sh 9

VERSION="${1:-9}"
IMAGE="twindb-backup-builder-rockylinux${VERSION}"

if [ -z "$VERSION" ]; then
    echo "Usage: $0 <rockylinux-version>"
    echo "Example: $0 9"
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Build the container image if it doesn't exist
if ! podman image exists "$IMAGE" 2>/dev/null; then
    echo "Building container image $IMAGE..."
    podman build -f "$SCRIPT_DIR/docker/Dockerfile.rockylinux${VERSION}" -t "$IMAGE" "$SCRIPT_DIR/docker"
fi

# Get version from git or use default
cd "$PROJECT_ROOT"
VERSION_TAG=$(git describe --tags --exact-match 2>/dev/null || git describe --tags --always 2>/dev/null || echo "v0.1.0")
VERSION_NUMBER=$(echo "$VERSION_TAG" | sed 's/^v//' | sed 's/-.*//')

# Create source tarball
echo "Creating source tarball..."
cd "$PROJECT_ROOT"
TARBALL="twindb-backup-${VERSION_NUMBER}.tar.gz"
tar --exclude='.git' --exclude='*.pyc' --exclude='__pycache__' \
    --exclude='tests' --exclude='vagrant' --exclude='omnibus' \
    --exclude='.github' --exclude='node_modules' --exclude='packaging/dist' \
    --exclude='bin' --exclude='build' --exclude='dist' \
    -czf "/tmp/${TARBALL}" .

# Detect architecture if not set
GOARCH_VAL="${GOARCH:-$(go env GOARCH 2>/dev/null || echo 'amd64')}"
echo "Building RPM package for architecture: $GOARCH_VAL"

# Map GOARCH to RPM architecture names
case "$GOARCH_VAL" in
    amd64)
        RPM_ARCH="x86_64"
        ;;
    arm64)
        RPM_ARCH="aarch64"
        ;;
    *)
        RPM_ARCH="$GOARCH_VAL"
        ;;
esac

# Run build in container
echo "Building RPM package..."
podman run --rm \
    -v "$PROJECT_ROOT:/build" \
    -v "/tmp/${TARBALL}:/tmp/${TARBALL}:ro" \
    -e VERSION="$VERSION_NUMBER" \
    -e TARBALL="/tmp/${TARBALL}" \
    -e GOARCH="$GOARCH_VAL" \
    -e RPM_ARCH="$RPM_ARCH" \
    "$IMAGE" \
    bash -c "
        cd /build
        mkdir -p ~/rpmbuild/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS}
        cp /tmp/${TARBALL} ~/rpmbuild/SOURCES/
        cp packaging/rpm/twindb-backup.spec ~/rpmbuild/SPECS/
        sed -i \"s/Version:.*/Version:        ${VERSION_NUMBER}/\" ~/rpmbuild/SPECS/twindb-backup.spec
        # Set target architecture for rpmbuild
        echo \"%define _target_cpu ${RPM_ARCH}\" >> ~/rpmbuild/SPECS/twindb-backup.spec
        export CGO_ENABLED=0
        export GOOS=linux
        export GOARCH=${GOARCH_VAL}
        # Use --nodeps to skip BuildRequires check since we install Go manually from upstream
        rpmbuild --nodeps -ba ~/rpmbuild/SPECS/twindb-backup.spec
        mkdir -p /build/packaging/dist
        cp ~/rpmbuild/RPMS/*/*.rpm /build/packaging/dist/ 2>/dev/null || true
        cp ~/rpmbuild/SRPMS/*.rpm /build/packaging/dist/ 2>/dev/null || true
    "

echo "RPM packages built in packaging/dist/"
