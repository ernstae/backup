# WORM (Write Once, Read Many) Implementation Analysis

## Current State Analysis

### Files That Get Modified

1. **Status Files** (Modified on every backup):
   - `status/mysql.json` - Overwritten with updated backup list
   - `status/binlog.json` - Overwritten with updated binlog list
   - `status/files.json` - Overwritten with updated files list
   - `status/*.sha256` - Checksum files overwritten

2. **Backup Files** (Write-once, but deleted):
   - Backup artifacts (`.xbstream.gz`, etc.) - Written once ✅
   - Binlog files - Written once ✅
   - File backups - Written once ✅
   - **BUT**: Deleted by retention policy ❌

3. **Operations That Modify Files**:
   - `destination.Put()` - Can overwrite existing files
   - `destination.Delete()` - Used by retention policy
   - Status file saves - Overwrite entire file

---

## WORM Requirements

### Core Principles
1. **Write Once**: Files cannot be modified after creation
2. **Read Many**: Files can be read unlimited times
3. **Immutable**: Files cannot be deleted (or only after retention period expires)
4. **Append-Only**: Status/metadata must use append-only logs

---

## Implementation Challenges

### 1. Status Files (HIGH COMPLEXITY)

**Current Approach**: Single JSON file overwritten each backup
```go
// Current: Overwrites entire file
SaveMySQL(ctx, host, status) -> Put("status/mysql.json", newContent)
```

**WORM Approach**: Append-only log with snapshot/compaction
```go
// WORM: Append-only log
SaveMySQL(ctx, host, backup) -> Put("status/mysql-{timestamp}.log", backupEntry)
// Read: Load all logs and merge
LoadMySQL(ctx, host) -> Get all logs, merge, return latest state
```

**Complexity**: **MEDIUM-HIGH**
- Need log-based status storage
- Need log compaction/rotation strategy
- Need efficient log reading/merging
- Backward compatibility with existing JSON files

---

### 2. Backup File Immutability (LOW COMPLEXITY)

**Current Approach**: Files written once, but can be deleted
```go
// Current: Can delete old backups
dst.Delete(ctx, backup.Key)
```

**WORM Approach**: Prevent deletion or use retention locks
```go
// WORM: Check if file exists, fail if it does
if exists(key) {
    return ErrFileExists // WORM violation
}
// OR: Use storage-level retention locks (S3 Object Lock, etc.)
```

**Complexity**: **LOW-MEDIUM**
- Add existence check before `Put()`
- Disable `Delete()` when WORM enabled
- Use storage-level WORM features when available

---

### 3. Retention Policy (MEDIUM COMPLEXITY)

**Current Approach**: Deletes old backups based on retention rules
```go
// Current: Deletes old backups
applyMySQLRetention() -> dst.Delete(oldBackup.Key)
```

**WORM Approach**: Mark for deletion, don't actually delete
```go
// WORM: Mark as expired, but don't delete
// OR: Use storage-level retention periods
// OR: Disable retention when WORM enabled
```

**Complexity**: **MEDIUM**
- Need to disable or modify retention behavior
- May need storage-level retention support
- Status files still need to track expired backups

---

## Feasibility Assessment

### Overall Feasibility: **MEDIUM** (Moderate Complexity)

**Easy Parts** ✅:
- Backup files are already write-once
- Can add existence checks before writes
- Can disable Delete() operations

**Moderate Parts** ⚠️:
- Status file append-only log implementation
- Retention policy modifications
- Storage backend WORM support detection

**Hard Parts** ❌:
- Efficient log compaction/rotation
- Backward compatibility with existing status files
- Performance impact of log-based status

---

## Implementation Strategy

### Option A: Software-Level WORM (Recommended)

**Approach**: Implement WORM at application level
- Add `WORMEnabled` config flag
- Check file existence before `Put()` (fail if exists)
- Disable `Delete()` when WORM enabled
- Convert status files to append-only logs

**Pros**:
- ✅ Works with any storage backend
- ✅ Full control over WORM behavior
- ✅ Can be runtime option

**Cons**:
- ⚠️ Status file complexity
- ⚠️ Performance overhead (log reading)
- ⚠️ Storage still allows deletion (not true WORM)

---

### Option B: Storage-Level WORM (Best for Compliance)

**Approach**: Use storage backend WORM features
- S3 Object Lock (Governance/Compliance mode)
- GCS Object Retention/Holds
- Azure Blob Immutability Policies
- File system WORM features

**Pros**:
- ✅ True WORM at storage level
- ✅ Compliance-ready
- ✅ No application changes for backup files

**Cons**:
- ❌ Backend-specific implementation
- ❌ Status files still need app-level WORM
- ❌ Not all backends support WORM

---

### Option C: Hybrid Approach (Recommended)

**Approach**: Combine both strategies
- Use storage-level WORM when available
- Use app-level WORM for status files and unsupported backends
- Runtime option to enable/disable

**Pros**:
- ✅ Best of both worlds
- ✅ Works with all backends
- ✅ True WORM where supported

**Cons**:
- ⚠️ Most complex to implement
- ⚠️ Need backend capability detection

---

## Recommended Implementation Plan

### Phase 1: Basic WORM (Runtime Option) - **EASY**

1. Add `worm.enabled` config flag
2. Add existence check in `Put()` wrapper
3. Disable `Delete()` when WORM enabled
4. Document retention policy behavior

**Effort**: ~2-3 days
**Impact**: Prevents file modifications, but status files still overwrite

---

### Phase 2: Status File WORM - **MEDIUM**

1. Implement append-only log for status files
2. Add log compaction/rotation
3. Efficient log reading/merging
4. Backward compatibility with JSON files

**Effort**: ~1-2 weeks
**Impact**: True WORM for all files

---

### Phase 3: Storage-Level Integration - **MEDIUM-HARD**

1. Detect storage backend WORM capabilities
2. Use S3 Object Lock when available
3. Use GCS retention holds when available
4. Fall back to app-level WORM

**Effort**: ~2-3 weeks
**Impact**: Compliance-ready, true WORM

---

## Configuration Example

```yaml
worm:
  enabled: true
  mode: "software"  # software | storage | hybrid
  retention_lock: true  # Use storage-level retention when available
  status_log_compaction: true  # Compact status logs periodically
  status_log_retention_days: 90  # Keep status logs for 90 days
```

---

## Runtime Option Feasibility

**Answer**: **YES, FEASIBLE** ✅

**Complexity**: **MEDIUM** (2-4 weeks implementation)

**Key Considerations**:
1. Status files need log-based approach (biggest change)
2. Retention policy needs modification (disable deletes)
3. Storage backend support varies (may need backend-specific code)
4. Performance impact of log-based status (needs optimization)

**Recommendation**: Start with Phase 1 (basic WORM) as runtime option, then iterate based on requirements.
