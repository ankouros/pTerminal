#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"

required_files=(
  "docs/testing-suite.md"
  "TESTS/README.md"
  "scripts/tests/run-suite.sh"
)

missing=0
for path in "${required_files[@]}"; do
  if [[ ! -f "$path" ]]; then
    echo "Missing testing suite entry point: $path" >&2
    missing=1
  fi
done

if [[ $missing -ne 0 ]]; then
  exit 1
fi

check_pattern() {
  local file="$1"
  local pattern="$2"
  if command -v rg >/dev/null 2>&1; then
    rg -q "$pattern" "$file"
  else
    grep -q "$pattern" "$file"
  fi
}

check_pattern "Makefile" "^tests\\.report:"
check_pattern "Makefile" "^samakia\\.testing\\.check:"
check_pattern "Makefile" "^samakia\\.testing\\.verify:"
check_pattern "Makefile" "^samakia\\.testing\\.accept:"

echo "PASS: samakia testing suite entry points present"
