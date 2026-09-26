#!/usr/bin/env bash

# Validate Flux custom resources and kustomize overlays using kubeconform.
# Adapted from the upstream d2-fleet/d2-infra/d2-apps scripts/validate.sh
# (Copyright 2023-2026 The Flux authors, Apache-2.0); pointed at this
# monorepo's flux/ areas instead of a standalone repo root.
#
# This script downloads the Flux OpenAPI schemas, then it validates the
# Flux custom resources and the kustomize overlays using kubeconform.
# Meant to be run locally and in CI before changes are merged on main.
#
# Prerequisites
# - yq >= 4.50
# - kustomize >= 5.8
# - kubeconform >= 0.7

set -o errexit
set -o nounset
set -o pipefail
# Strict-mode contract matches scripts/tag-release.sh (errexit + nounset +
# pipefail). scripts/fetch-references.sh omits errexit by design — its header
# documents why (continue-on-error accounting).

# mirror kustomize-controller build options
kustomize_flags=("--load-restrictor=LoadRestrictionsNone")
kustomize_config="kustomization.yaml"

# Keep copyFrom/copyTo stub Secrets (ResourceSet templates) and Terraform
# varsFrom handoffs out of kubeconform: they carry no schema-meaningful fields
# (plain `data`/`stringData`-less copies). No SOPS usage remains in this repo.
kubeconform_flags=("-skip=Secret")
kubeconform_config=("-strict" "-ignore-missing-schemas" "-schema-location" "default" "-schema-location" "/tmp/flux-crd-schemas" "-verbose")

# root directory to validate
root_dir="."

# directories to exclude from validation
exclude_dirs=()

# Directories auto-detected as non-Kubernetes. Ancestor dirs (not just the file
# dir) are skipped: some are Terraform-enclosing shells whose .tf files live in
# a nested terraform/ dir (find -name searches the whole subtree, so a flat
# *.tf check alone would miss e.g. netbird/terraform/../controllers).
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

# Schema pins track this repo's Flux pins: operator v0.60.0
# (flux/fleet/terraform/versions.yaml operator_chart_version) and flux2 v2.9.5
# (group_vars/all.yml, distribution/actions, .flox fluxcd). Bump all together.
# sha256 values were taken from the release assets on 2026-09-26.
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

# Detect directories containing Terraform files, Helm charts, or kustomize overlays.
# *.tf/Chart.yaml matching is name-exact (no globs). Only the marker file's own
# directory is skipped: Terraform-enclosing shells (e.g. netbird/controllers
# over the shared terraform/ root) hold only empty-shell kustomizations that
# the kustomize pass below still builds, so they must stay in raw validation.
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

validate_kubernetes_manifests() {
  echo "INFO - Validating Kubernetes manifests"
  while IFS= read -r -d $'\0' file; do
    dir="$(dirname "$file")"
    if is_excluded_dir "$dir"; then
      continue
    fi
    kubeconform "${kubeconform_flags[@]}" "${kubeconform_config[@]}" "${file}"
  done < <(find "$root_dir" -path '*/.*' -prune -o -type f -name '*.yaml' -print0)
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
detect_excluded_dirs
validate_yaml_syntax
validate_kubernetes_manifests
validate_kustomize_overlays
echo "INFO - All validations passed"
