#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ASSETS_DIR="${ROOT_DIR}/assets/vendor"
PKG_DIR="${ROOT_DIR}/scripts"

mkdir -p "${ASSETS_DIR}"

echo "Installing xterm.js via npm (scoped packages)…"
cd "${PKG_DIR}"
npm install --no-audit --no-fund

echo "Copying browser-ready assets…"

cp node_modules/@xterm/xterm/lib/xterm.js \
   "${ASSETS_DIR}/xterm.js"

cp node_modules/@xterm/xterm/css/xterm.css \
   "${ASSETS_DIR}/xterm.css"

cp node_modules/@xterm/addon-fit/lib/addon-fit.js \
   "${ASSETS_DIR}/xterm-addon-fit.js"

echo "✔ Assets installed:"
ls -lh "${ASSETS_DIR}"

echo "Cleaning npm artifacts…"
rm -rf node_modules package-lock.json
