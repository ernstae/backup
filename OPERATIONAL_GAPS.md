# Operational Gaps Analysis

This document identifies gaps in backup functionality that would make the tool more useful for operators in production environments.

## 🔴 **CRITICAL** - Health & Resilience

### 1. Health Check Command ❌ **MISSING**

**Gap:** No health check command to verify system status.

**Impact:** Operators cannot easily verify:
- Backup system is functioning
- Last backup age/staleness
- Destination connectivity
- MySQL connectivity
- Configuration validity

**Proposed Solution:**
```sh
twindb-backup health [--check-backups] [--check-destination] [--check-mysql]
```

**Output should include:**
- Last backup timestamp per interval
- Backup age (hours/days since last backup)
- Destination connectivity status
- MySQL connectivity status
- Configuration validity
- Lock file status
- Disk space availability
- Exit code 0 = healthy, 1 = unhealthy

**Status:** ✅ **IMPLEMENTED** (CLI `health` command with JSON/text output and exit codes)

**Use Cases:**
- Monitoring system health checks
- Pre-flight checks before manual backups
- Troubleshooting connectivity issues
- CI/CD health checks

---

### 2. Backup Staleness Detection ❌ **MISSING**

**Gap:** No automatic detection of stale/missing backups.

**Impact:** Operators may not notice:
- Backups stopped running
- Backup failures went unnoticed
- Scheduled backups not executing

**Proposed Solution:**
- Add `health` command with staleness detection
- Emit metrics for backup age
- Alert when backup age exceeds threshold
- Check for expected backups per interval

**Example:**
```sh
twindb-backup health --warn-age daily=25h --warn-age hourly=2h
# Warns if daily backup is older than 25 hours
# Warns if hourly backup is older than 2 hours
```

**Status:** ✅ **IMPLEMENTED** (staleness checks in `health`, with adaptive hourly detection and hard thresholds)

---

### 3. Pre-flight Checks ❌ **MISSING**

**Gap:** No validation before starting backup.

**Impact:** Backups fail after consuming resources/time, wasting system resources and operator time.

**Proposed Solution:**
- Add pre-flight checks before backup starts:
  - MySQL connectivity (test connection with `SELECT 1`)
  - Destination connectivity (test write/read permissions)
  - Disk space availability (check local temp space and destination)
  - Required binaries present (xtrabackup, xbstream, xbcloud, gpg, etc.)
  - Configuration validity (validate YAML structure and required fields)
  - Lock file status (check if another backup is running)
  - GPG key availability (if encryption enabled)
- Fail fast with clear error messages
- Optionally integrate with `health` command for reusable checks

**Status:** ✅ **IMPLEMENTED** (pre-flight checks run automatically before each backup, reusing health check functions)

**Implementation:**
- Created `internal/pipeline/preflight.go` with check functions
- Checks run at start of `RunBackup()` before acquiring lock
- Returns early with descriptive errors if any check fails
- Reuses existing health check functions for consistency
- Checks include:
  - MySQL connectivity (if MySQL backups enabled)
  - Destination write capability (tests actual write/delete)
  - Disk space availability
  - Required binaries (xtrabackup, xbstream, mysql, etc.)
  - GPG key availability (if encryption enabled)
  - Configuration validity (required fields)

---

### 4. Retry Logic for External Tools ❌ **MISSING**

**Gap:** No retry/backoff for transient failures (already identified in CRITICAL_REVIEW.md).

**Impact:** Transient network errors cause hard failures, leading to unnecessary backup failures and operator intervention.

**Proposed Solution:**
- Add retry with exponential backoff for transient errors
- Configurable retry attempts and backoff per operation type
- Retry for: SSH operations, GCS operations, xbcloud uploads/downloads
- Distinguish transient vs permanent errors (network timeout vs authentication failure)
- Log retry attempts for observability
- Emit metrics for retry counts and success rates

**Configuration:**
```yaml
retry:
  enabled: true
  max_attempts: 3
  initial_backoff: 1s
  max_backoff: 30s
  backoff_multiplier: 2
  retryable_errors:
    - "network timeout"
    - "connection refused"
    - "temporary failure"
```

**Implementation:**
- Create `internal/retry/retry.go` with retry wrapper
- Wrap destination operations (Put, Get, Delete)
- Use context for cancellation during retries
- Consider jitter to prevent thundering herd

---

## 🟠 **HIGH PRIORITY** - Confidence & Visibility

### 5. Automatic Backup Verification ❌ **MISSING**

**Gap:** No automatic verification after backup completes.

**Impact:** Corrupted backups may go undetected until restore is needed.

**Proposed Solution:**
- Add config option: `verify.auto_verify: true`
- Automatically run `verify` after each backup
- Emit metrics for verification results
- Fail backup if verification fails (optional)

**Configuration:**
```yaml
verify:
  auto_verify: true
  auto_verify_fail_backup: false  # Don't fail backup if verify fails, just alert
  depth_bytes: 32768
```

---

### 6. Progress Reporting ❌ **MISSING**

**Gap:** No progress indicators during backup operations.

**Impact:** Operators cannot:
- Monitor backup progress
- Estimate completion time
- Identify stuck backups
- Provide status updates to stakeholders

**Proposed Solution:**
- Add progress reporting for long-running operations
- Show bytes transferred, percentage complete, ETA
- Use structured logging for progress events
- Support `--progress` flag for human-readable output

**Example Output:**
```
Backing up MySQL...
  Progress: 45.2% (2.3 GB / 5.1 GB)
  Speed: 125 MB/s
  ETA: 22 seconds
```

**Implementation:**
- Use `io.TeeReader` to count bytes
- Periodically emit progress metrics
- Support JSON progress output for automation

---

### 7. Backup Size Reporting ❌ **MISSING**

**Gap:** Limited visibility into backup sizes and storage usage. While backup metadata includes `Bytes` field, there's no command to aggregate and report on storage usage.

**Impact:** Operators cannot:
- Monitor storage growth over time
- Plan capacity requirements
- Identify unexpectedly large backups
- Track compression effectiveness
- Estimate storage costs

**Proposed Solution:**
- Add size information to `summary` command (show size per backup)
- Add `stats` command for aggregated storage usage
- Show compressed vs uncompressed sizes (if available)
- Track storage trends over time
- Support filtering by interval, date range, backup type

**Example:**
```sh
twindb-backup stats
# Shows:
# - Total backup size (all intervals)
# - Size by interval (hourly, daily, weekly, etc.)
# - Size by backup type (full vs incremental)
# - Compression ratio (if metadata available)
# - Storage usage trends (last 30 days)
# - Projected storage needs

twindb-backup stats --interval daily --last 7d
# Shows daily backup sizes for last 7 days
```

**Note:** Current backup metadata includes `Bytes` field in `MySQLBackup`, `BinlogBackup`, and `FileBackup` structs, so the data is available - just needs aggregation and reporting.

---

### 8. Test Command ❌ **MISSING**

**Gap:** No way to test connectivity and configuration without running a backup.

**Impact:** Operators must run full backup to test configuration, wasting time and resources. Configuration errors are discovered too late.

**Proposed Solution:**
```sh
twindb-backup test [--destination] [--mysql] [--gpg] [--binaries] [--all]
```

**Tests:**
- MySQL connectivity (test connection, verify credentials)
- Destination connectivity (S3, SSH, GCS, etc.) - test read/write permissions
- GPG key availability (verify recipient key exists in keyring)
- Binary availability (xtrabackup, xbcloud, xbstream, gpg, etc.) - check PATH
- Configuration validity (validate YAML structure, required fields)
- Disk space (check local temp space and destination)
- Permissions (check file permissions for config, lock file, etc.)
- Status store accessibility (test reading/writing status files)

**Output:**
- Exit code 0 = all tests passed
- Exit code 1 = one or more tests failed
- Detailed output per test (pass/fail with reason)
- JSON output option for automation

**Use Cases:**
- Pre-deployment validation
- Troubleshooting connectivity issues
- CI/CD pipeline validation
- Configuration change verification

---

## 🟡 **MEDIUM PRIORITY** - Alerting & Operations

### 9. Real-time Status Updates ❌ **MISSING**

**Gap:** No way to monitor backup progress in real-time.

**Impact:** Operators must wait for backup to complete or check logs.

**Proposed Solution:**
- Add `--follow` flag to status commands
- Stream progress updates
- Show live backup status
- Support JSON streaming for automation

---

### 10. Email Notifications ❌ **MISSING**

**Gap:** README mentions email notifications, but not implemented.

**Impact:** Operators must manually check for backup failures.

**Proposed Solution:**
```yaml
notifications:
  email:
    enabled: true
    smtp_server: smtp.example.com
    smtp_port: 587
    smtp_user: backup@example.com
    smtp_password: ${SMTP_PASSWORD}
    from: backup@example.com
    to:
      - ops@example.com
    on_failure: true
    on_success: false
    on_stale: true
```

**Events to notify:**
- Backup failure
- Backup success (optional)
- Stale backup detected
- Verification failure
- Destination connectivity failure

---

### 11. Webhook Notifications ❌ **MISSING**

**Gap:** No webhook support for integration with modern alerting systems.

**Impact:** Cannot integrate with PagerDuty, Slack, OpsGenie, etc.

**Proposed Solution:**
```yaml
notifications:
  webhook:
    enabled: true
    url: https://hooks.slack.com/services/...
    on_failure: true
    on_success: false
    headers:
      Authorization: Bearer ${WEBHOOK_TOKEN}
```

---

### 12. Alert on Missing Backups ❌ **MISSING**

**Gap:** No automatic alerting when expected backups don't occur.

**Impact:** Backup failures may go unnoticed for hours/days.

**Proposed Solution:**
- Health check command with alerting
- Metrics for backup age
- Integration with monitoring systems
- Configurable thresholds per interval

---

### 13. Cleanup Command ❌ **MISSING**

**Gap:** No manual cleanup command for old backups.

**Impact:** Operators must manually delete old backups or wait for retention.

**Proposed Solution:**
```sh
twindb-backup cleanup [--dry-run] [--older-than 30d] [--keep 10]
```

**Features:**
- Dry-run mode
- Cleanup by age
- Cleanup by count
- Respect WORM mode
- Cleanup local and remote separately

---

### 14. Repair Command ❌ **MISSING**

**Gap:** No way to repair corrupted status files.

**Impact:** Corrupted status files block backups.

**Proposed Solution:**
```sh
twindb-backup repair [--status-file] [--rebuild-from-logs]
```

**Features:**
- Rebuild status from logs
- Fix corrupted checksums
- Recover from partial writes
- Validate and fix status inconsistencies

---

### 15. Dry-run Mode ❌ **MISSING**

**Gap:** No way to simulate backup operations.

**Impact:** Operators cannot test configuration changes safely.

**Proposed Solution:**
```sh
twindb-backup backup daily --dry-run
```

**Features:**
- Simulate backup without actually running
- Show what would be backed up
- Validate configuration
- Test connectivity
- Estimate backup size/duration

---

### 16. Resume Capability ❌ **MISSING**

**Gap:** Failed backups cannot be resumed; must restart from beginning.

**Impact:** Large backups that fail near completion waste time and resources.

**Proposed Solution:**
- Detect partial backups
- Support resume from last checkpoint
- Store backup state for resume
- Clean up partial backups on success

**Note:** This is complex for streaming backups; may require checkpointing.

---

## 🟢 **LOW PRIORITY** - Enhanced Features

### 17. Backup Tagging/Labeling ❌ **MISSING**

**Gap:** No way to tag or label backups.

**Impact:** Cannot organize backups by purpose (pre-upgrade, migration, etc.).

**Proposed Solution:**
```sh
twindb-backup backup daily --tag "pre-upgrade" --tag "migration"
```

**Features:**
- Add tags to backups
- Filter backups by tags
- Search backups by tags
- Store tags in backup metadata

---

### 18. Backup Notes/Annotations ❌ **MISSING**

**Gap:** No way to add notes to backups.

**Impact:** Cannot document why a backup was taken or special circumstances.

**Proposed Solution:**
```sh
twindb-backup backup daily --note "Pre-migration backup"
```

**Features:**
- Add notes to backups
- View notes in status/summary
- Search backups by notes

---

### 19. Backup Search/Filtering ❌ **MISSING**

**Gap:** Limited filtering capabilities in `ls` and `summary` commands.

**Impact:** Hard to find specific backups in large environments.

**Proposed Solution:**
```sh
twindb-backup ls --type mysql --after "2025-01-01" --before "2025-01-15" --type full
twindb-backup summary --interval daily --limit 10
```

**Filters:**
- By date range
- By backup type (full/incremental)
- By interval
- By GTID range
- By size range
- By tags

---

### 20. Storage Usage Reporting ❌ **MISSING**

**Gap:** No comprehensive storage usage reporting.

**Impact:** Cannot monitor storage costs or plan capacity.

**Proposed Solution:**
```sh
twindb-backup storage [--by-interval] [--by-age] [--trends]
```

**Features:**
- Total storage usage
- Storage by interval
- Storage by age
- Storage trends over time
- Projected storage needs
- Cost estimation (if configured)

---

### 21. Backup Speed Metrics ❌ **MISSING**

**Gap:** Limited performance metrics.

**Impact:** Cannot identify performance bottlenecks.

**Proposed Solution:**
- Add backup speed metrics (MB/s)
- Add compression ratio metrics
- Add network throughput metrics
- Track performance trends
- Alert on performance degradation

**Metrics to add:**
- `twindb.mysql.backup_speed_mbps`
- `twindb.mysql.compression_ratio`
- `twindb.mysql.network_throughput_mbps`

---

### 22. RTO/RPO Metrics ❌ **MISSING**

**Gap:** No Recovery Time Objective (RTO) or Recovery Point Objective (RPO) metrics.

**Impact:** Cannot measure SLA compliance.

**Proposed Solution:**
- Calculate RPO from backup frequency
- Calculate RTO from restore test times
- Emit metrics for RTO/RPO
- Track RTO/RPO trends
- Alert on SLA violations

---

### 23. Backup Comparison ❌ **MISSING**

**Gap:** No way to compare backups.

**Impact:** Cannot identify differences between backups.

**Proposed Solution:**
```sh
twindb-backup compare <backup1> <backup2>
```

**Features:**
- Compare backup sizes
- Compare GTID ranges
- Compare timestamps
- Compare configurations
- Show differences

---

### 24. Backup Export/Import ❌ **MISSING**

**Gap:** No way to export/import backup metadata.

**Impact:** Cannot migrate backups between systems or share backup catalogs.

**Proposed Solution:**
```sh
twindb-backup export --output backup-catalog.json
twindb-backup import --input backup-catalog.json
```

**Features:**
- Export backup metadata
- Import backup metadata
- Merge backup catalogs
- Backup catalog backup/restore

---

### 25. Backup Scheduling Validation ❌ **MISSING**

**Gap:** No validation of backup schedule conflicts or gaps.

**Impact:** May have backup schedule issues (overlaps, gaps, etc.).

**Proposed Solution:**
- Validate backup intervals
- Check for schedule conflicts
- Identify backup gaps
- Recommend optimal schedules

---

## 📊 **Summary by Priority**

### Critical (Must Have)
1. Health check command (implemented)
2. Backup staleness detection (implemented)
3. Pre-flight checks (implemented)
4. Retry logic for external tools

### High Priority (Should Have)
5. Automatic backup verification
6. Progress reporting
7. Backup size reporting
8. Test command

### Medium Priority (Nice to Have)
9. Real-time status updates
10. Email notifications
11. Webhook notifications
12. Alert on missing backups
13. Cleanup command
14. Repair command
15. Dry-run mode
16. Resume capability

### Low Priority (Future Enhancements)
17. Backup tagging/labeling
18. Backup notes/annotations
19. Backup search/filtering
20. Storage usage reporting
21. Backup speed metrics
22. RTO/RPO metrics
23. Backup comparison
24. Backup export/import
25. Backup scheduling validation

---

## 🎯 **Recommended Implementation Order**

### Phase 1: Critical Operations (Immediate)
1. **Health check command** - Implemented
2. **Backup staleness detection** - Implemented (adaptive hourly + defaults)
3. **Pre-flight checks** - Implemented (gated before backup execution)
4. **Retry logic** - Handle transient failures

### Phase 2: Confidence & Visibility (Short-term)
5. **Automatic verification** - Detect corruption early
6. **Progress reporting** - Operator visibility
7. **Backup size reporting** - Capacity planning
8. **Test command** - Configuration validation

### Phase 3: Alerting & Operations (Short-term)
9. **Real-time status updates** - Operator feedback
10. **Email notifications** - Basic alerting
11. **Webhook notifications** - Modern integrations
12. **Alert on missing backups** - Proactive monitoring
13. **Cleanup command** - Operator control over storage
14. **Repair command** - Recover from status corruption

### Phase 4: Resilience (Medium-term)
15. **Dry-run mode** - Safe testing
16. **Resume capability** - Reduce wasted work on failures

### Phase 5: Enhanced Features (Long-term)
17. **Backup tagging** - Organization
18. **Storage reporting** - Cost management
19. **RTO/RPO metrics** - SLA tracking
20. **Backup comparison** - Differential insight
21. **Export/import metadata** - Catalog portability
22. **Scheduling validation** - Prevent gaps and conflicts

---

## 💡 **Additional Considerations**

### Integration Points
- **Prometheus metrics** - Native Prometheus exporter (not just StatsD/Datadog)
  - Current implementation only supports StatsD/Datadog
  - Prometheus is more common in Kubernetes environments
- **OpenTelemetry** - Modern observability standard
  - Would enable better distributed tracing
  - Better integration with modern observability stacks
- **Health check endpoints** - HTTP endpoint for health checks
  - Enable Kubernetes liveness/readiness probes
  - Enable load balancer health checks
  - RESTful health check API
- **REST API** - Programmatic access to backup operations
  - Enable automation and integration
  - Support for backup orchestration tools
  - API for backup status, triggering backups, etc.

### Operational Safety
- **Backup validation before deletion** - Verify backup exists and is accessible before applying retention
  - Current retention logic filters status but doesn't verify files exist
  - Important for WORM mode where files can't be recreated
- **Backup chain validation** - Ensure incremental chains are complete
  - Validate that all incremental backups have valid parent backups
  - Detect broken incremental chains
  - Warn on orphaned incremental backups
- **Binlog gap detection** - Detect missing binlogs in PITR range
  - Validate binlog continuity for PITR
  - Detect gaps in binlog sequence
  - Warn on incomplete binlog coverage
- **Backup integrity checks** - Periodic full verification
  - Schedule automatic verification of old backups
  - Detect bit rot or corruption over time
  - Alert on integrity failures

### Performance
- **Backup deduplication** - Identify duplicate backups
- **Incremental backup optimization** - Better incremental strategies
- **Parallel backup operations** - Backup multiple databases in parallel
- **Backup streaming optimization** - Optimize stream processing

### Compliance
- **Backup audit logging** - Log all backup operations
- **Backup access logging** - Track who accessed backups
- **Compliance reporting** - Generate compliance reports
- **Backup encryption at rest** - Ensure all backups are encrypted

---

## 🚫 **Intentionally Omitted Features**

### Clone Command ✅ **INTENTIONALLY OMITTED**

**Status:** Not implemented by design.

**Reason:** MySQL 8.0.17 (released July 2019) introduced the built-in [CLONE plugin](https://dev.mysql.com/doc/refman/8.0/en/clone-plugin.html) which provides native support for cloning MySQL instances. The plugin offers:
- Physical snapshot cloning of InnoDB data
- Local and remote cloning capabilities
- Native MySQL integration
- Better performance than external tools

**Impact:** Users of MySQL 8.0.17+ should use the native `CLONE` plugin instead:
```sql
INSTALL PLUGIN clone SONAME 'mysql_clone.so';
CLONE LOCAL DATA DIRECTORY = '/path/to/clone';
-- or
CLONE INSTANCE FROM 'user'@'host':port IDENTIFIED BY 'password' DATA DIRECTORY = '/path/to/clone';
```

**For older MySQL versions:** The Python version's `clone` command remains available for MySQL 5.7 and earlier versions that don't have the CLONE plugin.

---

## 📝 **Notes**

This analysis focuses on operational gaps that would improve the tool's usefulness for operators. Some items (like MariaDB support, Galera support) are already documented in `CURRENT_STATE_EVALUATION.md` and are not repeated here.

### Relationship Between Features

- **Health Check vs Pre-flight Checks**: The `health` command is for ongoing monitoring and status verification, while pre-flight checks run automatically before each backup to fail fast. Pre-flight checks could leverage health check functions for code reuse.

- **Test Command vs Pre-flight Checks**: The `test` command is for manual validation and troubleshooting, while pre-flight checks are automatic. Both should use the same underlying validation functions.

- **Progress Reporting vs Real-time Status**: Progress reporting shows detailed progress during a single operation, while real-time status shows overall system state across multiple operations.

### Existing Capabilities to Leverage

- **Metrics System**: The existing `internal/metrics` package could be extended to emit health check metrics, backup age metrics, and performance metrics.

- **Status Store**: The existing `internal/status` package already tracks backup metadata including timestamps, sizes, and types - this data can be used for staleness detection, size reporting, and storage analysis.

- **Lock System**: The existing `internal/lock` package could be extended to provide lock status information for health checks.

- **Configuration System**: The existing `internal/config` package already validates configuration structure - this could be extended for more comprehensive validation.

The priority order reflects typical operator needs:
1. **Health & Resilience** - Know if system is working and handle failures gracefully
2. **Confidence & Visibility** - Verify backups and see what's happening
3. **Alerting & Operations** - Get notified of issues and manage backups
4. **Resilience** - Advanced failure recovery capabilities
5. **Enhanced Features** - Advanced capabilities for power users
