#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MOUNT="/work"
CONTAINER_NAME="tabura-playwright"

if command -v podman >/dev/null 2>&1; then
  RUNTIME=podman
elif command -v docker >/dev/null 2>&1; then
  RUNTIME=docker
else
  echo "playwright.sh: podman or docker required" >&2
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
