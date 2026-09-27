#!/usr/bin/env bash
# Fetch the Multus thick-plugin bundle into upstream/ for packaging and
# re-apply the two local narrowings as script steps (documented below).
# Usage: bash projects/helm-multus/ci/fetch.sh [--out DIR] [--check]
#   --out DIR  staging dir (default: <chart>/upstream)
#   --check    verify the URL for Chart.yaml's version answers 200 (no download)
# Version contract: Chart.yaml version == upstream release sans leading v
# (e.g. 4.3.0 -> v4.3.0); tag bumps move Chart.yaml version + appVersion together.
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
    -h|--help) sed -n '2,9p' "$0"; exit 0 ;;
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
URL="https://raw.githubusercontent.com/k8snetworkplumbingwg/multus-cni/v${VERSION}/deployments/multus-daemonset-thick.yml"

if [[ "$CHECK" == true ]]; then
  code="$(curl -sSL --fail -o /dev/null -w '%{http_code}' --max-time 30 "$URL")"
  [[ "$code" == "200" ]] || { echo "upstream check failed: $URL -> HTTP $code" >&2; exit 1; }
  echo "OK: $URL"
  exit 0
fi

mkdir -p "$OUT"
# Integrity: --fail rejects HTTP errors; the marker/kind greps below prove
# shape. None of these upstreams publishes a detached signature for the
# fetched YAML, so no signature check is possible here — if a publisher
# starts signing, verify the signature before the shape checks.
RAW="$OUT/multus-daemonset-thick.yml.raw"
curl -sSL --fail --retry 3 --max-time 120 -o "$RAW" "$URL"
grep -q 'network-attachment-definitions.k8s.cni.cncf.io' "$RAW" \
  || { echo "shape check failed: NAD CRD missing in $RAW" >&2; exit 1; }

# Narrowing 1/2 — ClusterRole: upstream grants k8s.cni.cncf.io:* on verbs *;
# this repo keeps NAD get/list/watch + pods get/list/watch/update +
# pods/status get/update/patch + events create/patch (matches the previously
# vendored flux/infra/components/multus/controllers/base/multus-daemonset.yaml).
# Narrowing 2/2 — images: upstream ships snapshot-thick placeholders; both
# image fields (daemon + install-multus-binary init container) re-pin together
# to ghcr.io/k8snetworkplumbingwg/multus-cni:<tag>-thick (never snapshot-thick).
python3 - "$RAW" "$OUT/multus-daemonset-thick.yml" "$VERSION" <<'PY'
import re, sys
raw_path, out_path, version = sys.argv[1:4]
s = open(raw_path).read()
wide = """  - apiGroups: ["k8s.cni.cncf.io"]
    resources:
      - '*'
    verbs:
      - '*'"""
# Exact narrowing from the previously vendored
# flux/infra/components/multus/controllers/base/multus-daemonset.yaml:
# NAD get/list/watch + pods get/list/watch/update + pods/status
# get/update/patch + events create/patch.
narrow = """  - apiGroups: ["k8s.cni.cncf.io"]
    resources:
      - network-attachment-definitions
    verbs:
      - get
      - list
      - watch
  - apiGroups:
      - ""
    resources:
      - pods
    verbs:
      - get
      - list
      - watch
      - update
  - apiGroups:
      - ""
    resources:
      - pods/status
    verbs:
      - get
      - update
      - patch
  - apiGroups:
      - ""
      - events.k8s.io
    resources:
      - events
    verbs:
      - create
      - patch"""
assert wide in s, "upstream ClusterRole block changed shape; update the narrowing"
start = s.index(wide)
# End of the upstream rules block: the events rule ending "- update" followed by "---".
end_marker = "      - update\n---"
end = s.index(end_marker, start) + len("      - update\n")
s = s[:start] + narrow + "\n" + s[end:]
n = s.count("image: ghcr.io/k8snetworkplumbingwg/multus-cni:snapshot-thick")
assert n == 2, f"expected 2 snapshot-thick images, found {n}; update the re-pin"
s = s.replace(
  "image: ghcr.io/k8snetworkplumbingwg/multus-cni:snapshot-thick",
  f"image: ghcr.io/k8snetworkplumbingwg/multus-cni:v{version}-thick",
)
open(out_path, "w").write(s)
print(f"narrowed ClusterRole + re-pinned 2 images to v{version}-thick")
PY
rm -f "$RAW"
grep -q "image: ghcr.io/k8snetworkplumbingwg/multus-cni:v${VERSION}-thick" "$OUT/multus-daemonset-thick.yml" \
  || { echo "shape check failed: re-pinned image missing" >&2; exit 1; }
if grep -q "snapshot-thick" "$OUT/multus-daemonset-thick.yml"; then
  echo "shape check failed: snapshot-thick placeholder survived" >&2; exit 1
fi
echo "staged $URL -> $OUT/multus-daemonset-thick.yml ($(wc -c <"$OUT/multus-daemonset-thick.yml") bytes)"
