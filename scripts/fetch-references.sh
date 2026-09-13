#!/usr/bin/env bash
# Manual-only helper: wipes and re-clones upstream reference docs into
# /tmp/home-ops-docs. No in-repo callers; run by hand when refreshing
# local reference material.
#
# Fetch method is selectable via the FETCH_MODE environment variable or the
# --mode/-m CLI flag (flag wins):
#   http  git clone over HTTPS (default, preserves historical behavior)
#   ssh   git clone over SSH (git@github.com:, for SSH-auth environments)
#   zip   download the branch ZIP over HTTPS and extract it (no git needed)
set -Eeuo pipefail

FETCH_MODE="${FETCH_MODE:-http}"

# Globals for error-identifiable logging. The ERR trap runs in the caller's
# context, so the current fetch target is tracked in globals (not locals).
_CURRENT_DEST=""
_CURRENT_URL=""
_CURRENT_BRANCH=""
_TMP_PATHS=()

log() {
  printf '%s [fetch-references][%s] %s\n' "$(date '+%Y-%m-%dT%H:%M:%S%z')" "$FETCH_MODE" "$*"
}

log_error() {
  printf '%s [fetch-references][%s] ERROR: %s\n' "$(date '+%Y-%m-%dT%H:%M:%S%z')" "$FETCH_MODE" "$*" >&2
}

on_error() {
  log_error "FAILED: ${_CURRENT_DEST:-unknown} mode=${FETCH_MODE} url=${_CURRENT_URL:-unknown} branch=${_CURRENT_BRANCH:-<default>}"
}
trap on_error ERR

cleanup_tmps() {
  if ((${#_TMP_PATHS[@]})); then
    rm -rf "${_TMP_PATHS[@]}" 2>/dev/null || true
  fi
}
trap cleanup_tmps EXIT

usage() {
  cat <<'EOF'
Usage: fetch-references.sh [--mode http|ssh|zip] [--help]

Wipes and re-fetches upstream reference docs into /tmp/home-ops-docs.

Options:
  -m, --mode MODE   Fetch method: http (default), ssh, or zip.
                    Overrides the FETCH_MODE environment variable.
  -h, --help        Show this help and exit.

Environment:
  FETCH_MODE        Same as --mode; default is http.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -m|--mode)
      if [[ $# -lt 2 ]]; then
        echo "error: --mode requires an argument (http|ssh|zip)" >&2
        exit 1
      fi
      FETCH_MODE="$2"
      shift 2
      ;;
    --mode=*)
      FETCH_MODE="${1#*=}"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

case "$FETCH_MODE" in
  http|ssh|zip)
    ;;
  *)
    echo "error: invalid mode '$FETCH_MODE' (expected one of: http, ssh, zip)" >&2
    exit 1
    ;;
esac

# fetch_repo <dest-dir> <https-url> [branch]
# Fetches one repo into <dest-dir> (relative to /tmp/home-ops-docs) using the
# method selected by FETCH_MODE. The SSH URL is derived by rewriting the
# https://github.com/ prefix to git@github.com:, and the ZIP URL follows the
# https://github.com/<org>/<repo>/archive/refs/heads/<branch>.zip pattern
# (stripping any trailing slash or .git suffix). An empty branch means the
# repo's default branch: plain `git clone --depth 1` for http/ssh, and the
# GitHub HEAD archive for zip.
fetch_repo() {
  local dest="$1"
  local http_url="$2"
  local branch="${3:-}"
  local path ssh_url zip_url tmp_zip tmp_dir resolved_url
  local -a extracted

  http_url="${http_url%/}"
  path="${http_url#https://github.com/}"
  path="${path%.git}"
  ssh_url="git@github.com:${path}.git"

  case "$FETCH_MODE" in
    http)
      resolved_url="$http_url"
      ;;
    ssh)
      resolved_url="$ssh_url"
      ;;
    zip)
      if [[ -n "$branch" ]]; then
        resolved_url="https://github.com/${path}/archive/refs/heads/${branch}.zip"
      else
        resolved_url="https://github.com/${path}/archive/HEAD.zip"
      fi
      ;;
  esac

  _CURRENT_DEST="$dest"
  _CURRENT_URL="$resolved_url"
  _CURRENT_BRANCH="$branch"
  log "==> [${FETCH_MODE}] ${dest} (${resolved_url})"

  case "$FETCH_MODE" in
    http)
      if [[ -n "$branch" ]]; then
        git clone "$http_url" -b "$branch" --depth 1 "$dest"
      else
        git clone "$http_url" --depth 1 "$dest"
      fi
      ;;
    ssh)
      if [[ -n "$branch" ]]; then
        git clone "$ssh_url" -b "$branch" --depth 1 "$dest"
      else
        git clone "$ssh_url" --depth 1 "$dest"
      fi
      ;;
    zip)
      zip_url="$resolved_url"
      tmp_zip="$(mktemp /tmp/fetch-references-XXXXXX.zip)"
      tmp_dir="$(mktemp -d /tmp/fetch-references-XXXXXX)"
      _TMP_PATHS+=("$tmp_zip" "$tmp_dir")
      curl -fsSL -o "$tmp_zip" "$zip_url"
      unzip -q "$tmp_zip" -d "$tmp_dir"
      rm -f "$tmp_zip"
      mkdir -p "$(dirname "$dest")"
      rm -rf "$dest"
      extracted=("$tmp_dir"/*)
      mv "${extracted[0]}" "$dest"
      rm -rf "$tmp_dir"
      ;;
  esac

  log "ok: ${dest}"
  _CURRENT_DEST=""
  _CURRENT_URL=""
  _CURRENT_BRANCH=""
}

# fetch_pdf <dest-file> <url>
# Downloads a single file (used for the two PDF guides) with the same
# start/ok/FAILED logging as fetch_repo.
fetch_pdf() {
  local dest="$1"
  local url="$2"

  _CURRENT_DEST="$dest"
  _CURRENT_URL="$url"
  _CURRENT_BRANCH=""
  log "==> [${FETCH_MODE}] ${dest} (${url})"
  mkdir -p "$(dirname "$dest")"
  curl -fSL -o "$dest" "$url"
  log "ok: ${dest}"
  _CURRENT_DEST=""
  _CURRENT_URL=""
  _CURRENT_BRANCH=""
}

rm -rf /tmp/home-ops-docs
mkdir -p /tmp/home-ops-docs
cd /tmp/home-ops-docs

# /tmp/home-ops-docs/talos-docs/talos-v1.14.yaml + /tmp/home-ops-docs/talos-docs/public/talos/v1.14
fetch_repo talos-docs https://github.com/siderolabs/docs main

# /tmp/home-ops-docs/talos-system-extension-docs/README.md
fetch_repo talos-system-extension-docs https://github.com/siderolabs/extensions main

# /tmp/home-ops-docs/kubectl-kustomize-docs/site/content/en
fetch_repo kubectl-kustomize-docs https://github.com/kubernetes-sigs/cli-experimental master

# /tmp/home-ops-docs/helm-docs/docs
fetch_repo helm-docs https://github.com/helm/helm-www main

# /tmp/home-ops-docs/flux-docs/content/en/flux/_index.md
fetch_repo flux-docs https://github.com/fluxcd/website v2-9

# /tmp/home-ops-docs/flux-d2-docs/d2.pdf
fetch_pdf /tmp/home-ops-docs/flux-d2-docs/d2.pdf https://raw.githubusercontent.com/controlplaneio-fluxcd/distribution/main/guides/ControlPlane_Flux_D2_Reference_Architecture_Guide.pdf
fetch_repo flux-d2-docs/d2-fleet https://github.com/controlplaneio-fluxcd/d2-fleet main
fetch_repo flux-d2-docs/d2-infra https://github.com/controlplaneio-fluxcd/d2-infra main
fetch_repo flux-d2-docs/d2-apps https://github.com/controlplaneio-fluxcd/d2-apps main

# /tmp/home-ops-docs/flux-d1-docs/d1.pdf
fetch_pdf /tmp/home-ops-docs/flux-d1-docs/d1.pdf https://raw.githubusercontent.com/controlplaneio-fluxcd/distribution/main/guides/ControlPlane_Flux_D1_Reference_Architecture_Guide.pdf

# /tmp/home-ops-docs/flux-operator-docs/docs
fetch_repo flux-operator-docs https://github.com/controlplaneio-fluxcd/flux-operator main

# /tmp/home-ops-docs/flux-operator-bootstrap-terraform-docs/README.md
fetch_repo flux-operator-bootstrap-terraform-docs https://github.com/controlplaneio-fluxcd/terraform-kubernetes-flux-operator-bootstrap main

# /tmp/home-ops-docs/flux-tofu-controller-docs/docs/index.md
fetch_repo flux-tofu-controller-docs https://github.com/flux-iac/tofu-controller main

# /tmp/home-ops-docs/k8s-gateway-api-docs/site/hugo.toml + /tmp/home-ops-docs/k8s-gateway-api-docs/site/content/en
fetch_repo k8s-gateway-api-docs https://github.com/kubernetes-sigs/gateway-api main

# /tmp/home-ops-docs/cilium-docs/Documentation/index.rst
fetch_repo cilium-docs https://github.com/cilium/cilium v1.20

# /tmp/home-ops-docs/coredns-docs/content/manual/toc.md
fetch_repo coredns-docs https://github.com/coredns/coredns.io master

# /tmp/home-ops-docs/external-secret-operator-docs/docs/index.md
fetch_repo external-secret-operator-docs https://github.com/external-secrets/external-secrets main

# /tmp/home-ops-docs/pass-cli-docs/docs/public/docs/index.md 
fetch_repo pass-cli-docs https://github.com/protonpass/pass-cli main

# /tmp/home-ops-docs/metrics-server-docs/README.md
fetch_repo metrics-server-docs https://github.com/kubernetes-sigs/metrics-server master

# /tmp/home-ops-docs/cert-manager-docs/content/docs/manifest.json
fetch_repo cert-manager-docs https://github.com/cert-manager/website master

# /tmp/home-ops-docs/external-dns-docs/mkdocs.yml + /tmp/home-ops-docs/external-dns-docs/docs
fetch_repo external-dns-docs https://github.com/kubernetes-sigs/external-dns master

# /tmp/home-ops-docs/netbird-docs/src/pages/ipa
fetch_repo netbird-docs https://github.com/netbirdio/docs/ main

# /tmp/home-ops-docs/local-path-provisioner-docs/README.md
fetch_repo local-path-provisioner-docs https://github.com/rancher/local-path-provisioner master

# /tmp/home-ops-docs/cnpg-docs/website/versioned_docs/version-1.30/index.md
fetch_repo cnpg-docs https://github.com/cloudnative-pg/docs main

# /tmp/home-ops-docs/altinity-clickhouse-operator-docs/docs/README.md
fetch_repo altinity-clickhouse-operator-docs https://github.com/Altinity/clickhouse-operator master

# /tmp/home-ops-docs/clickhouse-docs/docs/clickstack/deployment/helm.mdx
fetch_repo clickhouse-docs https://github.com/clickhouse/clickhouse master

# /tmp/home-ops-docs/clickstack-helm-docs/README.md
fetch_repo clickstack-helm-charts-docs https://github.com/ClickHouse/ClickStack-helm-charts main

# /tmp/home-ops-docs/dragonfly-operator-docs/docs
fetch_repo dragonfly-operator-docs https://github.com/dragonflydb/documentation main

# /tmp/home-ops-docs/zitadel-docs/apps/docs/content
fetch_repo zitadel-docs https://github.com/zitadel/zitadel main

# /tmp/home-ops-docs/zitadel-docs/README.md
fetch_repo zitadel-helm-charts-docs https://github.com/zitadel/zitadel-charts main

# /tmp/home-ops-docs/oauth2-proxy-docs/docs/versioned_docs/version-7.15.x
fetch_repo oauth2-proxy-docs https://github.com/oauth2-proxy/oauth2-proxy master

# /tmp/home-ops-docs/seaweedfs-operator-docs/README.md
fetch_repo seaweedfs-operator-docs https://github.com/seaweedfs/seaweedfs-operator master

# /tmp/home-ops-docs/seaweedfs-docs/Home.md + https://seaweedfs.com/docs/deploy/
fetch_repo seaweedfs-docs https://github.com/seaweedfs/seaweedfs.wiki.git ""

# /tmp/home-ops-docs/seaweedfs-csi-docs/README.md
fetch_repo seaweedfs-csi-docs https://github.com/seaweedfs/seaweedfs-csi-driver master

# /tmp/home-ops-docs/k8s-cosi-docs/docs/src
fetch_repo k8s-cosi-docs https://github.com/kubernetes-sigs/container-object-storage-interface main

# /tmp/home-ops-docs/seaweedfs-cosi-docs/README.md
fetch_repo seaweedfs-cosi-docs https://github.com/seaweedfs/seaweedfs-cosi-driver main

# /tmp/home-ops-docs/multus-docs/docs
fetch_repo multus-docs https://github.com/k8snetworkplumbingwg/multus-cni master

# /tmp/home-ops-docs/kubevirt-docs/docs/index.md
fetch_repo kubevirt-docs https://github.com/kubevirt/user-guide main

# /tmp/home-ops-docs/headlamp-docs/docs/index.md
fetch_repo headlamp-docs https://github.com/kubernetes-sigs/headlamp main

# /tmp/home-ops-docs/headlamp-kubevirt-plugin-docs/README.md
fetch_repo headlamp-kubevirt-plugin-docs https://github.com/naval-group/headlamp-kubevirt main

# /tmp/home-ops-docs/coder-docs/docs
fetch_repo coder-docs https://github.com/coder/coder main
