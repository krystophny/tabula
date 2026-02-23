#!/usr/bin/env bash
set -euo pipefail

# Auto-provision Piper TTS server with English + German voices.
# Everything goes into ~/.local/share/tabura-piper-tts/
#
# Prerequisites: curl, python3 (3.10+)
# GPU not required - Piper uses ONNX and runs ~100x realtime on CPU.

PIPER_DIR="${HOME}/.local/share/tabura-piper-tts"
MODEL_DIR="${PIPER_DIR}/models"
VENV_DIR="${PIPER_DIR}/venv"
SERVER_SCRIPT="$(cd "$(dirname "$0")" && pwd)/piper_tts_server.py"

HF_BASE="https://huggingface.co/rhasspy/piper-voices/resolve/main"

declare -A MODELS=(
    ["en_GB-alan-medium"]="en/en_GB/alan/medium"
    ["de_DE-karlsson-low"]="de/de_DE/karlsson/low"
)

mkdir -p "$MODEL_DIR"

# --- Step 1: Download voice models ---

for model in "${!MODELS[@]}"; do
    subpath="${MODELS[$model]}"
    onnx="${MODEL_DIR}/${model}.onnx"
    json="${MODEL_DIR}/${model}.onnx.json"

    if [ -f "$onnx" ] && [ -f "$json" ]; then
        echo "Model already exists: $model"
        continue
    fi

    echo "Downloading model: $model ..."
    curl -fsSL -o "$onnx" "${HF_BASE}/${subpath}/${model}.onnx"
    curl -fsSL -o "$json" "${HF_BASE}/${subpath}/${model}.onnx.json"
    echo "  $(du -h "$onnx" | cut -f1) $onnx"
done

# --- Step 2: Python venv + dependencies ---

if [ -d "$VENV_DIR" ] && "$VENV_DIR/bin/pip" show piper-tts >/dev/null 2>&1; then
    echo "Python venv already provisioned: $VENV_DIR"
else
    echo "Creating Python venv and installing dependencies..."
    python3 -m venv "$VENV_DIR"
    # shellcheck disable=SC1091
    source "${VENV_DIR}/bin/activate"
    pip install --upgrade pip

    pip install piper-tts fastapi 'uvicorn[standard]'

    deactivate
    echo "Dependencies installed."
fi

# --- Done ---

echo ""
echo "=== Piper TTS Setup Complete ==="
echo "  Install dir: $PIPER_DIR"
echo "  Models:      $MODEL_DIR"
echo "  Venv:        $VENV_DIR"
echo "  Server:      $SERVER_SCRIPT"
echo ""
echo "Next steps:"
echo "  1. Run: scripts/install-tabura-user-units.sh"
echo "  2. systemctl --user start tabura-piper-tts.service"
echo "  3. Test:"
echo "     curl -X POST http://127.0.0.1:8424/v1/audio/speech \\"
echo "       -H 'Content-Type: application/json' \\"
echo "       -d '{\"input\":\"Hello world\",\"voice\":\"en\"}' > /tmp/test.wav"
echo "     aplay /tmp/test.wav"
