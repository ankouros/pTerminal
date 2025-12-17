#!/usr/bin/env bash
set -euo pipefail

APP_ID="com.github.ankouros.pterminal"
MANIFEST="packaging/flatpak/${APP_ID}.yml"
BUILD_DIR="${BUILD_DIR:-.flatpak-build}"
REPO_DIR="${REPO_DIR:-.flatpak-repo}"
DIST_DIR="${DIST_DIR:-dist}"

if ! command -v flatpak-builder >/dev/null 2>&1; then
  echo "flatpak-builder not found. Install flatpak-builder first." >&2
  exit 2
fi

mkdir -p "${DIST_DIR}"

if ! flatpak remotes --user | awk '{print $1}' | grep -qx flathub; then
  flatpak remote-add --user --if-not-exists flathub https://flathub.org/repo/flathub.flatpakrepo
fi

rm -rf "${BUILD_DIR}" "${REPO_DIR}"
mkdir -p "${REPO_DIR}"

flatpak-builder \
  --user \
  --force-clean \
  --repo="${REPO_DIR}" \
  --install-deps-from=flathub \
  "${BUILD_DIR}" \
  "${MANIFEST}"

bundle="${DIST_DIR}/pterminal.flatpak"
flatpak build-bundle "${REPO_DIR}" "${bundle}" "${APP_ID}"

if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "${bundle}" > "${bundle}.sha256"
fi

echo "Created: ${bundle}"
