#!/usr/bin/env bash
set -euo pipefail

root="${1:-.}"

find "$root" -type f -name '*.go' \
  -not -path "$root/.cache/*" \
  -not -path "$root/vendor/*" \
  -not -path "$root/specs/*" \
  -not -path "$root/bin/*" \
  -not -path "$root/dist/*" \
  -not -path "$root/release/*" \
  -print0 | xargs -0 -r gofmt -w
