#!/usr/bin/env bash
# Idempotent installer for the ourmemory server on Debian/Ubuntu.
# Run as root after copying the binary and public/ into /opt/ourmemory.
set -euo pipefail

APP=/opt/ourmemory
ENVF=$APP/ourmemory.env
DATADIR=$APP/data
BIN=$APP/ourmemory

if [ ! -x "$BIN" ]; then
  echo "missing $BIN" >&2
  exit 1
fi

if ! id ourmemory >/dev/null 2>&1; then
  useradd --system --home "$APP" --shell /usr/sbin/nologin ourmemory
fi
mkdir -p "$DATADIR" "$APP/public"
chown -R ourmemory:ourmemory "$DATADIR" "$APP/public"
chmod 700 "$DATADIR"

if [ ! -f "$ENVF" ]; then
  ADMIN_PW="$(openssl rand -base64 24 | tr -d '/+=' | cut -c1-20)"
  MASTER_KEY="$(openssl rand -hex 32)"
  NODE_SECRET="$(openssl rand -hex 24)"
  cat >"$ENVF" <<EOF
OM_BASE_HOST=om.autozteam.cn
OM_BASE_URL=https://om.autozteam.cn
OM_MASTER_KEY=$MASTER_KEY
OM_SHARED_MAX_BYTES=5368709120
OM_CACHE_BYTES=10737418240
OM_MAX_UPLOAD=8589934592
OM_DEFAULT_PRIVATE_QUOTA=1073741824
OM_NODE_SECRET=$NODE_SECRET
OM_SKYMAIL_BASE=https://skymail.ink
OM_DEBUG=1
OPENLIST_ADMIN_PASSWORD=$ADMIN_PW
OPENLIST_SITE_URL=https://om.autozteam.cn
EOF
  chmod 600 "$ENVF"
  printf '%s\n' "$ADMIN_PW" >"$APP/ADMIN_PASSWORD.txt"
  printf '%s\n' "$NODE_SECRET" >"$APP/NODE_SECRET.txt"
  chmod 600 "$APP/ADMIN_PASSWORD.txt" "$APP/NODE_SECRET.txt"
fi

# The OpenList config is read before the HTTP listener starts. Creating it
# explicitly avoids the upstream default port 5244 and relative paths.
if [ ! -f "$DATADIR/config.json" ]; then
  JWT_SECRET="$(openssl rand -hex 16)"
  cat >"$DATADIR/config.json" <<EOF
{
  "force": false,
  "site_url": "https://om.autozteam.cn",
  "cdn": "",
  "jwt_secret": "$JWT_SECRET",
  "token_expires_in": 48,
  "database": {
    "type": "sqlite3",
    "host": "",
    "port": 0,
    "user": "",
    "password": "",
    "name": "",
    "db_file": "$DATADIR/data.db",
    "table_prefix": "x_",
    "ssl_mode": "",
    "dsn": ""
  },
  "scheme": {
    "address": "0.0.0.0",
    "http_port": 5259,
    "https_port": -1,
    "force_https": false,
    "cert_file": "",
    "key_file": "",
    "unix_file": "",
    "unix_file_perm": "",
    "enable_h2c": false,
    "enable_h3": false
  },
  "temp_dir": "$DATADIR/temp",
  "bleve_dir": "$DATADIR/bleve",
  "dist_dir": "$APP/public/dist",
  "log": {
    "enable": true,
    "name": "$DATADIR/log/log.log",
    "max_size": 50,
    "max_backups": 30,
    "max_age": 28,
    "compress": false,
    "filter": {"enable": false, "filters": []}
  },
  "delayed_start": 0,
  "auto_memory_limit": 0,
  "min_free_memory": 0,
  "max_block_limit": 0,
  "max_connections": 0,
  "max_concurrency": 64,
  "tls_insecure_skip_verify": false,
  "cors": {"allow_origins": ["*"], "allow_methods": ["*"], "allow_headers": ["*"]},
  "s3": {"enable": false, "port": 5246, "ssl": false, "multipart_ttl": "24h"},
  "ftp": {"enable": false, "listen": ":5221"},
  "sftp": {"enable": false, "listen": ":5222"},
  "mcp": {"enable": false},
  "last_launched_version": "dev",
  "proxy_address": ""
}
EOF
  chown ourmemory:ourmemory "$DATADIR/config.json"
  chmod 600 "$DATADIR/config.json"
fi

install -m 0644 "$APP/deploy/ourmemory.service" /etc/systemd/system/ourmemory.service
systemctl daemon-reload
systemctl enable ourmemory >/dev/null
systemctl restart ourmemory
sleep 2
systemctl --no-pager --full status ourmemory | head -20
echo
echo "admin password: $APP/ADMIN_PASSWORD.txt"
echo "node secret:    $APP/NODE_SECRET.txt"
echo "local check:    curl -fsS http://127.0.0.1:5259/api/om/me"
