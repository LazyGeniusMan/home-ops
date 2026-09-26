{{- /* COSI bucketaccessclasses CRD, staged at publish time by ci/fetch.sh. */ -}}
{{- $bundle := .Files.Get "upstream/objectstorage.k8s.io_bucketaccessclasses.yaml" }}
{{- if not $bundle }}{{ fail "upstream bundle missing: run ci/fetch.sh to stage it under upstream/ before packaging" }}{{ end }}
{{ tpl $bundle . }}
