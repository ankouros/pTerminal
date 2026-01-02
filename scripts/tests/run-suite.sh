#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"

tests_dir="${TESTS_DIR:-$repo_root/TESTS}"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
run_dir="$tests_dir/run-$timestamp"
mkdir -p "$run_dir"

full_suite=0
if [[ "${1:-}" == "--full" || "${PTERMINAL_TEST_FULL:-}" == "1" ]]; then
  full_suite=1
fi

results=()

run_cmd() {
  local name="$1"
  shift
  local log="$run_dir/${name}.log"
  {
    echo "# $(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "+ $*"
  } > "$log"
  if "$@" >>"$log" 2>&1; then
    results+=("$name:0")
  else
    local status=$?
    results+=("$name:$status")
  fi
}

run_cmd "env" bash -c "go version && uname -a"
run_cmd "go-vet" go vet ./...
run_cmd "go-test" go test ./...

if [[ "$full_suite" -eq 1 ]]; then
  run_cmd "go-test-json" go test -json ./...
  run_cmd "go-test-race" go test -race ./...
fi

summary="$run_dir/summary.md"
{
  echo "# Test Suite Report"
  echo
  echo "- Started: $timestamp"
  echo "- Repo: $repo_root"
  echo "- Full suite: $full_suite"
  echo
  echo "## Results"
  overall=0
  for entry in "${results[@]}"; do
    name="${entry%%:*}"
    status="${entry##*:}"
    if [[ "$status" -eq 0 ]]; then
      echo "- $name: PASS"
    else
      echo "- $name: FAIL ($status)"
      overall=1
    fi
  done
  echo
  echo "## Notes"
  echo "- Logs are stored alongside this summary."
} > "$summary"

exit "${overall:-0}"
