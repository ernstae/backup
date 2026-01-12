# Best Practices

Recommended practices for using TwinDB Backup in production.

## Configuration

### Security

1. **Use encryption for sensitive data:**
   ```yaml
   encryption:
     gpg:
       enabled: true
       recipient: backup@example.com
   ```

2. **Store credentials securely:**
   - Use environment variables: `${AWS_ACCESS_KEY_ID}`
   - Use IAM roles when possible (S3/Azure)
   - Restrict file permissions: `chmod 600 /etc/twindb/twindb-backup.yaml`

3. **Enable WORM mode for compliance:**
   ```yaml
   worm:
     enabled: true
     retention_lock: true
   ```

### Backup Strategy

1. **Full backups daily:**
   ```yaml
   mysql:
     full_backup: daily
   backup:
     intervals:
       daily: true
   ```

2. **Incremental backups hourly:**
   ```yaml
   backup:
     intervals:
       hourly: true
   ```

3. **Binlog backups frequently:**
   - Run hourly binlog-only backups for minimal RPO
   - Set appropriate `expire_log_days` based on retention needs

4. **Retention policy:**
   ```yaml
   retention:
     remote:
       hourly: 24    # 1 day
       daily: 7      # 1 week
       weekly: 4     # 1 month
       monthly: 12   # 1 year
       yearly: 3     # 3 years
   ```

### Performance

1. **Use parallel compression:**
   ```yaml
   compression:
     method: pigz    # or lbzip2
     threads: 4
   ```

2. **Optimize compression level:**
   - Level 6 is a good balance (default)
   - Higher levels (7-9) use more CPU but save space
   - Lower levels (1-3) are faster but larger

3. **Keep local copy for fast restores:**
   ```yaml
   destination:
     keep_local_path: /var/backup/local
   ```

## Scheduling

### Systemd Timers

1. **Use systemd timers instead of cron:**
   - Prevents overlapping runs
   - Better logging and monitoring
   - Automatic retry on failure

2. **Add randomization to prevent thundering herd:**
   ```ini
   [Timer]
   OnCalendar=daily
   RandomizedDelaySec=300
   ```

3. **Separate timers for different intervals:**
   - `twindb-backup@hourly.timer` for binlogs
   - `twindb-backup@daily.timer` for full backups

### Backup Windows

1. **Schedule full backups during low-traffic periods:**
   - Typically midnight to 4 AM
   - Consider database load patterns

2. **Run binlog backups frequently:**
   - Every hour for minimal RPO
   - Can run more frequently if needed

## Monitoring

### Metrics

1. **Enable metrics for monitoring:**
   ```yaml
   metrics:
     transport: datadog
     address: 127.0.0.1:8125
     prefix: twindb
     tags:
       - env:production
   ```

2. **Monitor key metrics:**
   - `twindb.mysql.backup_time`: Backup duration
   - `twindb.mysql.backup_bytes`: Backup size
   - `twindb.mysql.binlog_backup_count`: Binlog count
   - `twindb.mysql.backup_running_seconds`: Detect stuck backups
   - `twindb.system.preflight_failure`: Pre-flight check failures (critical - indicates system readiness issues)

3. **Set up alerts:**
   - **Pre-flight check failures** - Alert immediately (any failure prevents backups from starting)
   - Backup failures (no completion in expected time)
   - Backup duration exceeding threshold (see [Metrics Documentation](metrics.md) for thresholds)
   - Missing backups (use adaptive staleness detection for hourly)
   - Stuck operations (running too long)

   See [Metrics Documentation](metrics.md) for comprehensive monitoring guidelines, suggested thresholds, and PromQL/AlertManager examples.

### Logging

1. **Use structured JSON logs:**
   ```sh
   twindb-backup backup daily --log-file /var/log/twindb-backup.json
   ```

2. **Rotate logs:**
   - Use logrotate or systemd journal
   - Keep logs for troubleshooting

3. **Monitor logs for errors:**
   ```sh
   journalctl -u twindb-backup@daily -f | grep -i error
   ```

## Storage

### Destination Selection

1. **Use xbcloud for S3/Azure:**
   - Efficient streaming
   - Native integration
   - Automatic retry

2. **Use SSH for simple setups:**
   - Good for on-premise
   - Simple configuration
   - No cloud dependencies

3. **Use GCS for Google Cloud:**
   - Native integration
   - Good performance
   - Cost-effective

### Storage Best Practices

1. **Use separate buckets/containers:**
   - Separate production and staging
   - Easier management
   - Better security

2. **Enable versioning (if supported):**
   - Additional protection
   - Recovery from accidental deletion
   - WORM mode provides immutability

3. **Monitor storage costs:**
   - Review retention policies
   - Consider lifecycle policies
   - Archive old backups if needed

## Disaster Recovery

### Backup Verification

1. **Regularly verify backups:**
   ```sh
   twindb-backup verify mysql "mysql-2025-01-15_00_00_00.xbstream.gz"
   ```

2. **Test restores:**
   - Monthly restore tests
   - Verify PITR capability
   - Document restore procedures

### Recovery Procedures

1. **Document restore steps:**
   - List available backups
   - Restore commands
   - PITR procedures
   - Post-restore verification

2. **Practice disaster recovery:**
   - Regular DR drills
   - Time recovery procedures
   - Identify bottlenecks

### Backup Retention

1. **Follow 3-2-1 rule:**
   - 3 copies of data
   - 2 different media types
   - 1 off-site copy

2. **Consider compliance requirements:**
   - GDPR: Right to deletion
   - SOX: 7-year retention
   - HIPAA: 6-year retention

## Maintenance

### Regular Tasks

1. **Review backup status:**
   ```sh
   twindb-backup summary
   ```

2. **Check disk space:**
   - Monitor local storage
   - Monitor remote storage
   - Clean up old backups if needed

3. **Update configuration:**
   - Review retention policies
   - Adjust backup schedules
   - Update credentials

### Updates

1. **Test updates in staging:**
   - Verify compatibility
   - Test restore procedures
   - Check for breaking changes

2. **Backup before updates:**
   - Always backup before upgrading
   - Verify backup integrity
   - Document rollback procedure

## Troubleshooting

1. **Keep logs:**
   - Enable debug logging for troubleshooting
   - Keep logs for analysis
   - Use structured logging

2. **Monitor metrics and health:**
   - Use `twindb-backup health` for operational status
   - Monitor metrics for quantitative data (duration, size, frequency)
   - Track backup duration trends
   - Monitor failure rates
   - Alert on anomalies using thresholds from [Metrics Documentation](metrics.md)

3. **Document issues:**
   - Keep runbook of common issues
   - Document solutions
   - Share knowledge with team

## Security Considerations

1. **Access control:**
   - Limit who can modify config
   - Use read-only access for backups
   - Implement least privilege

2. **Encryption:**
   - Encrypt backups at rest
   - Encrypt backups in transit
   - Use strong encryption keys

3. **Audit:**
   - Log all backup operations
   - Monitor access to backups
   - Review access logs regularly

## Performance Tuning

1. **Database:**
   - Optimize MySQL configuration
   - Use `skip_backup_locks` if appropriate
   - Monitor database performance during backups

2. **Network:**
   - Use dedicated network for backups
   - Consider bandwidth limits
   - Monitor network utilization

3. **Storage:**
   - Use fast storage for local copies
   - Optimize remote storage access
   - Consider storage tiering
