#!/usr/bin/env bash
# install_ollama.sh — Install Ollama and pull the models Career Agent Core needs.
#
# Supports:
#   - macOS (Homebrew, or points to the official .dmg)
#   - Debian / Ubuntu / Fedora / Arch / openSUSE and other systemd Linux distros
#   - Immutable/ostree distros (Bazzite, Silverblue, Kinoite, Aurora, SteamOS)
#     via a no-root user-space install with a systemd --user service
#   - WSL (treated as Linux; falls back to nohup if systemd is unavailable)
#   - Windows: run scripts/install_ollama.ps1 from PowerShell instead
#
# Usage:
#   ./scripts/install_ollama.sh              # auto-detect best install method
#   ./scripts/install_ollama.sh --user       # force user-space install (no sudo)
#   ./scripts/install_ollama.sh --system     # force system install (official script)
#   ./scripts/install_ollama.sh --no-models  # install only, skip model downloads
#
# Models pulled: read from .env in the repo root when present (OLLAMA_MODEL,
# OLLAMA_VISION_MODEL, OLLAMA_EMBED_MODEL, OLLAMA_HOST), else these installer
# defaults. A real environment variable of the same name always wins over
# .env (so `OLLAMA_MODEL=x ./scripts/install_ollama.sh` still works):
#   OLLAMA_MODEL        (default llama3.1)        text generation
#   OLLAMA_VISION_MODEL (default llava)           screenshot form mapping
#   OLLAMA_EMBED_MODEL  (default nomic-embed-text) embeddings / RAG
#
# After pulling (or with --no-models), the script confirms every configured
# model is actually present via GET /api/tags. Normally this is fatal on
# failure; with --no-models it's a warning, since skipping downloads was the
# point.

set -euo pipefail

log()  { printf '\033[1;36m[install]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[install]\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31m[install]\033[0m %s\n' "$*" >&2; exit 1; }

MODE="auto"        # auto | user | system
PULL_MODELS=true

for arg in "$@"; do
    case "$arg" in
        --user)      MODE="user" ;;
        --system)    MODE="system" ;;
        --no-models) PULL_MODELS=false ;;
        -h|--help)   grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
        *) echo "Unknown option: $arg (see --help)"; exit 1 ;;
    esac
done

# ---------------------------------------------------------------- .env

# Repo root relative to this script's own location (not $PWD), so
# ./scripts/install_ollama.sh and bash /abs/path/scripts/install_ollama.sh
# behave the same.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$REPO_ROOT/.env"

# Pull a single key's value out of .env without sourcing the file. .env is
# user-authored and can hold secrets (ANTHROPIC_API_KEY, IMAP_APP_PASSWORD);
# sourcing it would execute arbitrary shell and pull unrelated variables into
# this script's environment. Only ever call this with one of the four model
# keys below. Handles an optional "export " prefix, single- or double-quoted
# values, an unquoted trailing '# comment', and surrounding whitespace. Blank
# lines and '#'-commented lines never match the anchor, so they're skipped
# automatically -- which is what keeps .env.example's commented-out
# OLLAMA_MODEL recommendation from being read as configuration.
#
# The quote handling matches godotenv's (cmd/agent loads .env with it), so the
# installer and the agent resolve the same file to the same model names. That
# agreement is the entire point of bugs.md #441; a parser that disagreed with
# the agent's would just relocate the bug.
env_file_get() {
    local key="$1" line value
    [ -f "$ENV_FILE" ] || return 1
    line="$(grep -E "^[[:space:]]*(export[[:space:]]+)?${key}=" "$ENV_FILE" | tail -n1)" || return 1
    [ -n "$line" ] || return 1
    value="${line#*=}"
    value="${value#"${value%%[![:space:]]*}"}"
    case "$value" in
        # Quoted: take everything up to the closing quote, so a trailing
        # comment outside the quotes is dropped and a '#' inside them is kept.
        \"*) value="${value#\"}"; value="${value%%\"*}" ;;
        \'*) value="${value#\'}"; value="${value%%\'*}" ;;
        # Unquoted: a '#' begins a comment. No Ollama model name contains one.
        *)   value="${value%%#*}" ;;
    esac
    value="${value%"${value##*[![:space:]]}"}"
    printf '%s' "$value"
}

ENV_TEXT_MODEL="" ENV_VISION_MODEL="" ENV_EMBED_MODEL="" ENV_OLLAMA_HOST=""
if [ -f "$ENV_FILE" ]; then
    ENV_TEXT_MODEL="$(env_file_get OLLAMA_MODEL || true)"
    ENV_VISION_MODEL="$(env_file_get OLLAMA_VISION_MODEL || true)"
    ENV_EMBED_MODEL="$(env_file_get OLLAMA_EMBED_MODEL || true)"
    ENV_OLLAMA_HOST="$(env_file_get OLLAMA_HOST || true)"
fi

TEXT_MODEL="${OLLAMA_MODEL:-${ENV_TEXT_MODEL:-llama3.1}}"
VISION_MODEL="${OLLAMA_VISION_MODEL:-${ENV_VISION_MODEL:-llava}}"
EMBED_MODEL="${OLLAMA_EMBED_MODEL:-${ENV_EMBED_MODEL:-nomic-embed-text}}"
OLLAMA_URL="${OLLAMA_HOST:-${ENV_OLLAMA_HOST:-http://localhost:11434}}"

if [ -f "$ENV_FILE" ]; then
    log "Read .env (keys it leaves unset fall back to installer defaults): text=$TEXT_MODEL vision=$VISION_MODEL embed=$EMBED_MODEL"
else
    log "No .env found — using installer defaults: text=$TEXT_MODEL vision=$VISION_MODEL embed=$EMBED_MODEL. Copy .env.example to .env first to have this installer follow your configured models."
fi

# ---------------------------------------------------------------- OS detection

OS="$(uname -s)"
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) die "Unsupported architecture: $ARCH" ;;
esac

DISTRO=""
IS_OSTREE=false
IS_WSL=false
if [ "$OS" = "Linux" ]; then
    if [ -r /etc/os-release ]; then
        # shellcheck disable=SC1091
        DISTRO="$(. /etc/os-release && echo "${ID:-unknown}")"
    fi
    [ -e /run/ostree-booted ] && IS_OSTREE=true
    grep -qi microsoft /proc/version 2>/dev/null && IS_WSL=true
fi

case "$OS" in
    Linux)
        log "Detected Linux distro: ${DISTRO:-unknown} (arch: $ARCH)"
        $IS_OSTREE && log "Immutable (ostree) system detected — e.g. Bazzite/Silverblue/Kinoite."
        $IS_WSL && log "Running under WSL."
        ;;
    Darwin)
        log "Detected macOS (arch: $ARCH)"
        ;;
    MINGW*|MSYS*|CYGWIN*)
        die "On Windows, run scripts/install_ollama.ps1 from PowerShell instead."
        ;;
    *)
        die "Unsupported OS: $OS. See https://ollama.com/download"
        ;;
esac

# ---------------------------------------------------------------- install

have_ollama() { command -v ollama >/dev/null 2>&1; }

server_up() { curl -fsS --max-time 2 "$OLLAMA_URL/api/version" >/dev/null 2>&1; }

install_macos() {
    if command -v brew >/dev/null 2>&1; then
        log "Installing Ollama via Homebrew..."
        brew install ollama
        log "Starting Ollama as a background service..."
        brew services start ollama || warn "brew services failed; run 'ollama serve' manually."
    else
        die "Homebrew not found. Download Ollama from https://ollama.com/download/mac and re-run this script to pull models."
    fi
}

install_linux_system() {
    log "Installing Ollama via the official installer (requires sudo)..."
    curl -fsSL https://ollama.com/install.sh | sh
}

# Download an Ollama release bundle, preferring the current .tar.zst format
# with a .tgz fallback for older releases (mirrors the official installer).
fetch_bundle() {
    local name="$1" dest="$2"
    local base="https://ollama.com/download"
    if command -v zstd >/dev/null 2>&1 \
        && curl -fsIL --max-time 15 "$base/$name.tar.zst" >/dev/null 2>&1; then
        log "Downloading $base/$name.tar.zst ..."
        curl -fL --progress-bar "$base/$name.tar.zst" | zstd -d | tar -xf - -C "$dest"
    else
        log "Downloading $base/$name.tgz ..."
        curl -fL --progress-bar "$base/$name.tgz" | tar -xzf - -C "$dest"
    fi
}

install_linux_user() {
    local prefix="$HOME/.local/share/ollama"
    log "Installing Ollama into $prefix (no root required)..."
    mkdir -p "$prefix" "$HOME/.local/bin"

    fetch_bundle "ollama-linux-${ARCH}" "$prefix"

    # AMD GPUs need the additional ROCm runtime bundle
    if [ "$ARCH" = "amd64" ] && command -v lspci >/dev/null 2>&1 \
        && lspci -d ::0300 2>/dev/null | grep -qi 'AMD\|ATI'; then
        log "AMD GPU detected — downloading ROCm runtime bundle..."
        fetch_bundle "ollama-linux-${ARCH}-rocm" "$prefix" \
            || warn "ROCm bundle download failed; Ollama will fall back to CPU."
    fi

    # Release bundles have shipped both layouts: bin/ollama and ./ollama
    local ollama_bin
    if [ -x "$prefix/bin/ollama" ]; then
        ollama_bin="$prefix/bin/ollama"
    elif [ -x "$prefix/ollama" ]; then
        ollama_bin="$prefix/ollama"
    else
        die "Extraction succeeded but no ollama binary found under $prefix"
    fi
    ln -sf "$ollama_bin" "$HOME/.local/bin/ollama"
    case ":$PATH:" in
        *":$HOME/.local/bin:"*) ;;
        *) warn "~/.local/bin is not on your PATH — add it to your shell profile." ;;
    esac

    if command -v systemctl >/dev/null 2>&1 && systemctl --user show-environment >/dev/null 2>&1; then
        log "Creating systemd user service..."
        mkdir -p "$HOME/.config/systemd/user"
        cat > "$HOME/.config/systemd/user/ollama.service" <<EOF
[Unit]
Description=Ollama (user)
After=network-online.target

[Service]
ExecStart=$ollama_bin serve
Restart=always
RestartSec=3

[Install]
WantedBy=default.target
EOF
        systemctl --user daemon-reload
        systemctl --user enable --now ollama.service
    else
        warn "systemd user session unavailable — starting 'ollama serve' with nohup."
        nohup "$ollama_bin" serve >"$prefix/serve.log" 2>&1 &
    fi
}

if have_ollama; then
    log "Ollama binary already installed: $(command -v ollama)"
else
    case "$OS" in
        Darwin) install_macos ;;
        Linux)
            if [ "$MODE" = "user" ]; then
                install_linux_user
            elif [ "$MODE" = "system" ]; then
                install_linux_system
            elif $IS_OSTREE || ! sudo -n true 2>/dev/null; then
                # Immutable distro, or no passwordless sudo: user-space install
                # avoids any prompt and works on Bazzite/Silverblue out of the box.
                log "Using user-space install (pass --system to use the official sudo installer)."
                install_linux_user
            else
                install_linux_system
            fi
            ;;
    esac
fi

export PATH="$HOME/.local/bin:$PATH"

# ---------------------------------------------------------------- start server

if ! server_up; then
    log "Waiting for the Ollama server to come up at $OLLAMA_URL..."
    if command -v systemctl >/dev/null 2>&1; then
        systemctl --user start ollama.service 2>/dev/null \
            || sudo -n systemctl start ollama 2>/dev/null \
            || true
    fi
    for _ in $(seq 1 30); do
        server_up && break
        sleep 1
    done
    if ! server_up; then
        # Last resort: run the server directly in the background
        warn "Server not responding — starting 'ollama serve' directly."
        nohup ollama serve >/tmp/ollama-serve.log 2>&1 &
        for _ in $(seq 1 15); do
            server_up && break
            sleep 1
        done
    fi
fi

server_up || die "Ollama server did not start. Check 'ollama serve' output and re-run."
log "Ollama server is running: $(curl -fsS "$OLLAMA_URL/api/version")"

# ---------------------------------------------------------------- pull models

if $PULL_MODELS; then
    log "Pulling models (this downloads several GB on first run)..."
    for model in "$TEXT_MODEL" "$VISION_MODEL" "$EMBED_MODEL"; do
        log "  -> ollama pull $model"
        ollama pull "$model"
    done
else
    log "Skipping model downloads (--no-models). Pull later with:"
    log "  ollama pull $TEXT_MODEL && ollama pull $VISION_MODEL && ollama pull $EMBED_MODEL"
fi

# ---------------------------------------------------------------- verify models

# Ollama's /api/tags reports untagged pulls with an implicit ":latest" (a
# pulled "nomic-embed-text" shows up as "nomic-embed-text:latest"). Add the
# suffix to any bare name so both sides compare the same way.
normalize_model_name() {
    case "$1" in
        *:*) printf '%s' "$1" ;;
        *)   printf '%s:latest' "$1" ;;
    esac
}

# Confirm every model this script was configured to pull is actually present.
# fatal=true dies naming what's missing and the exact pull command to run;
# fatal=false (the --no-models path) only warns, since the user opted out.
verify_models() {
    local fatal="$1" model wanted found line m
    shift
    local raw installed missing=()
    # Fetch and extract separately. A reachable server with an empty library
    # returns {"models":[]}, which yields no names -- the same empty string as
    # an unreachable server, but a completely different problem with a
    # different fix, so the two must not share one message.
    if ! raw="$(curl -fsS --max-time 5 "$OLLAMA_URL/api/tags" 2>/dev/null)"; then
        if $fatal; then
            die "Could not reach $OLLAMA_URL/api/tags to verify models. Check 'ollama serve' output and re-run."
        fi
        warn "Could not reach $OLLAMA_URL/api/tags to verify models — skipping verification."
        return 0
    fi
    installed="$(printf '%s' "$raw" | grep -o '"name":"[^"]*"' | sed -E 's/"name":"([^"]*)"/\1/')" || installed=""
    if [ -z "$installed" ]; then
        warn "$OLLAMA_URL reports no installed models at all."
    fi
    for model in "$@"; do
        wanted="$(normalize_model_name "$model")"
        found=false
        while IFS= read -r line; do
            [ "$(normalize_model_name "$line")" = "$wanted" ] && { found=true; break; }
        done <<< "$installed"
        $found || missing+=("$model")
    done
    if [ "${#missing[@]}" -eq 0 ]; then
        log "Verified all configured models are present: $*"
        return 0
    fi
    if $fatal; then
        for m in "${missing[@]}"; do
            warn "  missing: $m  (run: ollama pull $m)"
        done
        die "Model verification failed after pulling — see missing models above."
    else
        warn "Model verification: these configured models are not installed (--no-models was used, so this is not fatal):"
        for m in "${missing[@]}"; do
            warn "  missing: $m  (run: ollama pull $m)"
        done
    fi
}

verify_models "$PULL_MODELS" "$TEXT_MODEL" "$VISION_MODEL" "$EMBED_MODEL"

log "Done. Career Agent Core will use Ollama automatically (LLM_PROVIDER defaults to 'ollama')."
