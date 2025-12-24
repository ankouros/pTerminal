#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"

env_file="${ENV_FILE:-${ROOT_DIR}/scripts/docker/.env}"
if [[ -f "${env_file}" ]]; then
  # shellcheck disable=SC1090
  source "${env_file}"
fi

# Snap-packaged Docker runs in a separate mount namespace where `/tmp` does not
# reflect the host. In that case, bind-mount sources must be prefixed with
# `/var/lib/snapd/hostfs` so the daemon sees the real host filesystem.
hostfs_prefix=""
snap_docker=false
if docker info 2>/dev/null | grep -q "Docker Root Dir: /var/snap/docker/"; then
  hostfs_prefix="/var/lib/snapd/hostfs"
  snap_docker=true
  echo "Detected Docker snap; using host filesystem prefix: ${hostfs_prefix}" >&2
fi

host_path() {
  local p="$1"
  if [[ -n "${hostfs_prefix}" && "${p}" == /* ]]; then
    echo "${hostfs_prefix}${p}"
    return 0
  fi
  echo "${p}"
}

requested="${1:-}"

image_name="${IMAGE_NAME:-pterminal}"
image_tag="${IMAGE_TAG:-latest}"

hub_user="${DOCKERHUB_USERNAME:-ankouros}"
ghcr_user="${GHCR_USERNAME:-ankouros}"

hub_image="${hub_user}/${image_name}:${image_tag}"
ghcr_image="ghcr.io/${ghcr_user}/${image_name}:${image_tag}"

if [[ -n "${requested}" ]]; then
  hub_image="${requested}"
  ghcr_image="${requested}"
fi

home="${HOME:-/home/$(id -un)}"
cfg_dir="${XDG_CONFIG_HOME:-${home}/.config}/pterminal"
dl_dir="${XDG_DOWNLOAD_DIR:-${home}/Downloads}"

mkdir -p "${cfg_dir}" || true
mkdir -p "${dl_dir}" || true

docker_args=(
  --rm
  --shm-size=512m
  --user "$(id -u):$(id -g)"
  --volume "$(host_path "${cfg_dir}"):/home/pterminal/.config/pterminal"
  --volume "$(host_path "${dl_dir}"):/home/pterminal/Downloads"
  --env HOME=/home/pterminal
  --env XDG_CONFIG_HOME=/home/pterminal/.config
  --env XDG_DOWNLOAD_DIR=/home/pterminal/Downloads
  --env NO_AT_BRIDGE=1
)

force_software_render=false
gpu_requested=false

if [[ "${PTERMINAL_GPU:-}" == "1" ]]; then
  gpu_requested=true
fi

if [[ "${PTERMINAL_SOFTWARE_RENDER:-}" == "1" || "${PTERMINAL_DISABLE_GPU:-}" == "1" ]]; then
  force_software_render=true
elif [[ "${gpu_requested}" == "false" ]]; then
  force_software_render=true
fi

if [[ "${snap_docker}" == "true" && "${gpu_requested}" == "false" ]]; then
  force_software_render=true
  docker_args+=(--env GSETTINGS_BACKEND=memory)
fi
if [[ "${PTERMINAL_DISABLE_GSETTINGS:-}" == "1" ]]; then
  docker_args+=(--env GSETTINGS_BACKEND=memory)
fi

if [[ "${force_software_render}" == "true" ]]; then
  echo "Using software rendering (set PTERMINAL_GPU=1 to try GPU)." >&2
  docker_args+=(--env LIBGL_ALWAYS_SOFTWARE=1)
  docker_args+=(--env WEBKIT_DISABLE_COMPOSITING_MODE=1)
  docker_args+=(--env MESA_LOADER_DRIVER_OVERRIDE=llvmpipe)
  docker_args+=(--env GALLIUM_DRIVER=llvmpipe)
fi

# Preserve host group memberships inside the container so device permissions
# (e.g. /dev/dri render/video) keep working when running as a non-root user.
for gid in $(id -G); do
  docker_args+=(--group-add "${gid}")
done

group_gid() {
  local name="$1"
  getent group "${name}" | awk -F: '{print $3}' | head -n1
}

render_gid="$(group_gid "render")"
video_gid="$(group_gid "video")"

if [[ -n "${render_gid}" ]]; then
  docker_args+=(--group-add "${render_gid}")
fi
if [[ -n "${video_gid}" ]]; then
  docker_args+=(--group-add "${video_gid}")
fi

if [[ -n "${XDG_RUNTIME_DIR:-}" && -d "${XDG_RUNTIME_DIR}" ]]; then
  docker_args+=(
    --env XDG_RUNTIME_DIR=/tmp/xdg-runtime
    --volume "$(host_path "${XDG_RUNTIME_DIR}"):/tmp/xdg-runtime"
  )

  if [[ -n "${WAYLAND_DISPLAY:-}" && -S "${XDG_RUNTIME_DIR}/${WAYLAND_DISPLAY}" ]]; then
    docker_args+=(--env WAYLAND_DISPLAY="${WAYLAND_DISPLAY}")
  fi

  if [[ -n "${DBUS_SESSION_BUS_ADDRESS:-}" ]]; then
    bus_host="unix:path=${XDG_RUNTIME_DIR}/bus"
    bus_container="unix:path=/tmp/xdg-runtime/bus"
    if [[ "${DBUS_SESSION_BUS_ADDRESS}" == "${bus_host}" ]]; then
      docker_args+=(--env DBUS_SESSION_BUS_ADDRESS="${bus_container}")
    else
      docker_args+=(--env DBUS_SESSION_BUS_ADDRESS="${DBUS_SESSION_BUS_ADDRESS}")
    fi
  fi
fi

if [[ -n "${DISPLAY:-}" && -d /tmp/.X11-unix ]]; then
  docker_args+=(
    --env DISPLAY="${DISPLAY}"
    --volume "$(host_path /tmp/.X11-unix):/tmp/.X11-unix"
  )

  xauth_src="${XAUTHORITY:-${home}/.Xauthority}"
  xauth_tmp="${XDG_RUNTIME_DIR:-/tmp}/pterminal.docker.xauth"

  if command -v xauth >/dev/null 2>&1 && [[ -f "${xauth_src}" ]]; then
    rm -f "${xauth_tmp}" 2>/dev/null || true
    touch "${xauth_tmp}"
    chmod 0600 "${xauth_tmp}" || true

    # Build an authority file that matches the container environment.
    # (Common fix when the host entry is like "hostname/unix:10" but container hostname differs.)
    if xauth -f "${xauth_src}" nlist "${DISPLAY}" 2>/dev/null | sed -e 's/^..../ffff/' | xauth -f "${xauth_tmp}" nmerge - >/dev/null 2>&1; then
      docker_args+=(
        --env XAUTHORITY=/tmp/.Xauthority
        --volume "$(host_path "${xauth_tmp}"):/tmp/.Xauthority:ro"
      )
    else
      echo "Note: could not generate Docker XAUTH file; falling back to mounting ${xauth_src}" >&2
      docker_args+=(--env XAUTHLOCALHOSTNAME="$(hostname)")
      docker_args+=(
        --env XAUTHORITY=/tmp/.Xauthority
        --volume "$(host_path "${xauth_src}"):/tmp/.Xauthority:ro"
      )
    fi
  elif [[ -f "${xauth_src}" ]]; then
    docker_args+=(--env XAUTHLOCALHOSTNAME="$(hostname)")
    docker_args+=(
      --env XAUTHORITY=/tmp/.Xauthority
      --volume "$(host_path "${xauth_src}"):/tmp/.Xauthority:ro"
    )
  else
    echo "Note: XAUTHORITY not found (${xauth_src}); if X11 connect fails, try:" >&2
    echo "  xhost +local:docker" >&2
  fi
fi

if [[ -d /dev/dri ]]; then
  docker_args+=(--device /dev/dri)
  render_node=""
  for node in /dev/dri/renderD*; do
    if [[ -e "${node}" ]]; then
      render_node="${node}"
      break
    fi
  done
  if [[ -n "${render_node}" && -z "${render_gid}" && ! -r "${render_node}" ]]; then
    echo "Warning: ${render_node} is not readable; forcing software rendering." >&2
    docker_args+=(--env LIBGL_ALWAYS_SOFTWARE=1)
    docker_args+=(--env WEBKIT_DISABLE_COMPOSITING_MODE=1)
  fi
fi

pull_one() {
  local img="$1"
  local label="$2"

  if docker pull "${img}" >/dev/null 2>&1; then
    echo "Pulled (${label}): ${img}"
    return 0
  fi
  echo "Could not pull (${label}): ${img}" >&2
  return 1
}

inspect_created() {
  local img="$1"
  docker image inspect --format '{{.Created}}' "${img}" 2>/dev/null || true
}

have_local() {
  local img="$1"
  docker image inspect "${img}" >/dev/null 2>&1
}

best_image=""
best_created=""

pulled_any=false
if pull_one "${ghcr_image}" "ghcr"; then pulled_any=true; fi
if pull_one "${hub_image}" "dockerhub"; then pulled_any=true; fi

if have_local "${ghcr_image}"; then
  c="$(inspect_created "${ghcr_image}")"
  if [[ -n "${c}" ]]; then
    best_image="${ghcr_image}"
    best_created="${c}"
  fi
fi

if have_local "${hub_image}"; then
  c="$(inspect_created "${hub_image}")"
  if [[ -n "${c}" ]]; then
    if [[ -z "${best_created}" || "${c}" > "${best_created}" ]]; then
      best_image="${hub_image}"
      best_created="${c}"
    fi
  fi
fi

if [[ -z "${best_image}" ]]; then
  echo "No runnable image found locally, and pulls failed." >&2
  echo "Tried: ${ghcr_image} and ${hub_image}" >&2
  exit 1
fi

if [[ "${pulled_any}" == false ]]; then
  echo "Warning: running local cached image (could not pull updates)." >&2
fi

echo "Running: ${best_image}"
exec docker run "${docker_args[@]}" "${best_image}"
