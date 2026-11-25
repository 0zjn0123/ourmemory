#!/usr/bin/env bash
# Backup ourmemory metadata + packs. Run as root (or the ourmemory user).
# Safe to run while the service is up; packs are append-only, and the SQLite
# backup uses the online backup API. Restore requires stopping the service.
set -euo pipefail

DATA_DIR="${OM_DATA_DIR:-/opt/ourmemory/data}"
DEST="${OM_BACKUP_DIR:-/opt/ourmemory/backups}"
KEEP="${OM_BACKUP_KEEP:-14}"
STAMP="$(date +%Y%m%d-%H%M%S)"
OUT="$DEST/ourmemory-$STAMP"

mkdir -p "$OUT"
chmod 700 "$DEST" "$OUT"

if command -v sqlite3 >/dev/null 2>&1; then
  sqlite3 "$DATA_DIR/om.db" ".backup '$OUT/om.db'"
else
  cp "$DATA_DIR/om.db" "$OUT/om.db"
  [ -f "$DATA_DIR/om.db-wal" ] && cp "$DATA_DIR/om.db-wal" "$OUT/om.db-wal" || true
  [ -f "$DATA_DIR/om.db-shm" ] && cp "$DATA_DIR/om.db-shm" "$OUT/om.db-shm" || true
fi

# config.json contains no secrets, but master.key does.
cp "$DATA_DIR/config.json" "$OUT/config.json" 2>/dev/null || true
[ -f "$DATA_DIR/master.key" ] && cp "$DATA_DIR/master.key" "$OUT/master.key" || true

mkdir -p "$OUT/packs"
cp -a "$DATA_DIR/packs/." "$OUT/packs/"

tar -C "$DEST" -czf "$DEST/ourmemory-$STAMP.tar.gz" "ourmemory-$STAMP"
rm -rf "$OUT"
chmod 600 "$DEST/ourmemory-$STAMP.tar.gz"

find "$DEST" -name 'ourmemory-*.tar.gz' -type f -mtime +"$KEEP" -delete
echo "backup: $DEST/ourmemory-$STAMP.tar.gz"
