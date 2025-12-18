#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-}"
IMAGE="${2:-}"
VERSION_PREFIX="${3:-}"

if [[ -z "${TARGET}" || -z "${IMAGE}" || -z "${VERSION_PREFIX}" ]]; then
  echo "Usage: $0 <target> <docker-image> <version-prefix>" >&2
  exit 2
fi

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist"
mkdir -p "${DIST_DIR}"

GO_VERSION="${GO_VERSION:-1.22.10}"
GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-amd64}"
docker_extra_args=()

echo "Building portable bundle for ${TARGET} in ${IMAGE} (GO ${GO_VERSION}, ${GOOS}/${GOARCH})"

docker run --rm \
  -v "${ROOT_DIR}:/src" \
  -w /src \
  -e HOST_UID="$(id -u)" \
  -e HOST_GID="$(id -g)" \
  -e TARGET="${TARGET}" \
  -e VERSION_PREFIX="${VERSION_PREFIX}" \
  -e GO_VERSION="${GO_VERSION}" \
  -e GOOS="${GOOS}" \
  -e GOARCH="${GOARCH}" \
  -e SLES12_SCC_CREDENTIALS="${SLES12_SCC_CREDENTIALS:-}" \
  -e SLES12_SCC_REGCODE="${SLES12_SCC_REGCODE:-}" \
  -e SLES12_SCC_EMAIL="${SLES12_SCC_EMAIL:-}" \
  -e SLES12_ADDITIONAL_PRODUCTS="${SLES12_ADDITIONAL_PRODUCTS:-}" \
  "${docker_extra_args[@]}" \
  "${IMAGE}" \
  bash -lc '
    set -euo pipefail

    install_go() {
      local ver="$1"
      local arch="$2"
      local os="$3"

      if [[ -x /usr/local/go/bin/go ]]; then
        /usr/local/go/bin/go version || true
        return 0
      fi

      local url="https://go.dev/dl/go${ver}.${os}-${arch}.tar.gz"
      echo "Downloading Go: ${url}"
      (
        mkdir -p /tmp/go-install
        cd /tmp/go-install
        curl -fsSL "${url}" -o go.tgz
        rm -rf /usr/local/go
        tar -C /usr/local -xzf go.tgz
      )
      /usr/local/go/bin/go version
    }

    ensure_webkit_pc_compat() {
      if pkg-config --exists webkit2gtk-4.0; then
        return 0
      fi
      if ! pkg-config --exists webkit2gtk-4.1; then
        echo "Neither webkit2gtk-4.0 nor webkit2gtk-4.1 found via pkg-config" >&2
        pkg-config --list-all | grep -i webkit || true
        return 1
      fi

      local pc_paths
      pc_paths="$(pkg-config --variable pc_path pkg-config 2>/dev/null || true)"
      for dir in ${pc_paths//:/ }; do
        if [[ -f "${dir}/webkit2gtk-4.1.pc" ]]; then
          ln -sf "${dir}/webkit2gtk-4.1.pc" "${dir}/webkit2gtk-4.0.pc"
          echo "Created pkg-config alias: ${dir}/webkit2gtk-4.0.pc -> webkit2gtk-4.1.pc"
          return 0
        fi
      done

      echo "Could not locate webkit2gtk-4.1.pc in pkg-config path: ${pc_paths}" >&2
      return 1
    }

    maybe_suseconnect_register() {
      if ! command -v SUSEConnect >/dev/null 2>&1; then
        echo "SUSEConnect not present; skipping registration" >&2
        return 0
      fi

      if [[ -n "${SLES12_SCC_REGCODE:-}" ]]; then
        echo "Registering SLES container via SUSEConnect regcode..."
        if [[ -n "${SLES12_SCC_EMAIL:-}" ]]; then
          SUSEConnect -r "${SLES12_SCC_REGCODE}" -e "${SLES12_SCC_EMAIL}" || true
        else
          SUSEConnect -r "${SLES12_SCC_REGCODE}" || true
        fi
      elif [[ -n "${SLES12_SCC_CREDENTIALS:-}" ]]; then
        echo "Registering SLES container via SUSEConnect credentials file..."
        umask 077
        cred="/tmp/suseconnect.credentials"
        printf "%s" "${SLES12_SCC_CREDENTIALS}" > "${cred}"
        SUSEConnect --credentials "${cred}" || true
        rm -f "${cred}" || true
      else
        echo "No SLES registration secrets provided; continuing without registration." >&2
      fi

      if [[ -n "${SLES12_ADDITIONAL_PRODUCTS:-}" ]]; then
        echo "Adding SUSE products: ${SLES12_ADDITIONAL_PRODUCTS}"
        IFS=',' read -ra prods <<< "${SLES12_ADDITIONAL_PRODUCTS}"
        for p in "${prods[@]}"; do
          p="$(echo "${p}" | xargs)"
          [[ -z "${p}" ]] && continue
          SUSEConnect -p "${p}" || true
        done
      fi
    }

    case "${TARGET}" in
      ubuntu24)
        export DEBIAN_FRONTEND=noninteractive
        apt-get update -y
        apt-get install -y --no-install-recommends \
          ca-certificates curl git make gcc g++ pkg-config xz-utils tar gzip \
          libgtk-3-dev libwebkit2gtk-4.1-dev
        update-ca-certificates || true
        ensure_webkit_pc_compat
        ;;
      sles15)
        zypper -n refresh
        zypper -n install -y \
          ca-certificates ca-certificates-mozilla curl git make gcc gcc-c++ pkg-config tar gzip xz \
          gtk3-devel webkit2gtk3-devel
        update-ca-certificates || true
        ensure_webkit_pc_compat
        ;;
      sles12)
        maybe_suseconnect_register
        zypper -n refresh
        zypper -n install -y \
          ca-certificates ca-certificates-mozilla curl git make gcc gcc-c++ pkg-config tar gzip xz \
          gtk3-devel webkit2gtk3-devel
        update-ca-certificates || true
        ensure_webkit_pc_compat
        ;;
      *)
        echo "Unknown target: ${TARGET}" >&2
        exit 2
        ;;
    esac

    install_go "${GO_VERSION}" "${GOARCH}" "${GOOS}"
    export PATH="/usr/local/go/bin:${PATH}"

    test -f internal/ui/assets/vendor/xterm.js || (echo "Missing xterm assets; run make assets on host before CI build" >&2; exit 2)

    make portable VERSION="${VERSION_PREFIX}-${TARGET}" GOOS="${GOOS}" GOARCH="${GOARCH}"

    # Ensure mounted workspace artifacts are writable by the host user.
    if [[ -n "${HOST_UID:-}" && -n "${HOST_GID:-}" ]]; then
      chown -R "${HOST_UID}:${HOST_GID}" /src/release /src/dist 2>/dev/null || true
    fi
  '

# Pick up the generated portable tarball and rename to requested artifact name.
src_tgz="$(ls -1 "${ROOT_DIR}/release"/pterminal-"${VERSION_PREFIX}"-"${TARGET}"-linux-"${GOARCH}"-portable.tar.gz 2>/dev/null | head -n 1 || true)"
if [[ -z "${src_tgz}" ]]; then
  src_tgz="$(ls -1 "${ROOT_DIR}/release"/pterminal-"${VERSION_PREFIX}"-"${TARGET}"-linux-"${GOARCH}"-portable.tar.gz 2>/dev/null | head -n 1 || true)"
fi
if [[ -z "${src_tgz}" ]]; then
  echo "Could not find portable tarball in ${ROOT_DIR}/release/ (VERSION_PREFIX=${VERSION_PREFIX}, TARGET=${TARGET})" >&2
  ls -la "${ROOT_DIR}/release" || true
  exit 4
fi

out="${DIST_DIR}/pterminal-${TARGET}-portable.tar.gz"
cp -f "${src_tgz}" "${out}"

if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "${out}" > "${out}.sha256"
fi

echo "Created: ${out}"
