#!/usr/bin/env bash
# Pre-commit guard: reject build artifacts and credentials that must never
# be committed (only *.template files and {{ }} placeholders are committed).
# Reads staged paths (added/copied/modified) from argv or `git diff --cached`.
set -euo pipefail

patterns=(
  'talosconfig$'
  'kubeconfig$'
  'secrets\.bundle\.yml$'
  'proton-pass-pat$'
  '(^|/)pat\.yml$'
  '\.tfvars$'
  '\.tfstate(\..*)?$'
  '\.terraform\.lock\.hcl$'
  '\.terraformrc$'
  '(^|/)\.terraform/'
  'crash\.log$'
  'override\.tf(\.json)?$'
  '_override\.tf(\.json)?$'
  '(^|/)\.ansible/'
  '(^|/)\.env(\.|$)'
  '\.pem$'
  '\.key$'
  '\.p12$'
  '\.pfx$'
  '\.token$'
  '(^|/)auth\.json$'
  '\.log$'
  '\.tgz$'
  'coverage\.out$'
  'cover\.out$'
  'projects/helm-[^/]+/upstream/'
  'projects/helm-[^/]+/\.fetch-staging/'
)
allow_patterns=(
  '\.example$'
  '(^|/)pat\.yml\.template$'
)

paths=("$@")
if [ "${#paths[@]}" -eq 0 ]; then
  mapfile -t paths < <(git diff --cached --name-only --diff-filter=ACM)
fi

fail=0
for p in "${paths[@]}"; do
  [ -n "$p" ] || continue
  allowed=0
  for a in "${allow_patterns[@]}"; do
    if [[ "$p" =~ $a ]]; then
      allowed=1
      break
    fi
  done
  [ "$allowed" -eq 1 ] && continue
  for pat in "${patterns[@]}"; do
    if [[ "$p" =~ $pat ]]; then
      echo "BLOCKED: $p matches forbidden artifact pattern ($pat)" >&2
      fail=1
      break
    fi
  done
done
if [ "$fail" -ne 0 ]; then
  echo "Commit only *.template files and {{ }} placeholders; build artifacts stay gitignored." >&2
  exit 1
fi
