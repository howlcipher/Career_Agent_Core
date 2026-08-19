#!/usr/bin/env bash
# install_slopslint.sh
# Deterministically installs or verifies the pinned slopslint binary (v0.1.0).

set -euo pipefail

PINNED_VERSION="0.1.0"
INSTALL_DIR="${HOME}/.local/bin"
mkdir -p "${INSTALL_DIR}"

if command -v slopslint >/dev/null 2>&1; then
    CURRENT_VER=$(slopslint --version | awk '{print $2}')
    if [ "${CURRENT_VER}" = "${PINNED_VERSION}" ]; then
        echo "slopslint v${PINNED_VERSION} is already installed at $(which slopslint)."
        exit 0
    fi
fi

if [ -f "${INSTALL_DIR}/slopslint" ]; then
    chmod +x "${INSTALL_DIR}/slopslint"
    VER=$("${INSTALL_DIR}/slopslint" --version 2>/dev/null | awk '{print $2}' || true)
    if [ "${VER}" = "${PINNED_VERSION}" ]; then
        echo "slopslint verified at ${INSTALL_DIR}/slopslint"
        exit 0
    fi
fi

# Detect platform and architecture
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "${OS}" in
    linux)
        case "${ARCH}" in
            x86_64|amd64)
                ASSET_NAME="slopslint-linux-x64"
                EXPECTED_SHA="0db7c2d06b8d72d7feedf218580fab7b672cfc7784c0d78546f743575799f75a"
                ;;
            aarch64|arm64)
                ASSET_NAME="slopslint-linux-arm64"
                EXPECTED_SHA="3da51e2826d32d0f9e849450e80ac1b2e32c0139ee12f41f8f653c93479ea629"
                ;;
            *)
                echo "Unsupported Linux architecture: ${ARCH}" >&2
                exit 1
                ;;
        esac
        ;;
    darwin)
        case "${ARCH}" in
            x86_64|amd64)
                ASSET_NAME="slopslint-darwin-x64"
                EXPECTED_SHA="868ff3c228e76bbf70472344342b259fc421a61560c6ae3073690ad124b4a8a3"
                ;;
            arm64|aarch64)
                ASSET_NAME="slopslint-darwin-arm64"
                EXPECTED_SHA="357c49feb1586572a077978a7938906c15336b1bce1b60c5785258466c77f06b"
                ;;
            *)
                echo "Unsupported Darwin architecture: ${ARCH}" >&2
                exit 1
                ;;
        esac
        ;;
    *)
        echo "Unsupported OS: ${OS}" >&2
        exit 1
        ;;
esac

DOWNLOAD_URL="https://github.com/thellmwhisperer/slopslint/releases/download/v${PINNED_VERSION}/${ASSET_NAME}"
TEMP_BIN="$(mktemp "${TMPDIR:-/tmp}/slopslint.XXXXXX")"
trap 'rm -f "${TEMP_BIN}"' EXIT

echo "Downloading ${ASSET_NAME} from ${DOWNLOAD_URL}..."
if command -v curl >/dev/null 2>&1; then
    curl -fsSL "${DOWNLOAD_URL}" -o "${TEMP_BIN}"
elif command -v wget >/dev/null 2>&1; then
    wget -qO "${TEMP_BIN}" "${DOWNLOAD_URL}"
else
    echo "Neither curl nor wget is available for downloading slopslint" >&2
    exit 1
fi

# Verify SHA256 checksum
if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL_SHA=$(sha256sum "${TEMP_BIN}" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
    ACTUAL_SHA=$(shasum -a 256 "${TEMP_BIN}" | awk '{print $1}')
else
    echo "Cannot verify sha256: neither sha256sum nor shasum found" >&2
    exit 1
fi

if [ "${ACTUAL_SHA}" != "${EXPECTED_SHA}" ]; then
    echo "Checksum mismatch for ${ASSET_NAME}: expected ${EXPECTED_SHA}, got ${ACTUAL_SHA}" >&2
    exit 1
fi

mv "${TEMP_BIN}" "${INSTALL_DIR}/slopslint"
chmod +x "${INSTALL_DIR}/slopslint"
trap - EXIT

echo "slopslint v${PINNED_VERSION} installed successfully at ${INSTALL_DIR}/slopslint (SHA256: ${ACTUAL_SHA})."
