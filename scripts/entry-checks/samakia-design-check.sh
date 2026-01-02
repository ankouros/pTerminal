#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"

required_files=(
  "README.md"
  "AGENTS.md"
  "CONTRACTS.md"
  "SECURITY.md"
  "ROADMAP.md"
  "CHANGELOG.md"
  "docs/README.md"
  "docs/change-checklist.md"
  "ARCHITECTURE.md"
  "DECISIONS.md"
  "docs/concepts/samakia-integration.md"
  "docs/concepts/samakia-development-flow.md"
  "docs/concepts/samakia-inventory-import.md"
)

missing=0
for path in "${required_files[@]}"; do
  if [[ ! -f "$path" ]]; then
    echo "Missing entry point: $path" >&2
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

check_pattern "ROADMAP.md" "Strategy \(Design to Production\)"
check_pattern "Makefile" "^samakia\.design\.check:"
check_pattern "Makefile" "^samakia\.verify:"
check_pattern "Makefile" "^samakia\.accept:"

echo "PASS: samakia design entry points present"
