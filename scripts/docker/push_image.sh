#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"

env_file="${ENV_FILE:-${ROOT_DIR}/scripts/docker/.env}"
if [[ -f "${env_file}" ]]; then
  # shellcheck disable=SC1090
  source "${env_file}"
fi

: "${DOCKERHUB_USERNAME:?Set DOCKERHUB_USERNAME (or create scripts/docker/.env)}"
: "${DOCKERHUB_TOKEN:?Set DOCKERHUB_TOKEN (or create scripts/docker/.env)}"

: "${GHCR_USERNAME:=ankouros}"
: "${GHCR_TOKEN:=}"

image_name="${IMAGE_NAME:-pterminal}"
image_tag="${IMAGE_TAG:-latest}"

hub_image="${DOCKERHUB_USERNAME}/${image_name}:${image_tag}"
ghcr_image="ghcr.io/${GHCR_USERNAME}/${image_name}:${image_tag}"

cd "${ROOT_DIR}"

echo "Logging into Docker Hub as ${DOCKERHUB_USERNAME}"
printf "%s" "${DOCKERHUB_TOKEN}" | docker login -u "${DOCKERHUB_USERNAME}" --password-stdin

if [[ -n "${GHCR_TOKEN}" ]]; then
  echo "Logging into GHCR as ${GHCR_USERNAME}"
  printf "%s" "${GHCR_TOKEN}" | docker login ghcr.io -u "${GHCR_USERNAME}" --password-stdin
else
  echo "GHCR_TOKEN not set; will skip GHCR push." >&2
fi

echo "Building Docker image: ${hub_image}"
docker build -t "${hub_image}" .

echo "Tagging: ${hub_image} -> ${ghcr_image}"
docker tag "${hub_image}" "${ghcr_image}"

echo "Pushing (Docker Hub): ${hub_image}"
docker push "${hub_image}"

if [[ -n "${GHCR_TOKEN}" ]]; then
  echo "Pushing (GHCR): ${ghcr_image}"
  docker push "${ghcr_image}"
fi

echo "Publishing Docker repository overview..."
"${ROOT_DIR}/scripts/docker/publish_readme.sh"
