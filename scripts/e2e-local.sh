#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# ---------------------------------------------------------------------------
# Pre-flight: all required services must be running
# ---------------------------------------------------------------------------

fail() { printf 'FATAL: %s\n' "$1" >&2; exit 1; }

printf 'Checking services...\n'

curl -fsS --max-time 3 http://127.0.0.1:8420/api/setup >/dev/null \
  || fail 'Tabura web server not running on :8420'

curl -fsS --max-time 3 -o /dev/null -w '' \
  -X POST http://127.0.0.1:8424/v1/audio/speech \
  -H 'Content-Type: application/json' \
  -d '{"input":"ok","voice":"en","response_format":"wav"}' \
  || fail 'Piper TTS not running on :8424'

curl -fsS --max-time 3 http://127.0.0.1:8427/healthz >/dev/null \
  || fail 'voxtype STT not running on :8427'

command -v ffmpeg >/dev/null 2>&1 \
  || fail 'ffmpeg not installed'

printf 'All services OK.\n'

# ---------------------------------------------------------------------------
# Generate speech WAV via Piper and pad with silence for VAD offset detection
# ---------------------------------------------------------------------------

SPEECH_WAV="/tmp/tabura-e2e-speech-raw.wav"
PADDED_WAV="/tmp/tabura-e2e-speech.wav"

printf 'Generating speech WAV via Piper TTS...\n'
curl -sS -X POST http://127.0.0.1:8424/v1/audio/speech \
  -H 'Content-Type: application/json' \
  -d '{"input":"Hello, this is a test of voice recording.","voice":"en","response_format":"wav"}' \
  -o "$SPEECH_WAV"

# Pad: 0.5s silence before speech + speech + 3s silence after (VAD needs silence to auto-stop)
ffmpeg -hide_banner -loglevel error -nostdin -y \
  -f lavfi -t 0.5 -i anullsrc=r=22050:cl=mono \
  -i "$SPEECH_WAV" \
  -f lavfi -t 3 -i anullsrc=r=22050:cl=mono \
  -filter_complex '[0][1][2]concat=n=3:v=0:a=1[out]' \
  -map '[out]' -ar 22050 -ac 1 -c:a pcm_s16le "$PADDED_WAV"

printf 'Audio ready: %s\n' "$PADDED_WAV"

# ---------------------------------------------------------------------------
# Run Playwright E2E tests
# ---------------------------------------------------------------------------

export E2E_AUDIO_FILE="$PADDED_WAV"
cd "$ROOT_DIR"
exec npx playwright test --config=playwright.e2e.config.ts "$@"
