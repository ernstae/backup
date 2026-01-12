#!/bin/bash
set -e

# Build RPM packages for multiple architectures
# Usage: ./packaging/build-rpm-multiarch.sh <rockylinux-version> [arch1 arch2 ...]
# Example: ./packaging/build-rpm-multiarch.sh 9 amd64 arm64

VERSION="${1:-9}"
ARCHES="${@:2}"
if [ -z "$ARCHES" ]; then
    ARCHES="amd64 arm64"
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

IMAGE="twindb-backup-builder-rockylinux${VERSION}"

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
TARBALL="twindb-backup-${VERSION_NUMBER}.tar.gz"
tar --exclude='.git' --exclude='*.pyc' --exclude='__pycache__' \
    --exclude='tests' --exclude='vagrant' --exclude='omnibus' \
    --exclude='.github' --exclude='node_modules' --exclude='packaging/dist' \
    --exclude='bin' --exclude='build' --exclude='dist' \
    -czf "/tmp/${TARBALL}" .

# Build for each architecture
for ARCH in $ARCHES; do
    echo "Building RPM package for $ARCH..."
    mkdir -p "$PROJECT_ROOT/packaging/dist"
    
    # Map GOARCH to RPM architecture names
    case "$ARCH" in
        amd64)
            RPM_ARCH="x86_64"
            ;;
        arm64)
            RPM_ARCH="aarch64"
            ;;
        *)
            RPM_ARCH="$ARCH"
            ;;
    esac
    
    podman run --rm \
        -v "$PROJECT_ROOT:/build" \
        -v "/tmp/${TARBALL}:/tmp/${TARBALL}:ro" \
        -e VERSION="$VERSION_NUMBER" \
        -e TARBALL="/tmp/${TARBALL}" \
        -e GOARCH="$ARCH" \
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
            export GOARCH=${ARCH}
            # Use --nodeps to skip BuildRequires check since we install Go manually from upstream
            rpmbuild --nodeps -ba ~/rpmbuild/SPECS/twindb-backup.spec
            mkdir -p /build/packaging/dist
            # Copy architecture-specific packages
            for pkg in ~/rpmbuild/RPMS/*/*.rpm; do
                if [ -f \"\$pkg\" ]; then
                    cp \"\$pkg\" /build/packaging/dist/
                fi
            done
            # Copy SRPM (source package, architecture-independent)
            cp ~/rpmbuild/SRPMS/*.rpm /build/packaging/dist/ 2>/dev/null || true
        "
done

echo "RPM packages built in packaging/dist/"
