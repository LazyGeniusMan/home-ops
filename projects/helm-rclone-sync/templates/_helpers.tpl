{{/*
Shared helpers for helm-rclone-sync. All helper names carry the
"helm-rclone-sync." prefix. Every bit of per-backend env/volume logic lives
here; templates/cronjob.yaml only includes these helpers, so all 10 sync
directions (pvc-rwo | pvc-rwx | s3 | proton-drive on either side) render from
one generic template with no per-direction duplication.
*/}}

{{/* Full name: <release>-<chart>, honouring nameOverride/fullnameOverride. */}}
{{- define "helm-rclone-sync.fullname" -}}
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

{{- define "helm-rclone-sync.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "helm-rclone-sync.selectorLabels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "helm-rclone-sync.labels" -}}
helm.sh/chart: {{ include "helm-rclone-sync.chart" . }}
{{ include "helm-rclone-sync.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/* Container image. rclone.version overrides image.tag; the result must be an exact pin, never "latest". */}}
{{- define "helm-rclone-sync.image" -}}
{{- $tag := .Values.rclone.version | default .Values.image.tag -}}
{{- printf "%s:%s" .Values.image.repository (required "image.tag (or rclone.version override) is required; pin an exact version, never \"latest\"" $tag) -}}
{{- end -}}

{{/* rclone.operation must be a one-shot transfer: sync (mirror) or copy. Never anything long-lived. */}}
{{- define "helm-rclone-sync.operation" -}}
{{- if not (has .Values.rclone.operation (list "sync" "copy")) -}}
{{- fail (printf "rclone.operation %q is invalid: must be \"sync\" or \"copy\" (one-shot only)" (.Values.rclone.operation | toString)) -}}
{{- end -}}
{{- print .Values.rclone.operation -}}
{{- end -}}

{{/* restartPolicy must satisfy the Jobs requirement (OnFailure or Never). */}}
{{- define "helm-rclone-sync.restartPolicy" -}}
{{- if not (has .Values.restartPolicy (list "OnFailure" "Never")) -}}
{{- fail (printf "restartPolicy %q is invalid: Jobs require OnFailure or Never" (.Values.restartPolicy | toString)) -}}
{{- end -}}
{{- print .Values.restartPolicy -}}
{{- end -}}

{{- define "helm-rclone-sync.concurrencyPolicy" -}}
{{- if not (has .Values.concurrencyPolicy (list "Allow" "Forbid" "Replace")) -}}
{{- fail (printf "concurrencyPolicy %q is invalid: must be Allow, Forbid or Replace (keep Forbid, see README)" (.Values.concurrencyPolicy | toString)) -}}
{{- end -}}
{{- print .Values.concurrencyPolicy -}}
{{- end -}}

{{/* Endpoint type guard. Expects dict {type, role}. */}}
{{- define "helm-rclone-sync.validateType" -}}
{{- if not .type -}}
{{- fail (printf "%s.type is required: must be one of pvc-rwo, pvc-rwx, s3, proton-drive" .role) -}}
{{- end -}}
{{- if not (has .type (list "pvc-rwo" "pvc-rwx" "s3" "proton-drive")) -}}
{{- fail (printf "%s.type %q is invalid: must be one of pvc-rwo, pvc-rwx, s3, proton-drive" .role .type) -}}
{{- end -}}
{{- end -}}

{{/*
Remote name for the RCLONE_CONFIG_<REMOTE>_* prefix. Expects
dict {endpoint, default}. An explicit remoteName wins (uppercased, validated
so the resulting env vars are legal); otherwise the side default (SRC/DST).
*/}}
{{- define "helm-rclone-sync.remoteName" -}}
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

{{/*
Render one env entry from any of the four value sources.
Expects dict {name, field, ctx} where ctx names the values location for errors.
  value:        literal, e.g. {value: "my-bucket/backups"}
  secretRef:    {name, key} -> valueFrom.secretKeyRef
  configMapRef: {name, key} -> valueFrom.configMapKeyRef
  esoRef / existingSecret: {name, key} -> valueFrom.secretKeyRef against the
    Secret that External Secrets Operator already synced (consume, not create).
A plain string field is shorthand for {value: <string>}.
*/}}
{{- define "helm-rclone-sync.renderEnv" -}}
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

{{/*
Required credential field: fail fast on absent/null/empty, else render the env entry.
Expects dict {name, creds, key, ctx, desc}.

Merge note: Helm deep-merges user maps over the chart defaults at the FIELD
level, so a key the user omits would otherwise be silently inherited from the
demo defaults. Required keys therefore carry NO usable default — the demo
credentials they would inherit (e.g. CHANGEME accessKeyId) are called out in
values.yaml and any key that must truly be absent to prove fail-fast is
nulled by the caller. hasKey/null/empty checks below reject all three states.
*/}}
{{- define "helm-rclone-sync.renderRequired" -}}
{{- $ctx := printf "%s: %s" .ctx .desc -}}
{{- if not (hasKey .creds .key) }}{{ fail (printf "%s is required" $ctx) }}{{ end -}}
{{- $f := index .creds .key -}}
{{- if kindIs "invalid" $f }}{{ fail (printf "%s is required (got null)" $ctx) }}{{ end -}}
{{- if kindIs "string" $f }}{{ if eq $f "" }}{{ fail (printf "%s is required (got empty string)" $ctx) }}{{ end }}{{ end -}}
{{- if and (kindIs "map" $f) (hasKey $f "value") }}{{ if eq ($f.value | toString) "" }}{{ fail (printf "%s is required (got empty value)" $ctx) }}{{ end }}{{ end -}}
{{ include "helm-rclone-sync.renderEnv" (dict "name" .name "field" $f "ctx" $ctx) }}
{{- end -}}

{{/*
Optional credential field: skip when absent/null/empty-literal, else render.
Expects dict {name, creds, key, ctx}.
*/}}
{{- define "helm-rclone-sync.renderOptional" -}}
{{- if hasKey .creds .key -}}
{{- $f := index .creds .key -}}
{{- $skip := false -}}
{{- if kindIs "invalid" $f }}{{ $skip = true }}{{ end -}}
{{- if kindIs "string" $f }}{{ if eq $f "" }}{{ $skip = true }}{{ end }}{{ end -}}
{{- if and (kindIs "map" $f) (hasKey $f "value") }}{{ if eq ($f.value | toString) "" }}{{ $skip = true }}{{ end }}{{ end -}}
{{- if not $skip }}
{{ include "helm-rclone-sync.renderEnv" (dict "name" .name "field" $f "ctx" (printf "%s" .ctx)) }}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Uri presence guard. Helm deep-merges a user-supplied uri map over the chart's
default uri map, so an empty-string/experimental default could otherwise leak
into a ref-sourced uri (e.g. {value: ""} shadow-merging with {secretRef: …​}).
This guard fails the render when the uri key is absent or null, forcing each
fixture / invocation to spell the uri it means.
Expects dict {endpoint, role}.
*/}}
{{- define "helm-rclone-sync.requireUri" -}}
{{- if not (hasKey .endpoint "uri") }}{{ fail (printf "%s.uri is required: set the claim name (pvc-*), bucket/path (s3), or path (proton-drive) via one of value, secretRef, configMapRef, esoRef" .role) }}{{ end -}}
{{- $uri := .endpoint.uri -}}
{{- if kindIs "invalid" $uri }}{{ fail (printf "%s.uri is required (got null): set the claim name (pvc-*), bucket/path (s3), or path (proton-drive) via one of value, secretRef, configMapRef, esoRef" .role) }}{{ end -}}
{{- end -}}

{{/*
True ("1") when a uri uses a ref source (needs the <PREFIX>_PATH env
indirection). Helm deep-merges the chart default uri map ({value: …​}) with a
user-supplied ref map ({secretRef: …​}), yielding BOTH keys present; the
explicitly-set ref key must win, so any of secretRef/configMapRef/esoRef/
existingSecret present means "ref".
*/}}
{{- define "helm-rclone-sync.uriIsRef" -}}
{{- $uri := .uri -}}
{{- if kindIs "map" $uri -}}
{{- if or (hasKey $uri "secretRef") (hasKey $uri "configMapRef") (hasKey $uri "esoRef") (hasKey $uri "existingSecret") -}}1{{- end -}}
{{- end -}}
{{- end -}}

{{/* Literal path suffix for a remote uri (caller handles the ref case via <PREFIX>_PATH). */}}
{{- define "helm-rclone-sync.uriLiteral" -}}
{{- $uri := .uri -}}
{{- if kindIs "invalid" $uri }}{{- print "" -}}
{{- else if kindIs "string" $uri }}{{- print $uri -}}
{{- else if and (kindIs "map" $uri) (hasKey $uri "value") }}{{- print ($uri.value | toString) -}}
{{- end -}}
{{- end -}}

{{/*
Existing claim name for a pvc-* endpoint. claimName cannot use valueFrom, so
ref sources fail fast here with a clear message. Expects dict {endpoint, role}.

Merge note: a ref key (secretRef/…) anywhere in the merged uri map means the
caller asked for a ref — the deep-merged default {value: …​} key must NOT
shadow it (Helm merges maps at field level, so both keys can be present).
*/}}
{{- define "helm-rclone-sync.claimName" -}}
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

{{/*
Rclone path argument for one endpoint. pvc-* endpoints resolve to their mount
path; remotes resolve to REMOTE:path, or REMOTE:$(<PREFIX>_PATH) when the uri
comes from a ref source (Kubernetes $(VAR) expansion fills it in at runtime).
Expects dict {endpoint, role, prefix, slot} where slot is "src"/"dst".
*/}}
{{- define "helm-rclone-sync.endpointArg" -}}
{{- $ep := .endpoint -}}
{{- include "helm-rclone-sync.validateType" (dict "type" $ep.type "role" .role) -}}
{{- if or (eq $ep.type "pvc-rwo") (eq $ep.type "pvc-rwx") -}}
{{- printf "/mnt/%s" .slot -}}
{{- else -}}
{{- $remote := include "helm-rclone-sync.remoteName" (dict "endpoint" $ep "default" .prefix) -}}
{{- if eq (include "helm-rclone-sync.uriIsRef" (dict "uri" $ep.uri) | trim) "1" -}}
{{- printf "%s:$(%s_PATH)" $remote .prefix -}}
{{- else -}}
{{- $path := include "helm-rclone-sync.uriLiteral" (dict "uri" $ep.uri) | trim -}}
{{- printf "%s:%s" $remote $path -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Full env list for one endpoint (remote type vars via the four value sources,
plus <PREFIX>_PATH when the remote uri itself is a ref). pvc-* endpoints emit
nothing. Expects dict {endpoint, role, prefix}.
*/}}
{{- define "helm-rclone-sync.endpointEnv" -}}
{{- $ep := .endpoint -}}
{{- $role := .role -}}
{{- $prefix := .prefix -}}
{{- include "helm-rclone-sync.validateType" (dict "type" $ep.type "role" $role) -}}
{{- if or (eq $ep.type "s3") (eq $ep.type "proton-drive") -}}
{{- $remote := include "helm-rclone-sync.remoteName" (dict "endpoint" $ep "default" $prefix) -}}
{{- $creds := $ep.credentials | default dict -}}
{{- $ctx := printf "%s (type %s, remote %s)" $role $ep.type $remote -}}
{{- if eq $ep.type "s3" }}
- name: RCLONE_CONFIG_{{ $remote }}_TYPE
  value: "s3"
{{ include "helm-rclone-sync.renderRequired" (dict "name" (printf "RCLONE_CONFIG_%s_PROVIDER" $remote) "creds" $creds "key" "provider" "ctx" $ctx "desc" "s3 credentials.provider") }}
{{ include "helm-rclone-sync.renderRequired" (dict "name" (printf "RCLONE_CONFIG_%s_ACCESS_KEY_ID" $remote) "creds" $creds "key" "accessKeyId" "ctx" $ctx "desc" "s3 credentials.accessKeyId") }}
{{ include "helm-rclone-sync.renderRequired" (dict "name" (printf "RCLONE_CONFIG_%s_SECRET_ACCESS_KEY" $remote) "creds" $creds "key" "secretAccessKey" "ctx" $ctx "desc" "s3 credentials.secretAccessKey") }}
{{ include "helm-rclone-sync.renderRequired" (dict "name" (printf "RCLONE_CONFIG_%s_REGION" $remote) "creds" $creds "key" "region" "ctx" $ctx "desc" "s3 credentials.region") }}
{{ include "helm-rclone-sync.renderOptional" (dict "name" (printf "RCLONE_CONFIG_%s_ENDPOINT" $remote) "creds" $creds "key" "endpoint" "ctx" $ctx) }}
{{ include "helm-rclone-sync.renderOptional" (dict "name" (printf "RCLONE_CONFIG_%s_ENV_AUTH" $remote) "creds" $creds "key" "envAuth" "ctx" $ctx) }}
{{- else }}
- name: RCLONE_CONFIG_{{ $remote }}_TYPE
  value: "protondrive"
{{ include "helm-rclone-sync.renderRequired" (dict "name" (printf "RCLONE_CONFIG_%s_USERNAME" $remote) "creds" $creds "key" "username" "ctx" $ctx "desc" "proton-drive credentials.username") }}
{{ include "helm-rclone-sync.renderRequired" (dict "name" (printf "RCLONE_CONFIG_%s_PASSWORD" $remote) "creds" $creds "key" "password" "ctx" $ctx "desc" "proton-drive credentials.password (must be rclone-obscured, see README)") }}
{{ include "helm-rclone-sync.renderOptional" (dict "name" (printf "RCLONE_CONFIG_%s_MAILBOX_PASSWORD" $remote) "creds" $creds "key" "mailboxPassword" "ctx" $ctx) }}
{{ include "helm-rclone-sync.renderOptional" (dict "name" (printf "RCLONE_CONFIG_%s_OTP_SECRET_KEY" $remote) "creds" $creds "key" "otpSecretKey" "ctx" $ctx) }}
{{ include "helm-rclone-sync.renderOptional" (dict "name" (printf "RCLONE_CONFIG_%s_2FA" $remote) "creds" $creds "key" "twoFa" "ctx" $ctx) }}
{{ include "helm-rclone-sync.renderOptional" (dict "name" (printf "RCLONE_CONFIG_%s_CLIENT_UID" $remote) "creds" $creds "key" "clientUid" "ctx" $ctx) }}
{{ include "helm-rclone-sync.renderOptional" (dict "name" (printf "RCLONE_CONFIG_%s_CLIENT_ACCESS_TOKEN" $remote) "creds" $creds "key" "clientAccessToken" "ctx" $ctx) }}
{{ include "helm-rclone-sync.renderOptional" (dict "name" (printf "RCLONE_CONFIG_%s_CLIENT_REFRESH_TOKEN" $remote) "creds" $creds "key" "clientRefreshToken" "ctx" $ctx) }}
{{- end -}}
{{- if eq (include "helm-rclone-sync.uriIsRef" (dict "uri" $ep.uri) | trim) "1" }}
{{- /* Strip any deep-merged default {value: …​} key so renderEnv sees exactly one source. */}}
{{- $pathField := dict -}}
{{- range $k := list "secretRef" "configMapRef" "esoRef" "existingSecret" -}}
{{- if hasKey $ep.uri $k }}{{ $pathField = set $pathField $k (index $ep.uri $k) }}{{ end -}}
{{- end }}
{{ include "helm-rclone-sync.renderEnv" (dict "name" (printf "%s_PATH" $prefix) "field" $pathField "ctx" (printf "%s.uri (remote path)" $role)) }}
{{- end }}
{{- end }}
{{- end -}}

{{/* One persistentVolumeClaim volume item for a pvc-* endpoint, else "". Expects dict {endpoint, role, vol}. */}}
{{- define "helm-rclone-sync.endpointVolume" -}}
{{- $ep := .endpoint -}}
{{- include "helm-rclone-sync.validateType" (dict "type" $ep.type "role" .role) -}}
{{- if or (eq $ep.type "pvc-rwo") (eq $ep.type "pvc-rwx") -}}
- name: {{ .vol }}
  persistentVolumeClaim:
    claimName: {{ include "helm-rclone-sync.claimName" (dict "endpoint" $ep "role" .role) }}
{{- end -}}
{{- end -}}

{{/*
One volumeMount item for a pvc-* endpoint, else "". The source mount is
readOnly (sync/copy never writes to the source); the destination mount is
writable. Expects dict {endpoint, role, vol, slot, readOnly}.
*/}}
{{- define "helm-rclone-sync.endpointVolumeMount" -}}
{{- $ep := .endpoint -}}
{{- include "helm-rclone-sync.validateType" (dict "type" $ep.type "role" .role) -}}
{{- if or (eq $ep.type "pvc-rwo") (eq $ep.type "pvc-rwx") -}}
- name: {{ .vol }}
  mountPath: /mnt/{{ .slot }}
  readOnly: {{ .readOnly }}
{{- end -}}
{{- end -}}

{{/* Full `volumes:` block for the pod, or "" when neither side is a PVC (remote-to-remote). */}}
{{- define "helm-rclone-sync.volumes" -}}
{{- $src := include "helm-rclone-sync.endpointVolume" (dict "endpoint" .source "role" "source" "vol" "src") | trim -}}
{{- $dst := include "helm-rclone-sync.endpointVolume" (dict "endpoint" .destination "role" "destination" "vol" "dst") | trim -}}
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

{{/*
Merged pod `affinity:` block (YAML, or "" when neither source is set).
User-supplied .Values.affinity is the base; when .Values.coLocateWith selects
the owning workload's pods ({matchLabels, matchExpressions, optional
topologyKey, optional namespaces}), a required podAffinity term (topologyKey
kubernetes.io/hostname by default) is appended to any user-supplied
requiredDuringScheduling terms — merged, never replacing them. Expects the
root context.
*/}}
{{- define "helm-rclone-sync.affinity" -}}
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
{{- define "helm-rclone-sync.volumeMounts" -}}
{{- $src := include "helm-rclone-sync.endpointVolumeMount" (dict "endpoint" .source "role" "source" "vol" "src" "slot" "src" "readOnly" "true") | trim -}}
{{- $dst := include "helm-rclone-sync.endpointVolumeMount" (dict "endpoint" .destination "role" "destination" "vol" "dst" "slot" "dst" "readOnly" "false") | trim -}}
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
