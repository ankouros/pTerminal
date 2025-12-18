#!/usr/bin/env bash
set -euo pipefail

# Creates minimal pkg-config .pc fallbacks for environments where some .pc files
# are missing (common with enterprise module systems).
#
# Usage:
#   scripts/ensure_pkgconfig_fallback.sh <output-dir>
#
# The caller should prepend <output-dir> to PKG_CONFIG_PATH for the build.

out_dir="${1:-}"
if [[ -z "${out_dir}" ]]; then
  echo "Usage: $0 <output-dir>" >&2
  exit 2
fi

mkdir -p "${out_dir}"

ensure_bzip2_pc() {
  if pkg-config --exists bzip2 2>/dev/null; then
    return 0
  fi

  local header_ok="false"
  if [[ -f /usr/include/bzlib.h ]] || [[ -f /usr/local/include/bzlib.h ]]; then
    header_ok="true"
  fi

  local lib_ok="false"
  if ldconfig -p 2>/dev/null | grep -qE '\<libbz2\.so'; then
    lib_ok="true"
  elif [[ -e /usr/lib/libbz2.so ]] || [[ -e /usr/lib64/libbz2.so ]] || [[ -e /lib/libbz2.so ]] || [[ -e /lib64/libbz2.so ]]; then
    lib_ok="true"
  fi

  # If the header is missing, the build will likely fail anyway; emit a clear hint.
  # We still generate the .pc so that freetype2's Requires can resolve, but linking
  # may fail later if libbz2 isn't available.
  cat >"${out_dir}/bzip2.pc" <<'EOF'
prefix=/usr
exec_prefix=${prefix}
libdir=${exec_prefix}/lib
includedir=${prefix}/include

Name: bzip2
Description: bzip2 compression library (fallback pkg-config)
Version: 1.0.8
Libs: -lbz2
Cflags:
EOF

  if [[ "${header_ok}" != "true" ]]; then
    echo "WARNING: Created fallback bzip2.pc, but bzlib.h was not found in /usr/include or /usr/local/include." >&2
    echo "         Install a bzip2 development package (e.g. bzip2-devel) or load a module that provides it." >&2
  fi
  if [[ "${lib_ok}" != "true" ]]; then
    echo "WARNING: Created fallback bzip2.pc, but libbz2.so was not found in the dynamic linker search paths." >&2
    echo "         Linking may fail unless bzip2 runtime libs are installed/available." >&2
  fi
}

ensure_bzip2_pc
