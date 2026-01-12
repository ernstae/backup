# Multi-Architecture Build Support

The packaging system now supports building for multiple architectures (amd64 and arm64).

## Architecture Support

- **amd64** (x86_64) - Intel/AMD 64-bit
- **arm64** (aarch64) - ARM 64-bit

## Building for Multiple Architectures

### Using Makefile

**Build all packages for all architectures:**
```bash
make build-packages
```
This builds RPM packages for RockyLinux 8, 9, 10 and DEB packages for Ubuntu, for both amd64 and arm64.

**Build for single architecture (current system):**
```bash
make build-packages-single-arch
```

### Using Build Scripts Directly

**RPM - Single architecture:**
```bash
# Build for specific architecture
GOARCH=amd64 ./packaging/build-rpm.sh 9
GOARCH=arm64 ./packaging/build-rpm.sh 9

# Or use multi-arch script
./packaging/build-rpm-multiarch.sh 9 amd64 arm64
```

**DEB - Single architecture:**
```bash
# Build for specific architecture
GOARCH=amd64 ./packaging/build-deb.sh
GOARCH=arm64 ./packaging/build-deb.sh

# Or use multi-arch script
./packaging/build-deb-multiarch.sh amd64 arm64
```

## GitHub Actions

The GitHub Actions workflow automatically builds for both architectures using a build matrix:
- Each platform (RockyLinux 8/9/10, Ubuntu) builds for both amd64 and arm64
- Artifacts are uploaded separately per architecture
- All packages are included in the release

## Architecture Detection

Build scripts automatically detect the architecture:
1. Uses `GOARCH` environment variable if set
2. Falls back to `go env GOARCH` if available
3. Defaults to `amd64` if neither is available

## Package Naming

- **RPM**: Architecture is included in the package filename by rpmbuild (e.g., `twindb-backup-0.1.0-1.el9.aarch64.rpm`)
- **DEB**: Architecture is included in the package filename by dpkg-buildpackage (e.g., `twindb-backup_0.1.0-1_arm64.deb`)

## Cross-Compilation

Go's cross-compilation is used, so you can build for any architecture from any platform:
- Build arm64 packages on amd64 systems
- Build amd64 packages on arm64 systems

### Build Strategies

**Single-Host Cross-Compilation** (Current Default):
- Builds all architectures on one host
- Uses Go's native cross-compilation (`CGO_ENABLED=0`)
- Works well for pure Go binaries
- Used by `make build-packages`

**Native Builders Per Architecture** (Recommended for Production):
- Build each architecture on matching native hardware/containers
- More reliable, follows distro best practices
- Better for complex builds or when CGO is enabled
- Can be achieved by running builds on native builders with matching `GOARCH`

### Build Dependencies

**Note**: Go is installed from upstream tarball (not via package manager) to ensure version consistency:
- Ubuntu 24.04 has Go 1.22.2 (too old), so we use upstream Go 1.23+
- RockyLinux 8/10 may have old versions, so we use upstream Go 1.23+
- Build scripts use `-d` (DEB) or `--nodeps` (RPM) flags to skip dependency checks
- This is safe because we control the build environment completely
- No need for architecture-specific build containers

## Notes

- Source packages (SRPM) are architecture-independent and only built once
- The DEB control file uses `Architecture: any` to support all architectures
- All builds use static linking (`CGO_ENABLED=0`) for maximum portability
