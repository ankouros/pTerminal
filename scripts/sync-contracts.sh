#!/usr/bin/env bash
set -euo pipefail

SPECS_PATH="${SAMAKIA_SPECS_PATH:-/home/aggelos/samakia-specs}"
SOURCE="${SPECS_PATH}/repo-contracts/pterminal.md"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TARGET="${ROOT_DIR}/CONTRACTS.md"

if [[ ! -f "${SOURCE}" ]]; then
  echo "Spec contract not found: ${SOURCE}" >&2
  echo "Set SAMAKIA_SPECS_PATH or clone samakia-specs." >&2
  exit 1
fi

tmp="$(mktemp)"
cp "${SOURCE}" "${tmp}"
mv "${tmp}" "${TARGET}"

echo "Synced CONTRACTS.md from ${SOURCE}"
