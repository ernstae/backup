# TwinDB Backup v5.0.0

This documentation covers TwinDB Backup version 5.0.0, the Go rewrite.

## Scope

Version 5.0.0 focuses on MySQL backups with xtrabackup/xbcloud, PITR
(binlog-based recovery), pluggable destinations, and systemd scheduling.
It also captures MySQL runtime configuration and stores status metadata
alongside backups.

## Requirements

- Go 1.23+ (required for builds)
- xtrabackup, xbcloud, xbstream, mysqlbinlog installed on the host
- systemd (for production scheduling)

## Quick Start

1. Build:

```sh
CGO_ENABLED=0 GOOS=linux go build -o bin/twindb-backup ./cmd/twindb-backup
```

2. Create a config file (YAML). See Configuration.
3. Run a backup:

```sh
twindb-backup backup daily --config /etc/twindb-backup.yaml
```

4. Restore with PITR:

```sh
twindb-backup restore mysql --to-dir /restore --pitr-gtid "uuid:1-123"
```

## Health Checks

Use the health command to validate connectivity, dependencies, and staleness:

```sh
twindb-backup health
twindb-backup health --check-backups --warn-age daily=25h --warn-age hourly=2h
```

## Documentation Map

- **Installation**: build, binaries, and dependencies
- **Configuration**: YAML schema, examples, and reference
- **CLI**: command reference, flags, and examples
- **Destinations**: xbcloud, ssh, gcs, local storage backends
- **WORM Storage**: write-once/read-many behavior and compliance
- **PITR Restore**: GTID/time restore workflows and examples
- **Metrics**: statsd/datadog integration and emitted metrics
- **Systemd Scheduling**: timers for non-overlap execution
- **Packaging**: RPM/DEB builds and release artifacts
- **Testing**: unit/integration tests and podman suite
- **Architecture**: component overview and data flow diagrams
- **Troubleshooting**: common issues and solutions
- **Best Practices**: production recommendations and guidelines
- **Authors**: project origins and Go rewrite attribution
