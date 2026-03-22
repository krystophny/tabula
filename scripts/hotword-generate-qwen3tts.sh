#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${TABURA_HOTWORD_QWEN3TTS_COMMAND:-}" ]]; then
  echo "Qwen3-TTS generator is not installed. Set TABURA_HOTWORD_QWEN3TTS_COMMAND to an executable." >&2
  exit 1
fi

exec "${TABURA_HOTWORD_QWEN3TTS_COMMAND}" "$@"

