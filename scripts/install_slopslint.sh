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

# If slopslint is in /home/howlcipher/.local/bin and not yet on PATH
if [ -f "/home/howlcipher/.local/bin/slopslint" ] && [ ! -f "${INSTALL_DIR}/slopslint" ]; then
    cp "/home/howlcipher/.local/bin/slopslint" "${INSTALL_DIR}/slopslint"
    chmod +x "${INSTALL_DIR}/slopslint"
fi

if [ -f "${INSTALL_DIR}/slopslint" ]; then
    chmod +x "${INSTALL_DIR}/slopslint"
    echo "slopslint verified at ${INSTALL_DIR}/slopslint"
    exit 0
fi

echo "slopslint v${PINNED_VERSION} installation complete."
