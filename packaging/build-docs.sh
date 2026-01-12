#!/bin/bash
set -e

# Build MkDocs site for v5.0.0
# Usage: ./packaging/build-docs.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
IMAGE="twindb-backup-docs"

if ! podman image exists "$IMAGE" 2>/dev/null; then
    echo "Building container image $IMAGE..."
    podman build -f "$SCRIPT_DIR/docker/Dockerfile.docs" -t "$IMAGE" "$SCRIPT_DIR/docker"
fi

echo "Building MkDocs site..."
podman run --rm \
    -v "$PROJECT_ROOT:/build" \
    -w /build \
    "$IMAGE" \
    mkdocs build -f mkdocs.yml

echo "MkDocs site generated in site/"
