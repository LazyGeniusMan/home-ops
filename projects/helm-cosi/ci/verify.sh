#!/usr/bin/env bash
# Verification: lint, fetch dry-run shape asserts, no-CRD-committed guard.
# Run from anywhere: bash projects/helm-cosi/ci/verify.sh (resolves the repo root itself).
# shellcheck disable=SC2015  # &&/|| assertion idiom: ok/bad cannot fail.
set -euo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHART_NAME="$(basename "$CHART_DIR")"
REPO_ROOT="$(cd "$CHART_DIR/../.." && pwd)"
CHART="projects/$CHART_NAME"
cd "$REPO_ROOT"
OUT=$(mktemp -d)
trap 'rm -rf "$OUT"' EXIT

pass=0
fail=0
ok()   { pass=$((pass + 1)); echo "PASS: $1"; }
bad()  { fail=$((fail + 1)); echo "FAIL: $1"; }

# -- 1. helm lint -----------------------------------------------------------
if helm lint "$CHART" >"$OUT/lint.txt" 2>&1; then ok "helm lint"; else
  bad "helm lint"; sed 's/^/  /' "$OUT/lint.txt"
fi

# -- 2. version contract: Chart.yaml version == appVersion sans leading v ----
VERSION="$(yq '.version' "$CHART/Chart.yaml" 2>/dev/null || grep -E '^version:' "$CHART/Chart.yaml" | awk '{print $2}')"
APPVERSION="$(yq '.appVersion' "$CHART/Chart.yaml" 2>/dev/null || grep -E '^appVersion:' "$CHART/Chart.yaml" | awk '{print $2}')"
VERSION="$(echo "$VERSION" | tr -d '"[:space:]')"
APPVERSION="$(echo "$APPVERSION" | tr -d '"[:space:]')"
[[ "$APPVERSION" == "v$VERSION" ]] \
  && ok "version contract (version $VERSION, appVersion $APPVERSION)" \
  || bad "version contract (version $VERSION, appVersion $APPVERSION)"

# -- 3. upstream URL(s) answer 200 for the pinned version (no download) ------
if bash "$CHART/ci/fetch.sh" --check >"$OUT/check.txt" 2>&1; then
  ok "fetch --check"
  sed 's/^/  /' "$OUT/check.txt"
else
  bad "fetch --check"; sed 's/^/  /' "$OUT/check.txt"
fi

# -- 4. staged bundle(s) render (fetch to temp dir, copy into a working
# chart copy, render) ------------------------------------------------------
STAGE="$OUT/stage"
if bash "$CHART/ci/fetch.sh" --out "$STAGE" >"$OUT/fetch.txt" 2>&1; then
  ok "fetch stages bundle"
  sed 's/^/  /' "$OUT/fetch.txt"
  # Render from a working copy so a pre-staged upstream/ in the checkout is never wiped.
  WORK="$OUT/chart-work"
  cp -a "$CHART" "$WORK"
  mkdir -p "$WORK/upstream"
  cp "$STAGE/"*.yaml "$WORK/upstream/"
  if helm template staged "$WORK" >"$OUT/render.yaml" 2>"$OUT/render.err"; then
    ok "helm template renders staged bundle"
    CRD_COUNT="$(grep -c 'kind: CustomResourceDefinition' "$OUT/render.yaml" || true)"
    [[ "$CRD_COUNT" == "5" ]] \
      && ok "rendered bundle has 5 CRD(s)" \
      || bad "rendered bundle has $CRD_COUNT CRDs (expected 5)"
    for crd in bucketaccessclasses bucketaccesses bucketclaims bucketclasses buckets; do
      grep -q "name: $crd.objectstorage.k8s.io" "$OUT/render.yaml" \
        && ok "rendered bundle carries $crd CRD" || bad "rendered bundle carries $crd CRD"
    done
  else
    bad "helm template renders staged bundle"; sed 's/^/  /' "$OUT/render.err"
  fi
else
  bad "fetch stages bundle"; sed 's/^/  /' "$OUT/fetch.txt"
fi

# -- 5. fail-fast: bare render without staged upstream must fail -------------
# (bare checkout has no upstream/ dir, so the template fail() must fire.)
if helm template bare "$CHART" >"$OUT/bare.yaml" 2>"$OUT/bare.err"; then
  bad "bare render without staged upstream rendered but must fail"
elif grep -q 'upstream bundle missing' "$OUT/bare.err" && grep -q 'ci/fetch.sh' "$OUT/bare.err"; then
  ok "bare render fails fast (points at ci/fetch.sh)"
else
  bad "bare render fails without fetch pointer"; sed 's/^/  /' "$OUT/bare.err"
fi

# -- 6. no CRD bundles committed (only fixtures allowed) ----------------------
if git ls-files "$CHART" | grep -Ev 'ci/|README.md|Chart.yaml|values.yaml|templates/(bucketaccessclasses.yaml.tpl|bucketaccesses.yaml.tpl|bucketclaims.yaml.tpl|bucketclasses.yaml.tpl|buckets.yaml.tpl|_helpers.tpl|NOTES.txt)' | grep -q .; then
  bad "unexpected committed files under $CHART"; git ls-files "$CHART" | sed 's/^/  /'
else
  ok "no CRD bundles committed under $CHART"
fi
if git ls-files "$CHART" | grep -Eq 'upstream/|\.fetch-staging/'; then
  bad "staged upstream committed under $CHART"
else
  ok "no staged upstream committed under $CHART"
fi

echo "---"
echo "PASS: $pass  FAIL: $fail"
[[ "$fail" -eq 0 ]]
