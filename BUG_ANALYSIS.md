# Bug Analysis - Packaging and Code Review

## Issues Identified

### 1. ✅ FALSE: GOARCH Handling is Consistent

**Claim**: `build-rpm.sh` ignores GOARCH while `build-deb.sh` respects it.

**Reality**: Both scripts handle GOARCH identically:
- Both detect: `ARCH="${GOARCH:-$(go env GOARCH 2>/dev/null || echo 'amd64')}"`
- Both pass to container: `-e GOARCH="$ARCH"`
- Both export in build: `export GOARCH=${ARCH}`

**Status**: ✅ **NO BUG** - Both scripts are consistent.

---

### 2. ⚠️ VALID: MySQL Config Parsing String Matching Issues

**Issue**: `strings.Contains()` is used instead of checking if directives are at the start of the line or properly formatted.

**Problems**:
1. Line 97: `if strings.Contains(line, "!includedir")` - Matches anywhere in line
   - False positive: `# comment about !includedir` would match
   - False positive: `option=!includedir` would match
2. Line 116: `if strings.Contains(line, "!include") && !strings.Contains(line, "!includedir")` - Better but still uses Contains
   - False positive: `# !include directive` would match
   - False positive: `some_option=!include` would match

**Impact**: Could incorrectly parse config files with comments or values containing these strings.

**Fix Needed**: Use proper directive matching (check for `!include`/`!includedir` at start of line after trimming).

---

### 3. ⚠️ VALID: DEB Changelog Dirtying Git Tree

**Issue**: `build-deb.sh` modifies `packaging/deb/changelog` in the source tree (line 24-25) before copying to `debian/`.

**Problem**:
```bash
sed -i "s/^twindb-backup (.*)/twindb-backup (${VERSION_NUMBER}-1) unstable; urgency=medium/" \
    "$SCRIPT_DIR/deb/changelog"
```
This modifies the tracked file, potentially dirtying the git tree if run locally.

**Impact**: 
- Local builds could leave modified files
- CI builds are fine (clean checkout)
- Could cause confusion if developer runs build locally

**Fix Needed**: Only modify the copy in `debian/` directory, not the source file.

---

### 4. ⚠️ VALID: Possible Redundant Status Merging

**Issue**: In `internal/pipeline/backup.go`, the code:
1. Loads existing status (line 304)
2. Adds new backup to loaded status (line 311)
3. Saves status (line 312)

But if the backup already exists (same key), it might be added twice.

**Current Code**:
```go
localMySQLStatus, _ := localStore.LoadMySQL(ctx, host)
if localMySQLStatus != nil {
    localMySQLStatus.Add(backup)
    _ = localStore.SaveMySQL(ctx, host, localMySQLStatus)
}
```

**Problem**: `Add()` always appends, doesn't check for duplicates. If a backup with the same key already exists, it will be added again.

**Impact**: Could create duplicate entries in status file if backup is run multiple times with same key.

**Fix Needed**: Check if backup with same key exists before adding, or use a map-based deduplication.

---

## Summary

| Issue | Status | Severity | Fix Needed | Fixed |
|-------|--------|----------|------------|-------|
| GOARCH handling | ✅ FALSE | - | No fix needed | N/A |
| MySQL config parsing | ⚠️ VALID | Medium | Yes - improve directive matching | ✅ Fixed |
| DEB changelog git dirtying | ⚠️ VALID | Low | Yes - only modify copy | ✅ Fixed |
| Status redundant merging | ⚠️ VALID | Medium | Yes - deduplicate backups | ✅ Fixed |

## Fixes Applied

### 1. MySQL Config Parsing ✅
- Changed from `strings.Contains()` to `strings.HasPrefix()` for directive matching
- Now only matches `!include` and `!includedir` at the start of lines (after trimming)
- Prevents false positives from comments or option values containing these strings
- Added test case `TestDiscoverConfigFiles_IgnoreFalsePositives` to verify

### 2. DEB Changelog Git Dirtying ✅
- Removed modification of source `packaging/deb/changelog` file
- Now only modifies the copy in `debian/changelog` inside the container
- Source file remains untouched, preventing git tree dirtying

### 3. Status Redundant Merging ✅
- Updated `MySQLStatus.Add()`, `BinlogStatus.Add()`, and `FilesStatus.Add()` methods
- Now checks if backup with same key exists and replaces it instead of appending
- Prevents duplicate entries when same backup is added multiple times
- Added test case `TestMySQLStatus_Add_DeduplicatesByKey` to verify
