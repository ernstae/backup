# Go Rewrite Plan

This document captures the initial design for a Go rewrite of TwinDB Backup.
It prioritizes MySQL backups, binlog-based PITR, xbcloud integration, and
pluggable storage backends, with feature parity as the long-term goal.

## Goals

- Keep feature parity with the Python tool, while focusing implementation on
  MySQL backups, binlogs, and PITR first.
- Use a modern Cobra-based CLI with a YAML config format.
- Prefer xbcloud for S3 and Azure, with a pluggable storage interface that
  permits a future SDK-based backend.
- Replace cron/anacron with systemd timers to avoid overlapping runs.

## Non-goals (initial phase)

- Full feature parity for file/directory backups (initially basic coverage only).
- SDK-based S3/Azure uploads (deferred; xbcloud is primary path).

## CLI Sketch

- `twindb-backup backup <interval> [--binlogs-only]`
- `twindb-backup restore mysql --to-dir <dir> [--pitr-gtid <set> | --pitr-time <ts>]`
- `twindb-backup status [--type mysql|binlog]`
- `twindb-backup ls [--type mysql|binlog]`
- `twindb-backup verify mysql <backup-id>`
- `twindb-backup config validate|print`

## YAML Config (Draft)

```yaml
version: 1
paths:
  state_dir: /var/lib/twindb-backup
  lock_file: /var/lock/twindb-backup.lock

source:
  backup_dirs:
    - /etc
    - /var/www
  tar_options: ""
  backup_mysql: true

mysql:
  defaults_file: /etc/mysql/my.cnf
  mysql_binary: /usr/bin/mysql
  mysqlbinlog_binary: /usr/bin/mysqlbinlog
  xtrabackup_binary: /usr/bin/xtrabackup
  xbcloud_binary: /usr/bin/xbcloud
  xbstream_binary: /usr/bin/xbstream
  full_backup: daily
  expire_log_days: 7
  capture_runtime_config: true
  skip_backup_locks: false
  redo_log_archive_dir: ""

backup:
  intervals:
    hourly: true
    daily: true
    weekly: true
    monthly: true
    yearly: true

compression:
  method: gzip
  level: 6
  threads: 4

encryption:
  gpg:
    enabled: false
    recipient: backupuser@domain.com
    keyring: /root/.gnupg/pubring.kbx
    secret_keyring: /root/.gnupg/private-keys-v1.d
    binary: /usr/bin/gpg

destination:
  kind: xbcloud
  provider: s3
  remote_path: /backups/mysql
  keep_local_path: /var/backup/local

s3:
  bucket: twindb-backups
  region: us-east-1
  access_key: ${AWS_ACCESS_KEY_ID}
  secret_key: ${AWS_SECRET_ACCESS_KEY}
  endpoint: ""

azure:
  connection_string: ${AZURE_STORAGE_CONNECTION_STRING}
  container: twindb-backups

gcs:
  bucket: twindb-backups
  credentials_file: /etc/twindb/gcs.json
  gsutil_binary: gsutil

ssh:
  host: backup.example.com
  user: backup
  port: 22
  key_path: /root/.ssh/id_rsa
  strict_host_key_checking: true
  known_hosts_file: /root/.ssh/known_hosts

retention:
  remote:
    hourly: 24
    daily: 7
    weekly: 4
    monthly: 12
    yearly: 3
  local:
    hourly: 1
    daily: 1
    weekly: 0
    monthly: 0
    yearly: 0

metrics:
  transport: statsd   # statsd|datadog|none
  address: 127.0.0.1:8125
  prefix: twindb
  tags:
    - env:dev
    - service:twindb-backup
  interval_seconds: 10

verify:
  depth_bytes: 32768
```

## Logging

- Global flags: `--debug` (enables verbose command logging) and `--log-file` (JSON log file).

## Metrics

Completion metrics (parity with Python tool):
- `twindb.mysql.backup_time`
- `twindb.mysql.restore_time`
- `twindb.files.backup_time`

Additional metrics:
- `twindb.mysql.binlog_backup_time`
- `twindb.mysql.verify_time`
- `twindb.mysql.binlog_backup_count`
- `twindb.mysql.binlog_backup_bytes`
- `twindb.mysql.verify_bytes_read`
- `twindb.mysql.backup_bytes`
- `twindb.mysql.binlog_apply_time`
- `twindb.mysql.binlog_apply_count`
- `twindb.files.backup_bytes`

Span metrics (emitted at start and periodically while running):
- `twindb.mysql.backup_start`
- `twindb.mysql.backup_running_seconds`
- `twindb.mysql.binlog_backup_start`
- `twindb.mysql.binlog_backup_running_seconds`
- `twindb.mysql.restore_start`
- `twindb.mysql.restore_running_seconds`
- `twindb.mysql.verify_start`
- `twindb.mysql.verify_running_seconds`
- `twindb.files.backup_start`
- `twindb.files.backup_running_seconds`

Tags added at emission time (DogStatsD only):
- `host:<hostname>`
- `interval:<interval>` for backup/binlog
- `type:full|incremental` for mysql backup
- `pitr:gtid|time|none` for restore
- `backup:<backup-name>` for verify

Status integrity:
- Status files are accompanied by `.sha256` sidecar files using SHA-256 (FIPS 180-4 compliant) checksums.
```

## Architecture Overview

- `cmd/twindb-backup`: Cobra CLI.
- `internal/config`: YAML parser and validation.
- `internal/lock`: file-based locking with queue support (one active, one waiting, rest rejected).
- `internal/pipeline`: streaming backup pipeline (sources -> modifiers -> destinations).
- `internal/destination`: pluggable storage interface, xbcloud backend first.
- `internal/status`: metadata store for backups and binlogs (including GTID).
- `internal/retention`: remote/local retention policy enforcement.
- `internal/restore`: applies full/incremental + binlogs to reach PITR targets.

## PITR (GTID/time)

- Capture `gtid_executed` and binlog coordinates at backup time.
- Store metadata in a status file in the destination (e.g. `status/mysql.json`).
- Restore uses `mysqlbinlog` with GTID/time filters to apply binlogs to the
  desired point in time.

## Status and Metadata Storage

- Default: status files live in the destination (blob storage), not on local disk.
- Local state only includes transient lock files and temporary working data.

## Lock File Handling

The Go implementation includes improved lock file handling that prevents concurrent backup runs and resource waste:

- **One active process**: The first process acquires the lock and runs the backup.
- **One waiting process**: If the lock is held, the next process becomes the waiting process and waits with a timeout.
- **Additional processes rejected**: Any further processes are rejected immediately with a clear error message.

### Implementation Details

- Uses `syscall.Flock()` for file locking (similar to Python's `fcntl.flock()`).
- Uses a `.waiting` file to track the waiting process (atomic creation with `O_EXCL`).
- Timeout calculation matches Python's behavior (half of interval duration):
  - Hourly: 30 minutes
  - Daily: 12 hours
  - Weekly: 84 hours
  - Monthly: 360 hours
  - Yearly: 4380 hours
- Automatic cleanup of waiting file when lock is acquired or timeout occurs.
- Lock file path is configurable via `paths.lock_file` (defaults to `/var/lock/twindb-backup.lock`).

### Benefits

- **Prevents resource waste**: Only one process waits, not many (improvement over Python version).
- **Clear feedback**: Rejected processes get immediate error messages.
- **Safe**: Uses file locking to prevent race conditions.
- **Automatic cleanup**: Waiting file is removed when lock is acquired or timeout occurs.

## Resource Management

### Open Files Limit
- Automatically sets open file descriptor limit to at least 150,000 at backup startup
- Uses system maximum if available
- Prevents "too many open files" errors during large database backups
- Gracefully handles systems with lower limits or insufficient permissions

### XtraBackup Memory Usage
- Automatically calculates available system memory (half of available, matching Python behavior)
- Passes `--use-memory=<bytes>` to xtrabackup prepare operations
- Optimizes memory usage for backup preparation
- Prevents failures on low-memory systems
- Reads from `/proc/meminfo` on Linux systems

## Configuration File Management

### Config File Capture
- Automatically discovers all MySQL configuration files during backup
- Recursively follows `!include` and `!includedir` directives
- Handles nested includes and prevents circular references
- Starts from `defaults-file` and common paths (`/etc/my.cnf`, `/etc/mysql/my.cnf`)
- Optionally queries MySQL runtime variables for additional config file paths
- Stores all discovered config files in backup status

### Config File Restoration
- Automatically restores configuration files during restore
- Files are restored to `_config/` directory in the restore destination
- Preserves original directory structure (e.g., `/etc/my.cnf.d/server.cnf` -> `_config/etc/my.cnf.d/server.cnf`)
- All config files from the backup are available for reference after restore

## systemd Scheduling

- Provide `twindb-backup@.service` and timer templates.
- Lock file handling prevents overlap automatically (no need for external `flock` in systemd).
- Add `RandomizedDelaySec` to smooth load.

## Packaging

- Primary target: RPM.
- Secondary target: DEB if demand warrants.
