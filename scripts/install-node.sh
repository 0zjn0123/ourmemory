#!/usr/bin/env bash
# Install the ourmemory storage-contributor agent on a Linux host.
# Run as root. The omnode binary must be next to this script or in PATH.
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "run as root" >&2
  exit 1
fi

SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -x "$SRC_DIR/omnode" ]; then
  BIN="$SRC_DIR/omnode"
elif command -v omnode >/dev/null 2>&1; then
  BIN="$(command -v omnode)"
else
  echo "omnode binary not found next to this script" >&2
  exit 1
fi

read -r -p "Main server URL [https://om.autozteam.cn]: " SERVER
SERVER="${SERVER:-https://om.autozteam.cn}"
read -r -p "How many GiB do you want to contribute? [10]: " GB
GB="${GB:-10}"
read -r -p "Your ourmemory username (for the contributor title): " OM_USER
read -r -p "Node registration secret (from the server admin): " NODE_SECRET
read -r -p "Node name [$(hostname)]: " NODE_NAME
NODE_NAME="${NODE_NAME:-$(hostname)}"
read -r -p "Also store encrypted private-vault chunks? [Y/n]: " PRIVATE
PRIVATE="${PRIVATE:-Y}"

case "$PRIVATE" in
  n|N|no|NO) PRIVATE_FLAG="false" ;;
  *) PRIVATE_FLAG="true" ;;
esac

install -m 0755 "$BIN" /usr/local/bin/omnode
mkdir -p /var/lib/omnode
chmod 700 /var/lib/omnode

cat >/etc/omnode.env <<EOF
OM_SERVER=$SERVER
OM_NODE_SECRET=$NODE_SECRET
OM_NODE_NAME=$NODE_NAME
OM_NODE_CAPACITY_GB=$GB
OM_NODE_PRIVATE=$PRIVATE_FLAG
OM_NODE_USER=$OM_USER
OM_NODE_DATA=/var/lib/omnode
EOF
chmod 600 /etc/omnode.env

cat >/etc/systemd/system/omnode.service <<'EOF'
[Unit]
Description=ourmemory storage contributor
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/omnode.env
ExecStart=/usr/local/bin/omnode
Restart=always
RestartSec=10
LimitNOFILE=1048576
NoNewPrivileges=true
ProtectSystem=full
ReadWritePaths=/var/lib/omnode

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable omnode >/dev/null
systemctl restart omnode
sleep 2
systemctl --no-pager --full status omnode | head -20
echo
echo "Agent logs: journalctl -u omnode -f"
