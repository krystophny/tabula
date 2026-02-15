#!/usr/bin/env bash
set -euo pipefail

LOCK_FILE="${XDG_RUNTIME_DIR:-/tmp}/tabula-dev-reload.lock"
mkdir -p "$(dirname "$LOCK_FILE")"
exec 9>"$LOCK_FILE"
if ! flock -n 9; then
  exit 0
fi

# Coalesce rapid save bursts.
sleep 0.35

systemctl --user restart tabula-mcp.service
systemctl --user restart tabula-web.service
