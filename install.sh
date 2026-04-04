#!/bin/sh
set -eu

REPO="nikhil-mishra03/Shroud"
BINARY="shroud"

# Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  darwin) OS="darwin" ;;
  linux)  OS="linux" ;;
  *)
    echo "Error: unsupported OS: $OS"
    exit 1
    ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "Error: unsupported architecture: $ARCH"
    exit 1
    ;;
esac

# Fetch latest release tag
echo "Fetching latest release..."
TAG="$(curl -sS "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"

if [ -z "$TAG" ]; then
  echo "Error: could not determine latest release"
  exit 1
fi

echo "Latest release: $TAG"

# Build download URL
ARCHIVE="${BINARY}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${TAG}/${ARCHIVE}"

# Choose install directory
if [ "$(id -u)" -eq 0 ]; then
  INSTALL_DIR="/usr/local/bin"
else
  INSTALL_DIR="${HOME}/.local/bin"
  mkdir -p "$INSTALL_DIR"
fi

# Download and extract
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading ${URL}..."
curl -sSL "$URL" -o "${TMPDIR}/${ARCHIVE}"

echo "Installing to ${INSTALL_DIR}..."
tar -xzf "${TMPDIR}/${ARCHIVE}" -C "$TMPDIR"
mv "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
chmod +x "${INSTALL_DIR}/${BINARY}"

# Verify
if command -v "$BINARY" >/dev/null 2>&1; then
  echo ""
  echo "shroud ${TAG} installed successfully!"
  echo "Run: shroud run claude"
else
  echo ""
  echo "shroud installed to ${INSTALL_DIR}/${BINARY}"
  echo ""
  echo "WARNING: ${INSTALL_DIR} is not on your PATH."
  echo "Add it with:"
  echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
  echo ""
  echo "Then run: shroud run claude"
fi
