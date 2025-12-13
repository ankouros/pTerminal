#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ASSETS_DIR="${ROOT_DIR}/internal/ui/assets/vendor"
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

cp node_modules/@xterm/addon-search/lib/addon-search.js \
   "${ASSETS_DIR}/xterm-addon-search.js"

cp node_modules/@xterm/addon-web-links/lib/addon-web-links.js \
   "${ASSETS_DIR}/xterm-addon-web-links.js"

cp node_modules/@xterm/addon-webgl/lib/addon-webgl.js \
   "${ASSETS_DIR}/xterm-addon-webgl.js"

cp node_modules/@xterm/addon-serialize/lib/addon-serialize.js \
   "${ASSETS_DIR}/xterm-addon-serialize.js"

cp node_modules/@xterm/addon-unicode11/lib/addon-unicode11.js \
   "${ASSETS_DIR}/xterm-addon-unicode11.js"

cp node_modules/@xterm/addon-ligatures/lib/addon-ligatures.js \
   "${ASSETS_DIR}/xterm-addon-ligatures.js"

cp node_modules/@xterm/addon-image/lib/addon-image.js \
   "${ASSETS_DIR}/xterm-addon-image.js"

cp node_modules/@xterm/addon-clipboard/lib/addon-clipboard.js \
   "${ASSETS_DIR}/xterm-addon-clipboard.js"

echo "✔ Assets installed:"
ls -lh "${ASSETS_DIR}"

echo "Cleaning npm artifacts…"
rm -rf node_modules package-lock.json
