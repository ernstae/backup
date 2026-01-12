#!/usr/bin/env sh
set -eu

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
POD_NAME="twindb-suite"
MYSQL_CONTAINER="twindb-mysql"
BACKUP_IMAGE="twindb-backup-runner:dev"
SSHD_IMAGE="twindb-sshd:dev"
STATSD_IMAGE="twindb-statsd:dev"
DATA_VOLUME="twindb-mysql-data"
SSHD_VOLUME="twindb-ssh-data"
STATSD_VOLUME="twindb-statsd-data"

export XBCLOUD_MOCK_DIR="${ROOT_DIR}/tests/integration/_storage"
export GSUTIL_MOCK_DIR="${ROOT_DIR}/tests/integration/_gcs"
rm -rf "$XBCLOUD_MOCK_DIR" "$GSUTIL_MOCK_DIR" "$ROOT_DIR/tests/integration/_local" "$ROOT_DIR/tests/integration/_restore"
mkdir -p "$XBCLOUD_MOCK_DIR" "$GSUTIL_MOCK_DIR" "$ROOT_DIR/tests/integration/_local" "$ROOT_DIR/tests/integration/_restore" "$ROOT_DIR/tests/integration/sample_dir"

printf "Building Go binary...\n"
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 make build-go

printf "Building backup runner image...\n"
podman build -f tests/integration/Containerfile.backup -t "$BACKUP_IMAGE" .

printf "Building sshd image...\n"
podman build -f tests/integration/sshd/Containerfile -t "$SSHD_IMAGE" tests/integration/sshd

printf "Building statsd capture image...\n"
podman build -f tests/integration/statsd/Containerfile -t "$STATSD_IMAGE" tests/integration/statsd

if podman pod exists "$POD_NAME"; then
  podman pod rm -f "$POD_NAME"
fi

podman volume inspect "$DATA_VOLUME" >/dev/null 2>&1 || podman volume create "$DATA_VOLUME" >/dev/null
podman volume inspect "$SSHD_VOLUME" >/dev/null 2>&1 || podman volume create "$SSHD_VOLUME" >/dev/null
podman volume inspect "$STATSD_VOLUME" >/dev/null 2>&1 || podman volume create "$STATSD_VOLUME" >/dev/null

podman run --rm -v "$SSHD_VOLUME":/data alpine sh -c "rm -rf /data/*"
podman run --rm -v "$STATSD_VOLUME":/data alpine sh -c "rm -rf /data/*"

printf "Generating SSH key...\n"
SSH_DIR="$ROOT_DIR/tests/integration/_ssh"
mkdir -p "$SSH_DIR"
if [ ! -f "$SSH_DIR/id_ed25519" ]; then
  ssh-keygen -t ed25519 -N "" -f "$SSH_DIR/id_ed25519" >/dev/null
fi

printf "Starting pod and services...\n"
podman pod create --name "$POD_NAME" -p 3306:3306 -p 2222:2222 -p 8125:8125/udp

podman run -d --pod "$POD_NAME" --name "$MYSQL_CONTAINER" \
  -v "$DATA_VOLUME":/var/lib/mysql \
  -e MYSQL_ROOT_PASSWORD=rootpass \
  percona/percona-server:8.0 \
  --server-id=1 \
  --log-bin=mysql-bin \
  --binlog-format=ROW \
  --gtid-mode=ON \
  --enforce-gtid-consistency=ON \
  --binlog-row-image=FULL

podman run -d --pod "$POD_NAME" --name twindb-sshd \
  -v "$SSHD_VOLUME":/data \
  -v "$SSH_DIR/id_ed25519.pub":/authorized_keys:ro \
  "$SSHD_IMAGE"

podman run -d --pod "$POD_NAME" --name twindb-statsd \
  -v "$STATSD_VOLUME":/data \
  "$STATSD_IMAGE"

printf "Waiting for MySQL to be ready...\n"
for i in $(seq 1 30); do
  if podman exec "$MYSQL_CONTAINER" mysqladmin ping -prootpass --silent >/dev/null 2>&1; then
    break
  fi
  sleep 2
  if [ "$i" -eq 30 ]; then
    echo "MySQL did not become ready" >&2
    exit 1
  fi
 done

printf "Creating demo data...\n"
podman exec "$MYSQL_CONTAINER" mysql -uroot -prootpass -e "CREATE DATABASE IF NOT EXISTS demo; USE demo; CREATE TABLE IF NOT EXISTS t (id INT PRIMARY KEY, v VARCHAR(64)); INSERT INTO t VALUES (1, 'before-backup') ON DUPLICATE KEY UPDATE v='before-backup';"

printf "Preparing sample files...\n"
echo "alpha" > "$ROOT_DIR/tests/integration/sample_dir/a.txt"

run_backup() {
  dest="$1"
  remote_path="$2"
  restore_dir="/work/tests/integration/_restore/${dest}"

  cat > "$ROOT_DIR/tests/integration/twindb-backup-suite.yaml" <<CONFIG
version: 1
paths:
  state_dir: /tmp/twindb-backup/state
  lock_file: /tmp/twindb-backup/lock

source:
  backup_dirs:
    - /work/tests/integration/sample_dir
  tar_options: ""
  backup_mysql: true

mysql:
  defaults_file: /work/tests/integration/my.cnf
  mysql_binary: mysql
  mysqlbinlog_binary: mysqlbinlog
  xtrabackup_binary: xtrabackup
  xbcloud_binary: /work/tests/integration/xbcloud-mock.sh
  xbstream_binary: xbstream
  full_backup: daily
  expire_log_days: 7
  capture_runtime_config: true

backup:
  intervals:
    hourly: true
    daily: true
    weekly: false
    monthly: false
    yearly: false

compression:
  method: gzip
  level: 6
  threads: 1

encryption:
  gpg:
    enabled: true
    recipient: backup@twindb.local
    keyring: /work/tests/integration/_gpg/pubring.kbx
    secret_keyring: /work/tests/integration/_gpg/private-keys-v1.d
    binary: gpg

destination:
  kind: ${dest}
  provider: s3
  remote_path: ${remote_path}
  keep_local_path: ""

s3:
  bucket: mock-bucket
  region: us-east-1
  access_key: mock
  secret_key: mock
  endpoint: ""

azure:
  connection_string: ""
  container: ""

gcs:
  bucket: mock-bucket
  credentials_file: ""
  gsutil_binary: /work/tests/integration/gsutil-mock.sh

ssh:
  host: 127.0.0.1
  user: backup
  port: 2222
  key_path: /work/tests/integration/_ssh/id_ed25519
  strict_host_key_checking: false

retention:
  remote:
    hourly: 24
    daily: 7
    weekly: 4
    monthly: 12
    yearly: 3
  local:
    hourly: 0
    daily: 0
    weekly: 0
    monthly: 0
    yearly: 0

metrics:
  transport: statsd
  address: 127.0.0.1:8125
  prefix: twindb
  tags:
    - env:test
    - service:twindb-backup
  interval_seconds: 2

verify:
  depth_bytes: 4096

worm:
  enabled: false  # Disable WORM for integration tests to allow file overwrites
CONFIG

  podman run --rm --pod "$POD_NAME" \
    -e XBCLOUD_MOCK_DIR=/work/tests/integration/_storage \
    -e GSUTIL_MOCK_DIR=/work/tests/integration/_gcs \
    -v "$ROOT_DIR":/work \
    -v "$DATA_VOLUME":/var/lib/mysql:ro \
    -v "$SSHD_VOLUME":/data \
    -w /work \
    "$BACKUP_IMAGE" \
    /work/bin/twindb-backup --config /work/tests/integration/twindb-backup-suite.yaml backup daily

  podman run --rm --pod "$POD_NAME" \
    -e XBCLOUD_MOCK_DIR=/work/tests/integration/_storage \
    -e GSUTIL_MOCK_DIR=/work/tests/integration/_gcs \
    -v "$ROOT_DIR":/work \
    -v "$DATA_VOLUME":/var/lib/mysql:ro \
    -v "$SSHD_VOLUME":/data \
    -w /work \
    "$BACKUP_IMAGE" \
    /work/bin/twindb-backup --config /work/tests/integration/twindb-backup-suite.yaml backup hourly --binlogs-only

  last_backup=$(podman run --rm --pod "$POD_NAME" \
    -e XBCLOUD_MOCK_DIR=/work/tests/integration/_storage \
    -e GSUTIL_MOCK_DIR=/work/tests/integration/_gcs \
    -v "$ROOT_DIR":/work \
    -v "$DATA_VOLUME":/var/lib/mysql:ro \
    -v "$SSHD_VOLUME":/data \
    -w /work \
    "$BACKUP_IMAGE" \
    /work/bin/twindb-backup --config /work/tests/integration/twindb-backup-suite.yaml ls --type mysql | tail -n 1)

  podman run --rm --pod "$POD_NAME" \
    -e XBCLOUD_MOCK_DIR=/work/tests/integration/_storage \
    -e GSUTIL_MOCK_DIR=/work/tests/integration/_gcs \
    -v "$ROOT_DIR":/work \
    -v "$DATA_VOLUME":/var/lib/mysql:ro \
    -v "$SSHD_VOLUME":/data \
    -w /work \
    "$BACKUP_IMAGE" \
    sh -c "rm -rf ${restore_dir} && mkdir -p ${restore_dir} && /work/bin/twindb-backup --config /work/tests/integration/twindb-backup-suite.yaml restore mysql --to-dir ${restore_dir} --backup-id ${last_backup}"

  podman run --rm --pod "$POD_NAME" \
    -e XBCLOUD_MOCK_DIR=/work/tests/integration/_storage \
    -e GSUTIL_MOCK_DIR=/work/tests/integration/_gcs \
    -v "$ROOT_DIR":/work \
    -v "$DATA_VOLUME":/var/lib/mysql:ro \
    -v "$SSHD_VOLUME":/data \
    -w /work \
    "$BACKUP_IMAGE" \
    /work/bin/twindb-backup --config /work/tests/integration/twindb-backup-suite.yaml verify mysql "${last_backup}"
}

printf "Generating GPG key...\n"
GPG_DIR="$ROOT_DIR/tests/integration/_gpg"
mkdir -p "$GPG_DIR"
if [ ! -d "$GPG_DIR/private-keys-v1.d" ]; then
  podman run --rm -v "$GPG_DIR":/gpg -v "$ROOT_DIR/tests/integration/gpg-genkey.conf":/gpg/genkey.conf:ro -w /gpg "$BACKUP_IMAGE" sh -c "gpg --batch --homedir /gpg --generate-key /gpg/genkey.conf"
fi

printf "Running destination suite...\n"
run_backup local /work/tests/integration/_local
run_backup ssh /data/backups
run_backup gcs /backups/mysql
run_backup xbcloud /backups/mysql

printf "Metrics captured in: %s\n" "$STATSD_VOLUME"

printf "To stop and remove the pod: podman pod rm -f %s\n" "$POD_NAME"
