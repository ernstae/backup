# Findings Evaluation

## Issue Analysis

### 1. ⚠️ **VALID - HIGH**: RPM Architecture Metadata Mismatch

**Finding**: RPM builds emit packages with container's native arch metadata even when `GOARCH=arm64`, so arm64 packages will be mislabeled.

**Current State**:
- `rpmbuild` uses the container's native architecture (`%{_arch}`) for package metadata
- `GOARCH` is set correctly for Go compilation, but rpmbuild doesn't know about it
- Result: Building arm64 on amd64 container produces RPM labeled as `x86_64` with arm64 binary inside

**Impact**: **HIGH** - Packages won't install on correct architecture hosts

**Fix Required**:
- Map `GOARCH` to RPM architecture names (amd64 → x86_64, arm64 → aarch64)
- Set `%{_target_cpu}` macro or use `rpmbuild --target` if supported
- Alternative: Set `_target_cpu` macro in spec file based on GOARCH

**Status**: ⚠️ **VALID - NEEDS FIX**

---

### 2. ⚠️ **PARTIALLY VALID - MEDIUM**: Go Version Not Pinned

**Finding**: Build containers install distro golang packages which may be below 1.23.

**Current State**:
- RockyLinux 9: Has Go 1.25.3 (✅ Meets requirement)
- Ubuntu 24.04: Has Go 1.22.2 (❌ Below 1.23 requirement)

**Impact**: **MEDIUM** - Ubuntu builds will fail, RockyLinux is fine

**Fix Required**:
- Ubuntu: Install Go 1.23+ from upstream or use snap/golang.org
- RockyLinux: Already has 1.25.3, but should pin to ensure consistency
- Consider using official Go binary distribution for both

**Status**: ⚠️ **PARTIALLY VALID - Ubuntu needs fix, RockyLinux OK**

---

### 3. ⚠️ **VALID - LOW**: Context.Background() in DiscoverConfigFiles

**Finding**: `DiscoverConfigFiles` uses `context.Background()` for `client.Variables()`, so it can hang even if caller's context is cancelled.

**Current State**:
```go
func DiscoverConfigFiles(defaultsFile string, client *Client) (map[string]string, error) {
    // ...
    if client != nil {
        if vars, err := client.Variables(context.Background()); err == nil {
            // ...
        }
    }
}
```

**Impact**: **LOW** - Only affects MySQL variable query, not file discovery. But still violates context propagation best practices.

**Fix Required**:
- Add `context.Context` parameter to `DiscoverConfigFiles`
- Pass context to `client.Variables(ctx)`
- Update all call sites

**Status**: ⚠️ **VALID - SHOULD FIX**

---

## Summary

| Finding | Status | Severity | Action Required | Fixed |
|---------|--------|----------|-----------------|-------|
| RPM arch metadata | ⚠️ VALID | HIGH | Fix rpmbuild target arch | ✅ Fixed |
| Go version pinning | ⚠️ PARTIAL | MEDIUM | Fix Ubuntu, pin RockyLinux | ✅ Fixed |
| Context.Background() | ⚠️ VALID | LOW | Add context parameter | ✅ Fixed |

## Fixes Applied

### 1. RPM Architecture Metadata ✅
- **Fix**: Map `GOARCH` to RPM architecture names (amd64 → x86_64, arm64 → aarch64)
- **Implementation**: Set `%define _target_cpu ${RPM_ARCH}` in spec file before building
- **Files Updated**:
  - `packaging/build-rpm.sh` - Added architecture mapping and `_target_cpu` definition
  - `packaging/build-rpm-multiarch.sh` - Same fix for multi-arch script

### 2. Go Version Pinning ✅
- **Fix**: Install Go 1.23+ from official source when distro package is too old
- **Implementation**:
  - Ubuntu: Always install from upstream (distro has 1.22.2)
  - RockyLinux 8: Always install from upstream (may be old)
  - RockyLinux 9/10: Check distro version, use if >= 1.23, otherwise install from upstream
- **Files Updated**:
  - `packaging/docker/Dockerfile.ubuntu` - Install Go 1.23.0 from upstream
  - `packaging/docker/Dockerfile.rockylinux8` - Install Go 1.23.0 from upstream
  - `packaging/docker/Dockerfile.rockylinux9` - Check version, install if needed
  - `packaging/docker/Dockerfile.rockylinux10` - Check version, install if needed

### 3. Context Parameter ✅
- **Fix**: Add `context.Context` as first parameter to `DiscoverConfigFiles`
- **Implementation**: 
  - Updated function signature: `DiscoverConfigFiles(ctx context.Context, defaultsFile string, client *Client)`
  - Pass context to `client.Variables(ctx)` instead of `context.Background()`
  - Updated all call sites
- **Files Updated**:
  - `internal/mysql/config.go` - Added context parameter
  - `internal/pipeline/backup.go` - Pass context from caller
  - `internal/pipeline/config_integration_test.go` - Use `context.Background()` in tests
  - `internal/mysql/config_test.go` - Use `context.Background()` in tests
