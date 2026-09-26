{{- /* Gateway API standard-channel bundle, staged at publish time by ci/fetch.sh. */ -}}
{{- $bundle := .Files.Get "upstream/standard-install.yaml" }}
{{- if not $bundle }}{{ fail "upstream bundle missing: run ci/fetch.sh to stage it under upstream/ before packaging" }}{{ end }}
{{ tpl $bundle . }}
