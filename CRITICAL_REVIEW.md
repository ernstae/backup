# Critical Review (Go Rewrite)

This document merges the original `CODE_REVIEW.md` with recent findings. It prioritizes unresolved gaps, regressions, and operational risks relative to the Python implementation.

## Findings (Ordered by Severity)

- High: MariaDB is not supported (no vendor detection, `mariabackup`/`mbstream` handling), so MariaDB backups/restores will fail (`internal/mysql`, `internal/pipeline`, `internal/restore`).
- High: Galera/XtraDB Cluster metadata is not handled; `xtrabackup_galera_info` parsing and `grastate.dat` generation are missing, so clustered restores are incomplete (`internal/restore`).
- High: Status format is not backward compatible with Python (base64 + MD5). There is no migration path or compatibility reader, so existing status histories are ignored (`internal/status/status.go`).
- Medium: File backups exist but file restore is missing, leaving the file backup feature incomplete (`internal/cli`, `internal/restore`).
- Medium: PITR binlog application writes temporary binlogs to disk without cleanup, so repeated restores can leak space under `/tmp` (`internal/restore/restore.go`).
- Medium: Status writes are non-atomic and checksum sidecars are written separately. A mid-write interruption can leave mismatched state that blocks future runs (`internal/status/status.go`, destination backends).
- Medium: `verify` only reads a shallow prefix, so corruption after `verify.depth_bytes` is not detected (`internal/verify/verify.go`, `docs/GO_REWRITE.md`).
- Medium: External tool backends (ssh/gsutil/xbcloud) have no retry/backoff, so transient errors surface as hard failures (`internal/destination/ssh/ssh.go`, `internal/destination/gcs/gcs.go`, `internal/destination/xbcloud/xbcloud.go`).
- Low: PITR does not validate full binlog coverage between base backup and target (possible silent gaps) (`internal/restore/restore.go`).

## Parity Gaps (Python vs Go)

- Clone command is intentionally omitted (Python `twindb-backup clone mysql`).
  - **Reason:** MySQL 8.0.17+ includes the built-in CLONE plugin, making external clone functionality redundant.
  - **Recommendation:** Use MySQL's native `CLONE` plugin for MySQL 8.0.17+.
- Share command is missing (Python `twindb-backup share <s3_url>`).
- Restore-from-cache functionality is missing (Python `--cache` support).
- Measures logging (`/var/log/twindb-backup-measures.log`) is missing.
- File restore command is missing (see finding above).
- Status compatibility/migration from Python format is missing (see finding above).
- MariaDB and Galera support are missing (see findings above).

## Implemented Parity and Improvements

- Lock handling is implemented and improved (one active + one waiting, immediate rejection otherwise).
- KeepLocal and local retention are implemented for MySQL and file backups.
- MySQL config capture/restoration is implemented with include handling.
- Open-files limit and xtrabackup prepare memory sizing are implemented.
- Status integrity uses SHA-256 sidecar checksums (FIPS 180-4 compliant) rather than MD5.
- Logging supports JSON output with `--log-file` and verbose `--debug`.
- Metrics include start/running/finish spans plus binlog/verify/restore counters.

## Compatibility and Migration Notes

- Status files are plain JSON with `.sha256` sidecars. Python-generated status (base64+MD5) is ignored.
- No migration helper exists; users must regenerate status data or a compatibility reader is needed.

## Testing Notes

- Integration tests run in Podman, using mock xbcloud/gsutil backends.
- No live cloud (S3/Azure/GCS) integration coverage is included yet.
- MariaDB/Galera test coverage is absent.
