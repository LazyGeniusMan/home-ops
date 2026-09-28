#!/usr/bin/env bash
# Per-service Go gate: CI test-leg mirror for prek hooks.
# Usage: scripts/hooks/go-gate.sh fast|full projects/<svc>
# fast (pre-commit): vet + gofmt + tidy -diff + build — offline, ~2s.
# full (pre-push): fast + test + lint + vuln — whole module, ~10s.
set -euo pipefail

mode="${1:?usage: go-gate.sh fast|full <svc-dir>}"
svc="${2:?usage: go-gate.sh fast|full <svc-dir>}"
root="$(git rev-parse --show-toplevel)"
cd "$root/$svc" || exit 1

fail=0
step() { # name, cmd...
  local name="$1"
  shift
  echo "--- $name"
  if "$@"; then
    echo "PASS: $name"
  else
    echo "FAIL: $name"
    fail=1
  fi
}
step "go vet" go vet ./...
gofmt_out="$(gofmt -l .)"
if [ -z "$gofmt_out" ]; then
  echo "PASS: gofmt"
else
  echo "FAIL: gofmt"
  printf '%s\n' "$gofmt_out"
  fail=1
fi
step "go mod tidy -diff" go mod tidy -diff
step "go build" go build ./...
if [ "$mode" = "full" ]; then
  step "go test" go test -race -shuffle=on -coverprofile=coverage.out ./...
  step "golangci-lint" golangci-lint run ./...
  step "govulncheck" govulncheck ./...
fi
exit "$fail"
