#!/usr/bin/env bash
set -euo pipefail

VOXTYPE_REPO="${VOXTYPE_REPO:-https://github.com/peteonrails/voxtype.git}"
VOXTYPE_BRANCH="${VOXTYPE_BRANCH:-feature/single-daemon-openai-stt-api}"
INSTALL_DIR="${VOXTYPE_INSTALL_DIR:-${HOME}/.local/bin}"
ASSUME_YES="${TABURA_ASSUME_YES:-0}"

log()  { printf '[build-voxtype] %s\n' "$*"; }
fail() { printf '[build-voxtype] ERROR: %s\n' "$*" >&2; exit 1; }

confirm_default_yes() {
    local prompt="$1"
    if [ "$ASSUME_YES" = "1" ]; then return 0; fi
    if [ ! -t 0 ]; then return 0; fi
    local response
    read -r -p "$prompt [Y/n] " response
    case "$response" in
        "" | [Yy] | [Yy][Ee][Ss]) return 0 ;;
        *) return 1 ;;
    esac
}

print_help() {
    cat <<USAGE
Usage: scripts/build-voxtype-macos.sh [options]

Builds voxtype from source on macOS.
Voxtype provides local OpenAI-compatible speech-to-text.
The pinned branch includes macOS support with Metal GPU acceleration.

Options:
  --yes       Non-interactive mode
  -h, --help  Show this help

Environment:
  VOXTYPE_REPO         Git clone URL (default: peteonrails/voxtype)
  VOXTYPE_BRANCH       Branch to build (default: feature/single-daemon-openai-stt-api)
  VOXTYPE_INSTALL_DIR  Binary install directory (default: ~/.local/bin)
  TABURA_ASSUME_YES=1  Same as --yes
USAGE
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --yes) ASSUME_YES=1; shift ;;
        -h|--help) print_help; exit 0 ;;
        *) fail "unknown argument: $1" ;;
    esac
done

# --- Prerequisites ---

[ "$(uname -s)" = "Darwin" ] || fail "this script is for macOS only"

command -v cargo >/dev/null 2>&1 || fail "cargo not found; install Rust: https://rustup.rs"
command -v cmake >/dev/null 2>&1 || fail "cmake not found; install: brew install cmake"
command -v git >/dev/null 2>&1 || fail "git not found"

# --- Clone ---

BUILD_DIR="$(mktemp -d -t voxtype-build-XXXXXX)"
trap 'rm -rf "$BUILD_DIR"' EXIT

log "Cloning $VOXTYPE_REPO (branch: $VOXTYPE_BRANCH)"
git clone --depth 1 --branch "$VOXTYPE_BRANCH" "$VOXTYPE_REPO" "$BUILD_DIR/voxtype"

cd "$BUILD_DIR/voxtype"

# --- Build ---

# macOS native crates (mac-notification-sys, core-graphics, etc.) require
# Apple Clang. Override CC/CXX if they point to non-Apple compilers (e.g.
# Homebrew gcc) which lack -mmacos-version-min and ObjC framework support.
if [ -n "${CC:-}" ]; then
    case "$CC" in
        *gcc*|*g++*)
            log "Overriding CC=$CC with /usr/bin/cc (Apple Clang required)"
            export CC=/usr/bin/cc
            export CXX=/usr/bin/c++
            ;;
    esac
fi

ARCH="$(uname -m)"
FEATURES=""
if [ "$ARCH" = "arm64" ]; then
    FEATURES="gpu-metal"
    log "Building with Metal GPU support (Apple Silicon)"
else
    log "Building without GPU acceleration (Intel Mac)"
fi

log "Building voxtype (this may take several minutes on first build)"
if [ -n "$FEATURES" ]; then
    cargo build --release --features "$FEATURES"
else
    cargo build --release
fi

# --- Install ---

mkdir -p "$INSTALL_DIR"
cp target/release/voxtype "$INSTALL_DIR/voxtype"
chmod +x "$INSTALL_DIR/voxtype"

log "Installed: $INSTALL_DIR/voxtype"

if ! printf ':%s:' "$PATH" | grep -Fq ":${INSTALL_DIR}:"; then
    log "WARNING: $INSTALL_DIR is not in PATH; add it to your shell profile"
fi

# --- Download model ---

if confirm_default_yes "Download voxtype model large-v3-turbo (~1.5 GB)?"; then
    "$INSTALL_DIR/voxtype" setup --download --model large-v3-turbo --no-post-install || {
        log "WARNING: model download failed; you can retry later with:"
        log "  voxtype setup --download --model large-v3-turbo --no-post-install"
    }
fi

log "Build complete. Verify with: voxtype --version"
