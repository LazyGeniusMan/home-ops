#!/usr/bin/env bash
# Fetch the KubeVirt operator bundle into upstream/ for packaging.
# Usage: bash projects/helm-kubevirt/ci/fetch.sh [--out DIR] [--check]
#   --out DIR  staging dir (default: <chart>/upstream)
#   --check    verify the URL for Chart.yaml's version answers 200 (no download)
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
URL="https://github.com/kubevirt/kubevirt/releases/download/v${VERSION}/kubevirt-operator.yaml"

if [[ "$CHECK" == true ]]; then
  code="$(curl -sSL --fail -o /dev/null -w '%{http_code}' --max-time 30 "$URL")"
  [[ "$code" == "200" ]] || { echo "upstream check failed: $URL -> HTTP $code" >&2; exit 1; }
  echo "OK: $URL"
  exit 0
fi

mkdir -p "$OUT"
# --fail rejects HTTP errors; marker/kind greps prove shape (no upstream signature exists to check).
curl -sSL --fail --retry 3 --max-time 120 -o "$OUT/kubevirt-operator.yaml" "$URL"
grep -q 'operator.kubevirt.io' "$OUT/kubevirt-operator.yaml" \
  || { echo "shape check failed: operator marker missing in $OUT/kubevirt-operator.yaml" >&2; exit 1; }
grep -q 'kind: CustomResourceDefinition' "$OUT/kubevirt-operator.yaml" \
  || { echo "shape check failed: no CustomResourceDefinition in $OUT/kubevirt-operator.yaml" >&2; exit 1; }
echo "staged $URL -> $OUT/kubevirt-operator.yaml ($(wc -c <"$OUT/kubevirt-operator.yaml") bytes)"
