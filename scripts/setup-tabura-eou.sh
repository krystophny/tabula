#!/usr/bin/env bash
set -euo pipefail

# Auto-provision semantic EOU sidecar (LiveKit turn-detector).
# Installs a local Python venv and downloads model artifacts into:
#   ~/.local/share/tabura-eou/

EOU_DIR="${HOME}/.local/share/tabura-eou"
MODEL_DIR="${EOU_DIR}/model"
VENV_DIR="${EOU_DIR}/venv"
MODEL_REPO="${TABURA_EOU_MODEL_REPO:-livekit/turn-detector}"

mkdir -p "$MODEL_DIR"

if [ -d "$VENV_DIR" ] && "$VENV_DIR/bin/python" -c 'import onnxruntime, fastapi, transformers, huggingface_hub' >/dev/null 2>&1; then
    echo "Python venv already provisioned: $VENV_DIR"
else
    echo "Creating Python venv and installing dependencies..."
    python3 -m venv "$VENV_DIR"
    # shellcheck disable=SC1091
    source "${VENV_DIR}/bin/activate"
    pip install --upgrade pip
    if command -v nvidia-smi >/dev/null 2>&1; then
        pip install onnxruntime-gpu
    else
        pip install onnxruntime
    fi
    pip install fastapi 'uvicorn[standard]' transformers huggingface_hub numpy
    deactivate
fi

echo "Downloading EOU model snapshot: $MODEL_REPO"
"$VENV_DIR/bin/python" - <<PY
from huggingface_hub import snapshot_download
snapshot_download(
    repo_id="${MODEL_REPO}",
    local_dir="${MODEL_DIR}",
    local_dir_use_symlinks=False,
)
print("model snapshot ready at ${MODEL_DIR}")
PY

echo ""
echo "=== Tabura EOU Setup Complete ==="
echo "  Install dir: $EOU_DIR"
echo "  Model dir:   $MODEL_DIR"
echo "  Venv:        $VENV_DIR"
echo ""
echo "Next steps:"
echo "  1. Run: scripts/install-tabura-user-units.sh"
echo "  2. systemctl --user start tabura-eou.service"
echo "  3. Health: curl http://127.0.0.1:8425/health"

