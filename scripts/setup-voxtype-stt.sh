#!/usr/bin/env bash
set -euo pipefail

# Start voxtype daemon with the OpenAI-compatible STT service enabled.
# Hotkey (push-to-talk) remains active so a single voxtype process serves
# both the HTTP API and desktop voice input.

VOXTYPE_BIN="${VOXTYPE_BIN:-voxtype}"
HOST="${TABURA_STT_HOST:-127.0.0.1}"
PORT="${TABURA_STT_PORT:-8427}"
LANGUAGE_RAW="${TABURA_STT_LANGUAGE:-en,de}"
THREADS="${TABURA_STT_THREADS:-4}"
PROMPT="${TABURA_STT_PROMPT:-}"
MODEL="${TABURA_STT_MODEL:-large-v3-turbo}"
# Hotkey is read from voxtype's own config (~/.config/voxtype/config.toml).
# Only override if explicitly set via TABURA_STT_HOTKEY.
HOTKEY="${TABURA_STT_HOTKEY:-}"

if ! command -v "$VOXTYPE_BIN" >/dev/null 2>&1; then
  echo "voxtype binary not found: $VOXTYPE_BIN" >&2
  echo "Install voxtype and ensure it is in PATH (or set VOXTYPE_BIN)." >&2
  exit 1
fi
VOXTYPE_HELP="$("$VOXTYPE_BIN" --help 2>&1 || true)"
case "$VOXTYPE_HELP" in
  *"--service"*) ;;
  *)
  echo "voxtype binary does not expose the local STT service flags." >&2
  echo "Installed version: $("$VOXTYPE_BIN" --version 2>/dev/null || echo unknown)" >&2
  echo "Rebuild and reinstall the pinned branch via scripts/build-voxtype-macos.sh." >&2
  exit 1
  ;;
esac

LANGUAGE_CSV="$(printf '%s' "$LANGUAGE_RAW" | tr '[:upper:]' '[:lower:]' | tr -d '[:space:]')"
if [ -z "$LANGUAGE_CSV" ]; then
  LANGUAGE_CSV="en,de"
fi
PRIMARY_LANGUAGE="${LANGUAGE_CSV%%,*}"
LANGUAGE_MODE="$PRIMARY_LANGUAGE"
if [[ "$LANGUAGE_CSV" == *,* ]]; then
  LANGUAGE_MODE="auto"
fi

echo "Starting voxtype daemon with STT service at http://$HOST:$PORT (languages=$LANGUAGE_CSV model=$MODEL${HOTKEY:+ hotkey=$HOTKEY})"

export VOXTYPE_SERVICE_ENABLED=true
export VOXTYPE_SERVICE_HOST="$HOST"
export VOXTYPE_SERVICE_PORT="$PORT"
export VOXTYPE_SERVICE_ALLOWED_LANGUAGES="$LANGUAGE_CSV"
export VOXTYPE_LANGUAGE="$LANGUAGE_MODE"
export VOXTYPE_MODEL="$MODEL"
export VOXTYPE_THREADS="$THREADS"

args=(
  --service
  --service-host "$HOST"
  --service-port "$PORT"
  --model "$MODEL"
  --language "$LANGUAGE_MODE"
  --threads "$THREADS"
)
if [ -n "$HOTKEY" ]; then
  args+=(--hotkey "$HOTKEY")
fi
if [ -n "$PROMPT" ]; then
  args+=(--initial-prompt "$PROMPT")
fi
args+=(daemon)

exec "$VOXTYPE_BIN" "${args[@]}"
