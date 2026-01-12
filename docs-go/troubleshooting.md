# Troubleshooting

Common issues and solutions for TwinDB Backup.

## Lock Issues

### "backup rejected: another process is already waiting for the lock"

**Problem:** A backup is already running, and another process is waiting. A third process tried to start.

**Solution:**
- Wait for the current backup to complete
- Check running processes: `ps aux | grep twindb-backup`
- Verify lock file: `ls -l /var/lock/twindb-backup.lock`
- If stuck, manually remove lock file (only if no backup is running)

### "backup timeout: could not acquire lock within timeout period"

**Problem:** The waiting process timed out waiting for the lock.

**Solution:**
- Check if the active backup process is hung: `ps aux | grep xtrabackup`
- Review logs for errors: `journalctl -u twindb-backup@daily`
- Increase timeout (modify lock timeout calculation if needed)
- Kill hung processes if necessary: `pkill -9 xtrabackup`

## Pre-flight Check Failures

### "pre-flight checks failed"

**Problem:** One or more pre-flight checks failed before the backup started.

**Common causes:**
- MySQL connectivity issues
- Destination write permissions
- Missing required binaries
- Insufficient disk space
- GPG key not found (if encryption enabled)
- Configuration errors

**Solution:**
1. Review the error message to identify which check failed
2. Run `twindb-backup health` to diagnose issues
3. Fix the underlying problem (connectivity, permissions, missing tools, etc.)
4. Re-run the backup

**Example:**
```sh
# Pre-flight check failure
twindb-backup backup daily
# Error: pre-flight checks failed:
#   MySQL connectivity: MySQL connection failed: connection refused
#   Required binaries: binary_xtrabackup: Binary xtrabackup not found in PATH

# Diagnose with health check
twindb-backup health --check-mysql --check-dependencies

# Fix issues and retry
```

## Backup Failures

### "too many open files"

**Problem:** System hit the open file descriptor limit.

**Solution:**
- The backup automatically sets the limit to 150,000, but if it fails:
- Check current limit: `ulimit -n`
- Increase system limit: `echo "* soft nofile 150000" >> /etc/security/limits.conf`
- Restart the service

### "xtrabackup: error: cannot connect to MySQL server"

**Problem:** Cannot connect to MySQL.

**Solution:**
- Verify MySQL is running: `systemctl status mysql`
- Check `defaults_file` path in config
- Verify MySQL credentials in `defaults_file`
- Test connection: `mysql --defaults-file=/etc/mysql/my.cnf -e "SELECT 1"`

### "xbcloud: error: failed to upload"

**Problem:** Upload to S3/Azure failed.

**Solution:**
- Verify credentials: `aws s3 ls` or `az storage blob list`
- Check network connectivity
- Verify bucket/container exists
- Check IAM permissions for S3
- Review xbcloud logs for detailed error

### "gpg: error: no valid OpenPGP data found"

**Problem:** GPG encryption failed.

**Solution:**
- Verify GPG keyring exists: `gpg --list-keys`
- Check recipient key is in keyring
- Verify `secret_keyring` path is correct
- Test GPG manually: `echo "test" | gpg --encrypt --recipient backup@example.com`

## Restore Issues

### "backup not found"

**Problem:** Backup ID not found in status.

**Solution:**
- List available backups: `twindb-backup ls --type mysql`
- Verify backup exists in destination storage
- Check status store is accessible
- Verify hostname matches backup hostname

### "PITR failed: binlog not found"

**Problem:** Required binlog for PITR not found.

**Solution:**
- Verify binlogs are being backed up: `twindb-backup ls --type binlog`
- Check `expire_log_days` setting (binlogs may have expired)
- Verify binlog backup is running: `twindb-backup backup hourly --binlogs-only`
- Check binlog retention matches PITR requirements

### "xtrabackup --prepare failed"

**Problem:** Backup preparation failed.

**Solution:**
- Check disk space: `df -h`
- Verify backup files are complete (not corrupted)
- Check memory: `free -h` (may need to reduce `--use-memory`)
- Review xtrabackup logs for detailed error
- Try restoring from a different backup

## Configuration Issues

### "config path is required"

**Problem:** Config file not found.

**Solution:**
- Specify config explicitly: `--config /path/to/config.yaml`
- Verify default path exists: `/etc/twindb/twindb-backup.yaml`
- Check file permissions: `ls -l /etc/twindb/twindb-backup.yaml`

### "unsupported destination kind"

**Problem:** Invalid destination type.

**Solution:**
- Verify `destination.kind` is one of: `xbcloud`, `ssh`, `gcs`, `local`
- Check spelling and case sensitivity
- Review configuration file syntax

### "YAML syntax error"

**Problem:** Configuration file has syntax errors.

**Solution:**
- Validate config: `twindb-backup config validate`
- Check YAML indentation (must use spaces, not tabs)
- Verify all required fields are present
- Use a YAML validator online

## Performance Issues

### Backups are slow

**Problem:** Backup takes too long.

**Solution:**
- Enable parallel compression: `compression.threads: 4`
- Use faster compression: `compression.method: pigz` (parallel gzip)
- Check network bandwidth to destination
- Verify MySQL server performance
- Consider using `skip_backup_locks` if appropriate

### High memory usage

**Problem:** Backup uses too much memory.

**Solution:**
- The system automatically calculates memory for xtrabackup
- Check available memory: `free -h`
- Reduce compression threads if memory constrained
- Consider running backups during off-peak hours

## WORM Issues

### "file already exists (WORM: write once, read many)"

**Problem:** Trying to overwrite a file in WORM mode.

**Solution:**
- This is expected behavior in WORM mode
- Use unique backup names (automatic with timestamps)
- For testing, disable WORM: `worm.enabled: false`
- Verify backup names are unique (check for clock skew)

### Status snapshots not updating

**Problem:** Status snapshots appear stale.

**Solution:**
- Snapshots are periodic summaries, not real-time
- Check append-only logs for latest data
- Verify `status_log_compaction` is enabled
- Snapshots are overwritable (not WORM-protected)

## Logging and Debugging

### Enable debug logging

```sh
twindb-backup backup daily --debug --log-file /var/log/twindb-backup.json
```

### View systemd logs

```sh
# Service logs
journalctl -u twindb-backup@daily -f

# Timer logs
journalctl -u twindb-backup@daily.timer -f
```

### Check status store

```sh
# List backups
twindb-backup ls --type mysql

# View status
twindb-backup status --type mysql

# Summary
twindb-backup summary
```

## Common Error Messages

| Error | Cause | Solution |
|-------|-------|----------|
| `lock rejected` | Too many processes | Wait for backup to complete |
| `lock timeout` | Backup hung | Check for hung processes |
| `too many open files` | System limit | Increase ulimit |
| `cannot connect to MySQL` | MySQL down/wrong config | Check MySQL status and config |
| `xbcloud failed` | Credentials/network | Verify S3/Azure access |
| `backup not found` | Wrong backup ID | List backups with `ls` |
| `PITR binlog not found` | Binlog expired/missing | Check binlog backups |
| `WORM file exists` | Overwrite attempt | Use unique names or disable WORM |
