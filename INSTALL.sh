#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "INSTALL.sh must be run with sudo"
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

UBUNTU_SOURCES="/etc/apt/sources.list.d/ubuntu.sources"
UBUNTU_KEYRING="/usr/share/keyrings/ubuntu-archive-keyring.gpg"

create_sources() {
  cat <<'EOF' > "$UBUNTU_SOURCES"
Types: deb
URIs: http://archive.ubuntu.com/ubuntu/
Suites: noble noble-updates noble-backports
Components: main restricted universe multiverse
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg

Types: deb
URIs: http://security.ubuntu.com/ubuntu/
Suites: noble-security
Components: main restricted universe multiverse
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg
EOF
  chmod 644 "$UBUNTU_SOURCES"
}

if [ ! -f "$UBUNTU_SOURCES" ]; then
  read -p "Ubuntu sources ($UBUNTU_SOURCES) are missing. Create them with the canonical mirrors? [y/N] " resp
  if [[ "$resp" =~ ^[Yy]$ ]]; then
    create_sources
    echo "Created $UBUNTU_SOURCES"
  else
    echo "Skipping creation of Ubuntu sources; manual configuration required."
  fi
fi

if [ ! -f "$UBUNTU_KEYRING" ]; then
  read -p "Ubuntu keyring ($UBUNTU_KEYRING) is missing. Install ubuntu-keyring now? [y/N] " resp
  if [[ "$resp" =~ ^[Yy]$ ]]; then
    apt-get install -y ubuntu-keyring
  else
    echo "Skipping keyring install; apt may warn about unsigned packages."
  fi
fi

echo "Updating package lists from official repositories..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -y

PKGS=(
  golang-go
  pkg-config
  libgtk-3-dev
  libwebkit2gtk-4.1-dev
  nodejs
  git
)

PKGS_TO_INSTALL=()
for pkg in "${PKGS[@]}"; do
  if dpkg-query -W -f='${Status}' "$pkg" 2>/dev/null | grep -q "install ok installed"; then
    echo "$pkg already installed"
  else
    PKGS_TO_INSTALL+=("$pkg")
  fi
done

if [ "${#PKGS_TO_INSTALL[@]}" -gt 0 ]; then
  echo "Installing dependencies: ${PKGS_TO_INSTALL[*]}"
  apt-get install -y --no-install-recommends "${PKGS_TO_INSTALL[@]}"
else
  echo "All dependencies already satisfied via apt"
fi

if command -v npm >/dev/null; then
  echo "npm is already available"
else
  echo "npm command is not currently available; nodejs packages from official repos typically include npm."
  echo "If you need npm, install it manually (e.g., `apt-get install npm` after ensuring it does not conflict with nodejs)."
fi

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
ICON_THEME_DIR="/usr/share/icons/hicolor"
install -Dm644 packaging/pterminal.svg "$ICON_THEME_DIR/256x256/apps/pterminal.svg"
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
  gtk-update-icon-cache -f -t "$ICON_THEME_DIR"
else
  echo "gtk-update-icon-cache not found; desktop icon cache not refreshed"
fi

echo "Done. You can launch pTerminal from the desktop menu or by running 'pterminal'."
