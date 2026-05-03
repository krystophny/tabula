#!/usr/bin/env bash
# sls-smoke.sh — interactive one-shot smoke against the live slopshell stack
# running on this machine. Not invoked from CI; for manual verification.
#
# Requires:
#   - slopshell-web.service   (port 8420)
#   - private Slopshell control socket (unix socket under $XDG_RUNTIME_DIR/sloppy)
#   - optional: codex-app-server.service (port 8787) for --gpt smoke
#
# Usage: scripts/sls-smoke.sh

set -euo pipefail

BASE_URL="${SLOPSHELL_BASE_URL:-http://127.0.0.1:8420}"
SLS_BIN="${SLS_BIN:-$(command -v sls || true)}"
RESULTS=()

if [ -z "$SLS_BIN" ]; then
  echo "sls not found in PATH. Run scripts/build-sls.sh first or set SLS_BIN." >&2
  exit 1
fi

if ! curl -fsS "${BASE_URL}/api/setup" >/dev/null; then
  echo "slopshell web not reachable at ${BASE_URL}; is slopshell-web.service running?" >&2
  exit 1
fi

run_case() {
  local label="$1"
  local expected="$2"
  shift 2
  printf '\n=== %s\n' "$label"
  local out
  if ! out="$("$SLS_BIN" --base-url "$BASE_URL" --no-color "$@" 2>&1)"; then
    RESULTS+=("FAIL  ${label} (sls exited non-zero)")
    printf '%s\n' "$out"
    return
  fi
  printf '%s\n' "$out"
  if [ -z "$expected" ] || printf '%s' "$out" | grep -Fq "$expected"; then
    RESULTS+=("PASS  ${label}")
  else
    RESULTS+=("FAIL  ${label} (no match for ${expected@Q})")
  fi
}

run_case "shell-echo via shell tool" "hello-from-sls" \
  -p "Use shell to run: echo hello-from-sls"

run_case "email accounts via sloptools MCP" "@" \
  -p "list my email accounts briefly"

if curl -fsS --max-time 1 http://127.0.0.1:8787/ >/dev/null 2>&1 \
   || ss -lnt 2>/dev/null | awk '{print $4}' | grep -Fq ':8787'; then
  run_case "GPT one-shot via codex app-server" "" \
    --gpt -p "Answer with the single word: pong"
else
  RESULTS+=("SKIP  GPT one-shot (codex-app-server not listening on :8787)")
fi

printf '\n---\n'
for row in "${RESULTS[@]}"; do
  printf '%s\n' "$row"
done

case " ${RESULTS[*]} " in
  *" FAIL "*) exit 1 ;;
esac
exit 0
