#!/usr/bin/env bash
# Fetch the ServiceMonitor + PodMonitor CRDs into upstream/ for packaging.
# Usage: bash projects/helm-otel-monitoring-crds/ci/fetch.sh [--out DIR] [--check]
#   --out DIR  staging dir (default: <chart>/upstream)
#   --check    verify the URLs for Chart.yaml's version answer 200 (no download)
# Version contract: Chart.yaml version == upstream release sans leading v
# (e.g. 0.93.1 -> v0.93.1); tag bumps move Chart.yaml version + appVersion together.
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

# Monitoring scope ONLY: servicemonitors + podmonitors (no Prometheus/Alertmanager/Grafana).

url_for() { printf 'https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/v%s/example/prometheus-operator-crd/%s' "$VERSION" "$1"; }

if [[ "$CHECK" == true ]]; then
  for f in monitoring.coreos.com_servicemonitors.yaml monitoring.coreos.com_podmonitors.yaml; do
    code="$(curl -sSL --fail -o /dev/null -w '%{http_code}' --max-time 30 "$(url_for "$f")")"
    [[ "$code" == "200" ]] || { echo "upstream check failed: $(url_for "$f") -> HTTP $code" >&2; exit 1; }
    echo "OK: $(url_for "$f")"
  done
  exit 0
fi

mkdir -p "$OUT"
# --fail rejects HTTP errors; marker/kind greps prove shape (no upstream signature exists to check).
for f in monitoring.coreos.com_servicemonitors.yaml monitoring.coreos.com_podmonitors.yaml; do
  curl -sSL --fail --retry 3 --max-time 120 -o "$OUT/$f" "$(url_for "$f")"
  grep -q 'monitoring.coreos.com' "$OUT/$f" \
    || { echo "shape check failed: monitoring API group missing in $OUT/$f" >&2; exit 1; }
  grep -q 'kind: CustomResourceDefinition' "$OUT/$f" \
    || { echo "shape check failed: no CustomResourceDefinition in $OUT/$f" >&2; exit 1; }
  echo "staged $(url_for "$f") -> $OUT/$f ($(wc -c <"$OUT/$f") bytes)"
done
