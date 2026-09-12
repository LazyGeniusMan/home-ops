#!/usr/bin/env bash
# Manual verification for helm-rclone-sync: lint, render all 10 sync
# directions + the four-value-source fixture, assert the env-only contract
# (no rclone.conf anywhere), assert volume/env shape, and prove fail-fast
# errors. No in-repo callers; run by hand from the repo root:
#   bash projects/helm-rclone-sync/ci/verify.sh
set -euo pipefail

CHART=projects/helm-rclone-sync
CI="$CHART/ci"
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

# -- 2. render all fixtures (source/destination are required; there is no
# meaningful bare-defaults render, so defaults are NOT rendered) -------------
RENDERED=()
render() { # name, [values-file...]
  local name="$1"; shift
  helm template "$name" "$CHART" "$@" >"$OUT/$name.yaml"
  RENDERED+=("$OUT/$name.yaml")
}
for i in 01 02 03 04 05 06 07 08 09 10; do
  f="$CI"/values-direction-$i-*.yaml
  # shellcheck disable=SC2086
  if render "dir-$i" -f $f; then ok "render direction $i ($(basename $f))"; else
    bad "render direction $i ($(basename $f))"
  fi
done
if render sources-all-four -f "$CI/values-sources-all-four.yaml"; then
  ok "render four-value-source fixture"
else
  bad "render four-value-source fixture"
fi

# -- 3. env-only: no rclone.conf in ANY rendered manifest -------------------
# shellcheck disable=SC2068
if grep -ri 'rclone\.conf' ${RENDERED[@]}; then
  bad "rendered manifests mention rclone.conf"
else
  ok "no rclone.conf in any rendered manifest"
fi
# shellcheck disable=SC2068
if grep -rw -- '--config' ${RENDERED[@]}; then
  bad "rendered manifests carry a --config flag"
else
  ok "no --config file ref in any rendered manifest"
fi

# -- 4. guard + shape assertions --------------------------------------------
# shellcheck disable=SC2068
for f in ${RENDERED[@]}; do
  grep -q 'name: RCLONE_CONFIG' "$f" && grep -q 'value: /dev/null' "$f" \
    && ok "$(basename "$f") sets RCLONE_CONFIG=/dev/null guard" \
    || bad "$(basename "$f") sets RCLONE_CONFIG=/dev/null guard"
  grep -q 'concurrencyPolicy: Forbid' "$f" \
    && ok "$(basename "$f") concurrencyPolicy Forbid" \
    || bad "$(basename "$f") concurrencyPolicy Forbid"
  grep -q 'restartPolicy: OnFailure' "$f" \
    && ok "$(basename "$f") restartPolicy OnFailure" \
    || bad "$(basename "$f") restartPolicy OnFailure"
done

# remote-to-remote renders zero PVC volumes / zero volumeMounts
for d in dir-05 dir-10; do
  if grep -q 'persistentVolumeClaim' "$OUT/$d.yaml"; then bad "$d has no PVC volumes"; else ok "$d has no PVC volumes"; fi
  if grep -q 'volumeMounts' "$OUT/$d.yaml"; then bad "$d has no volumeMounts"; else ok "$d has no volumeMounts"; fi
done

# PVC sides resolve to existingClaim volumes
grep -q 'claimName: data-rwo' "$OUT/dir-01.yaml" \
  && ok "dir-01 source claimName data-rwo" || bad "dir-01 source claimName data-rwo"
grep -q 'claimName: media-rwx' "$OUT/dir-02.yaml" \
  && ok "dir-02 source claimName media-rwx" || bad "dir-02 source claimName media-rwx"
grep -q 'claimName: data-rwo' "$OUT/dir-06.yaml" \
  && ok "dir-06 destination claimName data-rwo" || bad "dir-06 destination claimName data-rwo"
grep -q 'readOnly: false' "$OUT/dir-06.yaml" \
  && ok "dir-06 destination mount writable" || bad "dir-06 destination mount writable"
# dir-06's only PVC is the destination, so no readOnly:true mount must exist
if grep -q 'readOnly: true' "$OUT/dir-06.yaml"; then
  bad "dir-06 has no readOnly source mount (source is a remote)"
else
  ok "dir-06 has no readOnly source mount (source is a remote)"
fi
grep -q 'readOnly: true' "$OUT/dir-01.yaml" \
  && ok "dir-01 source mount readOnly" || bad "dir-01 source mount readOnly"

# remote args render as REMOTE:path
grep -q 'DST:bucket1/nightly' "$OUT/dir-01.yaml" \
  && ok "dir-01 destination arg DST:bucket1/nightly" || bad "dir-01 destination arg DST:bucket1/nightly"
grep -q 'BACKUPPROTON:/media' "$OUT/dir-04.yaml" \
  && ok "dir-04 remoteName uppercased to BACKUPPROTON" || bad "dir-04 remoteName uppercased to BACKUPPROTON"
grep -q 'SRC:$(SRC_PATH)' "$OUT/dir-08.yaml" \
  && ok "dir-08 ref uri renders SRC:\$(SRC_PATH) indirection" || bad "dir-08 ref uri renders SRC:\$(SRC_PATH) indirection"
grep -q 'DST:$(DST_PATH)' "$OUT/dir-10.yaml" \
  && ok "dir-10 ref uri renders DST:\$(DST_PATH) indirection" || bad "dir-10 ref uri renders DST:\$(DST_PATH) indirection"

# four value sources each render correctly at least once
grep -q 'secretKeyRef' "$OUT/sources-all-four.yaml" \
  && ok "secretRef renders secretKeyRef" || bad "secretRef renders secretKeyRef"
grep -q 'configMapKeyRef' "$OUT/sources-all-four.yaml" \
  && ok "configMapRef renders configMapKeyRef" || bad "configMapRef renders configMapKeyRef"
grep -q 'name: rclone-proton-eso' "$OUT/sources-all-four.yaml" \
  && ok "esoRef consumes ESO-synced Secret" || bad "esoRef consumes ESO-synced Secret"

# version + schedule knobs
grep -q 'image: "rclone/rclone:1.75.0"' "$OUT/dir-07.yaml" \
  && ok "dir-07 rclone.version override renders" || bad "dir-07 rclone.version override renders"
grep -q 'timeZone: "America/New_York"' "$OUT/dir-07.yaml" \
  && ok "dir-07 timeZone renders" || bad "dir-07 timeZone renders"
grep -q 'schedule: "45 3' "$OUT/dir-07.yaml" \
  && ok "dir-07 schedule renders" || bad "dir-07 schedule renders"

# -- 5. fail-fast proofs -----------------------------------------------------
expect_fail() { # desc, values-file-or-empty, expected-msg-fragment, [helm args...]
  local desc="$1" values="$2" want="$3"; shift 3
  # shellcheck disable=SC2068
  if [ -n "$values" ]; then set -- -f "$values" $@; fi
  if helm template fail "$CHART" $@ >"$OUT/fail.yaml" 2>"$OUT/fail.err"; then
    bad "$desc rendered but must fail"
  elif grep -qF "$want" "$OUT/fail.err"; then
    ok "$desc fails fast: $want"
  else
    bad "$desc failed without expected message [$want]"; sed 's/^/  /' "$OUT/fail.err"
  fi
}
# NOTE: --set/--set-json merge INTO the demo defaults at the FIELD level, so
# whole-endpoint replacement must pass a complete endpoint map via --set-json:
# a partial map would deep-merge with the demo default's sibling keys.
# --set-json merges too, so "missing key" must be proven by nulling the key
# out of the merged map (a bare omit would inherit the demo default — that is
# Helm semantics, not a chart bug).
expect_fail "missing s3 secretAccessKey" \
  "" "secretAccessKey is required" \
  --set-json 'source={"type":"pvc-rwo","uri":{"value":"d"}}' \
  --set-json 'destination={"type":"s3","uri":{"value":"b"},"credentials":{"provider":{"value":"AWS"},"accessKeyId":{"value":"K"},"secretAccessKey":null,"region":{"value":"R"}}}'
expect_fail "bogus endpoint type" \
  "" "must be one of pvc-rwo" \
  --set-json 'source={"type":"nfs","uri":{"value":"d"}}' \
  --set-json 'destination={"type":"s3","uri":{"value":"b"},"credentials":{"provider":{"value":"AWS"},"accessKeyId":{"value":"K"},"secretAccessKey":{"value":"S"},"region":{"value":"R"}}}'
expect_fail "bogus operation" \
  "" 'must be "sync" or "copy"' \
  --set-json 'source={"type":"pvc-rwo","uri":{"value":"d"}}' \
  --set-json 'destination={"type":"s3","uri":{"value":"b"},"credentials":{"provider":{"value":"AWS"},"accessKeyId":{"value":"K"},"secretAccessKey":{"value":"S"},"region":{"value":"R"}}}' \
  --set rclone.operation=serve
expect_fail "pvc uri via secretRef" \
  "" "cannot use valueFrom" \
  --set-json 'source={"type":"s3","uri":{"value":"b"},"credentials":{"provider":{"value":"AWS"},"accessKeyId":{"value":"K"},"secretAccessKey":{"value":"S"},"region":{"value":"R"}}}' \
  --set-json 'destination={"type":"pvc-rwo","uri":{"secretRef":{"name":"s","key":"k"}}}'

# -- 6. optional extra checks (best effort) ----------------------------------
if command -v yamllint >/dev/null; then
  if yamllint -d "{extends: relaxed, rules: {line-length: {max: 120}}}" \
      "$CHART/Chart.yaml" "$CHART/values.yaml" "$CI"/values-*.yaml >"$OUT/yamllint.txt" 2>&1; then
    ok "yamllint chart + fixtures"
  else
    bad "yamllint chart + fixtures"; sed 's/^/  /' "$OUT/yamllint.txt"
  fi
fi
if command -v kubeconform >/dev/null; then
  if helm template kubeconform "$CHART" -f "$CI/values-direction-01-pvc-rwo-to-s3.yaml" \
      | kubeconform -strict -summary >"$OUT/kubeconform.txt" 2>&1; then
    ok "kubeconform CronJob (strict)"
  else
    bad "kubeconform CronJob (strict)"; sed 's/^/  /' "$OUT/kubeconform.txt"
  fi
fi

echo "---"
echo "verify.sh: $pass passed, $fail failed"
exit "$([ "$fail" -eq 0 ] && echo 0 || echo 1)"
