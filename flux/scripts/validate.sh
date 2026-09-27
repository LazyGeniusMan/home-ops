#!/usr/bin/env bash

# Validate Flux custom resources and kustomize overlays using kubeconform.
# Downloads the Flux OpenAPI schemas, then validates raw manifests and kustomize overlays. Run locally and in CI before merging to main.
#
# Prerequisites
# - yq >= 4.50
# - kustomize >= 5.8
# - kubeconform >= 0.7

set -o errexit
set -o nounset
set -o pipefail
# Strict mode: errexit + nounset + pipefail (fetch-references.sh omits errexit by design).

# mirror kustomize-controller build options
kustomize_flags=("--load-restrictor=LoadRestrictionsNone")
kustomize_config="kustomization.yaml"

# Raw manifests pre-filter copyFrom/copyTo stub Secrets via yq (no blanket `-skip=Secret`; no SOPS in this repo).
#
# Schema-dir contract: -schema-location points at the PARENT /tmp/flux-crd-schemas (kubeconform appends the version subdir itself).
kubeconform_flags=()
kubeconform_config=("-strict" "-ignore-missing-schemas" "-schema-location" "default" "-schema-location" "/tmp/flux-crd-schemas" "-verbose")

# copyFrom/copyTo annotation keys for stub Secrets (raw-manifest exclusion only).
copy_stub_annotation_from="fluxcd.controlplane.io/copyFrom"
copy_stub_annotation_to="fluxcd.controlplane.io/copyTo"

# root directory to validate
root_dir="."

# directories to exclude from validation
exclude_dirs=()

# Auto-detected non-Kubernetes dirs (ancestor dirs skipped: Terraform-enclosing shells nest .tf under terraform/).
declare -a auto_skip_dirs=()

# directories that are kustomize overlays
declare -a kustomize_dirs=()

usage() {
  echo "Usage: $0 [-d <dir>] [-e <dir>]... [-h]"
  echo ""
  echo "Validate Flux custom resources and kustomize overlays using kubeconform."
  echo ""
  echo "Options:"
  echo "  -d, --dir <dir>      Root directory to validate (default: current directory)"
  echo "  -e, --exclude <dir>  Directory to exclude from validation (can be repeated)"
  echo "  -h, --help           Show this help message"
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -d|--dir)
        if [[ -z "${2:-}" ]]; then
          echo "ERROR - --dir requires a directory argument" >&2
          exit 1
        fi
        root_dir="${2%/}"
        shift 2
        ;;
      -e|--exclude)
        if [[ -z "${2:-}" ]]; then
          echo "ERROR - --exclude requires a directory argument" >&2
          exit 1
        fi
        exclude_dirs+=("./${2#./}")
        shift 2
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "ERROR - Unknown argument: $1" >&2
        usage >&2
        exit 1
        ;;
    esac
  done
}

check_prerequisites() {
  local missing=0
  for cmd in yq kustomize kubeconform curl; do
    if ! command -v "$cmd" &> /dev/null; then
      echo "ERROR - $cmd is not installed" >&2
      missing=1
    fi
  done
  if [[ $missing -ne 0 ]]; then
    exit 1
  fi
}

# Schema pins track this repo's Flux pins (fleet terraform versions.yaml, group_vars/all.yml, .flox). Bump all together.
FLUX_OPERATOR_SCHEMA_VERSION="v0.60.0"
FLUX_OPERATOR_SCHEMA_SHA256="c062892eeac621948567464ae7688fcafa75c693bdfb170534cb221a56a194d8"
FLUX2_SCHEMA_VERSION="v2.9.5"
FLUX2_SCHEMA_SHA256="3c6c976df251e5a7e8c1c6a0ee63e6c28026d568b14ffa2f13cc32a6a564f238"

download_schemas() {
  echo "INFO - Downloading Flux OpenAPI schemas"
  local schema_dir="/tmp/flux-crd-schemas/master-standalone-strict"
  mkdir -p "$schema_dir"
  download_schema "https://github.com/controlplaneio-fluxcd/flux-operator/releases/download/${FLUX_OPERATOR_SCHEMA_VERSION}/crd-schemas.tar.gz" "$FLUX_OPERATOR_SCHEMA_SHA256" "$schema_dir"
  download_schema "https://github.com/fluxcd/flux2/releases/download/${FLUX2_SCHEMA_VERSION}/crd-schemas.tar.gz" "$FLUX2_SCHEMA_SHA256" "$schema_dir"
}

download_schema() {
  local url="$1"
  local want_sha="$2"
  local dest="$3"
  local tmp_tarball
  tmp_tarball="$(mktemp /tmp/flux-crd-schemas-XXXXXX.tar.gz)"
  # shellcheck disable=SC2064  # intentional immediate expansion: each call traps its own file.
  trap "rm -f '$tmp_tarball'" RETURN
  curl -fSL --retry 3 -o "$tmp_tarball" "$url"
  echo "${want_sha}  ${tmp_tarball}" | sha256sum -c -
  tar zxf "$tmp_tarball" -C "$dest"
}

# Normalize a path by stripping leading "./" for consistent comparisons
normalize_path() {
  local p="${1#./}"
  echo "${p%/}"
}

# Check if a path is under a user-excluded, auto-skipped, or kustomize directory
is_excluded_dir() {
  local path
  path="$(normalize_path "$1")"
  for dir in "${exclude_dirs[@]}"; do
    local d
    d="$(normalize_path "$dir")"
    if [[ "$path" == "$d"/* || "$path" == "$d" ]]; then
      return 0
    fi
  done
  for dir in "${auto_skip_dirs[@]}"; do
    local d
    d="$(normalize_path "$dir")"
    if [[ "$path" == "$d"/* || "$path" == "$d" ]]; then
      return 0
    fi
  done
  for dir in "${kustomize_dirs[@]}"; do
    local d
    d="$(normalize_path "$dir")"
    if [[ "$path" == "$d"/* || "$path" == "$d" ]]; then
      return 0
    fi
  done
  return 1
}

# Check if a path is under a user-excluded or auto-skipped directory (but not kustomize dirs)
is_non_kustomize_excluded_dir() {
  local path
  path="$(normalize_path "$1")"
  for dir in "${exclude_dirs[@]}" "${auto_skip_dirs[@]}"; do
    local d
    d="$(normalize_path "$dir")"
    if [[ "$path" == "$d"/* || "$path" == "$d" ]]; then
      return 0
    fi
  done
  return 1
}

# Detect Terraform/Helm/kustomize dirs. Only the marker file's own dir is skipped (enclosing shells still build in the kustomize pass).
detect_excluded_dirs() {
  while IFS= read -r -d $'\0' file; do
    auto_skip_dirs+=("$(dirname "$file")")
  done < <(find "$root_dir" -path '*/.*' -prune -o -type f \( -name '*.tf' -o -name 'Chart.yaml' \) -print0)

  while IFS= read -r -d $'\0' file; do
    kustomize_dirs+=("$(dirname "$file")")
  done < <(find "$root_dir" -path '*/.*' -prune -o -type f -name "$kustomize_config" -print0)
}

validate_yaml_syntax() {
  echo "INFO - Validating YAML syntax"
  while IFS= read -r -d $'\0' file; do
    dir="$(dirname "$file")"
    if is_excluded_dir "$dir"; then
      continue
    fi
    yq e 'true' "$file" > /dev/null
  done < <(find "$root_dir" -path '*/.*' -prune -o -type f -name '*.yaml' -print0)
}

# True when a file holds ONLY copyFrom/copyTo stub Secrets (mixed files are kept whole).
is_copy_stub_secret() {
  local file="$1"
  local total stubs
  total="$(yq ea '[select(.kind == "Secret")] | length' "$file")" || return 1
  if [[ "$total" == "0" ]]; then
    return 1
  fi
  stubs="$(yq ea --arg from "$copy_stub_annotation_from" --arg to "$copy_stub_annotation_to" \
    '[select(.kind == "Secret") | select(((.metadata.annotations // {}) | has($from)) or ((.metadata.annotations // {}) | has($to)))] | length' \
    "$file")" || return 1
  [[ "$stubs" == "$total" ]]
}

validate_kubernetes_manifests() {
  echo "INFO - Validating Kubernetes manifests"
  while IFS= read -r -d $'\0' file; do
    dir="$(dirname "$file")"
    if is_excluded_dir "$dir"; then
      continue
    fi
    if is_copy_stub_secret "$file"; then
      echo "INFO - Skipping copyFrom/copyTo stub Secret ${file}"
      continue
    fi
    kubeconform "${kubeconform_flags[@]}" "${kubeconform_config[@]}" "${file}"
  done < <(find "$root_dir" -path '*/.*' -prune -o -type f -name '*.yaml' -print0)
}

# Fail when a known Flux GVK has no local schema (-ignore-missing-schemas would hide mistyped apiVersion/kind).
probe_known_flux_schemas() {
  echo "INFO - Probing known Flux schemas"
  local schema_dir="/tmp/flux-crd-schemas/master-standalone-strict"
  local missing=0
  # "<apiVersion>/<Kind>:<schema-file>" pairs for the GVKs used under the validated root.
  local -a known=(
    "fluxcd.controlplane.io/v1/ResourceSet:resourceset-fluxcd-v1.json"
    "fluxcd.controlplane.io/v1/ResourceSetInputProvider:resourcesetinputprovider-fluxcd-v1.json"
    "fluxcd.controlplane.io/v1/FluxInstance:fluxinstance-fluxcd-v1.json"
    "fluxcd.controlplane.io/v1/FluxReport:fluxreport-fluxcd-v1.json"
    "source.toolkit.fluxcd.io/v1/OCIRepository:ocirepository-source-v1.json"
    "source.toolkit.fluxcd.io/v1/GitRepository:gitrepository-source-v1.json"
    "source.toolkit.fluxcd.io/v1/HelmChart:helmchart-source-v1.json"
    "source.toolkit.fluxcd.io/v1/HelmRepository:helmrepository-source-v1.json"
    "source.toolkit.fluxcd.io/v1/Bucket:bucket-source-v1.json"
    "source.toolkit.fluxcd.io/v1/ExternalArtifact:externalartifact-source-v1.json"
    "source.extensions.fluxcd.io/v1beta1/ArtifactGenerator:artifactgenerator-source-v1beta1.json"
    "kustomize.toolkit.fluxcd.io/v1/Kustomization:kustomization-kustomize-v1.json"
    "helm.toolkit.fluxcd.io/v2/HelmRelease:helmrelease-helm-v2.json"
    "image.toolkit.fluxcd.io/v1/ImagePolicy:imagepolicy-image-v1.json"
    "image.toolkit.fluxcd.io/v1/ImageRepository:imagerepository-image-v1.json"
    "image.toolkit.fluxcd.io/v1/ImageUpdateAutomation:imageupdateautomation-image-v1.json"
    "notification.toolkit.fluxcd.io/v1beta3/Alert:alert-notification-v1beta3.json"
    "notification.toolkit.fluxcd.io/v1beta3/Provider:provider-notification-v1beta3.json"
    "notification.toolkit.fluxcd.io/v1beta3/Receiver:receiver-notification-v1.json"
  )
  local entry file
  for entry in "${known[@]}"; do
    file="${entry##*:}"
    if [[ ! -f "${schema_dir}/${file}" ]]; then
      echo "ERROR - Missing schema for Flux GVK ${entry%%:*} (expected ${schema_dir}/${file})" >&2
      missing=1
    fi
  done
  if [[ $missing -ne 0 ]]; then
    exit 1
  fi
}

validate_kustomize_overlays() {
  while IFS= read -r -d $'\0' file; do
    dir="$(dirname "$file")"
    if is_non_kustomize_excluded_dir "$dir"; then
      continue
    fi
    echo "INFO - Validating kustomize overlay ${file/%$kustomize_config}"
    kustomize build "${file/%$kustomize_config}" "${kustomize_flags[@]}" | \
      kubeconform "${kubeconform_flags[@]}" "${kubeconform_config[@]}"
    if [[ ${PIPESTATUS[0]} != 0 || ${PIPESTATUS[1]} != 0 ]]; then
      exit 1
    fi
  done < <(find "$root_dir" -path '*/.*' -prune -o -type f -name "$kustomize_config" -print0)
}

# Main
parse_args "$@"
check_prerequisites
download_schemas
probe_known_flux_schemas
detect_excluded_dirs
validate_yaml_syntax
validate_kubernetes_manifests
validate_kustomize_overlays
echo "INFO - All validations passed"
