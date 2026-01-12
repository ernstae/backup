# Packaging Guide

This directory contains packaging files and build scripts for creating RPM and DEB packages for TwinDB Backup v5.0.0.

## Supported Platforms

### RPM (RockyLinux)
- RockyLinux 8
- RockyLinux 9
- RockyLinux 10

### DEB (Ubuntu)
- Ubuntu 24.04 LTS (noble)

## Prerequisites

- Podman or Docker installed
- Go 1.23+ (installed in build containers)
- Git

## Building Packages

### Build Documentation (MkDocs)

```bash
make docs-go
```

This builds the MkDocs site for v5.0.0 in `site/` using a containerized
MkDocs image defined in `packaging/docker/Dockerfile.docs`.

### Live Preview (MkDocs)

```bash
make docs-go-serve
```

This starts `mkdocs serve` inside the docs container and publishes it on
`http://localhost:8000`.

### Build All Packages (Multi-Architecture)

```bash
make build-packages
```

This will build RPM packages for all RockyLinux versions and a DEB package for Ubuntu, for both amd64 and arm64 architectures.

### Build Single Architecture

```bash
make build-packages-single-arch
```

This builds packages for the current system's architecture only.

### Build Individual Packages

**RPM (RockyLinux 8):**
```bash
make build-rpm VERSION=8
```

**RPM (RockyLinux 9):**
```bash
make build-rpm VERSION=9
```

**RPM (RockyLinux 10):**
```bash
make build-rpm VERSION=10
```

**DEB (Ubuntu):**
```bash
make build-deb
```

### Manual Build

You can also use the build scripts directly:

```bash
# RPM
./packaging/build-rpm.sh 8
./packaging/build-rpm.sh 9
./packaging/build-rpm.sh 10

# DEB
./packaging/build-deb.sh
```

## Build Process

1. **Container Images**: Build containers are created from Dockerfiles in `packaging/docker/`
2. **Source Preparation**: Source code is prepared (tarball for RPM, debian directory for DEB)
3. **Build**: Package is built inside the container
4. **Output**: Packages are placed in `packaging/dist/`

## Package Contents

Both RPM and DEB packages include:
- `/usr/bin/twindb-backup` - Main binary
- `/lib/systemd/system/twindb-backup@.service` - Systemd service template
- `/lib/systemd/system/twindb-backup@.timer` - Systemd timer template
- Documentation files

## Installation

### RPM (RockyLinux)

```bash
sudo rpm -ivh packaging/dist/twindb-backup-*.rpm
```

### DEB (Ubuntu)

```bash
sudo dpkg -i packaging/dist/twindb-backup_*.deb
sudo apt-get install -f  # Install dependencies if needed
```

## GitHub Actions

The `.github/workflows/build-packages.yml` workflow automatically:
- Builds packages for all platforms on tag pushes
- Creates GitHub releases with all packages attached
- Can be triggered manually via `workflow_dispatch`

## Version Management

Version is determined from:
1. Git tag (if building from a tag)
2. Default: `0.1.0`

To build a specific version:
```bash
git tag v1.0.0
git push origin v1.0.0
# GitHub Actions will build and release
```

## Directory Structure

```
packaging/
├── docker/              # Dockerfiles for build containers
│   ├── Dockerfile.rockylinux8
│   ├── Dockerfile.rockylinux9
│   ├── Dockerfile.rockylinux10
│   └── Dockerfile.ubuntu
├── rpm/                 # RPM packaging files
│   └── twindb-backup.spec
├── deb/                 # DEB packaging files
│   ├── control
│   ├── rules
│   ├── changelog
│   ├── compat
│   └── copyright
├── systemd/             # Systemd unit files
│   ├── twindb-backup@.service
│   └── twindb-backup@.timer
├── build-rpm.sh         # RPM build script
├── build-deb.sh         # DEB build script
├── dist/                # Output directory (created during build)
└── README.md            # This file
```
