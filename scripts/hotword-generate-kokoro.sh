#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${TABURA_HOTWORD_KOKORO_COMMAND:-}" ]]; then
  echo "Kokoro generator is not installed. Set TABURA_HOTWORD_KOKORO_COMMAND to an executable." >&2
  exit 1
fi

exec "${TABURA_HOTWORD_KOKORO_COMMAND}" "$@"
