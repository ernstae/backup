# Build Dependency Analysis & Recommendations

## Issues Identified

### 1. ⚠️ **VALID - HIGH**: DEB Build-Depends Mismatch

**Problem**: 
- `packaging/deb/control` declares `Build-Depends: golang-go (>= 1.23)`
- `Dockerfile.ubuntu` installs Go from upstream tarball (not `golang-go` package)
- `dpkg-checkbuilddeps` will fail because `golang-go` package is not installed

**Impact**: **HIGH** - DEB builds will fail dependency checks

---

### 2. ⚠️ **VALID - HIGH**: RPM BuildRequires Mismatch

**Problem**:
- `packaging/rpm/twindb-backup.spec` declares `BuildRequires: golang >= 1.23`
- Dockerfiles install Go from upstream tarball (not `golang` RPM)
- `rpmbuild` won't see the dependency satisfied

**Impact**: **HIGH** - RPM builds will fail dependency checks

---

### 3. ⚠️ **VALID - MEDIUM**: DEB Cross-Build Architecture Issues

**Problem**:
- `dpkg-buildpackage -a<arch>` used without `:native` build-deps or `-d` override
- When host arch differs from target (e.g., building arm64 on amd64 host), cross-compilation dependencies may not be properly handled

**Impact**: **MEDIUM** - Multi-arch builds may fail on single host

---

## Solution Options

### Option A: Override Dependencies (Recommended)

**For DEB**:
- Remove `golang-go` from `Build-Depends` OR mark as `:native` OR use `-d` flag
- Since we control the build environment and install Go manually, we don't need the package

**For RPM**:
- Remove `golang` from `BuildRequires` OR use `--nodeps` flag
- Similar reasoning - we install Go manually

**Pros**:
- ✅ Simple, works with current upstream Go installation
- ✅ No need to maintain local packages
- ✅ Builds work correctly

**Cons**:
- ⚠️ Less "correct" from packaging perspective
- ⚠️ Doesn't verify Go version at package level

---

### Option B: Install golang-go/golang Packages

**For DEB**:
- Install `golang-go` package from Ubuntu repos
- But Ubuntu 24.04 has 1.22.2 (too old), so we'd still need upstream

**For RPM**:
- Install `golang` package from RockyLinux repos
- RockyLinux 9 has 1.25.3 (OK), but 8/10 may be old

**Pros**:
- ✅ Satisfies Build-Depends/BuildRequires
- ✅ More "correct" packaging

**Cons**:
- ❌ Still need upstream Go for older distros
- ❌ Inconsistent approach (package vs tarball)
- ❌ Doesn't solve the problem completely

---

### Option C: Create Local Go Packages

**Approach**:
- Create local DEB/RPM packages for Go 1.23+ from upstream
- Install these in build containers
- Satisfies dependencies

**Pros**:
- ✅ Satisfies all dependencies
- ✅ Uses upstream Go

**Cons**:
- ❌ Complex to maintain
- ❌ Additional build step
- ❌ Overkill for this use case

---

## Recommended Solution

**Use Option A with modifications**:

1. **For DEB**: Remove `golang-go` from `Build-Depends` since we install Go manually
   - Alternative: Use `-d` flag to skip dependency checks if we want to keep it for documentation

2. **For RPM**: Remove `golang` from `BuildRequires` or use `--nodeps`
   - Alternative: Keep it for documentation but use `--nodeps` during build

3. **For Multi-Arch**: 
   - Document that Go natively supports cross-compilation
   - Consider using native builders per arch for production (more reliable)
   - Single-host cross-compilation works but may have edge cases

---

## Multi-Arch Build Strategy

### Current Approach: Single Host Cross-Compilation
- Builds all architectures on one host
- Uses Go's native cross-compilation
- Works for pure Go binaries

### Alternative: Native Builders Per Arch
- Build each architecture on matching native hardware/containers
- More reliable, follows distro best practices
- Better for production releases

**Recommendation**: Support both, but document that native builders are preferred for production.
