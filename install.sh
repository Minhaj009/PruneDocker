#!/usr/bin/env bash
set -e

# PruneDocker Universal One-Liner Installer
# Usage: curl -sSL https://raw.githubusercontent.com/Minhaj009/PruneDocker/main/install.sh | bash

REPO="Minhaj009/PruneDocker"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="prunedocker"

echo -e "\033[1;36m============================================================\033[0m"
echo -e "\033[1;36m  PruneDocker: Intelligent Docker Cache Optimizer Installer \033[0m"
echo -e "\033[1;36m============================================================\033[0m"

# 1. Detect OS & Architecture
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo -e "\033[1;31m[ERROR] Unsupported architecture: $ARCH\033[0m"; exit 1 ;;
esac

case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  *) echo -e "\033[1;31m[ERROR] Unsupported OS: $OS\033[0m"; exit 1 ;;
esac

echo -e "\033[1;32m[INFO]\033[0m Detected System: \033[1m${OS}/${ARCH}\033[0m"

# 2. Fetch Latest Release Version from GitHub API
LATEST_TAG=$(curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || true)

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

if [ -n "$LATEST_TAG" ]; then
  TARBALL="prunedocker_${LATEST_TAG#v}_${OS}_${ARCH}.tar.gz"
  DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${LATEST_TAG}/${TARBALL}"
  echo -e "\033[1;34m[INFO]\033[0m Downloading release \033[1m${LATEST_TAG}\033[0m from GitHub..."
  
  if curl -sSL -f "$DOWNLOAD_URL" -o "${TMP_DIR}/${TARBALL}"; then
    tar -xzf "${TMP_DIR}/${TARBALL}" -C "$TMP_DIR"
  else
    echo -e "\033[1;33m[WARN]\033[0m Release binary download failed, falling back to source build..."
    LATEST_TAG=""
  fi
fi

# Fallback: Compile from repository if release is not yet uploaded or Go is available
if [ ! -f "${TMP_DIR}/${BINARY_NAME}" ]; then
  if command -v go >/dev/null 2>&1; then
    echo -e "\033[1;34m[INFO]\033[0m Building latest prunedocker binary via Go toolchain..."
    git clone --depth 1 "https://github.com/${REPO}.git" "${TMP_DIR}/source"
    (cd "${TMP_DIR}/source" && go build -o "${TMP_DIR}/${BINARY_NAME}" ./cmd/prunedocker)
  else
    echo -e "\033[1;31m[ERROR]\033[0m Could not download binary and Go is not installed. Please install Go or check GitHub Releases.\033[0m"
    exit 1
  fi
fi

# 3. Install binary to /usr/local/bin
echo -e "\033[1;34m[INFO]\033[0m Installing binary to ${INSTALL_DIR}/${BINARY_NAME}..."
if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
  chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
else
  sudo mv "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
  sudo chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
fi

echo -e "\033[1;32m============================================================\033[0m"
echo -e "\033[1;32m  PruneDocker successfully installed to ${INSTALL_DIR}/${BINARY_NAME}!\033[0m"
echo -e "\033[1;32m============================================================\033[0m"
echo -e "Try running:\n"
echo -e "  \033[1mprunedocker analyze\033[0m        # Inspect layer tree and cache scores"
echo -e "  \033[1mprunedocker ui\033[0m             # Launch interactive Bubbletea TUI"
echo -e "  \033[1mprunedocker prune --dry-run\033[0m# Preview safe selective pruning"
