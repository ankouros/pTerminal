#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"

env_file="${ENV_FILE:-${ROOT_DIR}/scripts/docker/.env}"
if [[ -f "${env_file}" ]]; then
  # shellcheck disable=SC1090
  source "${env_file}"
fi

image_name="${IMAGE_NAME:-pterminal}"
image_tag="${IMAGE_TAG:-latest}"
hub_user="${DOCKERHUB_USERNAME:-ankouros}"
image_repo="${hub_user}/${image_name}"

cd "${ROOT_DIR}"

echo "Building Docker image: ${image_repo}:${image_tag}"
docker build -t "${image_repo}:${image_tag}" .
