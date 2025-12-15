#!/usr/bin/env bash
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
BIN="${DIR}/pterminal"

if [[ ! -x "${BIN}" ]]; then
  echo "Binary not found/executable: ${BIN}" >&2
  exit 1
fi

if [[ -x "${DIR}/check_deps.sh" ]]; then
  "${DIR}/check_deps.sh" "${BIN}" || true
fi

exec "${BIN}"

