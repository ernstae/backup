#!/usr/bin/env sh
set -eu

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
export XBCLOUD_MOCK_DIR="${ROOT_DIR}/tests/integration/_storage"
mkdir -p "$XBCLOUD_MOCK_DIR"

cd "$ROOT_DIR/tests/integration"

docker compose up -d

printf "Waiting for MySQL to be healthy...\n"
for i in $(seq 1 30); do
  if docker inspect --format='{{.State.Health.Status}}' twindb-mysql 2>/dev/null | grep -q healthy; then
    break
  fi
  sleep 2
  if [ "$i" -eq 30 ]; then
    echo "MySQL did not become healthy" >&2
    exit 1
  fi
 done

printf "Creating demo data...\n"
mysql --defaults-file=my.cnf -e "CREATE DATABASE IF NOT EXISTS demo; USE demo; CREATE TABLE IF NOT EXISTS t (id INT PRIMARY KEY, v VARCHAR(64)); INSERT INTO t VALUES (1, 'before-backup') ON DUPLICATE KEY UPDATE v='before-backup';"

cd "$ROOT_DIR"
make build-go

printf "Running full backup...\n"
./bin/twindb-backup --config tests/integration/twindb-backup.yaml backup daily

printf "Adding more data and backing up binlogs...\n"
mysql --defaults-file=tests/integration/my.cnf -e "USE demo; INSERT INTO t VALUES (2, 'after-backup') ON DUPLICATE KEY UPDATE v='after-backup';"
./bin/twindb-backup --config tests/integration/twindb-backup.yaml backup hourly --binlogs-only

printf "Artifacts are stored in %s\n" "$XBCLOUD_MOCK_DIR"
printf "To stop the container: docker compose -f tests/integration/docker-compose.yml down\n"
