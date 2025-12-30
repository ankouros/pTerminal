#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "INSTALL.sh must be run with sudo"
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Updating package lists from official repositories..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -y

PKGS=(
  golang-go
  pkg-config
  libgtk-3-dev
  libwebkit2gtk-4.1-dev
  nodejs
  npm
  git
)

echo "Installing dependencies: ${PKGS[*]}"
apt-get install -y --no-install-recommends "${PKGS[@]}"

cd "$SCRIPT_DIR"

echo "Cleaning previous builds..."
make clean

echo "Running Go test suite..."
go test ./...

echo "Building pTerminal..."
make build

echo "Installing binary to /usr/local/bin..."
install -Dm755 ./bin/pterminal /usr/local/bin/pterminal

echo "Installing desktop entry and icon..."
install -Dm644 packaging/pterminal.desktop /usr/share/applications/pterminal.desktop
install -Dm644 packaging/pterminal.svg /usr/share/icons/hicolor/256x256/apps/pterminal.svg

echo "Done. You can launch pTerminal from the desktop menu or by running 'pterminal'."
