#!/bin/bash
set -e

# Serve MkDocs site for v5.0.0
# Usage: ./packaging/serve-docs.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
IMAGE="twindb-backup-docs"

if ! podman image exists "$IMAGE" 2>/dev/null; then
    echo "Building container image $IMAGE..."
    podman build -f "$SCRIPT_DIR/docker/Dockerfile.docs" -t "$IMAGE" "$SCRIPT_DIR/docker"
fi

echo "Starting MkDocs server on http://localhost:8000 ..."
podman run --rm \
    -p 8000:8000 \
    -v "$PROJECT_ROOT:/build" \
    -w /build \
    "$IMAGE" \
    mkdocs serve -f mkdocs.yml -a 0.0.0.0:8000
