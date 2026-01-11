# Integration Suite

This suite exercises the major components using Podman containers:

- MySQL full backup + binlog-only backup
- File backups via tar
- Destinations: local, ssh, gcs (gsutil mock), xbcloud mock
- GPG encryption/decryption
- Metrics emission to a UDP StatsD capture container
- Restore + verify using the latest backup

## Run

```sh
./tests/integration/podman-suite.sh
```

## Artifacts

- Storage: `tests/integration/_storage` (xbcloud mock)
- GCS mock: `tests/integration/_gcs`
- SSH storage: `tests/integration/_ssh` (keys), remote files stored in `/data` volume
- StatsD metrics: captured in `twindb-statsd-data` volume

## Cleanup

```sh
podman pod rm -f twindb-suite
podman volume rm twindb-mysql-data twindb-ssh-data twindb-statsd-data
```
