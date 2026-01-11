# Destinations (Go)

## Local

```yaml
destination:
  kind: local
  remote_path: /var/backups/twindb
```

## SSH

```yaml
destination:
  kind: ssh
  remote_path: /var/backups/twindb

ssh:
  host: backup.example.com
  user: backup
  port: 22
  key_path: /root/.ssh/id_rsa
  strict_host_key_checking: true
  known_hosts_file: /root/.ssh/known_hosts
```

## GCS (gsutil)

```yaml
destination:
  kind: gcs
  remote_path: /backups/mysql

gcs:
  bucket: twindb-backups
  credentials_file: /etc/twindb/gcs.json
  gsutil_binary: gsutil
```
