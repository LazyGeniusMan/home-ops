{{/* Shared helpers (helm-rclone.* prefix). All per-backend env/volume logic lives here; cronjob.yaml only includes helpers, so all 10 directions render from one generic template. */}}

{{/* Full name: <release>-<chart>, honouring nameOverride/fullnameOverride. */}}
{{- define "helm-rclone.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "helm-rclone.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "helm-rclone.selectorLabels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "helm-rclone.labels" -}}
helm.sh/chart: {{ include "helm-rclone.chart" . }}
{{ include "helm-rclone.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/* Container image. rclone.version overrides image.tag and rclone.digest
overrides image.digest; the result must be tag@digest, never tag-only or
"latest". A version override without its own digest fails instead of going
tag-only: rclone.digest defaults to image.digest only when no version
override is set. */}}
{{- define "helm-rclone.image" -}}
{{- $versionOverride := .Values.rclone.version | default "" | toString -}}
{{- $digest := .Values.rclone.digest | default "" | toString -}}
{{- if eq $digest "" -}}
{{- if eq $versionOverride "" -}}
{{- $digest = .Values.image.digest | default "" | toString -}}
{{- end -}}
{{- end -}}
{{- $tag := $versionOverride | default .Values.image.tag -}}
{{- $tag = required "image.tag (or rclone.version override) is required; pin an exact version, never \"latest\"" $tag -}}
{{- if or (eq ($tag | toString) "latest") (hasPrefix "latest" ($tag | toString)) -}}
{{- fail "image tag must be an exact pin, never \"latest\"" -}}
{{- end -}}
{{- $digest = required "image.digest (or rclone.digest with rclone.version) is required; pin the registry digest, e.g. docker buildx imagetools inspect rclone/rclone:<tag>" $digest -}}
{{- printf "%s:%s@%s" .Values.image.repository $tag $digest -}}
{{- end -}}

{{/* rclone.operation must be a one-shot transfer: sync (mirror default) or copy. Never anything long-lived. */}}
{{- define "helm-rclone.operation" -}}
{{- if not (has .Values.rclone.operation (list "sync" "copy")) -}}
{{- fail (printf "rclone.operation %q is invalid: must be \"sync\" or \"copy\" (one-shot only)" (.Values.rclone.operation | toString)) -}}
{{- end -}}
{{- print .Values.rclone.operation -}}
{{- end -}}

{{/* restartPolicy must satisfy the Jobs requirement (OnFailure or Never). */}}
{{- define "helm-rclone.restartPolicy" -}}
{{- if not (has .Values.restartPolicy (list "OnFailure" "Never")) -}}
{{- fail (printf "restartPolicy %q is invalid: Jobs require OnFailure or Never" (.Values.restartPolicy | toString)) -}}
{{- end -}}
{{- print .Values.restartPolicy -}}
{{- end -}}

{{- define "helm-rclone.concurrencyPolicy" -}}
{{- if not (has .Values.concurrencyPolicy (list "Allow" "Forbid" "Replace")) -}}
{{- fail (printf "concurrencyPolicy %q is invalid: must be Allow, Forbid or Replace (keep Forbid, see README)" (.Values.concurrencyPolicy | toString)) -}}
{{- end -}}
{{- print .Values.concurrencyPolicy -}}
{{- end -}}

{{/* Endpoint type guard. Expects dict {type, role}. */}}
{{- define "helm-rclone.validateType" -}}
{{- if not .type -}}
{{- fail (printf "%s.type is required: must be one of pvc-rwo, pvc-rwx, s3, proton-drive" .role) -}}
{{- end -}}
{{- if not (has .type (list "pvc-rwo" "pvc-rwx" "s3" "proton-drive")) -}}
{{- fail (printf "%s.type %q is invalid: must be one of pvc-rwo, pvc-rwx, s3, proton-drive" .role .type) -}}
{{- end -}}
{{- end -}}

{{/* Remote name for RCLONE_CONFIG_<REMOTE>_*. Expects dict {endpoint, default}: explicit remoteName (uppercased, validated) wins, else the side default. */}}
{{- define "helm-rclone.remoteName" -}}
{{- $override := .endpoint.remoteName | default "" | toString -}}
{{- if $override -}}
{{- $upper := $override | upper -}}
{{- if not (regexMatch "^[A-Za-z][A-Za-z0-9_]*$" $upper) -}}
{{- fail (printf "remoteName %q is invalid: after uppercasing it must match ^[A-Za-z][A-Za-z0-9_]*$ so RCLONE_CONFIG_<REMOTE>_* variables are legal" $override) -}}
{{- end -}}
{{- print $upper -}}
{{- else -}}
{{- print .default -}}
{{- end -}}
{{- end -}}

{{/* One env entry from any of the four value sources (value | secretRef | configMapRef | esoRef/existingSecret; plain string = {value}). Expects dict {name, field, ctx}. */}}
{{- define "helm-rclone.renderEnv" -}}
{{- $name := .name -}}
{{- $field := .field -}}
{{- $ctx := .ctx -}}
{{- $allowed := list "value" "secretRef" "configMapRef" "esoRef" "existingSecret" -}}
- name: {{ $name }}
  {{- if kindIs "string" $field }}
  value: {{ $field | quote }}
  {{- else if kindIs "map" $field }}
  {{- $present := list -}}
  {{- range $k := keys $field -}}
  {{- if not (has $k $allowed) }}{{ fail (printf "%s: unknown value source %q (allowed: value, secretRef, configMapRef, esoRef/existingSecret)" $ctx $k) }}{{ end -}}
  {{- $present = append $present $k -}}
  {{- end -}}
  {{- if ne (len $present) 1 }}{{ fail (printf "%s: exactly one value source is required (value, secretRef, configMapRef, esoRef), got %d" $ctx (len $present)) }}{{ end -}}
  {{- $kind := index $present 0 -}}
  {{- if eq $kind "value" }}
  value: {{ $field.value | toString | quote }}
  {{- else if eq $kind "secretRef" }}
  valueFrom:
    secretKeyRef:
      name: {{ required (printf "%s.secretRef.name is required" $ctx) $field.secretRef.name }}
      key: {{ required (printf "%s.secretRef.key is required" $ctx) $field.secretRef.key }}
  {{- else if eq $kind "configMapRef" }}
  valueFrom:
    configMapKeyRef:
      name: {{ required (printf "%s.configMapRef.name is required" $ctx) $field.configMapRef.name }}
      key: {{ required (printf "%s.configMapRef.key is required" $ctx) $field.configMapRef.key }}
  {{- else }}
  {{- $ref := $field.esoRef | default $field.existingSecret }}
  valueFrom:
    secretKeyRef:
      name: {{ required (printf "%s.esoRef.name is required (Secret already synced by External Secrets Operator; this chart creates no ExternalSecrets)" $ctx) $ref.name }}
      key: {{ required (printf "%s.esoRef.key is required" $ctx) $ref.key }}
  {{- end -}}
  {{- else }}
  {{ fail (printf "%s: must be a string literal or a map with one of value, secretRef, configMapRef, esoRef" $ctx) }}
  {{- end -}}
{{- end -}}

{{/* Required credential field: fail fast on absent/null/empty/placeholder, else
render. Accepts any ref source (value/secretRef/configMapRef/esoRef) but
rejects literal placeholder sentinels that would otherwise pass validation.
The EXAMPLE guard matches the full AWS example-key shape
(^AKIA...EXAMPLE$) so real values containing "example" (e.g. a bucket named
example-backups) never trip it. */}}
{{- define "helm-rclone.renderRequired" -}}
{{- $ctx := printf "%s: %s" .ctx .desc -}}
{{- if not (hasKey .creds .key) }}{{ fail (printf "%s is required" $ctx) }}{{ end -}}
{{- $f := index .creds .key -}}
{{- if kindIs "invalid" $f }}{{ fail (printf "%s is required (got null)" $ctx) }}{{ end -}}
{{- if kindIs "string" $f }}{{ if eq $f "" }}{{ fail (printf "%s is required (got empty string)" $ctx) }}{{ end }}{{ end -}}
{{- $literal := "" -}}
{{- $hasLiteral := false -}}
{{- if kindIs "string" $f }}{{ $literal = $f }}{{ $hasLiteral = true }}{{ end -}}
{{- if and (kindIs "map" $f) (hasKey $f "value") }}{{ $literal = ($f.value | toString) }}{{ $hasLiteral = true }}{{ end -}}
{{- if and $hasLiteral (eq ($literal | toString | trim) "") }}{{ fail (printf "%s is required (got empty value)" $ctx) }}{{ end -}}
{{- if $hasLiteral -}}
{{- $upper := $literal | toString | upper | trim -}}
{{- if or (eq $upper "CHANGEME") (hasPrefix "CHANGEME" $upper) (eq $upper "REPLACE-ME") (hasPrefix "REPLACE-ME" $upper) (regexMatch "^AKIA[0-9A-Z]*EXAMPLE$" $upper) -}}
{{- fail (printf "%s is required (got placeholder %q: set a real value or a secretRef/configMapRef/esoRef)" $ctx $literal) -}}
{{- end -}}
{{- end -}}
{{ include "helm-rclone.renderEnv" (dict "name" .name "field" $f "ctx" $ctx) }}
{{- end -}}

{{/* Optional credential field: skip when absent/null/empty-literal, else render. Expects dict {name, creds, key, ctx}. */}}
{{- define "helm-rclone.renderOptional" -}}
{{- if hasKey .creds .key -}}
{{- $f := index .creds .key -}}
{{- $skip := false -}}
{{- if kindIs "invalid" $f }}{{ $skip = true }}{{ end -}}
{{- if kindIs "string" $f }}{{ if eq $f "" }}{{ $skip = true }}{{ end }}{{ end -}}
{{- if and (kindIs "map" $f) (hasKey $f "value") }}{{ if eq ($f.value | toString) "" }}{{ $skip = true }}{{ end }}{{ end -}}
{{- if not $skip }}
{{ include "helm-rclone.renderEnv" (dict "name" .name "field" $f "ctx" (printf "%s" .ctx)) }}
{{- end -}}
{{- end -}}
{{- end -}}

{{/* Uri presence guard: fails when the uri key is absent, null, or an empty
literal (empty string / empty value map). Ref sources (secretRef,
configMapRef, esoRef) always pass: their content resolves at runtime. */}}
{{- define "helm-rclone.requireUri" -}}
{{- if not (hasKey .endpoint "uri") }}{{ fail (printf "%s.uri is required: set the claim name (pvc-*), bucket/path (s3), or path (proton-drive) via one of value, secretRef, configMapRef, esoRef" .role) }}{{ end -}}
{{- $uri := .endpoint.uri -}}
{{- if kindIs "invalid" $uri }}{{ fail (printf "%s.uri is required (got null): set the claim name (pvc-*), bucket/path (s3), or path (proton-drive) via one of value, secretRef, configMapRef, esoRef" .role) }}{{ end -}}
{{- if kindIs "string" $uri }}{{ if eq ($uri | toString | trim) "" }}{{ fail (printf "%s.uri is required (got empty string): set the claim name (pvc-*), bucket/path (s3), or path (proton-drive)" .role) }}{{ end }}{{ end -}}
{{- if and (kindIs "map" $uri) (hasKey $uri "value") }}{{ if eq ($uri.value | toString | trim) "" }}{{ fail (printf "%s.uri is required (got empty value): set the claim name (pvc-*), bucket/path (s3), or path (proton-drive)" .role) }}{{ end }}{{ end -}}
{{- end -}}

{{/* "1" when a uri uses a ref source (needs <PREFIX>_PATH indirection). */}}
{{- define "helm-rclone.uriIsRef" -}}
{{- $uri := .uri -}}
{{- if kindIs "map" $uri -}}
{{- if or (hasKey $uri "secretRef") (hasKey $uri "configMapRef") (hasKey $uri "esoRef") (hasKey $uri "existingSecret") -}}1{{- end -}}
{{- end -}}
{{- end -}}

{{/* Literal path suffix for a remote uri (caller handles the ref case via <PREFIX>_PATH). */}}
{{- define "helm-rclone.uriLiteral" -}}
{{- $uri := .uri -}}
{{- if kindIs "invalid" $uri }}{{- print "" -}}
{{- else if kindIs "string" $uri }}{{- print $uri -}}
{{- else if and (kindIs "map" $uri) (hasKey $uri "value") }}{{- print ($uri.value | toString) -}}
{{- end -}}
{{- end -}}

{{/* Existing claim name for a pvc-* endpoint (claimName cannot use valueFrom: ref sources fail fast). Expects dict {endpoint, role}. */}}
{{- define "helm-rclone.claimName" -}}
{{- $ctx := printf "%s.uri (PVC claim name)" .role -}}
{{- $uri := .endpoint.uri -}}
{{- if kindIs "string" $uri -}}
{{- if eq $uri "" }}{{ fail (printf "%s is required: pvc uri must be the existing claim name" $ctx) }}{{ end -}}
{{- print $uri -}}
{{- else if kindIs "map" $uri -}}
{{- if or (hasKey $uri "secretRef") (hasKey $uri "configMapRef") (hasKey $uri "esoRef") (hasKey $uri "existingSecret") -}}
{{- fail (printf "%s: PVC claimName cannot use valueFrom (secretRef/configMapRef/esoRef); use a literal {value: <claim-name>} so the volume references the existing claim by name" $ctx) -}}
{{- else if hasKey $uri "value" -}}
{{- if eq ($uri.value | toString) "" }}{{ fail (printf "%s is required: pvc uri must be the existing claim name (got empty value)" $ctx) }}{{ end -}}
{{- print ($uri.value | toString) -}}
{{- else -}}
{{- fail (printf "%s is required: pvc uri must be the existing claim name (no value source set)" $ctx) -}}
{{- end -}}
{{- else -}}
{{- fail (printf "%s is required: pvc uri must be the existing claim name" $ctx) -}}
{{- end -}}
{{- end -}}

{{/* Rclone path arg: pvc-* → mount path; remotes → REMOTE:path (or REMOTE:$(<PREFIX>_PATH) for ref uris). Expects dict {endpoint, role, prefix, slot}. */}}
{{- define "helm-rclone.endpointArg" -}}
{{- $ep := .endpoint -}}
{{- include "helm-rclone.validateType" (dict "type" $ep.type "role" .role) -}}
{{- if or (eq $ep.type "pvc-rwo") (eq $ep.type "pvc-rwx") -}}
{{- printf "/mnt/%s" .slot -}}
{{- else -}}
{{- $remote := include "helm-rclone.remoteName" (dict "endpoint" $ep "default" .prefix) -}}
{{- if eq (include "helm-rclone.uriIsRef" (dict "uri" $ep.uri) | trim) "1" -}}
{{- printf "%s:$(%s_PATH)" $remote .prefix -}}
{{- else -}}
{{- $path := include "helm-rclone.uriLiteral" (dict "uri" $ep.uri) | trim -}}
{{- printf "%s:%s" $remote $path -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/* Full env list for one endpoint (pvc-* emits nothing). Expects dict {endpoint, role, prefix}. */}}
{{- define "helm-rclone.endpointEnv" -}}
{{- $ep := .endpoint -}}
{{- $role := .role -}}
{{- $prefix := .prefix -}}
{{- include "helm-rclone.validateType" (dict "type" $ep.type "role" $role) -}}
{{- if or (eq $ep.type "s3") (eq $ep.type "proton-drive") -}}
{{- $remote := include "helm-rclone.remoteName" (dict "endpoint" $ep "default" $prefix) -}}
{{- $creds := $ep.credentials | default dict -}}
{{- $ctx := printf "%s (type %s, remote %s)" $role $ep.type $remote -}}
{{- if eq $ep.type "s3" }}
- name: RCLONE_CONFIG_{{ $remote }}_TYPE
  value: "s3"
{{ include "helm-rclone.renderRequired" (dict "name" (printf "RCLONE_CONFIG_%s_PROVIDER" $remote) "creds" $creds "key" "provider" "ctx" $ctx "desc" "s3 credentials.provider") }}
{{ include "helm-rclone.renderRequired" (dict "name" (printf "RCLONE_CONFIG_%s_ACCESS_KEY_ID" $remote) "creds" $creds "key" "accessKeyId" "ctx" $ctx "desc" "s3 credentials.accessKeyId") }}
{{ include "helm-rclone.renderRequired" (dict "name" (printf "RCLONE_CONFIG_%s_SECRET_ACCESS_KEY" $remote) "creds" $creds "key" "secretAccessKey" "ctx" $ctx "desc" "s3 credentials.secretAccessKey") }}
{{ include "helm-rclone.renderRequired" (dict "name" (printf "RCLONE_CONFIG_%s_REGION" $remote) "creds" $creds "key" "region" "ctx" $ctx "desc" "s3 credentials.region") }}
{{ include "helm-rclone.renderOptional" (dict "name" (printf "RCLONE_CONFIG_%s_ENDPOINT" $remote) "creds" $creds "key" "endpoint" "ctx" $ctx) }}
{{ include "helm-rclone.renderOptional" (dict "name" (printf "RCLONE_CONFIG_%s_ENV_AUTH" $remote) "creds" $creds "key" "envAuth" "ctx" $ctx) }}
{{- else }}
- name: RCLONE_CONFIG_{{ $remote }}_TYPE
  value: "protondrive"
{{ include "helm-rclone.renderRequired" (dict "name" (printf "RCLONE_CONFIG_%s_USERNAME" $remote) "creds" $creds "key" "username" "ctx" $ctx "desc" "proton-drive credentials.username") }}
{{ include "helm-rclone.renderRequired" (dict "name" (printf "RCLONE_CONFIG_%s_PASSWORD" $remote) "creds" $creds "key" "password" "ctx" $ctx "desc" "proton-drive credentials.password (must be rclone-obscured, see README)") }}
{{ include "helm-rclone.renderOptional" (dict "name" (printf "RCLONE_CONFIG_%s_MAILBOX_PASSWORD" $remote) "creds" $creds "key" "mailboxPassword" "ctx" $ctx) }}
{{ include "helm-rclone.renderOptional" (dict "name" (printf "RCLONE_CONFIG_%s_OTP_SECRET_KEY" $remote) "creds" $creds "key" "otpSecretKey" "ctx" $ctx) }}
{{ include "helm-rclone.renderOptional" (dict "name" (printf "RCLONE_CONFIG_%s_2FA" $remote) "creds" $creds "key" "twoFa" "ctx" $ctx) }}
{{ include "helm-rclone.renderOptional" (dict "name" (printf "RCLONE_CONFIG_%s_CLIENT_UID" $remote) "creds" $creds "key" "clientUid" "ctx" $ctx) }}
{{ include "helm-rclone.renderOptional" (dict "name" (printf "RCLONE_CONFIG_%s_CLIENT_ACCESS_TOKEN" $remote) "creds" $creds "key" "clientAccessToken" "ctx" $ctx) }}
{{ include "helm-rclone.renderOptional" (dict "name" (printf "RCLONE_CONFIG_%s_CLIENT_REFRESH_TOKEN" $remote) "creds" $creds "key" "clientRefreshToken" "ctx" $ctx) }}
{{- end -}}
{{- if eq (include "helm-rclone.uriIsRef" (dict "uri" $ep.uri) | trim) "1" }}
{{- /* Drop the deep-merged default value key so renderEnv sees exactly one source. */}}
{{- $pathField := dict -}}
{{- range $k := list "secretRef" "configMapRef" "esoRef" "existingSecret" -}}
{{- if hasKey $ep.uri $k }}{{ $pathField = set $pathField $k (index $ep.uri $k) }}{{ end -}}
{{- end }}
{{ include "helm-rclone.renderEnv" (dict "name" (printf "%s_PATH" $prefix) "field" $pathField "ctx" (printf "%s.uri (remote path)" $role)) }}
{{- end }}
{{- end }}
{{- end -}}

{{/* One persistentVolumeClaim volume item for a pvc-* endpoint, else "". Expects dict {endpoint, role, vol}. */}}
{{- define "helm-rclone.endpointVolume" -}}
{{- $ep := .endpoint -}}
{{- include "helm-rclone.validateType" (dict "type" $ep.type "role" .role) -}}
{{- if or (eq $ep.type "pvc-rwo") (eq $ep.type "pvc-rwx") -}}
- name: {{ .vol }}
  persistentVolumeClaim:
    claimName: {{ include "helm-rclone.claimName" (dict "endpoint" $ep "role" .role) }}
{{- end -}}
{{- end -}}

{{/* One volumeMount for a pvc-* endpoint, else "". Expects dict {endpoint, role, vol, slot, readOnly}. */}}
{{- define "helm-rclone.endpointVolumeMount" -}}
{{- $ep := .endpoint -}}
{{- include "helm-rclone.validateType" (dict "type" $ep.type "role" .role) -}}
{{- if or (eq $ep.type "pvc-rwo") (eq $ep.type "pvc-rwx") -}}
- name: {{ .vol }}
  mountPath: /mnt/{{ .slot }}
  readOnly: {{ .readOnly }}
{{- end -}}
{{- end -}}

{{/* Full `volumes:` block for the pod, or "" when neither side is a PVC (remote-to-remote). */}}
{{- define "helm-rclone.volumes" -}}
{{- $src := include "helm-rclone.endpointVolume" (dict "endpoint" .source "role" "source" "vol" "src") | trim -}}
{{- $dst := include "helm-rclone.endpointVolume" (dict "endpoint" .destination "role" "destination" "vol" "dst") | trim -}}
{{- if or $src $dst -}}
volumes:
{{- if $src }}
{{ $src | indent 2 }}
{{- end -}}
{{- if $dst }}
{{ $dst | indent 2 }}
{{- end -}}
{{- end -}}
{{- end -}}

{{/* Merged pod affinity: user .Values.affinity base + required coLocateWith podAffinity term appended (merged, never replaced). Expects root context. */}}
{{- define "helm-rclone.affinity" -}}
{{- $user := .Values.affinity | default dict -}}
{{- $sel := .Values.coLocateWith | default dict -}}
{{- if $sel -}}
{{- if not (kindIs "map" $sel) }}{{ fail "coLocateWith must be a map with matchLabels and/or matchExpressions selecting the owning workload's pods (see README \"RWO same-node caveat\")" }}{{ end -}}
{{- $ml := $sel.matchLabels | default dict -}}
{{- $me := $sel.matchExpressions | default list -}}
{{- if and (empty $ml) (empty $me) }}{{ fail "coLocateWith requires matchLabels and/or matchExpressions selecting the owning workload's pods (see README \"RWO same-node caveat\")" }}{{ end -}}
{{- $ls := dict -}}
{{- if not (empty $ml) }}{{ $ls = set $ls "matchLabels" $ml }}{{ end -}}
{{- if not (empty $me) }}{{ $ls = set $ls "matchExpressions" $me }}{{ end -}}
{{- $term := dict "labelSelector" $ls "topologyKey" ($sel.topologyKey | default "kubernetes.io/hostname") -}}
{{- if $sel.namespaces }}{{ $term = set $term "namespaces" $sel.namespaces }}{{ end -}}
{{- $merged := deepCopy $user -}}
{{- $podAff := $merged.podAffinity | default dict -}}
{{- $existing := $podAff.requiredDuringSchedulingIgnoredDuringExecution | default list -}}
{{- $podAff = set $podAff "requiredDuringSchedulingIgnoredDuringExecution" (concat $existing (list $term)) -}}
{{- $merged = set $merged "podAffinity" $podAff -}}
{{ $merged | toYaml }}
{{- else -}}
{{- if $user }}
{{ $user | toYaml }}
{{- end -}}
{{- end -}}
{{- end -}}

{{/* Full `volumeMounts:` block for the container, or "" when neither side is a PVC. */}}
{{- define "helm-rclone.volumeMounts" -}}
{{- $src := include "helm-rclone.endpointVolumeMount" (dict "endpoint" .source "role" "source" "vol" "src" "slot" "src" "readOnly" "true") | trim -}}
{{- $dst := include "helm-rclone.endpointVolumeMount" (dict "endpoint" .destination "role" "destination" "vol" "dst" "slot" "dst" "readOnly" "false") | trim -}}
{{- if or $src $dst -}}
volumeMounts:
{{- if $src }}
{{ $src | indent 2 }}
{{- end -}}
{{- if $dst }}
{{ $dst | indent 2 }}
{{- end -}}
{{- end -}}
{{- end -}}
