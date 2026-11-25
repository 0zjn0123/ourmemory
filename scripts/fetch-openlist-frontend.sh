#!/usr/bin/env bash
# Fetch the upstream OpenList frontend used by the /@manage administration UI.
# The 24 MB dist is intentionally not committed to this repository.
set -euo pipefail
version="${1:-v4.2.10}"
root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
url="https://v4.gh-proxy.org/https://github.com/OpenListTeam/OpenList-Frontend/releases/download/${version}/openlist-frontend-dist-${version}.tar.gz"
echo "fetching ${url}"
curl -fL --retry 3 --connect-timeout 20 "$url" -o "$tmp/dist.tar.gz"
tar -xzf "$tmp/dist.tar.gz" -C "$tmp"
mkdir -p "$root/public/dist"
rm -rf "$root/public/dist/assets" "$root/public/dist/images" "$root/public/dist/static" "$root/public/dist/streamer"
cp -a "$tmp"/. "$root/public/dist"/
# Keep our launcher build's README if present.
printf 'OpenList frontend %s (fetched by scripts/fetch-openlist-frontend.sh)\n' "$version" > "$root/public/dist/README.md"
echo "done: $root/public/dist"
