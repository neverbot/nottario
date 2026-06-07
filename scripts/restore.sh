#!/usr/bin/env bash
# Restore a Nottario backup dump produced by the in-process backup
# goroutine. Defaults to the dev compose Postgres if no URL is given.
set -euo pipefail

if [ $# -lt 1 ]; then
  echo "Usage: $0 <dump-file> [database_url] [--yes]" >&2
  exit 2
fi

DUMP="$1"; shift
URL="${1:-postgres://nottario:nottario@localhost:5432/nottario?sslmode=disable}"
[ "${1-}" = "--yes" ] && YES=1 || YES=0
[ "${2-}" = "--yes" ] && YES=1

if [ ! -f "$DUMP" ]; then
  echo "$DUMP: not a file" >&2
  exit 1
fi

if [ "$YES" -ne 1 ]; then
  read -r -p "This will DROP existing data in $URL before restoring. Continue? [y/N] " ans
  case "$ans" in y|Y|yes|YES) ;; *) echo "aborted"; exit 1;; esac
fi

pg_restore --clean --if-exists --no-owner --no-privileges -d "$URL" "$DUMP"
echo "restore complete: $DUMP -> $URL"
