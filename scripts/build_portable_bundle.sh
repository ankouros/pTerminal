#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "Usage: $0 <built-binary-path> <out-dir>" >&2
  exit 2
fi

BIN_SRC="$1"
OUT_DIR="$2"

if [[ ! -x "${BIN_SRC}" ]]; then
  echo "Binary not found/executable: ${BIN_SRC}" >&2
  exit 1
fi

mkdir -p "${OUT_DIR}"

cp -f "${BIN_SRC}" "${OUT_DIR}/pterminal"
chmod +x "${OUT_DIR}/pterminal"

cp -f packaging/pterminal.desktop "${OUT_DIR}/" 2>/dev/null || true
cp -f packaging/pterminal.svg "${OUT_DIR}/" 2>/dev/null || true
cp -f packaging/release_README.md "${OUT_DIR}/README.md" 2>/dev/null || true
cp -f scripts/check_deps.sh "${OUT_DIR}/" 2>/dev/null || true

mkdir -p "${OUT_DIR}/lib" "${OUT_DIR}/libexec"

echo "Collecting shared library dependencies with ldd..."

mapfile -t libs < <(ldd "${OUT_DIR}/pterminal" | awk '{print $3}' | grep -E '^/' | sort -u)

should_skip_lib() {
  local p="$1"
  local b
  b="$(basename "$p")"
  # Do not bundle the dynamic loader or glibc itself (highly version-sensitive).
  case "$b" in
    ld-linux-*.so.*|libc.so.*|libpthread.so.*|librt.so.*|libm.so.*|libdl.so.*|libgcc_s.so.*)
      return 0
      ;;
  esac
  return 1
}

for p in "${libs[@]}"; do
  if should_skip_lib "$p"; then
    continue
  fi
  if [[ -f "$p" ]]; then
    cp --update=none "$p" "${OUT_DIR}/lib/" 2>/dev/null || cp "$p" "${OUT_DIR}/lib/" || true
  fi
done

echo "Attempting to bundle WebKitGTK helper processes/resources (best-effort)..."
webkit_lib="$(ldd "${OUT_DIR}/pterminal" | awk '/libwebkit2gtk/{print $3; exit}')"
if [[ -n "${webkit_lib:-}" && -f "${webkit_lib}" ]]; then
  base_dir="$(dirname "${webkit_lib}")"
  for d in "${base_dir}/webkit2gtk-4.1" "${base_dir}/webkit2gtk-4.0" "${base_dir}/webkit2gtk"; do
    if [[ -d "${d}" ]]; then
      for proc in WebKitWebProcess WebKitNetworkProcess WebKitGPUProcess WebKitPluginProcess; do
        if [[ -f "${d}/${proc}" ]]; then
          cp --update=none "${d}/${proc}" "${OUT_DIR}/libexec/" 2>/dev/null || cp "${d}/${proc}" "${OUT_DIR}/libexec/" || true
        fi
      done
      if [[ -d "${d}/resources" ]]; then
        mkdir -p "${OUT_DIR}/resources"
        cp -R "${d}/resources" "${OUT_DIR}/" 2>/dev/null || true
      fi
    fi
  done
fi

cat > "${OUT_DIR}/run_portable.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"

export LD_LIBRARY_PATH="${HERE}/lib:${LD_LIBRARY_PATH:-}"

# WebKitGTK helper processes (if bundled)
if [[ -x "${HERE}/libexec/WebKitWebProcess" ]]; then
  export WEBKIT_WEB_PROCESS_PATH="${HERE}/libexec/WebKitWebProcess"
fi
if [[ -x "${HERE}/libexec/WebKitNetworkProcess" ]]; then
  export WEBKIT_NETWORK_PROCESS_PATH="${HERE}/libexec/WebKitNetworkProcess"
fi
if [[ -x "${HERE}/libexec/WebKitGPUProcess" ]]; then
  export WEBKIT_GPU_PROCESS_PATH="${HERE}/libexec/WebKitGPUProcess"
fi
if [[ -x "${HERE}/libexec/WebKitPluginProcess" ]]; then
  export WEBKIT_PLUGIN_PROCESS_PATH="${HERE}/libexec/WebKitPluginProcess"
fi

exec "${HERE}/pterminal" "$@"
EOF

chmod +x "${OUT_DIR}/run_portable.sh" "${OUT_DIR}/check_deps.sh" 2>/dev/null || true

echo "Portable bundle created at: ${OUT_DIR}"
echo "Run: ${OUT_DIR}/check_deps.sh ${OUT_DIR}/pterminal"
echo "Then: ${OUT_DIR}/run_portable.sh"
