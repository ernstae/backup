# CLI

TwinDB Backup v5.0.0 uses a Cobra-based CLI with subcommands for backup, restore, status, and verification operations.

## Global Flags

- `--config`: Path to YAML config (default: `/etc/twindb/twindb-backup.yaml`)
- `--debug`: Enable verbose logging and command output (adds `--verbose` to xtrabackup/xbstream)
- `--log-file`: JSON log file path (structured logs for Splunk/ELK)

## Commands

### backup

Creates a backup for the specified interval (hourly, daily, weekly, monthly, or yearly).

```sh
twindb-backup backup <interval> [--binlogs-only]
```

**Arguments:**
- `interval`: `hourly` | `daily` | `weekly` | `monthly` | `yearly`

**Flags:**
- `--binlogs-only`: Skip full/incremental backup and only upload binary logs

**Examples:**

```sh
# Create a daily backup (full or incremental based on schedule)
twindb-backup backup daily --config /etc/twindb/twindb-backup.yaml

# Create only binlog backup (useful for frequent binlog uploads)
twindb-backup backup hourly --binlogs-only

# Weekly full backup
twindb-backup backup weekly
```

**What it does:**
1. Runs pre-flight checks (MySQL connectivity, destination write, disk space, binaries, GPG keys, configuration)
2. Prints current backup status (last backup age, type, GTID)
3. Acquires a lock (prevents concurrent runs)
4. Runs MySQL backup (if `source.backup_mysql: true`)
5. Compresses and encrypts the stream
6. Uploads to configured destination
7. Captures MySQL configuration files
8. Uploads binary logs
9. Updates status metadata
10. Applies retention policies

**Pre-flight Checks:**
Before starting a backup, the system automatically runs pre-flight checks to fail fast and avoid wasting resources:
- **MySQL connectivity**: Tests connection to MySQL server (if MySQL backups enabled)
- **Destination connectivity**: Tests write capability by writing and deleting a test file
- **Disk space**: Verifies sufficient space in `/tmp` for temporary files
- **Required binaries**: Validates that all required tools are installed and executable (xtrabackup, xbstream, mysql, etc.)
- **GPG key availability**: Verifies GPG recipient key exists in keyring (if encryption enabled)
- **Configuration validity**: Validates required configuration fields are present

If any pre-flight check fails, the backup is aborted immediately with a clear error message. This prevents backups from failing after consuming significant time and resources.

### health

Checks the health of the backup system including external tool dependencies, backup staleness, connectivity, and system resources.

```sh
twindb-backup health [--check-backups] [--check-destination] [--check-mysql] [--check-dependencies] [--warn-age interval=duration] [--json]
```

**Flags:**
- `--check-backups`: Check backup staleness only
- `--check-destination`: Check destination connectivity only
- `--check-mysql`: Check MySQL connectivity only
- `--check-dependencies`: Check external tool dependencies only
- `--warn-age`: Warn if backup is older than duration (e.g., `daily=25h`, `hourly=2h`)
- `--json`: Output results in JSON format

**Examples:**
```sh
# Check all health aspects
twindb-backup health

# Check only backup staleness with custom thresholds
twindb-backup health --check-backups --warn-age daily=25h --warn-age hourly=2h

# Check only dependencies
twindb-backup health --check-dependencies

# JSON output for monitoring systems
twindb-backup health --json
```

**What it checks:**
1. **External tool dependencies**: Validates that required binaries are installed and executable
   - `mysql`, `mysqlbinlog` (if MySQL backups enabled)
   - `xtrabackup`, `xbstream` (if MySQL backups enabled)
   - `xbcloud` (if using xbcloud destination)
   - `gpg` (if encryption enabled)
   - `gsutil` (if using GCS destination)
   - `ssh` (if using SSH destination)
2. **Backup staleness**: Checks age of last backup per interval
   - **Adaptive detection for hourly backups** (default): Uses historical backup durations to establish expected windows
     - Calculates average backup duration from last 10 backups
     - Expects next backup within max(average duration + 50% buffer, 1 hour interval)
     - Hard threshold of 12 hours (unacceptable if backup not completed)
     - Alerts if significantly outside expected window OR past hard threshold
     - **Can be overridden** with `--warn-age hourly=<duration>` to use fixed threshold instead
   - **Threshold-based detection** for other intervals (daily, weekly, monthly, yearly)
     - Uses `--warn-age` if provided, otherwise uses sensible defaults (26h for daily, 8d for weekly, etc.)
3. **MySQL connectivity**: Tests connection to MySQL server
4. **Destination connectivity**: Verifies destination is accessible
5. **Lock file status**: Checks if backup is in progress or stuck
6. **Disk space**: Monitors available space in `/tmp`

**Exit codes:**
- `0`: All checks passed (healthy or warnings only)
- `1`: One or more checks failed (unhealthy)

**Use cases:**
- Monitoring system health checks
- Pre-flight validation before backups
- Troubleshooting connectivity issues
- CI/CD pipeline validation
- Integration with alerting systems

**Adaptive Staleness Detection:**
For hourly backups, the health check uses adaptive detection that learns from historical backup patterns (unless overridden with `--warn-age hourly=<duration>`):
- **Large datasets**: If backups typically take 4-6 hours, the system adapts and won't alert after just 2 hours
- **Baseline establishment**: Requires at least 3 historical backups to establish a baseline
- **Hard threshold**: Always enforces a 12-hour maximum (unacceptable if backup not completed)
- **Variability handling**: Uses a 50% buffer on top of average duration to account for normal variation
- **Interval respect**: For fast backups (e.g., 10m avg), still respects the 1-hour interval schedule

Example: If your hourly backups average 5 hours, the system expects the next backup within 7.5 hours (5h + 50% buffer). If a backup is 8 hours old, it will warn. If it's 13 hours old, it will mark as unhealthy (past the 12-hour hard threshold).

**Overriding Adaptive Detection:**
You can override adaptive detection for hourly backups by providing a fixed threshold:
```sh
# Use fixed 2-hour threshold instead of adaptive detection
twindb-backup health --warn-age hourly=2h
```
This is useful when you want consistent alerting regardless of historical patterns.

---

### restore

Restores MySQL backups with optional Point-in-Time Recovery (PITR).

```sh
twindb-backup restore mysql --to-dir <dir> [--pitr-gtid <set> | --pitr-time <ts>] [--backup-id <id>]
```

**Flags:**
- `--to-dir`: Directory where backup will be restored
- `--pitr-gtid`: Recover to specific GTID set (e.g., `"uuid:1-123"`)
- `--pitr-time`: Recover to specific timestamp (e.g., `"2025-01-15 14:30:00"`)
- `--backup-id`: Specific backup to restore (defaults to latest)

**Examples:**

```sh
# Restore latest backup to /restore
twindb-backup restore mysql --to-dir /restore

# Restore specific backup
twindb-backup restore mysql --to-dir /restore --backup-id "mysql-2025-01-15_00_00_00.xbstream.gz"

# Restore to specific GTID
twindb-backup restore mysql --to-dir /restore --pitr-gtid "3E11FA47-71CA-11E1-9E33-C80AA9429562:1-12345"

# Restore to specific timestamp
twindb-backup restore mysql --to-dir /restore --pitr-time "2025-01-15 14:30:00"
```

**What it does:**
1. Loads backup metadata from status store
2. Downloads base backup (full)
3. Downloads and applies incrementals (if any)
4. Runs `xtrabackup --prepare`
5. Applies binlogs to reach PITR target (if specified)
6. Restores MySQL configuration files to `_config/` directory

### status

Displays backup status information.

```sh
twindb-backup status [--type mysql|binlog|files]
```

**Flags:**
- `--type`: Filter by backup type (default: all)

**Examples:**

```sh
# Show all status
twindb-backup status

# Show only MySQL backups
twindb-backup status --type mysql

# Show only binlog backups
twindb-backup status --type binlog
```

**Output format:**
- JSON status with backup metadata
- Includes GTID, LSN, timestamps, and file paths

### ls

Lists available backups.

```sh
twindb-backup ls [--type mysql|binlog|files]
```

**Flags:**
- `--type`: Filter by backup type (default: all)

**Examples:**

```sh
# List all backups
twindb-backup ls

# List only MySQL backups
twindb-backup ls --type mysql
```

**Output:**
- One backup per line
- Format: `<backup-id> <type> <interval> <timestamp>`

### verify

Verifies backup integrity by reading a sample of the backup file.

```sh
twindb-backup verify mysql <backup-id>
```

**Arguments:**
- `backup-id`: Backup identifier (e.g., `mysql-2025-01-15_00_00_00.xbstream.gz`)

**Examples:**

```sh
# Verify a specific backup
twindb-backup verify mysql "mysql-2025-01-15_00_00_00.xbstream.gz"
```

**What it does:**
1. Downloads backup from destination
2. Decompresses and decrypts (if needed)
3. Reads first `verify.depth_bytes` bytes
4. Verifies file is readable and not corrupted
5. Reports success or failure

### config

Configuration file utilities.

```sh
twindb-backup config validate [--config <path>]
twindb-backup config print [--config <path>]
```

**Subcommands:**
- `validate`: Validates YAML configuration file syntax and structure
- `print`: Prints the loaded configuration (useful for debugging)

**Examples:**

```sh
# Validate default config
twindb-backup config validate

# Validate custom config
twindb-backup config validate --config /path/to/config.yaml

# Print loaded config (for debugging)
twindb-backup config print
```

### summary

Displays a human-readable summary of MySQL backups.

```sh
twindb-backup summary
```

**Output:**
- Table format with backup name, type, interval, GTID, and finished time
- Highlights latest backup and latest full backup
- Sorted by finished time (most recent first)

**Example output:**
```
Backup Name                          Type        Interval  GTID              Finished
mysql-2025-01-15_14_00_00.xbstream   incremental hourly    uuid:1-12345      2025-01-15 14:00:00
mysql-2025-01-15_00_00_00.xbstream   full        daily     uuid:1-10000      2025-01-15 00:00:00
```

## Exit Codes

- `0`: Success
- `1`: General error
- `2`: Lock rejected (another process is waiting)
- `3`: Lock timeout
- `4`: Configuration error
- `5`: Backup/restore failure

## Common Workflows

### Daily Backup Schedule

```sh
# Full backup at midnight
twindb-backup backup daily

# Incremental backups every hour
twindb-backup backup hourly
```

### Disaster Recovery

```sh
# 1. List available backups
twindb-backup ls --type mysql

# 2. Verify backup integrity
twindb-backup verify mysql "mysql-2025-01-15_00_00_00.xbstream.gz"

# 3. Restore to latest point
twindb-backup restore mysql --to-dir /restore

# 4. Or restore to specific point in time
twindb-backup restore mysql --to-dir /restore --pitr-time "2025-01-15 14:30:00"
```

### Configuration Testing

```sh
# Validate configuration before running backups
twindb-backup config validate

# Check what configuration will be used
twindb-backup config print
```
