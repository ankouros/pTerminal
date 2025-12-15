#!/usr/bin/env bash
set -euo pipefail

BIN="${1:-./pterminal}"

if [[ ! -x "${BIN}" ]]; then
  echo "Binary not found/executable: ${BIN}" >&2
  exit 1
fi

if ! command -v ldd >/dev/null 2>&1; then
  echo "ldd not found; cannot verify shared library dependencies." >&2
  exit 1
fi

bindir="$(cd "$(dirname "${BIN}")" && pwd)"

extra_ldpath=""
if [[ -d "${bindir}/lib" ]]; then
  extra_ldpath="${bindir}/lib"
fi
if [[ -d "${bindir}/usr/lib" ]]; then
  extra_ldpath="${extra_ldpath:+${extra_ldpath}:}${bindir}/usr/lib"
fi
if [[ -d "${bindir}/usr/lib64" ]]; then
  extra_ldpath="${extra_ldpath:+${extra_ldpath}:}${bindir}/usr/lib64"
fi

echo "Checking shared library dependencies for: ${BIN}"
if [[ -n "${extra_ldpath}" ]]; then
  echo "Using LD_LIBRARY_PATH: ${extra_ldpath}"
  missing="$(LD_LIBRARY_PATH="${extra_ldpath}" ldd "${BIN}" | awk '/not found/{print $1}')"
else
  missing="$(ldd "${BIN}" | awk '/not found/{print $1}')"
fi

if [[ -n "${missing}" ]]; then
  echo "Missing libraries:"
  echo "${missing}"
  echo
  echo "This build depends on WebKitGTK/GTK runtime libraries provided by your distro."
  exit 2
fi

echo "OK: no missing shared libraries reported by ldd."
