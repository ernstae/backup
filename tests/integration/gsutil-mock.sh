#!/usr/bin/env sh
set -eu

if [ -z "${GSUTIL_MOCK_DIR:-}" ]; then
  echo "GSUTIL_MOCK_DIR is required" >&2
  exit 2
fi

operation="$1"
shift

case "$operation" in
  --version|-v|version)
    echo "gsutil version 5.27"
    exit 0
    ;;
  cp)
    if [ "$#" -lt 2 ]; then
      echo "not enough arguments" >&2
      exit 2
    fi
    # Expect: cp - gs://bucket/path
    while [ "$1" != "-" ]; do shift; done
    shift
    dest="$1"
    dest_path=$(echo "$dest" | sed -e 's#^gs://##')
    full_path="$GSUTIL_MOCK_DIR/$dest_path"
    mkdir -p "$(dirname "$full_path")"
    cat > "$full_path"
    ;;
  cat)
    if [ "$#" -lt 1 ]; then
      echo "not enough arguments" >&2
      exit 2
    fi
    src="$1"
    src_path=$(echo "$src" | sed -e 's#^gs://##')
    cat "$GSUTIL_MOCK_DIR/$src_path"
    ;;
  stat)
    if [ "$#" -lt 1 ]; then
      echo "not enough arguments" >&2
      exit 2
    fi
    src="$1"
    src_path=$(echo "$src" | sed -e 's#^gs://##')
    if [ ! -f "$GSUTIL_MOCK_DIR/$src_path" ]; then
      echo "Not Found" >&2
      exit 1
    fi
    ;;
  rm)
    if [ "$#" -lt 1 ]; then
      echo "not enough arguments" >&2
      exit 2
    fi
    src="$1"
    src_path=$(echo "$src" | sed -e 's#^gs://##')
    rm -f "$GSUTIL_MOCK_DIR/$src_path"
    ;;
  *)
    echo "unsupported operation: $operation" >&2
    exit 2
    ;;
 esac
