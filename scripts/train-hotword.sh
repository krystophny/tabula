#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TRAINER_DIR="${TABURA_HOTWORD_TRAINER_DIR:-$ROOT_DIR/tools/openwakeword-trainer}"
CONFIG_PATH="${TABURA_HOTWORD_CONFIG:-$ROOT_DIR/scripts/hotword-config.yaml}"
OUTPUT_DIR="${TABURA_HOTWORD_OUTPUT_DIR:-$ROOT_DIR/models/hotword}"
PYTHON_BIN="${PYTHON:-python3.12}"

if [[ ! -f "$CONFIG_PATH" ]]; then
  echo "hotword config not found: $CONFIG_PATH" >&2
  exit 1
fi

mkdir -p "$OUTPUT_DIR"

if [[ ! -d "$TRAINER_DIR/.git" ]]; then
  mkdir -p "$(dirname "$TRAINER_DIR")"
  git clone --depth=1 https://github.com/lgpearson1771/openwakeword-trainer "$TRAINER_DIR"
fi

VENV_DIR="$TRAINER_DIR/.venv"
if [[ ! -x "$VENV_DIR/bin/python" ]]; then
  "$PYTHON_BIN" -m venv "$VENV_DIR"
fi

"$VENV_DIR/bin/python" -m pip install --upgrade pip 'setuptools<82' wheel >/dev/null
# piper-phonemize has no wheels for Python >=3.12; use the community fix package.
"$VENV_DIR/bin/python" -m pip install piper-phonemize-fix >/dev/null
grep -v '^piper-phonemize' "$TRAINER_DIR/requirements.txt" \
  | "$VENV_DIR/bin/python" -m pip install -r /dev/stdin >/dev/null

TRAIN_CMD=("$VENV_DIR/bin/python" "$TRAINER_DIR/train_wakeword.py")

"${TRAIN_CMD[@]}" --config "$CONFIG_PATH" "$@"

MODEL_PATH="$OUTPUT_DIR/tabura.onnx"
if [[ ! -f "$MODEL_PATH" ]]; then
  echo "training finished but expected model missing: $MODEL_PATH" >&2
  exit 1
fi

echo "trained model: $MODEL_PATH"
