#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SUBMODULE_SPECS="${ROOT_DIR}/specs/samakia-specs"
DEFAULT_SPECS="/home/aggelos/samakia-specs"

if [[ -n "${SAMAKIA_SPECS_PATH:-}" ]]; then
  SPECS_PATH="${SAMAKIA_SPECS_PATH}"
elif [[ -d "${SUBMODULE_SPECS}" ]]; then
  SPECS_PATH="${SUBMODULE_SPECS}"
else
  SPECS_PATH="${DEFAULT_SPECS}"
fi

SOURCE="${SPECS_PATH}/repo-contracts/pterminal.md"
TARGET="${ROOT_DIR}/CONTRACTS.md"

if [[ ! -f "${SOURCE}" ]]; then
  echo "Spec contract not found: ${SOURCE}" >&2
  echo "Set SAMAKIA_SPECS_PATH or run: make specs-update" >&2
  exit 1
fi

tmp="$(mktemp)"
cp "${SOURCE}" "${tmp}"
mv "${tmp}" "${TARGET}"

echo "Synced CONTRACTS.md from ${SOURCE}"
