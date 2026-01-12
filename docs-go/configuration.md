# Configuration

TwinDB Backup v5.0.0 uses a YAML configuration file. Default path:
`/etc/twindb/twindb-backup.yaml` (override with `--config`).

## Example

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

worm:
  enabled: true
  mode: hybrid
  retention_lock: true
  status_log_compaction: true
  status_log_retention_days: 90
```

## Configuration Examples

### Minimal Configuration (S3)

```yaml
version: 1
paths:
  state_dir: /var/lib/twindb-backup
  lock_file: /var/lock/twindb-backup.lock

source:
  backup_mysql: true

mysql:
  defaults_file: /etc/mysql/my.cnf
  full_backup: daily

backup:
  intervals:
    daily: true

destination:
  kind: xbcloud
  provider: s3
  remote_path: /backups/mysql

s3:
  bucket: my-backups
  region: us-east-1
  access_key: ${AWS_ACCESS_KEY_ID}
  secret_key: ${AWS_SECRET_ACCESS_KEY}
```

### Production Configuration with Encryption

```yaml
version: 1
paths:
  state_dir: /var/lib/twindb-backup
  lock_file: /var/lock/twindb-backup.lock

source:
  backup_mysql: true

mysql:
  defaults_file: /etc/mysql/my.cnf
  full_backup: daily
  expire_log_days: 7
  capture_runtime_config: true
  skip_backup_locks: false

backup:
  intervals:
    hourly: true
    daily: true
    weekly: true
    monthly: true

compression:
  method: gzip
  level: 6
  threads: 4

encryption:
  gpg:
    enabled: true
    recipient: backup@example.com
    keyring: /root/.gnupg/pubring.kbx
    secret_keyring: /root/.gnupg/private-keys-v1.d
    binary: /usr/bin/gpg

destination:
  kind: xbcloud
  provider: s3
  remote_path: /backups/mysql
  keep_local_path: /var/backup/local

s3:
  bucket: production-backups
  region: us-west-2
  access_key: ${AWS_ACCESS_KEY_ID}
  secret_key: ${AWS_SECRET_ACCESS_KEY}

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

metrics:
  transport: datadog
  address: 127.0.0.1:8125
  prefix: twindb
  tags:
    - env:production
    - service:twindb-backup
  interval_seconds: 10

worm:
  enabled: true
  mode: hybrid
  retention_lock: true
```

### SSH Destination Configuration

```yaml
version: 1
paths:
  state_dir: /var/lib/twindb-backup
  lock_file: /var/lock/twindb-backup.lock

source:
  backup_mysql: true

mysql:
  defaults_file: /etc/mysql/my.cnf
  full_backup: daily

backup:
  intervals:
    daily: true

destination:
  kind: ssh
  remote_path: /backups/mysql

ssh:
  host: backup.example.com
  user: backup
  port: 22
  key_path: /root/.ssh/id_rsa
  strict_host_key_checking: false
  known_hosts_file: /root/.ssh/known_hosts
```

### Azure Blob Storage Configuration

```yaml
version: 1
paths:
  state_dir: /var/lib/twindb-backup
  lock_file: /var/lock/twindb-backup.lock

source:
  backup_mysql: true

mysql:
  defaults_file: /etc/mysql/my.cnf
  full_backup: daily

backup:
  intervals:
    daily: true

destination:
  kind: xbcloud
  provider: azure
  remote_path: /backups/mysql

azure:
  connection_string: ${AZURE_STORAGE_CONNECTION_STRING}
  container: twindb-backups
```

### Google Cloud Storage Configuration

```yaml
version: 1
paths:
  state_dir: /var/lib/twindb-backup
  lock_file: /var/lock/twindb-backup.lock

source:
  backup_mysql: true

mysql:
  defaults_file: /etc/mysql/my.cnf
  full_backup: daily

backup:
  intervals:
    daily: true

destination:
  kind: gcs
  remote_path: /backups/mysql

gcs:
  bucket: twindb-backups
  credentials_file: /etc/twindb/gcs.json
  gsutil_binary: gsutil
```

### File Backups Configuration

```yaml
version: 1
paths:
  state_dir: /var/lib/twindb-backup
  lock_file: /var/lock/twindb-backup.lock

source:
  backup_dirs:
    - /etc
    - /var/www
    - /opt/app
  tar_options: "--exclude='*.log'"
  backup_mysql: false

backup:
  intervals:
    daily: true

compression:
  method: gzip
  level: 6

destination:
  kind: xbcloud
  provider: s3
  remote_path: /backups/files

s3:
  bucket: my-backups
  region: us-east-1
  access_key: ${AWS_ACCESS_KEY_ID}
  secret_key: ${AWS_SECRET_ACCESS_KEY}
```

## Configuration Reference

### Paths

- `state_dir`: Directory for temporary state files (default: `/var/lib/twindb-backup`)
- `lock_file`: Path to lock file (default: `/var/lock/twindb-backup.lock`)

### Source

- `backup_dirs`: List of directories to backup (file backups)
- `tar_options`: Additional options passed to `tar` command
- `backup_mysql`: Enable MySQL backups (default: `true`)

### MySQL

- `defaults_file`: MySQL configuration file path (default: `/etc/mysql/my.cnf`)
- `mysql_binary`: Path to `mysql` binary
- `mysqlbinlog_binary`: Path to `mysqlbinlog` binary
- `xtrabackup_binary`: Path to `xtrabackup` binary
- `xbcloud_binary`: Path to `xbcloud` binary
- `xbstream_binary`: Path to `xbstream` binary
- `full_backup`: Interval for full backups (`hourly`|`daily`|`weekly`|`monthly`|`yearly`)
- `expire_log_days`: Days to keep binlogs (default: `7`)
- `capture_runtime_config`: Capture MySQL config files during backup (default: `true`)
- `skip_backup_locks`: Use `--no-backup-locks` with xtrabackup (default: `false`)
- `redo_log_archive_dir`: Enable redo log archiving (optional)

### Backup Intervals

- `hourly`: Enable hourly backups
- `daily`: Enable daily backups
- `weekly`: Enable weekly backups
- `monthly`: Enable monthly backups
- `yearly`: Enable yearly backups

### Compression

- `method`: Compression method (`gzip`|`bzip2`|`lbzip2`|`pigz`)
- `level`: Compression level (1-9, default: `6`)
- `threads`: Number of threads for parallel compression (default: `1`)

### Encryption

- `gpg.enabled`: Enable GPG encryption (default: `false`)
- `gpg.recipient`: GPG recipient email/key ID
- `gpg.keyring`: Path to GPG public keyring
- `gpg.secret_keyring`: Path to GPG secret keyring
- `gpg.binary`: Path to `gpg` binary

### Destination

- `kind`: Destination type (`xbcloud`|`ssh`|`gcs`|`local`)
- `provider`: Provider for xbcloud (`s3`|`azure`)
- `remote_path`: Remote path prefix for backups
- `keep_local_path`: Optional local copy path

### Retention

- `remote.hourly`: Number of hourly backups to keep
- `remote.daily`: Number of daily backups to keep
- `remote.weekly`: Number of weekly backups to keep
- `remote.monthly`: Number of monthly backups to keep
- `remote.yearly`: Number of yearly backups to keep
- `local.*`: Same structure for local retention

### Metrics

- `transport`: Metrics transport (`statsd`|`datadog`|`none`)
- `address`: StatsD/Datadog address
- `prefix`: Metric name prefix
- `tags`: List of tags (Datadog only)
- `interval_seconds`: Interval for span metrics (how often to emit `*_running_seconds` metrics)

See [Metrics Documentation](metrics.md) for comprehensive monitoring guidelines, suggested thresholds, and PromQL/AlertManager examples.

### Verify

- `depth_bytes`: Number of bytes to read for verification (default: `32768`)

### WORM

- `enabled`: Enable WORM mode (default: `true` when omitted)
- `mode`: WORM mode (`software`|`storage`|`hybrid`)
- `retention_lock`: Use storage-level retention when available
- `status_log_compaction`: Compact status logs periodically
- `status_log_retention_days`: Days to keep status logs (default: `90`)

## Notes

- `worm.enabled` defaults to `true` when omitted (secure by default).
- `strict_host_key_checking` can be set to `false` for automated SSH uploads.
- Paths and binary locations must match the runtime container/host.
- Environment variables can be used in configuration (e.g., `${AWS_ACCESS_KEY_ID}`).
- Binary paths default to standard locations if not specified.

## State And Status Metadata

- `paths.state_dir` stores temporary working files and local state.
- Status metadata is stored in the destination under
  `/<hostname>/status/` and includes snapshots plus append-only logs when
  WORM is enabled.
