#!/usr/bin/env sh
set -eu

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
POD_NAME="twindb-backup"
MYSQL_CONTAINER="twindb-mysql"
BACKUP_IMAGE="twindb-backup-runner:dev"
DATA_VOLUME="twindb-mysql-data"

export XBCLOUD_MOCK_DIR="${ROOT_DIR}/tests/integration/_storage"
mkdir -p "$XBCLOUD_MOCK_DIR"

cd "$ROOT_DIR"

printf "Building Go binary...\n"
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 make build-go

printf "Building backup runner image...\n"
podman build -f tests/integration/Containerfile.backup -t "$BACKUP_IMAGE" .

if podman pod exists "$POD_NAME"; then
  podman pod rm -f "$POD_NAME"
fi

printf "Starting pod and MySQL container...\n"
podman pod create --name "$POD_NAME" -p 3306:3306
podman volume inspect "$DATA_VOLUME" >/dev/null 2>&1 || podman volume create "$DATA_VOLUME" >/dev/null

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

printf "Running full backup...\n"
podman run --rm --pod "$POD_NAME" \
  -e XBCLOUD_MOCK_DIR=/work/tests/integration/_storage \
  -v "$ROOT_DIR":/work \
  -v "$DATA_VOLUME":/var/lib/mysql:ro \
  -w /work \
  "$BACKUP_IMAGE" \
  /work/bin/twindb-backup --config /work/tests/integration/twindb-backup.yaml backup daily

printf "Adding more data and backing up binlogs...\n"
podman exec "$MYSQL_CONTAINER" mysql -uroot -prootpass -e "USE demo; INSERT INTO t VALUES (2, 'after-backup') ON DUPLICATE KEY UPDATE v='after-backup';"

podman run --rm --pod "$POD_NAME" \
  -e XBCLOUD_MOCK_DIR=/work/tests/integration/_storage \
  -v "$ROOT_DIR":/work \
  -v "$DATA_VOLUME":/var/lib/mysql:ro \
  -w /work \
  "$BACKUP_IMAGE" \
  /work/bin/twindb-backup --config /work/tests/integration/twindb-backup.yaml backup hourly --binlogs-only

printf "\nBackup summary:\n"
printf "  - Full backup: ok\n"
printf "  - Binlog backup: ok\n"
printf "  - Storage path: %s\n" "$XBCLOUD_MOCK_DIR"
printf "  - Stored objects:\n"
find "$XBCLOUD_MOCK_DIR" -type f | sed 's/^/    /'

printf "\nArtifacts are stored in %s\n" "$XBCLOUD_MOCK_DIR"
printf "To stop and remove the pod: podman pod rm -f %s\n" "$POD_NAME"
