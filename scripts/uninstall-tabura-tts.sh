#!/usr/bin/env bash
set -euo pipefail

# Uninstall Chatterbox TTS server.
# Stops the service, removes systemd unit, deletes all data.

TTS_DIR="${HOME}/.local/share/tabura-tts"
UNIT_DST="${HOME}/.config/systemd/user"

echo "=== Tabura TTS Uninstall ==="

# Stop and disable the service
if systemctl --user is-active tabura-tts.service >/dev/null 2>&1; then
    echo "Stopping tabura-tts.service..."
    systemctl --user stop tabura-tts.service
fi
if systemctl --user is-enabled tabura-tts.service >/dev/null 2>&1; then
    echo "Disabling tabura-tts.service..."
    systemctl --user disable tabura-tts.service
fi

# Remove systemd unit
if [ -f "${UNIT_DST}/tabura-tts.service" ]; then
    rm "${UNIT_DST}/tabura-tts.service"
    systemctl --user daemon-reload
    echo "Removed systemd unit."
fi

# Remove HuggingFace cached model weights
HF_CACHE="${HOME}/.cache/huggingface/hub"
for d in "$HF_CACHE"/models--ResembleAI--chatterbox*; do
    if [ -d "$d" ]; then
        echo "Removing cached model weights: $d"
        rm -rf "$d"
    fi
done

# Remove TTS data directory (server, venv, reference voice)
if [ -d "$TTS_DIR" ]; then
    SIZE=$(du -sh "$TTS_DIR" 2>/dev/null | cut -f1)
    echo "Removing TTS directory ($SIZE): $TTS_DIR"
    rm -rf "$TTS_DIR"
else
    echo "TTS directory not found: $TTS_DIR"
fi

echo ""
echo "Tabura TTS uninstalled."
echo "To reinstall: bash scripts/setup-tabura-tts.sh"
