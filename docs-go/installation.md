# Installation

## Go Version

TwinDB Backup v5.0.0 requires Go 1.23+ for building.

If you are building packages, the packaging Dockerfiles install Go 1.23+
from upstream tarballs for consistency across distributions.
Local builds should also use Go 1.23+ to avoid mismatched toolchains.

## Build From Source

```sh
CGO_ENABLED=0 GOOS=linux go build -o bin/twindb-backup ./cmd/twindb-backup
```

## Runtime Dependencies

- `xtrabackup` for hot backups
- `xbcloud` for S3/Azure uploads
- `xbstream` for streaming
- `mysqlbinlog` for PITR
- `gpg` if encryption is enabled

These binaries should be available in the container or host PATH that runs
`twindb-backup`.
