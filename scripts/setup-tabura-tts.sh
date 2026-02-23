#!/usr/bin/env bash
set -euo pipefail

# Auto-provision Chatterbox TTS server + reference voice.
# Everything goes into ~/.local/share/tabura-tts/
#
# Prerequisites: curl, ffmpeg, python3, git, cargo/rustc (for tokenizers)
# GPU: NVIDIA with CUDA 12.6+ (auto-detected; falls back to CPU)

TTS_DIR="${HOME}/.local/share/tabura-tts"
REFERENCE_WAV="${TTS_DIR}/reference.wav"
SERVER_DIR="${TTS_DIR}/server"
PREDEFINED_VOICES_DIR="${SERVER_DIR}/predefined_voices"
VENV_DIR="${SERVER_DIR}/venv"

LIBRIVOX_URL="https://www.archive.org/download/wings_and_child_1210_librivox/wingsandthechild_01_nesbit_64kb.mp3"
CHATTERBOX_REPO="https://github.com/devnen/Chatterbox-TTS-Server.git"

mkdir -p "$TTS_DIR"

# --- Step 1: Reference voice ---

if [ -f "$REFERENCE_WAV" ]; then
    echo "Reference voice already exists: $REFERENCE_WAV"
else
    echo "Downloading LibriVox reference audio..."
    TMP_MP3="$(mktemp /tmp/tabura-tts-ref-XXXXXX.mp3)"
    trap 'rm -f "$TMP_MP3"' EXIT

    curl -fsSL -o "$TMP_MP3" "$LIBRIVOX_URL"
    echo "Downloaded $(du -h "$TMP_MP3" | cut -f1) reference audio."

    # Find first silence boundary after ~8s using silencedetect
    SILENCE_END=$(ffmpeg -i "$TMP_MP3" -af silencedetect=noise=-30dB:d=0.3 -f null - 2>&1 \
        | grep 'silence_end' \
        | awk '{ for(i=1;i<=NF;i++) if($i=="silence_end:") print $(i+1) }' \
        | awk '$1 >= 8 { print $1; exit }')

    if [ -z "$SILENCE_END" ]; then
        SILENCE_END="10.0"
        echo "No silence boundary found after 8s; using ${SILENCE_END}s cutoff."
    else
        echo "Cutting at silence boundary: ${SILENCE_END}s"
    fi

    ffmpeg -y -i "$TMP_MP3" -t "$SILENCE_END" -ar 24000 -ac 1 -acodec pcm_s16le "$REFERENCE_WAV" 2>/dev/null
    echo "Reference voice: $REFERENCE_WAV ($(du -h "$REFERENCE_WAV" | cut -f1))"
    rm -f "$TMP_MP3"
    trap - EXIT
fi

# --- Step 2: Clone Chatterbox TTS Server ---

if [ -d "$SERVER_DIR" ]; then
    echo "Chatterbox TTS Server already cloned: $SERVER_DIR"
else
    echo "Cloning Chatterbox TTS Server..."
    git clone "$CHATTERBOX_REPO" "$SERVER_DIR"
fi

# --- Step 3: Python venv + dependencies ---

if [ -d "$VENV_DIR" ] && "$VENV_DIR/bin/pip" show chatterbox-tts >/dev/null 2>&1; then
    echo "Python venv already provisioned: $VENV_DIR"
else
    echo "Creating Python venv and installing dependencies..."
    python3 -m venv "$VENV_DIR"
    # shellcheck disable=SC1091
    source "${VENV_DIR}/bin/activate"
    pip install --upgrade pip

    # Install CUDA PyTorch (cu126 for Blackwell+ GPUs); fall back to CPU
    if command -v nvidia-smi >/dev/null 2>&1; then
        echo "NVIDIA GPU detected, installing CUDA PyTorch..."
        pip install torch torchaudio --index-url https://download.pytorch.org/whl/cu126
    else
        echo "No NVIDIA GPU, installing CPU PyTorch..."
        pip install torch torchaudio --index-url https://download.pytorch.org/whl/cpu
    fi

    # Install chatterbox-tts (no-deps to avoid version-pinned torch conflict)
    pip install --no-deps chatterbox-tts@git+https://github.com/devnen/chatterbox-v2.git@master

    # Install remaining server + chatterbox dependencies
    pip install \
        fastapi 'uvicorn[standard]' python-multipart \
        numpy librosa safetensors pydub \
        descript-audio-codec \
        transformers tokenizers \
        conformer==0.3.2 diffusers==0.29.0 resemble-perth==1.0.1 \
        omegaconf pykakasi==2.3.0 s3tokenizer spacy-pkuseg

    deactivate
    echo "Dependencies installed."
fi

# --- Step 4: Copy reference voice ---

mkdir -p "$PREDEFINED_VOICES_DIR"
if [ ! -f "${PREDEFINED_VOICES_DIR}/tabura-default.wav" ]; then
    cp "$REFERENCE_WAV" "${PREDEFINED_VOICES_DIR}/tabura-default.wav"
    echo "Copied reference voice to predefined_voices/tabura-default.wav"
else
    echo "Predefined voice already exists."
fi

# --- Done ---

echo ""
echo "=== Tabura TTS Setup Complete ==="
echo "  TTS dir:        $TTS_DIR"
echo "  Reference voice: $REFERENCE_WAV"
echo "  Server dir:     $SERVER_DIR"
echo ""
echo "Next steps:"
echo "  1. Run: scripts/install-tabura-user-units.sh"
echo "  2. systemctl --user start tabura-tts.service"
echo "  3. Test (first call downloads model weights ~1 GB):"
echo "     curl -X POST http://127.0.0.1:8423/v1/audio/speech \\"
echo "       -H 'Content-Type: application/json' \\"
echo "       -d '{\"model\":\"chatterbox\",\"input\":\"Hello\",\"voice\":\"tabura-default\",\"response_format\":\"wav\"}' > /tmp/test.wav"
