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

have_header() {
  local path
  for path in "$@"; do
    if [[ -f "${path}" ]]; then
      return 0
    fi
  done
  return 1
}

have_so() {
  local regex="${1:?}"
  if ldconfig -p 2>/dev/null | grep -qE "${regex}"; then
    return 0
  fi
  return 1
}

have_so_in_fs() {
  local file="${1:?}"
  [[ -e "/usr/lib/${file}" || -e "/usr/lib64/${file}" || -e "/lib/${file}" || -e "/lib64/${file}" ]]
}

ensure_bzip2_pc() {
  if pkg-config --exists bzip2 2>/dev/null; then
    return 0
  fi

  local header_ok="false"
  local lib_ok="false"
  if have_header /usr/include/bzlib.h /usr/local/include/bzlib.h; then header_ok="true"; fi
  if have_so '\<libbz2\.so' || have_so_in_fs libbz2.so; then lib_ok="true"; fi

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

ensure_uuid_pc() {
  if pkg-config --exists uuid 2>/dev/null; then
    return 0
  fi

  local header_ok="false"
  local lib_ok="false"
  if have_header /usr/include/uuid/uuid.h /usr/local/include/uuid/uuid.h; then header_ok="true"; fi
  if have_so '\<libuuid\.so' || have_so_in_fs libuuid.so; then lib_ok="true"; fi

  cat >"${out_dir}/uuid.pc" <<'EOF'
prefix=/usr
exec_prefix=${prefix}
libdir=${exec_prefix}/lib
includedir=${prefix}/include

Name: uuid
Description: DCE compatible Universally Unique Identifier library (fallback pkg-config)
Version: 2.38.0
Libs: -luuid
Cflags:
EOF

  if [[ "${header_ok}" != "true" ]]; then
    echo "WARNING: Created fallback uuid.pc, but uuid/uuid.h was not found in /usr/include or /usr/local/include." >&2
    echo "         Install a uuid development package (e.g. util-linux-devel / libuuid-devel) or load a module that provides it." >&2
  fi
  if [[ "${lib_ok}" != "true" ]]; then
    echo "WARNING: Created fallback uuid.pc, but libuuid.so was not found in the dynamic linker search paths." >&2
    echo "         Linking may fail unless uuid runtime libs are installed/available." >&2
  fi
}

ensure_uuid_pc

ensure_zlib_pc() {
  if pkg-config --exists zlib 2>/dev/null; then
    return 0
  fi

  local header_ok="false"
  local lib_ok="false"
  if have_header /usr/include/zlib.h /usr/local/include/zlib.h; then header_ok="true"; fi
  if have_so '\<libz\.so' || have_so_in_fs libz.so; then lib_ok="true"; fi

  cat >"${out_dir}/zlib.pc" <<'EOF'
prefix=/usr
exec_prefix=${prefix}
libdir=${exec_prefix}/lib
includedir=${prefix}/include

Name: zlib
Description: zlib compression library (fallback pkg-config)
Version: 1.2.11
Libs: -lz
Cflags:
EOF

  if [[ "${header_ok}" != "true" ]]; then
    echo "WARNING: Created fallback zlib.pc, but zlib.h was not found in /usr/include or /usr/local/include." >&2
  fi
  if [[ "${lib_ok}" != "true" ]]; then
    echo "WARNING: Created fallback zlib.pc, but libz.so was not found in the dynamic linker search paths." >&2
  fi
}

ensure_zlib_pc

ensure_expat_pc() {
  if pkg-config --exists expat 2>/dev/null; then
    return 0
  fi

  local header_ok="false"
  local lib_ok="false"
  if have_header /usr/include/expat.h /usr/local/include/expat.h; then header_ok="true"; fi
  if have_so '\<libexpat\.so' || have_so_in_fs libexpat.so; then lib_ok="true"; fi

  cat >"${out_dir}/expat.pc" <<'EOF'
prefix=/usr
exec_prefix=${prefix}
libdir=${exec_prefix}/lib
includedir=${prefix}/include

Name: expat
Description: Expat XML parser library (fallback pkg-config)
Version: 2.5.0
Libs: -lexpat
Cflags:
EOF

  if [[ "${header_ok}" != "true" ]]; then
    echo "WARNING: Created fallback expat.pc, but expat.h was not found in /usr/include or /usr/local/include." >&2
  fi
  if [[ "${lib_ok}" != "true" ]]; then
    echo "WARNING: Created fallback expat.pc, but libexpat.so was not found in the dynamic linker search paths." >&2
  fi
}

ensure_expat_pc

ensure_libpng_pc() {
  # Common names that may be required by other .pc files.
  if pkg-config --exists libpng 2>/dev/null && pkg-config --exists libpng16 2>/dev/null; then
    return 0
  fi

  local libflag="-lpng"
  if have_so '\<libpng16\.so' || have_so_in_fs libpng16.so; then
    libflag="-lpng16"
  elif have_so '\<libpng\.so' || have_so_in_fs libpng.so; then
    libflag="-lpng"
  fi

  local header_ok="false"
  local lib_ok="false"
  if have_header /usr/include/png.h /usr/local/include/png.h; then header_ok="true"; fi
  if [[ "${libflag}" == "-lpng16" ]]; then
    if have_so '\<libpng16\.so' || have_so_in_fs libpng16.so; then lib_ok="true"; fi
  else
    if have_so '\<libpng\.so' || have_so_in_fs libpng.so; then lib_ok="true"; fi
  fi

  cat >"${out_dir}/libpng.pc" <<EOF
prefix=/usr
exec_prefix=\${prefix}
libdir=\${exec_prefix}/lib
includedir=\${prefix}/include

Name: libpng
Description: PNG image format decoding library (fallback pkg-config)
Version: 1.6.0
Libs: ${libflag}
Cflags:
EOF

  cat >"${out_dir}/libpng16.pc" <<EOF
prefix=/usr
exec_prefix=\${prefix}
libdir=\${exec_prefix}/lib
includedir=\${prefix}/include

Name: libpng16
Description: PNG image format decoding library (fallback pkg-config)
Version: 1.6.0
Libs: ${libflag}
Cflags:
EOF

  if [[ "${header_ok}" != "true" ]]; then
    echo "WARNING: Created fallback libpng.pc/libpng16.pc, but png.h was not found in /usr/include or /usr/local/include." >&2
  fi
  if [[ "${lib_ok}" != "true" ]]; then
    echo "WARNING: Created fallback libpng.pc/libpng16.pc, but libpng*.so was not found in the dynamic linker search paths." >&2
  fi
}

ensure_libpng_pc

ensure_brotli_pcs() {
  # Some freetype builds depend on brotli. Provide minimal .pc files if missing.
  for name in brotlicommon brotlidec brotlienc; do
    if pkg-config --exists "${name}" 2>/dev/null; then
      continue
    fi

    local lib="-l${name}"
    local so_regex="\\<lib${name}\\.so"
    local lib_ok="false"
    if have_so "${so_regex}" || have_so_in_fs "lib${name}.so"; then lib_ok="true"; fi

    cat >"${out_dir}/${name}.pc" <<EOF
prefix=/usr
exec_prefix=\${prefix}
libdir=\${exec_prefix}/lib
includedir=\${prefix}/include

Name: ${name}
Description: Brotli library component (fallback pkg-config)
Version: 1.0.9
Libs: ${lib}
Cflags:
EOF

    if [[ "${lib_ok}" != "true" ]]; then
      echo "WARNING: Created fallback ${name}.pc, but lib${name}.so was not found in the dynamic linker search paths." >&2
    fi
  done
}

ensure_brotli_pcs

ensure_pcre2_pc() {
  # glib-2.0 commonly Requires: libpcre2-8
  if pkg-config --exists libpcre2-8 2>/dev/null; then
    return 0
  fi

  local header_ok="false"
  local lib_ok="false"
  if have_header /usr/include/pcre2.h /usr/local/include/pcre2.h; then header_ok="true"; fi
  if have_so '\<libpcre2-8\.so' || have_so_in_fs 'libpcre2-8.so'; then lib_ok="true"; fi

  cat >"${out_dir}/libpcre2-8.pc" <<'EOF'
prefix=/usr
exec_prefix=${prefix}
libdir=${exec_prefix}/lib
includedir=${prefix}/include

Name: libpcre2-8
Description: Perl Compatible Regular Expressions library (PCRE2-8) (fallback pkg-config)
Version: 10.42
Libs: -lpcre2-8
Cflags:
EOF

  if [[ "${header_ok}" != "true" ]]; then
    echo "WARNING: Created fallback libpcre2-8.pc, but pcre2.h was not found in /usr/include or /usr/local/include." >&2
  fi
  if [[ "${lib_ok}" != "true" ]]; then
    echo "WARNING: Created fallback libpcre2-8.pc, but libpcre2-8.so was not found in the dynamic linker search paths." >&2
  fi
}

ensure_pcre2_pc

ensure_libffi_pc() {
  if pkg-config --exists libffi 2>/dev/null; then
    return 0
  fi

  local header_ok="false"
  local lib_ok="false"
  if have_header \
    /usr/include/ffi.h \
    /usr/local/include/ffi.h \
    /usr/include/x86_64-linux-gnu/ffi.h \
    /usr/include/aarch64-linux-gnu/ffi.h; then
    header_ok="true"
  fi
  if have_so '\<libffi\.so' || have_so_in_fs libffi.so; then lib_ok="true"; fi

  cat >"${out_dir}/libffi.pc" <<'EOF'
prefix=/usr
exec_prefix=${prefix}
libdir=${exec_prefix}/lib
includedir=${prefix}/include

Name: libffi
Description: Foreign Function Interface library (fallback pkg-config)
Version: 3.4.4
Libs: -lffi
Cflags:
EOF

  if [[ "${header_ok}" != "true" ]]; then
    echo "WARNING: Created fallback libffi.pc, but ffi.h was not found in common include paths." >&2
  fi
  if [[ "${lib_ok}" != "true" ]]; then
    echo "WARNING: Created fallback libffi.pc, but libffi.so was not found in the dynamic linker search paths." >&2
  fi
}

ensure_libffi_pc

ensure_libmount_pc() {
  if pkg-config --exists mount 2>/dev/null || pkg-config --exists libmount 2>/dev/null; then
    return 0
  fi

  local header_ok="false"
  local lib_ok="false"
  if have_header /usr/include/libmount/libmount.h /usr/local/include/libmount/libmount.h; then header_ok="true"; fi
  if have_so '\<libmount\.so' || have_so_in_fs libmount.so; then lib_ok="true"; fi

  cat >"${out_dir}/mount.pc" <<'EOF'
prefix=/usr
exec_prefix=${prefix}
libdir=${exec_prefix}/lib
includedir=${prefix}/include

Name: mount
Description: libmount (util-linux) (fallback pkg-config)
Version: 2.38.0
Libs: -lmount
Cflags:
EOF

  cat >"${out_dir}/libmount.pc" <<'EOF'
prefix=/usr
exec_prefix=${prefix}
libdir=${exec_prefix}/lib
includedir=${prefix}/include

Name: libmount
Description: libmount (util-linux) (fallback pkg-config)
Version: 2.38.0
Libs: -lmount
Cflags:
EOF

  if [[ "${header_ok}" != "true" ]]; then
    echo "WARNING: Created fallback mount.pc/libmount.pc, but libmount headers were not found in common include paths." >&2
  fi
  if [[ "${lib_ok}" != "true" ]]; then
    echo "WARNING: Created fallback mount.pc/libmount.pc, but libmount.so was not found in the dynamic linker search paths." >&2
  fi
}

ensure_libmount_pc

ensure_blkid_pc() {
  if pkg-config --exists blkid 2>/dev/null; then
    return 0
  fi

  local header_ok="false"
  local lib_ok="false"
  if have_header /usr/include/blkid/blkid.h /usr/local/include/blkid/blkid.h; then header_ok="true"; fi
  if have_so '\<libblkid\.so' || have_so_in_fs libblkid.so; then lib_ok="true"; fi

  cat >"${out_dir}/blkid.pc" <<'EOF'
prefix=/usr
exec_prefix=${prefix}
libdir=${exec_prefix}/lib
includedir=${prefix}/include

Name: blkid
Description: blkid (util-linux) (fallback pkg-config)
Version: 2.38.0
Libs: -lblkid
Cflags:
EOF

  if [[ "${header_ok}" != "true" ]]; then
    echo "WARNING: Created fallback blkid.pc, but blkid headers were not found in common include paths." >&2
  fi
  if [[ "${lib_ok}" != "true" ]]; then
    echo "WARNING: Created fallback blkid.pc, but libblkid.so was not found in the dynamic linker search paths." >&2
  fi
}

ensure_blkid_pc

ensure_pthread_stubs_pc() {
  if pkg-config --exists pthread-stubs 2>/dev/null; then
    return 0
  fi

  # Most toolchains provide pthreads via libc; many distros don't ship a separate
  # libpthread-stubs at all. Some X11-related .pc files still Require it, so a
  # minimal pkg-config stub is enough to satisfy resolution.
  cat >"${out_dir}/pthread-stubs.pc" <<'EOF'
prefix=/usr
exec_prefix=${prefix}
libdir=${exec_prefix}/lib
includedir=${prefix}/include

Name: pthread-stubs
Description: Stub pthread library (pkg-config fallback)
Version: 0.4
Libs:
Cflags:
EOF
}

ensure_pthread_stubs_pc

ensure_xau_pc() {
  if pkg-config --exists xau 2>/dev/null; then
    return 0
  fi

  local header_ok="false"
  local lib_ok="false"
  if have_header /usr/include/X11/Xauth.h /usr/local/include/X11/Xauth.h; then header_ok="true"; fi
  if have_so '\<libXau\.so' || have_so_in_fs libXau.so; then lib_ok="true"; fi

  cat >"${out_dir}/xau.pc" <<'EOF'
prefix=/usr
exec_prefix=${prefix}
libdir=${exec_prefix}/lib
includedir=${prefix}/include

Name: Xau
Description: X11 authorisation library (fallback pkg-config)
Version: 1.0.11
Libs: -lXau
Cflags:
EOF

  if [[ "${header_ok}" != "true" ]]; then
    echo "WARNING: Created fallback xau.pc, but X11/Xauth.h was not found in /usr/include or /usr/local/include." >&2
  fi
  if [[ "${lib_ok}" != "true" ]]; then
    echo "WARNING: Created fallback xau.pc, but libXau.so was not found in the dynamic linker search paths." >&2
  fi
}

ensure_xau_pc

ensure_xdmcp_pc() {
  if pkg-config --exists xdmcp 2>/dev/null; then
    return 0
  fi

  local header_ok="false"
  local lib_ok="false"
  if have_header /usr/include/X11/Xdmcp.h /usr/local/include/X11/Xdmcp.h; then header_ok="true"; fi
  if have_so '\<libXdmcp\.so' || have_so_in_fs libXdmcp.so; then lib_ok="true"; fi

  cat >"${out_dir}/xdmcp.pc" <<'EOF'
prefix=/usr
exec_prefix=${prefix}
libdir=${exec_prefix}/lib
includedir=${prefix}/include

Name: Xdmcp
Description: X11 Display Manager Control Protocol library (fallback pkg-config)
Version: 1.1.4
Libs: -lXdmcp
Cflags:
EOF

  if [[ "${header_ok}" != "true" ]]; then
    echo "WARNING: Created fallback xdmcp.pc, but X11/Xdmcp.h was not found in /usr/include or /usr/local/include." >&2
  fi
  if [[ "${lib_ok}" != "true" ]]; then
    echo "WARNING: Created fallback xdmcp.pc, but libXdmcp.so was not found in the dynamic linker search paths." >&2
  fi
}

ensure_xdmcp_pc
