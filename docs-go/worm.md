# WORM Storage

WORM (write-once/read-many) is enabled by default for status metadata and
backup artifacts. The destination wrapper enforces non-overwrite behavior
and can optionally use storage-level retention when supported.

## Configuration

```yaml
worm:
  enabled: true
  mode: hybrid           # software|storage|hybrid
  retention_lock: true   # enforce storage retention when available
  status_log_compaction: true
  status_log_retention_days: 90
```

## Behavior

- Existing objects are never overwritten.
- Status metadata is stored as append-only logs with optional compaction.
- When WORM is enabled, integration tests must use unique destination
  prefixes or disable WORM for test runs to avoid conflicts.
