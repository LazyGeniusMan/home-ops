#!/usr/bin/env bash
# Fetch the Gateway API standard-channel bundle into upstream/ for packaging.
# Usage: bash projects/helm-gateway-api/ci/fetch.sh [--out DIR] [--check]
#   --out DIR  staging dir (default: <chart>/upstream)
#   --check    verify the URL for Chart.yaml's version answers 200 (no download)
# Version contract: Chart.yaml version == upstream release sans leading v
# (e.g. 1.6.1 -> v1.6.1); tag bumps move Chart.yaml version + appVersion together.
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
URL="https://github.com/kubernetes-sigs/gateway-api/releases/download/v${VERSION}/standard-install.yaml"

if [[ "$CHECK" == true ]]; then
  code="$(curl -sSL -o /dev/null -w '%{http_code}' --max-time 30 "$URL")"
  [[ "$code" == "200" ]] || { echo "upstream check failed: $URL -> HTTP $code" >&2; exit 1; }
  echo "OK: $URL"
  exit 0
fi

mkdir -p "$OUT"
curl -sSL --retry 3 --max-time 120 -o "$OUT/standard-install.yaml" "$URL"
grep -q 'Gateway API Standard channel install' "$OUT/standard-install.yaml" \
  || { echo "shape check failed: standard-channel marker missing in $OUT/standard-install.yaml" >&2; exit 1; }
grep -q 'kind: CustomResourceDefinition' "$OUT/standard-install.yaml" \
  || { echo "shape check failed: no CustomResourceDefinition in $OUT/standard-install.yaml" >&2; exit 1; }
echo "staged $URL -> $OUT/standard-install.yaml ($(wc -c <"$OUT/standard-install.yaml") bytes)"
