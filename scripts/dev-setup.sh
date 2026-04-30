#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PLATFORM="$(uname -s)"
ARCH="$(uname -m)"
ASSUME_YES="${SLOPSHELL_ASSUME_YES:-0}"
# shellcheck source=scripts/lib/llama.sh
source "${REPO_ROOT}/scripts/lib/llama.sh"
# shellcheck source=scripts/lib/python.sh
source "${REPO_ROOT}/scripts/lib/python.sh"

log()  { printf '[dev-setup] %s\n' "$*"; }
warn() { printf '[dev-setup] WARNING: %s\n' "$*"; }
fail() { printf '[dev-setup] ERROR: %s\n' "$*" >&2; exit 1; }

codex_local_url_default() {
    if [ "$PLATFORM" = "Darwin" ]; then
        printf '%s' "http://127.0.0.1:8081/v1"
        return
    fi
    printf '%s' "http://127.0.0.1:8080/v1"
}

print_help() {
    cat <<USAGE
Usage: scripts/dev-setup.sh [options]

Sets up a complete Slopshell development environment from a repo checkout.

Steps performed:
  1. Detect platform and architecture
  2. Build slopshell binary from source
  3. Set up Piper TTS (venv + voice models)
  4. Detect existing local LLM or prepare LLM service
  5. Check for voxtype (STT)
  6. Install and start service definitions (systemd/launchd)
  7. Bootstrap a default project directory
  8. Print summary with endpoints and log locations

Options:
  --yes       Non-interactive mode (answer yes to all prompts)
  -h, --help  Show this help

Environment:
  SLOPSHELL_ASSUME_YES=1         Same as --yes
  SLOPSHELL_INTENT_LLM_URL=<url> Reuse an existing local LLM (skip LLM setup)
  SLOPSHELL_INTENT_LLM_URL=off   Disable intent LLM entirely
USAGE
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --yes) ASSUME_YES=1; shift ;;
        -h|--help) print_help; exit 0 ;;
        *) fail "unknown argument: $1" ;;
    esac
done

export SLOPSHELL_ASSUME_YES="$ASSUME_YES"

# --- Step 1: Platform detection ---

case "$PLATFORM" in
    Linux|Darwin) ;;
    *) fail "unsupported platform: $PLATFORM" ;;
esac
log "Platform: $PLATFORM ($ARCH)"

# --- Step 2: Required prerequisites ---

command -v go >/dev/null 2>&1 || fail "go not found (https://go.dev/dl/)"
command -v curl >/dev/null 2>&1 || fail "curl not found"
command -v codex >/dev/null 2>&1 || fail "codex not found (npm install -g @openai/codex)"
if [ "$PLATFORM" = "Darwin" ]; then
    slopshell_find_python3 3 10 >/dev/null 2>&1 || fail "python3 3.10+ not found"
else
    command -v python3 >/dev/null 2>&1 || fail "python3 not found"
fi

# --- Step 3: Build slopshell binary ---

BIN_DIR="${HOME}/.local/bin"
BIN_PATH="${BIN_DIR}/slopshell"
log "Building slopshell binary"
mkdir -p "$BIN_DIR"
(cd "$REPO_ROOT" && go build -o "$BIN_PATH" ./cmd/slopshell)
log "Built: $BIN_PATH"

if ! printf ':%s:' "$PATH" | grep -Fq ":${BIN_DIR}:"; then
    warn "${BIN_DIR} is not in PATH; add it to your shell profile"
fi

# --- Step 4: Piper TTS setup ---

log "Setting up Piper TTS"
if "$REPO_ROOT/scripts/setup-slopshell-piper-tts.sh"; then
    log "Piper TTS setup complete"
else
    warn "Piper TTS setup failed; TTS will be unavailable"
fi

# --- Step 5: LLM detection ---

if [ -z "${SLOPSHELL_INTENT_LLM_URL:-}" ]; then
    for port in 8081 8080; do
        if curl -fsS --max-time 2 "http://127.0.0.1:${port}/health" >/dev/null 2>&1; then
            export SLOPSHELL_INTENT_LLM_URL="http://127.0.0.1:${port}"
            log "Detected existing local LLM at $SLOPSHELL_INTENT_LLM_URL"
            break
        fi
    done
fi

if [ -z "${SLOPSHELL_INTENT_LLM_URL:-}" ]; then
    if [ "$PLATFORM" = "Darwin" ]; then
        export SLOPSHELL_INTENT_LLM_URL="http://127.0.0.1:8081"
    elif ! LLAMA_SERVER_BIN="$(slopshell_find_llama_server)"; then
        if [ -n "${SLOPSHELL_LLAMA_LAST_ERROR:-}" ]; then
            warn "llama-server not usable (${SLOPSHELL_LLAMA_LAST_ERROR}); intent LLM will be disabled"
        else
            warn "llama-server not found; intent LLM will be disabled"
        fi
        if [ "$PLATFORM" != "Darwin" ]; then
            warn "  Build llama.cpp and place llama-server in ~/.local/bin"
        fi
        export SLOPSHELL_INTENT_LLM_URL="off"
    else
        export LLAMA_SERVER_BIN
    fi
fi

# --- Step 6: STT (voxtype) check ---

if ! command -v voxtype >/dev/null 2>&1; then
    if [ "$PLATFORM" = "Darwin" ]; then
        command -v cargo >/dev/null 2>&1 || fail "cargo not found; install Rust with Homebrew: brew install rust"
        command -v cmake >/dev/null 2>&1 || fail "cmake not found; install: brew install cmake"
        log "Building voxtype from source for macOS"
        "$REPO_ROOT/scripts/build-voxtype-macos.sh" --yes
        log "voxtype built and installed successfully"
    else
        command -v voxtype >/dev/null 2>&1 || fail "voxtype not found; install: paru -S voxtype-bin"
    fi
fi

# --- Step 7: Install service definitions and start services ---

log "Installing and starting services"
"$REPO_ROOT/scripts/install-slopshell-user-units.sh"

# --- Step 8: Bootstrap default project ---

PROJECT_DIR="$REPO_ROOT"
log "Bootstrapping project at $PROJECT_DIR"
"$BIN_PATH" bootstrap --workspace-dir "$PROJECT_DIR"

# --- Step 9: Configure Codex MCP + local provider profiles ---

if [ -n "${SLOPSHELL_INTENT_LLM_URL:-}" ] && [ "${SLOPSHELL_INTENT_LLM_URL}" != "off" ]; then
    SLOPSHELL_CODEX_FAST_URL="${SLOPSHELL_INTENT_LLM_URL}/v1" \
    SLOPSHELL_CODEX_LOCAL_URL="$(codex_local_url_default)" \
    "$REPO_ROOT/scripts/setup-codex-mcp.sh"
else
    "$REPO_ROOT/scripts/setup-codex-mcp.sh"
fi

# --- Step 10: Summary ---

EFFECTIVE_LLM_URL="${SLOPSHELL_INTENT_LLM_URL:-http://127.0.0.1:8081}"
cat <<SUMMARY

=== Slopshell Dev Setup Complete ===
  Platform:    $PLATFORM ($ARCH)
  Binary:      $BIN_PATH
  Repo root:   $REPO_ROOT
  Project dir: $PROJECT_DIR

Endpoints:
  Web UI:  http://127.0.0.1:8420
  MCP:     unix:${XDG_RUNTIME_DIR:-$HOME/.cache}/sloppy/mcp.sock
  TTS:     http://127.0.0.1:8424/v1/audio/speech
  LLM:     $EFFECTIVE_LLM_URL
  Codex:   $(codex_local_url_default)
  STT:     http://127.0.0.1:8427/v1/audio/transcriptions

SUMMARY

if [ "$PLATFORM" = "Darwin" ]; then
    cat <<LOGS
Log files:
  /tmp/slopshell-web.log
  /tmp/slopshell-codex-app-server.log
  /tmp/slopshell-piper-tts.log
  /tmp/slopshell-llm.log
  /tmp/slopshell-stt.log
LOGS
else
    cat <<LOGS
Logs:
  journalctl --user -u slopshell-web.service
  journalctl --user -u slopshell-codex-app-server.service
  journalctl --user -u slopshell-piper-tts.service
  journalctl --user -u slopshell-llm.service
  journalctl --user -u slopshell-stt.service
LOGS
fi
