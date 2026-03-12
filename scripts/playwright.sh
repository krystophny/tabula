#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MOUNT="/work"
CONTAINER_NAME="tabura-playwright"

# ── runtime detection ────────────────────────────────────────────────
# Check whether a container daemon is actually reachable, not just
# whether the CLI binary exists.  On macOS the daemon (Docker Desktop,
# OrbStack, colima …) may be installed but not running.
runtime_ready() {
  if command -v podman >/dev/null 2>&1 && podman info >/dev/null 2>&1; then
    RUNTIME=podman; return 0
  fi
  if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
    RUNTIME=docker; return 0
  fi
  return 1
}

# ── native fallback ──────────────────────────────────────────────────
# When no container daemon is reachable, run Playwright natively.
# Firefox/WebKit browsers may not be installed; Chromium usually is.
# Set PLAYWRIGHT_NATIVE=1 to force this path even when a daemon exists.
run_native() {
  echo "playwright.sh: running natively (no container daemon available)" >&2
  cd "${ROOT_DIR}"
  exec npx playwright test "$@"
}

if [[ "${PLAYWRIGHT_NATIVE:-}" == "1" ]]; then
  run_native "$@"
fi

if ! runtime_ready; then
  HAS_CLI=""
  command -v docker >/dev/null 2>&1 && HAS_CLI="docker"
  command -v podman >/dev/null 2>&1 && HAS_CLI="podman"
  if [[ -n "${HAS_CLI}" ]]; then
    echo "playwright.sh: ${HAS_CLI} CLI found but daemon is not running" >&2
    echo "  start your container runtime, or set PLAYWRIGHT_NATIVE=1 to run natively" >&2
  else
    echo "playwright.sh: no container runtime found" >&2
    echo "  install podman/docker, or set PLAYWRIGHT_NATIVE=1 to run natively" >&2
  fi
  exit 1
fi

# Kill any stale container from a previous interrupted run.
if "${RUNTIME}" inspect "${CONTAINER_NAME}" >/dev/null 2>&1; then
  "${RUNTIME}" rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
fi

PW_VERSION="$(node -e "console.log(require('${ROOT_DIR}/node_modules/@playwright/test/package.json').version)")"
IMAGE="mcr.microsoft.com/playwright:v${PW_VERSION}-noble"

rewrite() {
  local arg="$1"
  if [[ "${arg}" == --*="${ROOT_DIR}"* ]]; then
    local prefix="${arg%%=*}"
    local value="${arg#*=}"
    printf '%s=%s' "${prefix}" "${MOUNT}${value#"${ROOT_DIR}"}"
  elif [[ "${arg}" == "${ROOT_DIR}"* ]]; then
    printf '%s' "${MOUNT}${arg#"${ROOT_DIR}"}"
  else
    printf '%s' "${arg}"
  fi
}

ARGS=()
for arg in "$@"; do
  ARGS+=("$(rewrite "${arg}")")
done

ENV_FLAGS=()
[[ -n "${CI:-}" ]] && ENV_FLAGS+=(-e "CI=${CI}")
if [[ -n "${PLAYWRIGHT_HTML_REPORT:-}" ]]; then
  ENV_FLAGS+=(-e "PLAYWRIGHT_HTML_REPORT=$(rewrite "${PLAYWRIGHT_HTML_REPORT}")")
fi

# Clean up container on any exit (interrupt, kill, error).
cleanup() { "${RUNTIME}" rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM

"${RUNTIME}" run --rm --name "${CONTAINER_NAME}" --ipc=host \
  -v "${ROOT_DIR}:${MOUNT}" \
  -w "${MOUNT}" \
  "${ENV_FLAGS[@]+"${ENV_FLAGS[@]}"}" \
  "${IMAGE}" \
  npx playwright test "${ARGS[@]+"${ARGS[@]}"}"
