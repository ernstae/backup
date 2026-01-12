# TwinDB Backup (Go Rewrite)

This repository contains the Go rewrite of TwinDB Backup (v5.0.0). The legacy
Python implementation has been removed from this branch.

## Quick Start

Build the CLI:

```sh
CGO_ENABLED=0 GOOS=linux go build -o bin/twindb-backup ./cmd/twindb-backup
```

Run a backup:

```sh
twindb-backup backup daily --config /etc/twindb/twindb-backup.yaml
```

Health checks:

```sh
twindb-backup health
```

## Documentation

- MkDocs site: `docs-go/` with `mkdocs.yml`
- Build docs: `make docs-go`
- Live preview: `make docs-go-serve` (http://localhost:8000)

## Testing

```sh
GOCACHE=$(pwd)/.gocache go test ./...
./tests/integration/podman-suite.sh
```

## Packaging

Containerized RPM/DEB build scripts live in `packaging/`.

