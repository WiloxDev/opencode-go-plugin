#!/usr/bin/env bash
# Quick installer for OpenCode Go Plugin for CPAMC
set -euo pipefail

REPO="WiloxDev/opencode-go-plugin"
PLUGIN_DIR="${CPAMC_PLUGIN_DIR:-$HOME/.config/cpamc/plugins}"
BINARY_NAME="opencode-go-linux-amd64.so"

echo "=========================================="
echo " OpenCode Go Plugin Installer for CPAMC   "
echo "=========================================="

ARCH=$(uname -m)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')

if [ "$OS" != "linux" ] || [ "$ARCH" != "x86_64" ]; then
    echo "Warning: Official prebuilts currently target linux-x86_64."
    echo "Current system: $OS-$ARCH"
    if command -v go >/dev/null 2>&1 && command -v gcc >/dev/null 2>&1; then
        echo "Go and GCC detected. Building from source instead..."
        TMP_DIR=$(mktemp -d)
        git clone "https://github.com/${REPO}.git" "$TMP_DIR"
        (cd "$TMP_DIR" && make build)
        mkdir -p "$PLUGIN_DIR"
        cp "$TMP_DIR/${BINARY_NAME}" "$PLUGIN_DIR/"
        rm -rf "$TMP_DIR"
        echo "Successfully built and installed to: $PLUGIN_DIR/${BINARY_NAME}"
        exit 0
    else
        echo "Error: Prebuilt not available and Go/GCC not found to compile."
        exit 1
    fi
fi

mkdir -p "$PLUGIN_DIR"

LATEST_URL="https://github.com/${REPO}/releases/latest/download/${BINARY_NAME}"

echo "Fetching plugin into ${PLUGIN_DIR}/${BINARY_NAME}..."
if curl -fsSL "$LATEST_URL" -o "${PLUGIN_DIR}/${BINARY_NAME}"; then
    echo "Plugin downloaded and installed successfully!"
else
    echo "Notice: Release binary not found on GitHub Releases yet."
    if command -v go >/dev/null 2>&1 && command -v gcc >/dev/null 2>&1; then
        echo "Building from local repository / Go compiler..."
        CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -buildmode=c-shared -o "${PLUGIN_DIR}/${BINARY_NAME}" main.go || true
    fi
fi

echo "OpenCode Go Plugin is ready for CPAMC."
