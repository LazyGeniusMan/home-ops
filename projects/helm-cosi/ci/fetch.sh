#!/usr/bin/env bash
# Fetch the 5 COSI CRDs into upstream/ for packaging.
# Usage: bash projects/helm-cosi/ci/fetch.sh [--out DIR] [--check]
#   --out DIR  staging dir (default: <chart>/upstream)
#   --check    verify the URLs for Chart.yaml's version answer 200 (no download)
# Version contract: see README (Chart.yaml version == upstream sans leading v).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHART_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CHART_NAME="$(basename "$CHART_DIR")"
REPO_ROOT="$(cd "$CHART_DIR/../.." && pwd)"
CHART="projects/$CHART_NAME"
cd "$REPO_ROOT"
OUT=""
CHECK=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --out) OUT="${2:-}"; shift 2 ;;
    --check) CHECK=true; shift ;;
    -h|--help) sed -n '2,7p' "$0"; exit 0 ;;
    *) echo "unknown flag '$1'. See --help." >&2; exit 1 ;;
  esac
done
[[ -n "$OUT" ]] || OUT="$CHART/upstream"

chart_version() {
  if command -v yq >/dev/null 2>&1; then yq '.version' "$CHART/Chart.yaml"
  else grep -E '^version:' "$CHART/Chart.yaml" | awk '{print $2}'
  fi
}

VERSION="$(chart_version | tr -d '"[:space:]')"
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] \
  || { echo "refusing non-semver Chart.yaml version: '$VERSION'" >&2; exit 1; }

# All 5 files re-vendor together from the tag; do NOT track main (v1alpha2).

url_for() { printf 'https://raw.githubusercontent.com/kubernetes-sigs/container-object-storage-interface/v%s/client/config/crd/%s' "$VERSION" "$1"; }

if [[ "$CHECK" == true ]]; then
  for f in objectstorage.k8s.io_bucketaccessclasses.yaml objectstorage.k8s.io_bucketaccesses.yaml objectstorage.k8s.io_bucketclaims.yaml objectstorage.k8s.io_bucketclasses.yaml objectstorage.k8s.io_buckets.yaml; do
    code="$(curl -sSL --fail -o /dev/null -w '%{http_code}' --max-time 30 "$(url_for "$f")")"
    [[ "$code" == "200" ]] || { echo "upstream check failed: $(url_for "$f") -> HTTP $code" >&2; exit 1; }
    echo "OK: $(url_for "$f")"
  done
  exit 0
fi

mkdir -p "$OUT"
# --fail rejects HTTP errors; marker/kind greps prove shape (no upstream signature exists to check).
for f in objectstorage.k8s.io_bucketaccessclasses.yaml objectstorage.k8s.io_bucketaccesses.yaml objectstorage.k8s.io_bucketclaims.yaml objectstorage.k8s.io_bucketclasses.yaml objectstorage.k8s.io_buckets.yaml; do
  curl -sSL --fail --retry 3 --max-time 120 -o "$OUT/$f" "$(url_for "$f")"
  grep -q 'objectstorage.k8s.io' "$OUT/$f" \
    || { echo "shape check failed: COSI API group missing in $OUT/$f" >&2; exit 1; }
  grep -q 'kind: CustomResourceDefinition' "$OUT/$f" \
    || { echo "shape check failed: no CustomResourceDefinition in $OUT/$f" >&2; exit 1; }
  echo "staged $(url_for "$f") -> $OUT/$f ($(wc -c <"$OUT/$f") bytes)"
done
