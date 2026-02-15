#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INTERVAL="${TABULA_DEV_WATCH_INTERVAL:-0.7}"

snapshot() {
  (
    cd "$REPO_ROOT"
    {
      find src/tabula -type f \
        \( -name '*.py' -o -name '*.js' -o -name '*.css' -o -name '*.html' -o -name '*.json' \) \
        -printf '%p %T@ %s\n' 2>/dev/null
      if [[ -f pyproject.toml ]]; then
        stat -c '%n %Y %s' pyproject.toml
      fi
    } | sort | sha256sum | awk '{print $1}'
  )
}

last_hash="$(snapshot)"

while true; do
  sleep "$INTERVAL"
  next_hash="$(snapshot)"
  if [[ "$next_hash" != "$last_hash" ]]; then
    last_hash="$next_hash"
    "$REPO_ROOT/scripts/tabula-dev-restart.sh"
  fi
done
