#!/usr/bin/env bash
# shellcheck disable=SC2015  # &&/|| assertion idiom: ok/bad cannot fail.
# Verification: lint, render all 10 directions + fixtures, assert env-only
# (no rclone.conf), volume/env shape, and fail-fast errors.
# Run from the repo root: bash projects/helm-rclone/ci/verify.sh
set -euo pipefail

CHART=projects/helm-rclone
CI="$CHART/ci"
OUT=$(mktemp -d)
trap 'rm -rf "$OUT"' EXIT

pass=0
fail=0
ok()   { pass=$((pass + 1)); echo "PASS: $1"; }
bad()  { fail=$((fail + 1)); echo "FAIL: $1"; }

# -- 1. helm lint -----------------------------------------------------------
# Lint with a direction fixture: values.yaml ships no real credentials by
# design (fail-fast guards reject empty/placeholder), so bare-defaults lint
# only proves the fail-fast fires. The fixture lint proves the chart is sound.
if helm lint "$CHART" -f "$CI/values-direction-01-pvc-rwo-to-s3.yaml" >"$OUT/lint.txt" 2>&1; then ok "helm lint"; else
  bad "helm lint"; sed 's/^/  /' "$OUT/lint.txt"
fi
# Bare-defaults render must still fail fast (fail-fast firing under template).
if helm template bare-defaults "$CHART" >"$OUT/bare-defaults.yaml" 2>"$OUT/bare-defaults.err"; then
  bad "bare-defaults render passed but must fail fast (values.yaml ships no credentials)"
elif grep -qF 'secretAccessKey' "$OUT/bare-defaults.err" || grep -qF 'is required' "$OUT/bare-defaults.err"; then
  ok "bare-defaults render fails fast (no credentials by design)"
else
  bad "bare-defaults render failed without a field-naming message"; sed 's/^/  /' "$OUT/bare-defaults.err"
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
  matches=("$CI"/values-direction-"$i"-*.yaml)
  f="${matches[0]}"
  if render "dir-$i" -f "$f"; then ok "render direction $i ($(basename "$f"))"; else
    bad "render direction $i ($(basename "$f"))"
  fi
done
if render sources-all-four -f "$CI/values-sources-all-four.yaml"; then
  ok "render four-value-source fixture"
else
  bad "render four-value-source fixture"
fi
if render colocate-rwo -f "$CI/values-colocate-rwo.yaml"; then
  ok "render RWO co-location fixture"
else
  bad "render RWO co-location fixture"
fi
if render colocate-rwo-merge -f "$CI/values-colocate-rwo-merge.yaml"; then
  ok "render RWO co-location merge fixture"
else
  bad "render RWO co-location merge fixture"
fi

# -- 3. env-only: no rclone.conf in ANY rendered manifest -------------------
if grep -ri 'rclone\.conf' "${RENDERED[@]}"; then
  bad "rendered manifests mention rclone.conf"
else
  ok "no rclone.conf in any rendered manifest"
fi
if grep -rw -- '--config' "${RENDERED[@]}"; then
  bad "rendered manifests carry a --config flag"
else
  ok "no --config file ref in any rendered manifest"
fi

# -- 4. guard + shape assertions --------------------------------------------
for f in "${RENDERED[@]}"; do
  grep -q 'name: RCLONE_CONFIG' "$f" && grep -q 'value: /dev/null' "$f" \
    && ok "$(basename "$f") sets RCLONE_CONFIG=/dev/null guard" \
    || bad "$(basename "$f") sets RCLONE_CONFIG=/dev/null guard"
  grep -q 'concurrencyPolicy: Forbid' "$f" \
    && ok "$(basename "$f") concurrencyPolicy Forbid" \
    || bad "$(basename "$f") concurrencyPolicy Forbid"
  grep -q 'restartPolicy: OnFailure' "$f" \
    && ok "$(basename "$f") restartPolicy OnFailure" \
    || bad "$(basename "$f") restartPolicy OnFailure"
  grep -q 'runAsNonRoot: true' "$f" \
    && ok "$(basename "$f") pod runs as non-root" \
    || bad "$(basename "$f") pod runs as non-root"
  grep -q 'type: RuntimeDefault' "$f" \
    && ok "$(basename "$f") seccomp RuntimeDefault" \
    || bad "$(basename "$f") seccomp RuntimeDefault"
  grep -q 'allowPrivilegeEscalation: false' "$f" \
    && ok "$(basename "$f") no privilege escalation" \
    || bad "$(basename "$f") no privilege escalation"
  grep -q 'readOnlyRootFilesystem: true' "$f" \
    && ok "$(basename "$f") read-only root filesystem" \
    || bad "$(basename "$f") read-only root filesystem"
  grep -q 'memory: 512Mi' "$f" \
    && ok "$(basename "$f") memory limit set" \
    || bad "$(basename "$f") memory limit set"
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

# RWO co-location: podAffinity renders with owner selector + hostname topology
grep -q 'podAffinity' "$OUT/colocate-rwo.yaml" \
  && ok "colocate-rwo renders podAffinity" || bad "colocate-rwo renders podAffinity"
grep -q 'requiredDuringSchedulingIgnoredDuringExecution' "$OUT/colocate-rwo.yaml" \
  && ok "colocate-rwo affinity is required (not preferred)" || bad "colocate-rwo affinity is required (not preferred)"
grep -q 'app: my-db' "$OUT/colocate-rwo.yaml" \
  && ok "colocate-rwo selects owner pods (app: my-db)" || bad "colocate-rwo selects owner pods (app: my-db)"
grep -q 'topologyKey: kubernetes.io/hostname' "$OUT/colocate-rwo.yaml" \
  && ok "colocate-rwo topologyKey is hostname" || bad "colocate-rwo topologyKey is hostname"
# Merge case: generated podAffinity coexists with user-supplied nodeAffinity
grep -q 'nodeAffinity' "$OUT/colocate-rwo-merge.yaml" \
  && ok "colocate-rwo-merge keeps user nodeAffinity" || bad "colocate-rwo-merge keeps user nodeAffinity"
grep -q 'podAffinity' "$OUT/colocate-rwo-merge.yaml" \
  && ok "colocate-rwo-merge adds podAffinity" || bad "colocate-rwo-merge adds podAffinity"
grep -q 'disktype' "$OUT/colocate-rwo-merge.yaml" \
  && ok "colocate-rwo-merge keeps user node selector terms" || bad "colocate-rwo-merge keeps user node selector terms"
# Absence: no podAffinity without coLocateWith (RWO default, RWX, remote-remote)
for d in dir-01 dir-07 dir-05 dir-10; do
  if grep -q 'podAffinity' "$OUT/$d.yaml"; then bad "$d injects no podAffinity without coLocateWith"; else ok "$d injects no podAffinity without coLocateWith"; fi
done

# remote args render as REMOTE:path
grep -q 'DST:bucket1/nightly' "$OUT/dir-01.yaml" \
  && ok "dir-01 destination arg DST:bucket1/nightly" || bad "dir-01 destination arg DST:bucket1/nightly"
grep -q 'BACKUPPROTON:/media' "$OUT/dir-04.yaml" \
  && ok "dir-04 remoteName uppercased to BACKUPPROTON" || bad "dir-04 remoteName uppercased to BACKUPPROTON"
# shellcheck disable=SC2016  # single quotes intentional: asserting the literal $(...) indirection marker in rendered YAML, not a shell expansion.
grep -q 'SRC:$(SRC_PATH)' "$OUT/dir-08.yaml" \
  && ok "dir-08 ref uri renders SRC:\$(SRC_PATH) indirection" || bad "dir-08 ref uri renders SRC:\$(SRC_PATH) indirection"
# shellcheck disable=SC2016  # single quotes intentional: asserting the literal $(...) indirection marker in rendered YAML, not a shell expansion.
grep -q 'DST:$(DST_PATH)' "$OUT/dir-10.yaml" \
  && ok "dir-10 ref uri renders DST:\$(DST_PATH) indirection" || bad "dir-10 ref uri renders DST:\$(DST_PATH) indirection"

# four value sources each render correctly at least once
grep -q 'secretKeyRef' "$OUT/sources-all-four.yaml" \
  && ok "secretRef renders secretKeyRef" || bad "secretRef renders secretKeyRef"
grep -q 'configMapKeyRef' "$OUT/sources-all-four.yaml" \
  && ok "configMapRef renders configMapKeyRef" || bad "configMapRef renders configMapKeyRef"
grep -q 'name: rclone-proton-eso' "$OUT/sources-all-four.yaml" \
  && ok "esoRef consumes ESO-synced Secret" || bad "esoRef consumes ESO-synced Secret"

# version + schedule knobs (tag@digest: tag-only renders fail, see image helper)
grep -q 'image: "rclone/rclone:1.75.0@sha256:' "$OUT/dir-07.yaml" \
  && ok "dir-07 rclone.version override renders tag@digest" || bad "dir-07 rclone.version override renders tag@digest"
if grep -q 'image: "rclone/rclone:[^"]*@sha256:' "$OUT/dir-07.yaml"; then
  ok "dir-07 image carries digest pin"
else
  bad "dir-07 image carries digest pin"
fi
for f in "${RENDERED[@]}"; do
  if grep -qE 'image: "rclone/rclone:[^"@]+"' "$f"; then
    bad "$(basename "$f") image is tag-only (digest required)"
  else
    ok "$(basename "$f") image is tag@digest"
  fi
  grep -q 'activeDeadlineSeconds: 3600' "$f" \
    && ok "$(basename "$f") activeDeadlineSeconds explicit" \
    || bad "$(basename "$f") activeDeadlineSeconds explicit"
  if grep -q 'CHANGEME\|REPLACE-ME' "$f"; then
    bad "$(basename "$f") carries a placeholder credential"
  else
    ok "$(basename "$f") carries no placeholder credential"
  fi
done
grep -q 'timeZone: "America/New_York"' "$OUT/dir-07.yaml" \
  && ok "dir-07 timeZone renders" || bad "dir-07 timeZone renders"
grep -q 'schedule: "45 3' "$OUT/dir-07.yaml" \
  && ok "dir-07 schedule renders" || bad "dir-07 schedule renders"

# -- 5. fail-fast proofs -----------------------------------------------------
expect_fail() { # desc, values-file-or-empty, expected-msg-fragment, [helm args...]
  local desc="$1" values="$2" want="$3"; shift 3
  if [ -n "$values" ]; then set -- -f "$values" "$@"; fi
  if helm template fail "$CHART" "$@" >"$OUT/fail.yaml" 2>"$OUT/fail.err"; then
    bad "$desc rendered but must fail"
  elif grep -qF "$want" "$OUT/fail.err"; then
    ok "$desc fails fast: $want"
  else
    bad "$desc failed without expected message [$want]"; sed 's/^/  /' "$OUT/fail.err"
  fi
}
# NOTE: Helm merges at field level; see README "Helm merge semantics".
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
# Placeholder literals pass shape validation but must fail fast (values.yaml
# ships no CHANGEME defaults; the guard rejects CHANGEME*/REPLACE-ME* plus
# the AWS example-key shape ^AKIA...EXAMPLE$).
expect_fail "placeholder credential" \
  "" "got placeholder" \
  --set-json 'source={"type":"pvc-rwo","uri":{"value":"d"}}' \
  --set-json 'destination={"type":"s3","uri":{"value":"b"},"credentials":{"provider":{"value":"AWS"},"accessKeyId":{"value":"CHANGEME"},"secretAccessKey":{"value":"S"},"region":{"value":"R"}}}'
expect_fail "aws example key" \
  "" "got placeholder" \
  --set-json 'source={"type":"pvc-rwo","uri":{"value":"d"}}' \
  --set-json 'destination={"type":"s3","uri":{"value":"b"},"credentials":{"provider":{"value":"AWS"},"accessKeyId":{"value":"AKIAIOSFODNN7EXAMPLE"},"secretAccessKey":{"value":"S"},"region":{"value":"R"}}}'
# Tag-only images are rejected: a version override without its digest fails.
expect_fail "tag-only version override" \
  "" "image.digest" \
  --set-json 'source={"type":"pvc-rwo","uri":{"value":"d"}}' \
  --set-json 'destination={"type":"s3","uri":{"value":"b"},"credentials":{"provider":{"value":"AWS"},"accessKeyId":{"value":"K"},"secretAccessKey":{"value":"S"},"region":{"value":"R"}}}' \
  --set rclone.version=1.75.1
# Null deadline would wedge Forbid CronJobs: an explicit value is required.
expect_fail "null activeDeadlineSeconds" \
  "" "activeDeadlineSeconds is required" \
  --set-json 'source={"type":"pvc-rwo","uri":{"value":"d"}}' \
  --set-json 'destination={"type":"s3","uri":{"value":"b"},"credentials":{"provider":{"value":"AWS"},"accessKeyId":{"value":"K"},"secretAccessKey":{"value":"S"},"region":{"value":"R"}}}' \
  --set activeDeadlineSeconds=null
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
