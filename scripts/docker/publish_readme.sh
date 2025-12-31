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

: "${GHCR_USERNAME:=${DOCKERHUB_USERNAME}}"
: "${GHCR_TOKEN:=}"
: "${GHCR_OWNER_TYPE:=user}"

readme_path="${DOCKER_README_PATH:-${ROOT_DIR}/docs/tutorials/docker-repository-overview.md}"
if [[ ! -f "${readme_path}" ]]; then
  echo "Missing ${readme_path}; cannot publish Docker repository overview." >&2
  exit 1
fi

image_name="${IMAGE_NAME:-pterminal}"

short_desc=$(python3 - <<'PY' "${readme_path}"
import sys
from pathlib import Path
path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")
lines = [line.strip() for line in text.splitlines()]
short = ""
for line in lines:
    if not line:
        continue
    if line.startswith("#"):
        continue
    short = line
    break
if not short:
    short = "pTerminal Docker image"
print(short[:300])
PY
)

payload=$(python3 - <<'PY' "${readme_path}" "${short_desc}"
import json
import sys
from pathlib import Path
path = Path(sys.argv[1])
short = sys.argv[2]
text = path.read_text(encoding="utf-8")
print(json.dumps({"full_description": text, "description": short}))
PY
)

login_resp="$(mktemp)"
http_code="$(curl -sS -o "${login_resp}" -w "%{http_code}" \
  -H "Content-Type: application/json" \
  -X POST -d "{\"username\":\"${DOCKERHUB_USERNAME}\",\"password\":\"${DOCKERHUB_TOKEN}\"}" \
  https://hub.docker.com/v2/users/login/)"

hub_token="$(python3 - <<'PY' "${login_resp}"
import json
import sys
from pathlib import Path

path = Path(sys.argv[1])
try:
    data = json.loads(path.read_text(encoding="utf-8"))
    print(data.get("token", ""))
except Exception:
    print("")
PY
)"

rm -f "${login_resp}"

if [[ "${http_code}" != "200" || -z "${hub_token}" ]]; then
  echo "Failed to authenticate with Docker Hub (HTTP ${http_code})." >&2
  exit 1
fi

hub_url="https://hub.docker.com/v2/repositories/${DOCKERHUB_USERNAME}/${image_name}/"

curl -s -X PATCH \
  -H "Authorization: JWT ${hub_token}" \
  -H "Content-Type: application/json" \
  -d "${payload}" \
  "${hub_url}" >/dev/null

echo "Updated Docker Hub description for ${DOCKERHUB_USERNAME}/${image_name}."

if [[ -n "${GHCR_TOKEN}" ]]; then
  gh_payload=$(python3 - <<'PY' "${short_desc}"
import json
import sys
print(json.dumps({"description": sys.argv[1]}))
PY
  )

  if [[ "${GHCR_OWNER_TYPE}" == "org" ]]; then
    gh_url="https://api.github.com/orgs/${GHCR_USERNAME}/packages/container/${image_name}"
  else
    gh_url="https://api.github.com/user/packages/container/${image_name}"
  fi

  curl -s -X PATCH \
    -H "Authorization: Bearer ${GHCR_TOKEN}" \
    -H "Accept: application/vnd.github+json" \
    -d "${gh_payload}" \
    "${gh_url}" >/dev/null || true

  echo "Updated GHCR package description for ${GHCR_USERNAME}/${image_name}."
else
  echo "GHCR_TOKEN not set; skipping GHCR description update." >&2
fi
