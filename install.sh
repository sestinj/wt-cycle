#!/bin/sh
set -e

REPO="sestinj/wt-cycle"
BINARY="wt-cycle"

# Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  *)
    echo "Error: Unsupported OS: $OS" >&2
    exit 1
    ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)
    echo "Error: Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

# Get latest release tag
TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$TAG" ]; then
  echo "Error: Could not determine latest release" >&2
  exit 1
fi

ASSET="${BINARY}-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET}"
CHECKSUM_URL="https://github.com/${REPO}/releases/download/${TAG}/checksums.txt"

echo "Downloading ${BINARY} ${TAG} for ${OS}/${ARCH}..."

# Create temp directory
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

# Download binary
curl -fsSL -o "${TMPDIR}/${ASSET}" "$URL"

# Download and verify checksums
if ! curl -fsSL -o "${TMPDIR}/checksums.txt" "$CHECKSUM_URL" 2>/dev/null; then
  echo "Error: couldn't download checksums.txt" >&2
  exit 1
fi

cd "$TMPDIR"
if ! grep -q "${ASSET}" checksums.txt 2>/dev/null; then
  echo "Error: no checksum entry for ${ASSET}" >&2
  exit 1
fi

EXPECTED=$(grep "${ASSET}" checksums.txt | awk '{print $1}')
ACTUAL=$(shasum -a 256 "${ASSET}" 2>/dev/null | awk '{print $1}')
if [ -z "$ACTUAL" ]; then
  ACTUAL=$(sha256sum "${ASSET}" 2>/dev/null | awk '{print $1}')
fi
if [ -z "$ACTUAL" ]; then
  echo "Error: couldn't verify checksum (no shasum or sha256sum found)" >&2
  exit 1
fi
if [ "$EXPECTED" != "$ACTUAL" ]; then
  echo "Error: checksum verification failed" >&2
  exit 1
fi
echo "Checksum verified."

# Install
chmod +x "${TMPDIR}/${ASSET}"

INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ]; then
  INSTALL_DIR="${HOME}/.local/bin"
  mkdir -p "$INSTALL_DIR"
fi

mv "${TMPDIR}/${ASSET}" "${INSTALL_DIR}/${BINARY}"

echo "Installed ${BINARY} ${TAG} to ${INSTALL_DIR}/${BINARY}"
"${INSTALL_DIR}/${BINARY}" --version
