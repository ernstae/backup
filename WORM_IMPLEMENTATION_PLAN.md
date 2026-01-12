# WORM Implementation Plan

## Summary

**Feasibility**: ✅ **YES - Feasible as runtime option**
**Complexity**: **MEDIUM** (2-4 weeks)
**Recommendation**: Implement in phases, starting with basic WORM

---

## Current File Modification Points

### 1. Status Files (Modified Every Backup)
- `status/mysql.json` - Overwritten with new backup list
- `status/binlog.json` - Overwritten with new binlog list  
- `status/files.json` - Overwritten with new files list
- `status/*.sha256` - Checksum files overwritten

### 2. Backup Files (Deleted by Retention)
- Backup artifacts deleted when retention expires
- Binlog files deleted when expired
- File backups deleted when expired

---

## Implementation Approach

### Phase 1: Basic WORM (Runtime Option) - **EASY** ⭐ Start Here

**Changes Needed**:
1. Add `worm.enabled` config flag
2. Create WORM wrapper for `Destination` interface
3. Check file existence before `Put()` (fail if exists)
4. Disable `Delete()` when WORM enabled
5. Document retention behavior (mark expired, don't delete)

**Files to Modify**:
- `internal/config/config.go` - Add WORM config
- `internal/destination/destination.go` - Add WORM wrapper
- `internal/pipeline/backup.go` - Skip retention deletes when WORM enabled
- `internal/pipeline/files.go` - Skip retention deletes when WORM enabled

**Effort**: 2-3 days
**Impact**: Prevents file modifications, but status files still overwrite

---

### Phase 2: Status File Append-Only Logs - **MEDIUM**

**Changes Needed**:
1. Implement append-only log format for status files
2. Change `SaveMySQL()` to append log entries instead of overwriting
3. Change `LoadMySQL()` to read and merge all log entries
4. Add log compaction/rotation mechanism
5. Maintain backward compatibility with existing JSON files

**Files to Modify**:
- `internal/status/status.go` - Log-based save/load
- `internal/status/log.go` - New log format and compaction

**Effort**: 1-2 weeks
**Impact**: True WORM for all files including status

---

### Phase 3: Storage-Level WORM Integration - **MEDIUM-HARD**

**Changes Needed**:
1. Detect storage backend WORM capabilities
2. Use S3 Object Lock (Governance/Compliance mode)
3. Use GCS Object Retention/Holds
4. Use Azure Blob Immutability Policies
5. Fall back to app-level WORM for unsupported backends

**Files to Modify**:
- `internal/destination/gcs/gcs.go` - GCS retention holds
- `internal/destination/s3/s3.go` - S3 Object Lock (if implemented)
- `internal/destination/xbcloud/xbcloud.go` - Check xbcloud WORM support

**Effort**: 2-3 weeks
**Impact**: Compliance-ready, true storage-level WORM

---

## Quick Start: Phase 1 Implementation

### 1. Add Config Flag

```yaml
worm:
  enabled: false  # Runtime option
  mode: "software"  # software | storage | hybrid
```

### 2. Create WORM Wrapper

```go
type WormDestination struct {
    dst destination.Destination
    enabled bool
}

func (w *WormDestination) Put(ctx context.Context, key string, r io.Reader) error {
    if w.enabled {
        // Check if file exists
        if _, err := w.dst.Get(ctx, key); err == nil {
            return ErrFileExists // WORM violation
        }
    }
    return w.dst.Put(ctx, key, r)
}

func (w *WormDestination) Delete(ctx context.Context, key string) error {
    if w.enabled {
        return ErrDeleteNotAllowed // WORM violation
    }
    return w.dst.Delete(ctx, key)
}
```

### 3. Modify Retention

```go
func applyMySQLRetention(cfg *config.Config, mysqlStatus *status.MySQLStatus, dst destination.Destination) {
    // ...
    if cfg.WORM.Enabled {
        // Don't delete, just mark as expired in status
        // OR: Use storage-level retention locks
        return
    }
    // Existing delete logic
}
```

---

## Status File Append-Only Log Format

### Current (Overwrites):
```
status/mysql.json
status/mysql.json.sha256
```

### WORM (Append-Only):
```
status/mysql-2024-01-01T00:00:00Z.log  # Backup entry
status/mysql-2024-01-02T00:00:00Z.log  # Backup entry
status/mysql-2024-01-03T00:00:00Z.log  # Backup entry
status/mysql.snapshot.json              # Periodic snapshot (for performance)
status/mysql.snapshot.json.sha256
```

**Log Entry Format**:
```json
{
  "timestamp": "2024-01-01T00:00:00Z",
  "action": "add",
  "backup": {
    "key": "...",
    "host": "...",
    ...
  }
}
```

**Reading**: Load snapshot + all logs after snapshot timestamp, merge

---

## Storage Backend WORM Support

| Backend | WORM Support | Implementation |
|---------|--------------|----------------|
| S3 (via xbcloud) | ✅ Object Lock | Use `--s3-object-lock` flag |
| GCS | ✅ Retention Holds | Use `gsutil retention` commands |
| Azure Blob | ✅ Immutability Policies | Use Azure SDK |
| Local FS | ❌ None | App-level WORM only |
| SSH | ❌ None | App-level WORM only |

---

## Retention Policy with WORM

### Option 1: Disable Retention (Simplest)
- When WORM enabled, don't delete any backups
- Status files track all backups (may grow large)
- Manual cleanup required

### Option 2: Storage-Level Retention
- Use storage backend retention features
- S3 Object Lock retention periods
- GCS retention holds with expiration
- App doesn't delete, storage enforces

### Option 3: Mark as Expired
- Mark backups as expired in status
- Don't actually delete files
- Status files filter expired backups
- Storage cleanup handled separately

---

## Performance Considerations

### Status File Logs
- **Reading**: O(n) where n = number of log entries
- **Mitigation**: Periodic snapshots reduce read cost
- **Compaction**: Merge logs into snapshot periodically

### File Existence Checks
- **Overhead**: One `Get()` call per `Put()`
- **Mitigation**: Cache existence checks, or rely on storage errors

---

## Testing Strategy

1. **Unit Tests**: WORM wrapper prevents overwrites/deletes
2. **Integration Tests**: Status file log reading/merging
3. **Backward Compatibility**: Load existing JSON status files
4. **Performance Tests**: Log-based status vs JSON status

---

## Migration Path

1. **Phase 1**: Enable basic WORM (no status file changes)
2. **Phase 2**: Migrate status files to logs (one-time migration)
3. **Phase 3**: Add storage-level WORM support

---

## Recommendation

**Start with Phase 1** as a runtime option:
- ✅ Quick to implement (2-3 days)
- ✅ Prevents backup file modifications
- ✅ Can be enabled/disabled per deployment
- ⚠️ Status files still overwrite (acceptable for MVP)

**Then evaluate Phase 2** based on requirements:
- If INFOSEC requires true WORM for status files
- If compliance requires append-only logs
- If audit trail is needed

**Phase 3** can be added later based on storage backend requirements.
