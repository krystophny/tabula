#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TRAINER_DIR="${TABURA_HOTWORD_TRAINER_DIR:-$ROOT_DIR/tools/openwakeword-trainer}"
CONFIG_PATH="${TABURA_HOTWORD_CONFIG:-$ROOT_DIR/scripts/hotword-config.yaml}"
OUTPUT_DIR="${TABURA_HOTWORD_OUTPUT_DIR:-$ROOT_DIR/models/hotword}"
SAMPLES_DIR="${TABURA_HOTWORD_SAMPLES_DIR:-$ROOT_DIR/data/hotword-samples}"
PYTHON_BIN="${PYTHON:-python3}"

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

"$VENV_DIR/bin/python" -m pip install --upgrade pip setuptools wheel >/dev/null
"$VENV_DIR/bin/python" -m pip install -e "$TRAINER_DIR" >/dev/null

TRAIN_CMD=()
if [[ -f "$TRAINER_DIR/train.py" ]]; then
  TRAIN_CMD=("$VENV_DIR/bin/python" "$TRAINER_DIR/train.py")
elif [[ -f "$TRAINER_DIR/scripts/train.py" ]]; then
  TRAIN_CMD=("$VENV_DIR/bin/python" "$TRAINER_DIR/scripts/train.py")
elif [[ -f "$TRAINER_DIR/openwakeword_trainer/train.py" ]]; then
  TRAIN_CMD=("$VENV_DIR/bin/python" -m openwakeword_trainer.train)
else
  echo "unable to locate trainer entrypoint inside $TRAINER_DIR" >&2
  exit 1
fi

EXTRA_ARGS=()
if [[ -d "$SAMPLES_DIR" ]]; then
  EXTRA_ARGS+=(--real-samples-dir "$SAMPLES_DIR")
fi

"${TRAIN_CMD[@]}" --config "$CONFIG_PATH" "${EXTRA_ARGS[@]}" "$@"

MODEL_PATH="$OUTPUT_DIR/hey_tabura.onnx"
if [[ ! -f "$MODEL_PATH" ]]; then
  echo "training finished but expected model missing: $MODEL_PATH" >&2
  exit 1
fi

echo "trained model: $MODEL_PATH"
