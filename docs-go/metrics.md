# Metrics

Metrics are emitted over StatsD or Datadog (DogStatsD) and include parity metrics from the legacy tool plus additional start/running metrics for real-time monitoring.

## Configuration

```yaml
metrics:
  transport: statsd   # statsd|datadog|none
  address: 127.0.0.1:8125
  prefix: twindb
  tags:
    - env:production
    - service:twindb-backup
    - cluster:mysql-primary
  interval_seconds: 10  # How often to emit running_seconds metrics
```

## Metric Reference

### Completion Metrics

These metrics are emitted once when an operation completes:

| Metric Name | Type | Unit | Description |
|------------|------|------|-------------|
| `twindb.mysql.backup_time` | Gauge | Seconds | Total time taken for MySQL backup (full or incremental) |
| `twindb.mysql.backup_bytes` | Gauge | Bytes | Total bytes backed up (compressed size) |
| `twindb.mysql.restore_time` | Gauge | Seconds | Total time taken for restore operation |
| `twindb.mysql.binlog_backup_time` | Gauge | Seconds | Time taken to backup binary logs |
| `twindb.mysql.binlog_backup_count` | Gauge | Count | Number of binary log files backed up |
| `twindb.mysql.binlog_backup_bytes` | Gauge | Bytes | Total bytes of binary logs backed up |
| `twindb.mysql.binlog_apply_time` | Gauge | Seconds | Time taken to apply binary logs during PITR |
| `twindb.mysql.binlog_apply_count` | Gauge | Count | Number of binary log events applied during PITR |
| `twindb.mysql.verify_time` | Gauge | Seconds | Time taken to verify backup integrity |
| `twindb.mysql.verify_bytes_read` | Gauge | Bytes | Bytes read during verification |
| `twindb.files.backup_time` | Gauge | Seconds | Total time taken for file/directory backup |
| `twindb.files.backup_bytes` | Gauge | Bytes | Total bytes backed up for files |
| `twindb.system.preflight_failure` | Gauge | Count | Pre-flight check failures (value: 1.0 per failure, sum to get count) |

### Span Metrics

These metrics are emitted at operation start and periodically while running:

| Metric Name | Type | Unit | Description |
|------------|------|------|-------------|
| `twindb.mysql.backup_start` | Gauge | Unix Timestamp | When backup operation started |
| `twindb.mysql.backup_running_seconds` | Gauge | Seconds | How long backup has been running (updated every `interval_seconds`) |
| `twindb.mysql.binlog_backup_start` | Gauge | Unix Timestamp | When binlog backup started |
| `twindb.mysql.binlog_backup_running_seconds` | Gauge | Seconds | How long binlog backup has been running |
| `twindb.mysql.restore_start` | Gauge | Unix Timestamp | When restore operation started |
| `twindb.mysql.restore_running_seconds` | Gauge | Seconds | How long restore has been running |
| `twindb.mysql.verify_start` | Gauge | Unix Timestamp | When verify operation started |
| `twindb.mysql.verify_running_seconds` | Gauge | Seconds | How long verify has been running |
| `twindb.files.backup_start` | Gauge | Unix Timestamp | When file backup started |
| `twindb.files.backup_running_seconds` | Gauge | Seconds | How long file backup has been running |

### Tags

Tags are attached to all metrics (Datadog/StatsD with tags support):

- `host:<hostname>` - Hostname where backup is running
- `interval:<interval>` - Backup interval (hourly, daily, weekly, monthly, yearly)
- `type:full|incremental` - Backup type
- `pitr:gtid|time|none` - PITR type (for restore operations)
- `backup:<backup-name>` - Backup identifier
- `component:<component>` - Component that failed (for preflight failures: `mysql_connectivity`, `destination_connectivity`, `disk_space`, `required_binaries`, `gpg_key`, `configuration_validity`)
- Custom tags from `metrics.tags` configuration

## Monitoring Guidelines

### Critical Metrics to Monitor

1. **Backup Duration** - How long backups take
2. **Backup Frequency** - How often backups complete
3. **Backup Size** - Size of backups (trends indicate data growth)
4. **Backup Failures** - Operations that don't complete
5. **Pre-flight Check Failures** - Pre-flight validation failures (indicates system readiness issues)
6. **Running Operations** - Long-running operations that may be stuck

### Suggested Production Thresholds

| Metric | Warning Threshold | Critical Threshold | Notes |
|--------|------------------|-------------------|-------|
| `mysql.backup_time` (hourly) | > 6 hours | > 12 hours | Large datasets may take 4-6 hours |
| `mysql.backup_time` (daily) | > 8 hours | > 16 hours | Full backups typically longer |
| `mysql.backup_time` (weekly) | > 12 hours | > 24 hours | Weekly full backups |
| `mysql.backup_running_seconds` | > 8 hours | > 12 hours | Stuck backup detection |
| `mysql.backup_bytes` | N/A | N/A | Monitor trends, not absolute values |
| `mysql.binlog_backup_time` | > 30 minutes | > 1 hour | Binlog backups should be fast |
| `mysql.restore_time` | > 2 hours | > 4 hours | Restore operations |
| `mysql.verify_time` | > 1 hour | > 2 hours | Verification operations |
| Backup frequency (hourly) | No backup in 8 hours | No backup in 12 hours | Use adaptive detection |
| Backup frequency (daily) | No backup in 26 hours | No backup in 30 hours | |
| Backup frequency (weekly) | No backup in 8 days | No backup in 10 days | |
| `system.preflight_failure` | Any failure | Any failure | Alert immediately on any pre-flight failure |

**Note:** Thresholds should be adjusted based on your dataset size, network bandwidth, and storage performance. Use the adaptive staleness detection for hourly backups (see Health Checks).

## Prometheus Integration

If you're using StatsD with Prometheus, you'll need a StatsD exporter to convert StatsD metrics to Prometheus format.

### StatsD Exporter Configuration

```yaml
# statsd-exporter mapping
mappings:
  - match: twindb.mysql.backup_time
    name: twindb_mysql_backup_time_seconds
    labels:
      category: mysql
      operation: backup
  - match: twindb.mysql.backup_bytes
    name: twindb_mysql_backup_bytes
    labels:
      category: mysql
      operation: backup
  - match: twindb.mysql.backup_running_seconds
    name: twindb_mysql_backup_running_seconds
    labels:
      category: mysql
      operation: backup
```

### PromQL Queries

#### Backup Duration (Average)

```promql
# Average backup duration over last 24 hours, grouped by interval
avg_over_time(twindb_mysql_backup_time_seconds[24h]) by (interval)
```

#### Backup Frequency

```promql
# Count of backups in last 24 hours, grouped by interval
count_over_time(twindb_mysql_backup_time_seconds[24h]) by (interval)
```

#### Currently Running Backups

```promql
# Backups that have been running for more than 1 hour
twindb_mysql_backup_running_seconds > 3600
```

#### Backup Size Trends

```promql
# Average backup size over last 7 days
avg_over_time(twindb_mysql_backup_bytes[7d]) by (interval, type)
```

#### Backup Success Rate

```promql
# Success rate: backups completed in last 24h / expected backups
count_over_time(twindb_mysql_backup_time_seconds[24h]) / 
  (24 * count(count_over_time(twindb_mysql_backup_time_seconds[24h]) by (interval)))
```

#### Long-Running Operations

```promql
# Operations running longer than expected
twindb_mysql_backup_running_seconds > 28800  # 8 hours
```

#### Pre-flight Check Failures

```promql
# Count of pre-flight failures in last hour, grouped by component
sum_over_time(twindb_system_preflight_failure[1h]) by (component, host)

# Total pre-flight failures in last 24 hours
sum(sum_over_time(twindb_system_preflight_failure[24h]))

# Pre-flight failures by component (table view)
sum_over_time(twindb_system_preflight_failure[1h]) by (component, host)

# Rate of pre-flight failures (failures per minute)
rate(twindb_system_preflight_failure[5m]) * 60
```

## AlertManager Rules

### Pre-flight Check Failure Alerts

Pre-flight check failures indicate system readiness issues that prevent backups from starting. These should be alerted immediately:

```yaml
# Alert on any pre-flight check failure
- alert: PreflightCheckFailure
  expr: sum(twindb_system_preflight_failure) > 0
  for: 0m
  labels:
    severity: critical
  annotations:
    summary: "Pre-flight check failed: {{ $labels.component }}"
    description: "Pre-flight check '{{ $labels.component }}' failed on {{ $labels.host }}. Backup will not start."

# Alert on specific component failures
- alert: MySQLConnectivityFailure
  expr: sum(twindb_system_preflight_failure{component="mysql_connectivity"}) > 0
  for: 0m
  labels:
    severity: critical
  annotations:
    summary: "MySQL connectivity check failed"
    description: "Cannot connect to MySQL server on {{ $labels.host }}"

- alert: DestinationWriteFailure
  expr: sum(twindb_system_preflight_failure{component="destination_connectivity"}) > 0
  for: 0m
  labels:
    severity: critical
  annotations:
    summary: "Destination write check failed"
    description: "Cannot write to backup destination on {{ $labels.host }}"

- alert: MissingRequiredBinaries
  expr: sum(twindb_system_preflight_failure{component="required_binaries"}) > 0
  for: 0m
  labels:
    severity: critical
  annotations:
    summary: "Required binaries missing"
    description: "One or more required binaries are missing or unavailable on {{ $labels.host }}"
```

**Note:** Pre-flight check failures are emitted as gauge metrics with value 1.0. Sum them over time to get a count of failures. The `component` tag identifies which check failed.

### Example AlertManager Configuration

```yaml
groups:
  - name: twindb_backup
    interval: 1m
    rules:
      # Critical: No hourly backup in 12 hours
      - alert: BackupStaleHourly
        expr: |
          (time() - max_over_time(twindb_mysql_backup_time_seconds[12h])) > 43200
        for: 15m
        labels:
          severity: critical
          component: backup
        annotations:
          summary: "Hourly backup is stale (no backup in 12 hours)"
          description: "Last hourly backup was {{ $value }} seconds ago"

      # Warning: No daily backup in 26 hours
      - alert: BackupStaleDaily
        expr: |
          (time() - max_over_time(twindb_mysql_backup_time_seconds{interval="daily"}[26h])) > 93600
        for: 15m
        labels:
          severity: warning
          component: backup
        annotations:
          summary: "Daily backup is stale (no backup in 26 hours)"
          description: "Last daily backup was {{ $value }} seconds ago"

      # Critical: Backup running too long
      - alert: BackupRunningTooLong
        expr: |
          twindb_mysql_backup_running_seconds > 43200  # 12 hours
        for: 5m
        labels:
          severity: critical
          component: backup
        annotations:
          summary: "Backup has been running for {{ $value }} seconds"
          description: "Backup on {{ $labels.host }} has been running for {{ $value | humanizeDuration }}"

      # Warning: Backup duration increasing
      - alert: BackupDurationIncreasing
        expr: |
          (
            avg_over_time(twindb_mysql_backup_time_seconds[1h]) /
            avg_over_time(twindb_mysql_backup_time_seconds[24h:1h])
          ) > 1.5
        for: 30m
        labels:
          severity: warning
          component: backup
        annotations:
          summary: "Backup duration is 50% longer than average"
          description: "Recent backups taking {{ $value | humanizeDuration }} vs average {{ $value | humanizeDuration }}"

      # Warning: Backup size significantly increased
      - alert: BackupSizeIncreased
        expr: |
          (
            avg_over_time(twindb_mysql_backup_bytes[1h]) /
            avg_over_time(twindb_mysql_backup_bytes[7d:1h])
          ) > 1.3
        for: 1h
        labels:
          severity: warning
          component: backup
        annotations:
          summary: "Backup size increased by 30%"
          description: "Recent backups are {{ $value }}x larger than 7-day average"

      # Critical: Binlog backup failed
      - alert: BinlogBackupFailed
        expr: |
          increase(twindb_mysql_binlog_backup_time_seconds[1h]) == 0
          and
          increase(twindb_mysql_backup_time_seconds[1h]) > 0
        for: 2h
        labels:
          severity: critical
          component: binlog
        annotations:
          summary: "Binlog backup not completing"
          description: "MySQL backups completing but binlog backups not running"

      # Warning: Restore operation taking too long
      - alert: RestoreRunningTooLong
        expr: |
          twindb_mysql_restore_running_seconds > 14400  # 4 hours
        for: 10m
        labels:
          severity: warning
          component: restore
        annotations:
          summary: "Restore operation running for {{ $value | humanizeDuration }}"
          description: "Restore on {{ $labels.host }} may be stuck"

      # Critical: No backups at all
      - alert: NoBackups
        expr: |
          time() - max_over_time(twindb_mysql_backup_time_seconds[48h]) > 172800
        for: 1h
        labels:
          severity: critical
          component: backup
        annotations:
          summary: "No backups completed in 48 hours"
          description: "Backup system appears to be completely down"
```

### PromQL for Grafana Dashboards

#### Backup Duration Over Time

```promql
# Line chart: Backup duration by interval
avg_over_time(twindb_mysql_backup_time_seconds[5m]) by (interval, type)
```

#### Backup Frequency

```promql
# Bar chart: Backups per day
count_over_time(twindb_mysql_backup_time_seconds[1d]) by (interval)
```

#### Backup Size Trends

```promql
# Area chart: Backup size over time
avg_over_time(twindb_mysql_backup_bytes[1h]) by (interval, type)
```

#### Backup Success Rate

```promql
# Gauge: Success rate percentage
(
  count_over_time(twindb_mysql_backup_time_seconds[24h]) /
  (24 * on(interval) group_left count(count_over_time(twindb_mysql_backup_time_seconds[24h]) by (interval)))
) * 100
```

#### Currently Running Operations

```promql
# Table: All running operations
twindb_mysql_backup_running_seconds > 0
```

## Best Practices

### 1. Monitor Backup Frequency

Use the `twindb-backup health` command or monitor backup completion metrics to ensure backups are running on schedule.

### 2. Track Backup Duration Trends

Monitor average backup duration over time. Sudden increases may indicate:
- Database growth
- Network issues
- Storage performance degradation
- Resource contention

### 3. Alert on Stuck Operations

Use `*_running_seconds` metrics to detect operations that have been running too long and may be stuck.

### 4. Monitor Backup Sizes

Track backup size trends to:
- Plan storage capacity
- Detect data growth anomalies
- Identify compression issues

### 5. Use Adaptive Thresholds

For hourly backups, use the adaptive staleness detection in the health check rather than fixed thresholds.

### 6. Combine Metrics with Health Checks

Use both metrics and the `twindb-backup health` command for comprehensive monitoring:
- Metrics: Quantitative data (duration, size, frequency)
- Health checks: Qualitative status (connectivity, dependencies, staleness)

### 7. Set Up Dashboards

Create Grafana dashboards showing:
- Backup duration trends
- Backup frequency
- Backup sizes
- Currently running operations
- Success rates
- Error rates

### 8. Monitor Restore Operations

Track restore metrics separately as they indicate disaster recovery readiness and performance.

## Integration Examples

### Datadog

Metrics are automatically sent to Datadog when using `transport: datadog`. Use Datadog's built-in alerting and dashboards.

### Prometheus + Grafana

1. Deploy StatsD exporter to convert StatsD metrics to Prometheus format
2. Configure Prometheus to scrape StatsD exporter
3. Create Grafana dashboards using PromQL queries above
4. Configure AlertManager rules as shown above

### CloudWatch

If using AWS, you can use CloudWatch StatsD agent to forward metrics to CloudWatch for monitoring and alerting.

## Troubleshooting

### Metrics Not Appearing

1. Check `metrics.transport` is not `none`
2. Verify `metrics.address` is correct
3. Ensure StatsD/Datadog agent is running and accessible
4. Check network connectivity to metrics endpoint
5. Verify metrics prefix matches your monitoring setup

### Metrics Appearing with Wrong Tags

1. Check `metrics.tags` configuration
2. Verify tag format (should be `key:value`)
3. For Datadog, ensure tags are properly formatted

### High Cardinality Issues

Avoid creating too many unique tag combinations. Stick to:
- `host`
- `interval`
- `type`
- Custom environment tags

Avoid adding dynamic values like backup names to tags unless necessary.
