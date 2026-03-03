#!/usr/bin/env bash
set -euo pipefail

# Download VAD + ONNX runtime assets for local serving.
# Files go to internal/web/static/vad/ (gitignored).
# Run once after clone or when upgrading versions.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST="${ROOT_DIR}/internal/web/static/vad"

VAD_VERSION="0.0.30"
ORT_VERSION="1.22.0"
VAD_CDN="https://cdn.jsdelivr.net/npm/@ricky0123/vad-web@${VAD_VERSION}/dist"
ORT_CDN="https://cdn.jsdelivr.net/npm/onnxruntime-web@${ORT_VERSION}/dist"

mkdir -p "${DEST}"

echo "Fetching vad-web@${VAD_VERSION} + onnxruntime-web@${ORT_VERSION} -> ${DEST}"

curl -fsSL -o "${DEST}/bundle.min.js"                "${VAD_CDN}/bundle.min.js"
curl -fsSL -o "${DEST}/silero_vad_v5.onnx"           "${VAD_CDN}/silero_vad_v5.onnx"
curl -fsSL -o "${DEST}/vad.worklet.bundle.min.js"    "${VAD_CDN}/vad.worklet.bundle.min.js"
curl -fsSL -o "${DEST}/ort.min.mjs"                   "${ORT_CDN}/ort.min.mjs"
curl -fsSL -o "${DEST}/ort-wasm-simd-threaded.mjs"   "${ORT_CDN}/ort-wasm-simd-threaded.mjs"
curl -fsSL -o "${DEST}/ort-wasm-simd-threaded.wasm"  "${ORT_CDN}/ort-wasm-simd-threaded.wasm"

echo "Done. $(du -sh "${DEST}" | cut -f1) total."
