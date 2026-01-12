# Packaging

TwinDB Backup v5.0.0 supports RPM and DEB packaging using containerized build
scripts.

## RPM

- `packaging/build-rpm.sh`
- `packaging/build-rpm-multiarch.sh`

## DEB

- `packaging/build-deb.sh`
- `packaging/build-deb-multiarch.sh`

## Notes

- Go 1.23+ is installed from upstream tarballs inside the build containers.
- RPM/DEB dependency checks are skipped because Go is provided by the build
  container and not the system package manager.
