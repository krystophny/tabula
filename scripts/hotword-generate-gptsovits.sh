#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${TABURA_HOTWORD_GPTSOVITS_COMMAND:-}" ]]; then
  echo "GPT-SoVITS generator is not installed. Set TABURA_HOTWORD_GPTSOVITS_COMMAND to an executable." >&2
  exit 1
fi

exec "${TABURA_HOTWORD_GPTSOVITS_COMMAND}" "$@"

