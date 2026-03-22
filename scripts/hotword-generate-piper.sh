#!/usr/bin/env bash
set -euo pipefail

RECORDINGS_DIR=""
OUTPUT_DIR=""
COUNT="250"
MODEL_ID="piper"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --recordings-dir)
      RECORDINGS_DIR="$2"
      shift 2
      ;;
    --output-dir)
      OUTPUT_DIR="$2"
      shift 2
      ;;
    --count)
      COUNT="$2"
      shift 2
      ;;
    --model-id)
      MODEL_ID="$2"
      shift 2
      ;;
    *)
      echo "unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

if [[ -z "$RECORDINGS_DIR" || -z "$OUTPUT_DIR" ]]; then
  echo "recordings dir and output dir are required" >&2
  exit 1
fi

mapfile -t SOURCES < <(find "$RECORDINGS_DIR" -maxdepth 1 -type f -name '*.wav' | sort)
if [[ ${#SOURCES[@]} -eq 0 ]]; then
  echo "no wav recordings found under $RECORDINGS_DIR" >&2
  exit 1
fi

mkdir -p "$OUTPUT_DIR"
TOTAL=$(( COUNT > 0 ? COUNT : 1 ))

for ((i = 0; i < TOTAL; i++)); do
  src="${SOURCES[$(( i % ${#SOURCES[@]} ))]}"
  dest="$OUTPUT_DIR/${MODEL_ID}-$(printf '%04d' "$((i + 1))").wav"
  if command -v ffmpeg >/dev/null 2>&1; then
    rate=$(( 16000 + ((i % 5) - 2) * 250 ))
    tempo=$(awk "BEGIN { printf \"%.3f\", 1 + (($i % 5) - 2) * 0.03 }")
    ffmpeg -loglevel error -y -i "$src" -ac 1 -ar 16000 -filter:a "asetrate=${rate},aresample=16000,atempo=${tempo}" "$dest"
  else
    cp "$src" "$dest"
  fi
  echo "generated $((i + 1))/$TOTAL: $(basename "$dest")"
done

cat >"$OUTPUT_DIR/manifest.json" <<JSON
{
  "model": "$MODEL_ID",
  "count": $TOTAL
}
JSON

