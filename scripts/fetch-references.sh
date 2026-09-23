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
#
# The script wipes and recreates /tmp/home-ops-docs on every run, then
# re-fetches each entry below. One failure never aborts the rest
# (continue-on-error with a non-zero exit + failed-dest summary at the end).
set -uo pipefail

FETCH_MODE="${FETCH_MODE:-http}"

# Continue-on-error bookkeeping: every fetch_repo/fetch_pdf call site records
# its own outcome, so one failure never aborts the remaining fetches. The
# script exits non-zero iff any fetch failed (see summary at the end).
_TMP_PATHS=()
SUCCESS_COUNT=0
FAIL_COUNT=0
FAILED_LIST=""

log() {
  printf '%s [fetch-references][%s] %s\n' "$(date '+%Y-%m-%dT%H:%M:%S%z')" "$FETCH_MODE" "$*"
}

log_error() {
  printf '%s [fetch-references][%s] ERROR: %s\n' "$(date '+%Y-%m-%dT%H:%M:%S%z')" "$FETCH_MODE" "$*" >&2
}

# Continue-on-error accounting. fetch_repo/fetch_pdf bump SUCCESS_COUNT
# themselves on success; every call site appends `|| record_fail <dest>` so a
# failure is counted (and logged with repo identity inside the function)
# without aborting the remaining fetches.
record_fail() {
  FAIL_COUNT=$((FAIL_COUNT + 1))
  FAILED_LIST+=" $1"
}

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
  local path ssh_url zip_url tmp_zip tmp_dir resolved_url fail_msg
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

  fail_msg="FAILED: ${dest} mode=${FETCH_MODE} url=${resolved_url} branch=${branch:-<default>}"
  log "==> [${FETCH_MODE}] ${dest} (${resolved_url})"

  case "$FETCH_MODE" in
    http)
      if [[ -n "$branch" ]]; then
        if ! git clone "$http_url" -b "$branch" --depth 1 "$dest"; then
          log_error "$fail_msg"
          return 1
        fi
      else
        if ! git clone "$http_url" --depth 1 "$dest"; then
          log_error "$fail_msg"
          return 1
        fi
      fi
      ;;
    ssh)
      if [[ -n "$branch" ]]; then
        if ! git clone "$ssh_url" -b "$branch" --depth 1 "$dest"; then
          log_error "$fail_msg"
          return 1
        fi
      else
        if ! git clone "$ssh_url" --depth 1 "$dest"; then
          log_error "$fail_msg"
          return 1
        fi
      fi
      ;;
    zip)
      if [[ "$path" == *.wiki ]]; then
        log ".wiki repo: falling back to git clone in zip mode (${dest})"
        fail_msg="FAILED: ${dest} mode=${FETCH_MODE} url=${http_url} branch=${branch:-<default>}"
        if [[ -n "$branch" ]]; then
          if ! git clone "$http_url" -b "$branch" --depth 1 "$dest"; then
            log_error "$fail_msg"
            return 1
          fi
        else
          if ! git clone "$http_url" --depth 1 "$dest"; then
            log_error "$fail_msg"
            return 1
          fi
        fi
      else
      zip_url="$resolved_url"
      tmp_zip="$(mktemp /tmp/fetch-references-XXXXXX.zip)"
      tmp_dir="$(mktemp -d /tmp/fetch-references-XXXXXX)"
      _TMP_PATHS+=("$tmp_zip" "$tmp_dir")
      if ! curl -fsSL -o "$tmp_zip" "$zip_url"; then
        log_error "$fail_msg"
        rm -rf "$tmp_zip" "$tmp_dir"
        return 1
      fi
      if ! unzip -q "$tmp_zip" -d "$tmp_dir"; then
        log_error "$fail_msg"
        rm -rf "$tmp_zip" "$tmp_dir"
        return 1
      fi
      rm -f "$tmp_zip"
      if ! mkdir -p "$(dirname "$dest")"; then
        log_error "$fail_msg"
        rm -rf "$tmp_dir"
        return 1
      fi
      rm -rf "$dest"
      extracted=("$tmp_dir"/*)
      if ((${#extracted[@]} == 0)) || [[ ! -e "${extracted[0]}" ]]; then
        log_error "$fail_msg"
        rm -rf "$tmp_dir"
        return 1
      fi
      if ! mv "${extracted[0]}" "$dest"; then
        log_error "$fail_msg"
        rm -rf "$tmp_dir"
        return 1
      fi
      rm -rf "$tmp_dir"
      fi
      ;;
  esac

  log "ok: ${dest}"
  SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
}

# fetch_pdf <dest-file> <url>
# Downloads a single file (used for the two PDF guides) with the same
# start/ok/FAILED logging as fetch_repo.
fetch_pdf() {
  local dest="$1"
  local url="$2"
  local fail_msg

  fail_msg="FAILED: ${dest} mode=${FETCH_MODE} url=${url} branch=<default>"
  log "==> [${FETCH_MODE}] ${dest} (${url})"
  if ! mkdir -p "$(dirname "$dest")"; then
    log_error "$fail_msg"
    return 1
  fi
  if ! curl -fSL -o "$dest" "$url"; then
    log_error "$fail_msg"
    return 1
  fi
  log "ok: ${dest}"
  SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
}

rm -rf /tmp/home-ops-docs
mkdir -p /tmp/home-ops-docs || exit 1
cd /tmp/home-ops-docs || exit 1

# /tmp/home-ops-docs/talos-docs/talos-v1.14.yaml + /tmp/home-ops-docs/talos-docs/public/talos/v1.14
fetch_repo talos-docs https://github.com/siderolabs/docs main || record_fail talos-docs

# /tmp/home-ops-docs/talos-system-extension-docs/README.md
fetch_repo talos-system-extension-docs https://github.com/siderolabs/extensions main || record_fail talos-system-extension-docs

# /tmp/home-ops-docs/kubectl-kustomize-docs/site/content/en
fetch_repo kubectl-kustomize-docs https://github.com/kubernetes-sigs/cli-experimental master || record_fail kubectl-kustomize-docs

# /tmp/home-ops-docs/helm-docs/docs
fetch_repo helm-docs https://github.com/helm/helm-www main || record_fail helm-docs

# /tmp/home-ops-docs/flux-docs/content/en/flux/_index.md
fetch_repo flux-docs https://github.com/fluxcd/website v2-9 || record_fail flux-docs

# /tmp/home-ops-docs/flux-d2-docs/d2.pdf
fetch_pdf /tmp/home-ops-docs/flux-d2-docs/d2.pdf https://raw.githubusercontent.com/controlplaneio-fluxcd/distribution/main/guides/ControlPlane_Flux_D2_Reference_Architecture_Guide.pdf || record_fail "/tmp/home-ops-docs/flux-d2-docs/d2.pdf"
fetch_repo flux-d2-docs/d2-fleet https://github.com/controlplaneio-fluxcd/d2-fleet main || record_fail flux-d2-docs/d2-fleet
fetch_repo flux-d2-docs/d2-infra https://github.com/controlplaneio-fluxcd/d2-infra main || record_fail flux-d2-docs/d2-infra
fetch_repo flux-d2-docs/d2-apps https://github.com/controlplaneio-fluxcd/d2-apps main || record_fail flux-d2-docs/d2-apps

# /tmp/home-ops-docs/flux-operator-docs/docs/web
fetch_repo flux-operator-docs https://github.com/controlplaneio-fluxcd/flux-operator main || record_fail flux-operator-docs

# /tmp/home-ops-docs/flux-operator-bootstrap-terraform-docs/README.md
fetch_repo flux-operator-bootstrap-terraform-docs https://github.com/controlplaneio-fluxcd/terraform-kubernetes-flux-operator-bootstrap main || record_fail flux-operator-bootstrap-terraform-docs

# /tmp/home-ops-docs/flux-tofu-controller-docs/docs (index.md, tfctl.md, branch-planner/)
fetch_repo flux-tofu-controller-docs https://github.com/flux-iac/tofu-controller main || record_fail flux-tofu-controller-docs

# /tmp/home-ops-docs/k8s-gateway-api-docs/site/hugo.toml + /tmp/home-ops-docs/k8s-gateway-api-docs/site/content/en
fetch_repo k8s-gateway-api-docs https://github.com/kubernetes-sigs/gateway-api main || record_fail k8s-gateway-api-docs

# /tmp/home-ops-docs/cilium-docs/Documentation/index.rst
fetch_repo cilium-docs https://github.com/cilium/cilium v1.20 || record_fail cilium-docs

# /tmp/home-ops-docs/coredns-docs/content/manual/toc.md
fetch_repo coredns-docs https://github.com/coredns/coredns.io master || record_fail coredns-docs

# /tmp/home-ops-docs/external-secret-operator-docs/docs/index.md
fetch_repo external-secret-operator-docs https://github.com/external-secrets/external-secrets main || record_fail external-secret-operator-docs

# /tmp/home-ops-docs/pass-cli-docs/docs/public/docs/index.md 
fetch_repo pass-cli-docs https://github.com/protonpass/pass-cli main || record_fail pass-cli-docs

# /tmp/home-ops-docs/metrics-server-docs/README.md
fetch_repo metrics-server-docs https://github.com/kubernetes-sigs/metrics-server master || record_fail metrics-server-docs

# /tmp/home-ops-docs/vpa-docs/vertical-pod-autoscaler/README.md
fetch_repo vpa-docs https://github.com/kubernetes/autoscaler master || record_fail vpa-docs

# /tmp/home-ops-docs/cert-manager-docs/content/docs/manifest.json
fetch_repo cert-manager-docs https://github.com/cert-manager/website master || record_fail cert-manager-docs

# /tmp/home-ops-docs/external-dns-docs/mkdocs.yml + /tmp/home-ops-docs/external-dns-docs/docs
fetch_repo external-dns-docs https://github.com/kubernetes-sigs/external-dns master || record_fail external-dns-docs

# /tmp/home-ops-docs/netbird-docs/src/pages/ipa
fetch_repo netbird-docs https://github.com/netbirdio/docs/ main || record_fail netbird-docs

# /tmp/home-ops-docs/netbird-terraform-provider-docs/README.md
fetch_repo netbird-terraform-provider-docs https://github.com/netbirdio/terraform-provider-netbird main || record_fail netbird-terraform-provider-docs

# /tmp/home-ops-docs/cloudflare-terraform-provider-docs/README.md
fetch_repo cloudflare-terraform-provider-docs https://github.com/cloudflare/terraform-provider-cloudflare main || record_fail cloudflare-terraform-provider-docs

# /tmp/home-ops-docs/local-path-provisioner-docs/README.md
fetch_repo local-path-provisioner-docs https://github.com/rancher/local-path-provisioner master || record_fail local-path-provisioner-docs

# /tmp/home-ops-docs/cnpg-docs/website/versioned_docs/version-1.30/index.md
fetch_repo cnpg-docs https://github.com/cloudnative-pg/docs main || record_fail cnpg-docs

# /tmp/home-ops-docs/altinity-clickhouse-operator-docs/docs/README.md
fetch_repo altinity-clickhouse-operator-docs https://github.com/Altinity/clickhouse-operator master || record_fail altinity-clickhouse-operator-docs

# /tmp/home-ops-docs/clickhouse-docs/docs/clickstack/deployment/helm.mdx
fetch_repo clickhouse-docs https://github.com/clickhouse/clickhouse master || record_fail clickhouse-docs

# /tmp/home-ops-docs/clickstack-helm-charts-docs/README.md
fetch_repo clickstack-helm-charts-docs https://github.com/ClickHouse/ClickStack-helm-charts main || record_fail clickstack-helm-charts-docs

# /tmp/home-ops-docs/dragonfly-operator-docs/docs
fetch_repo dragonfly-operator-docs https://github.com/dragonflydb/documentation main || record_fail dragonfly-operator-docs

# /tmp/home-ops-docs/zitadel-docs/apps/docs/content
fetch_repo zitadel-docs https://github.com/zitadel/zitadel main || record_fail zitadel-docs

# /tmp/home-ops-docs/zitadel-helm-charts-docs/README.md
fetch_repo zitadel-helm-charts-docs https://github.com/zitadel/zitadel-charts main || record_fail zitadel-helm-charts-docs

# /tmp/home-ops-docs/zitadel-terraform-provider-docs/README.md
fetch_repo zitadel-terraform-provider-docs https://github.com/zitadel/terraform-provider-zitadel main || record_fail zitadel-terraform-provider-docs

# /tmp/home-ops-docs/oauth2-proxy-docs/docs/versioned_docs/version-7.15.x
fetch_repo oauth2-proxy-docs https://github.com/oauth2-proxy/oauth2-proxy master || record_fail oauth2-proxy-docs

# /tmp/home-ops-docs/seaweedfs-operator-docs/README.md
fetch_repo seaweedfs-operator-docs https://github.com/seaweedfs/seaweedfs-operator master || record_fail seaweedfs-operator-docs

# /tmp/home-ops-docs/seaweedfs-docs/Home.md + https://seaweedfs.com/docs/deploy/
fetch_repo seaweedfs-docs https://github.com/seaweedfs/seaweedfs.wiki.git "" || record_fail seaweedfs-docs

# /tmp/home-ops-docs/seaweedfs-csi-docs/README.md
fetch_repo seaweedfs-csi-docs https://github.com/seaweedfs/seaweedfs-csi-driver master || record_fail seaweedfs-csi-docs

# /tmp/home-ops-docs/k8s-cosi-docs/docs/src
fetch_repo k8s-cosi-docs https://github.com/kubernetes-sigs/container-object-storage-interface main || record_fail k8s-cosi-docs

# /tmp/home-ops-docs/seaweedfs-cosi-docs/README.md
fetch_repo seaweedfs-cosi-docs https://github.com/seaweedfs/seaweedfs-cosi-driver main || record_fail seaweedfs-cosi-docs

# /tmp/home-ops-docs/multus-docs/docs
fetch_repo multus-docs https://github.com/k8snetworkplumbingwg/multus-cni master || record_fail multus-docs

# /tmp/home-ops-docs/kubevirt-docs/docs/index.md
fetch_repo kubevirt-docs https://github.com/kubevirt/user-guide main || record_fail kubevirt-docs

# /tmp/home-ops-docs/headlamp-docs/docs/index.md
fetch_repo headlamp-docs https://github.com/kubernetes-sigs/headlamp main || record_fail headlamp-docs

# /tmp/home-ops-docs/headlamp-kubevirt-plugin-docs/README.md
fetch_repo headlamp-kubevirt-plugin-docs https://github.com/naval-group/headlamp-kubevirt main || record_fail headlamp-kubevirt-plugin-docs

# /tmp/home-ops-docs/apprise-docs/locales/en
fetch_repo apprise-docs https://github.com/caronc/apprise-docs master || record_fail apprise-docs

# /tmp/home-ops-docs/apprise-api-py-docs/README.md
fetch_repo apprise-api-py-docs https://github.com/caronc/apprise-api master || record_fail apprise-api-py-docs

# /tmp/home-ops-docs/apprise-go-docs/README.md
fetch_repo apprise-go-docs https://github.com/unraid/apprise-go main || record_fail apprise-go-docs

# /tmp/home-ops-docs/tuwumel-docs/docs/README.md
fetch_repo tuwumel-docs https://github.com/matrix-construct/tuwunel main || record_fail tuwumel-docs

# /tmp/home-ops-docs/matrix-terraform-provider-docs/README.md
fetch_repo matrix-terraform-provider-docs https://github.com/raspbeguy/terraform-provider-matrix main || record_fail matrix-terraform-provider-docs

# /tmp/home-ops-docs/element-web-docs/docs
fetch_repo element-web-docs https://github.com/element-hq/element-web develop || record_fail element-web-docs

# /tmp/home-ops-docs/matrix-mautrix-bridge-docs/bridges
fetch_repo matrix-mautrix-bridge-docs https://github.com/mautrix/docs master || record_fail matrix-mautrix-bridge-docs

# /tmp/home-ops-docs/matrix-mautrix-discord-bridge-docs/README.md
fetch_repo matrix-mautrix-discord-bridge-docs https://github.com/mautrix/discord main || record_fail matrix-mautrix-discord-bridge-docs


# /tmp/home-ops-docs/coder-docs/docs
fetch_repo coder-docs https://github.com/coder/coder main || record_fail coder-docs

if ((FAIL_COUNT > 0)); then
  log_error "Summary: Succeeded: ${SUCCESS_COUNT}, Failed: ${FAIL_COUNT}; failed:${FAILED_LIST}"
  exit 1
fi
log "Summary: Succeeded: ${SUCCESS_COUNT}, Failed: ${FAIL_COUNT}"
exit 0
