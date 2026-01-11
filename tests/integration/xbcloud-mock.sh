#!/usr/bin/env sh
set -eu

if [ -z "${XBCLOUD_MOCK_DIR:-}" ]; then
  echo "XBCLOUD_MOCK_DIR is required" >&2
  exit 2
fi

if [ "$#" -lt 2 ]; then
  echo "not enough arguments" >&2
  exit 2
fi

operation=""
object_path=""
idx=0
for arg in "$@"; do
  idx=$((idx + 1))
  if [ "$idx" -eq $(($# - 1)) ]; then
    operation="$arg"
  elif [ "$idx" -eq "$#" ]; then
    object_path="$arg"
  fi
done

full_path="$XBCLOUD_MOCK_DIR/$object_path"

case "$operation" in
  put)
    mkdir -p "$(dirname "$full_path")"
    cat > "$full_path"
    ;;
  get)
    cat "$full_path"
    ;;
  delete)
    rm -f "$full_path"
    ;;
  *)
    echo "unsupported operation: $operation" >&2
    exit 2
    ;;
 esac
