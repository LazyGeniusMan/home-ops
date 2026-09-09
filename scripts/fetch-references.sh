##!/usr/bin/env bash
set -e

rm -rf /tmp/home-ops-docs
mkdir -p /tmp/home-ops-docs
cd /tmp/home-ops-docs

# /tmp/home-ops-docs/talos-docs/talos-v1.14.yaml + /tmp/home-ops-docs/talos-docs/public/talos/v1.14
git clone https://github.com/siderolabs/docs -b main --depth 1 talos-docs

# /tmp/home-ops-docs/talos-system-extension-docs/README.md
git clone https://github.com/siderolabs/extensions -b release-1.14 --depth 1 talos-system-extension-docs

# /tmp/home-ops-docs/kubectl-kustomize-docs/site/content/en
git clone https://github.com/kubernetes-sigs/cli-experimental -b master --depth 1 kubectl-kustomize-docs

# /tmp/home-ops-docs/helm-docs/docs
git clone https://github.com/helm/helm-www -b main --depth 1 helm-docs

# /tmp/home-ops-docs/flux-docs/content/en/flux/_index.md
git clone https://github.com/fluxcd/website -b v2-9 --depth 1 flux-docs

# /tmp/home-ops-docs/flux-d2-docs/d2.pdf
mkdir -p /tmp/home-ops-docs/flux-d2-docs
curl -o /tmp/home-ops-docs/flux-d2-docs/d2.pdf https://raw.githubusercontent.com/controlplaneio-fluxcd/distribution/main/guides/ControlPlane_Flux_D2_Reference_Architecture_Guide.pdf
git clone https://github.com/controlplaneio-fluxcd/d2-fleet -b main --depth 1  flux-d2-docs/d2-fleet
git clone https://github.com/controlplaneio-fluxcd/d2-infra -b main --depth 1  flux-d2-docs/d2-infra
git clone https://github.com/controlplaneio-fluxcd/d2-apps -b main --depth 1  flux-d2-docs/d2-apps

# /tmp/home-ops-docs/flux-d1-docs/d1.pdf
mkdir -p /tmp/home-ops-docs/flux-d1-docs
curl -o /tmp/home-ops-docs/flux-d1-docs/d1.pdf https://raw.githubusercontent.com/controlplaneio-fluxcd/distribution/main/guides/ControlPlane_Flux_D1_Reference_Architecture_Guide.pdf

# /tmp/home-ops-docs/flux-operator-docs/docs
git clone https://github.com/controlplaneio-fluxcd/flux-operator -b main --depth 1 flux-operator-docs

# /tmp/home-ops-docs/flux-operator-bootstrap-terraform-docs/README.md
git clone https://github.com/controlplaneio-fluxcd/terraform-kubernetes-flux-operator-bootstrap -b main --depth 1 flux-operator-bootstrap-terraform-docs

# /tmp/home-ops-docs/flux-tofu-controller-docs/docs/index.md
git clone https://github.com/flux-iac/tofu-controller -b main --depth 1 flux-tofu-controller-docs

# /tmp/home-ops-docs/k8s-gateway-api-docs/site/hugo.toml + /tmp/home-ops-docs/k8s-gateway-api-docs/site/content/en
git clone https://github.com/kubernetes-sigs/gateway-api -b main --depth 1 k8s-gateway-api-docs

# /tmp/home-ops-docs/cilium-docs/Documentation/index.rst
git clone https://github.com/cilium/cilium -b v1.20 --depth 1 cilium-docs

# /tmp/home-ops-docs/coredns-docs/content/manual/toc.md
git clone https://github.com/coredns/coredns.io -b master --depth 1 coredns-docs

# /tmp/home-ops-docs/external-secret-operator-docs/docs/index.md
git clone https://github.com/external-secrets/external-secrets -b main --depth 1 external-secret-operator-docs

# /tmp/home-ops-docs/pass-cli-docs/docs/public/docs/index.md 
git clone https://github.com/protonpass/pass-cli -b main --depth 1 pass-cli-docs

# /tmp/home-ops-docs/metrics-server-docs/README.md
git clone https://github.com/kubernetes-sigs/metrics-server -b master --depth 1 metrics-server-docs

# /tmp/home-ops-docs/cert-manager-docs/content/docs/manifest.json
git clone https://github.com/cert-manager/website -b master --depth 1 cert-manager-docs

# /tmp/home-ops-docs/external-dns-docs/mkdocs.yml + /tmp/home-ops-docs/external-dns-docs/docs
git clone https://github.com/kubernetes-sigs/external-dns -b master --depth 1 external-dns-docs

# /tmp/home-ops-docs/netbird-docs/src/pages/ipa
git clone https://github.com/netbirdio/docs/ -b main --depth 1 netbird-docs

# /tmp/home-ops-docs/local-path-provisioner-docs/README.md
git clone https://github.com/rancher/local-path-provisioner -b master --depth 1 local-path-provisioner-docs

# /tmp/home-ops-docs/cnpg-docs/website/versioned_docs/version-1.30/index.md
git clone https://github.com/cloudnative-pg/docs -b main --depth 1 cnpg-docs

# /tmp/home-ops-docs/altinity-clickhouse-operator-docs/docs/README.md
git clone https://github.com/Altinity/clickhouse-operator -b master --depth 1 altinity-clickhouse-operator-docs

# /tmp/home-ops-docs/clickhouse-docs/docs/clickstack/deployment/helm.mdx
git clone https://github.com/clickhouse/clickhouse -b master --depth 1 clickhouse-docs

# /tmp/home-ops-docs/clickstack-helm-docs/README.md
git clone https://github.com/ClickHouse/ClickStack-helm-charts -b main --depth 1 clickstack-helm-docs

# /tmp/home-ops-docs/dragonfly-operator-docs/docs
git clone https://github.com/dragonflydb/documentation -b main --depth 1 dragonfly-operator-docs

# /tmp/home-ops-docs/zitadel-docs/apps/docs/content
git clone https://github.com/zitadel/zitadel -b main --depth 1  zitadel-docs

# /tmp/home-ops-docs/oauth2-proxy-docs/docs/versioned_docs/version-7.15.x
git clone https://github.com/oauth2-proxy/oauth2-proxy -b master --depth 1 oauth2-proxy-docs

# /tmp/home-ops-docs/seaweedfs-operator-docs/README.md
git clone https://github.com/seaweedfs/seaweedfs-operator -b master --depth 1 seaweedfs-operator-docs

# /tmp/home-ops-docs/seaweedfs-docs/Home.md + https://seaweedfs.com/docs/deploy/
git clone https://github.com/seaweedfs/seaweedfs.wiki.git --depth 1 seaweedfs-docs

# /tmp/home-ops-docs/seaweedfs-csi-docs/README.md
git clone https://github.com/seaweedfs/seaweedfs-csi-driver -b master --depth 1 seaweedfs-csi-docs

# /tmp/home-ops-docs/k8s-cosi-docs/docs/src
git clone https://github.com/kubernetes-sigs/container-object-storage-interface -b main --depth 1 k8s-cosi-docs

# /tmp/home-ops-docs/seaweedfs-cosi-docs/README.md
git clone https://github.com/seaweedfs/seaweedfs-cosi-driver -b main --depth 1 seaweedfs-cosi-docs

# /tmp/home-ops-docs/rclone-docs/docs/content
git clone https://github.com/rclone/rclone -b master --depth 1 rclone-docs

# /tmp/home-ops-docs/multus-docs/docs
git clone https://github.com/k8snetworkplumbingwg/multus-cni -b master --depth 1 multus-docs

# /tmp/home-ops-docs/kubevirt-docs/docs/index.md
git clone https://github.com/kubevirt/user-guide -b main --depth 1 kubevirt-docs

# /tmp/home-ops-docs/headlamp-docs/docs/index.md
git clone https://github.com/kubernetes-sigs/headlamp -b main --depth 1 headlamp-docs

# /tmp/home-ops-docs/headlamp-kubevirt-plugin-docs/README.md
git clone https://github.com/naval-group/headlamp-kubevirt -b main --depth 1 headlamp-kubevirt-plugin-docs

# /tmp/home-ops-docs/coder-docs/docs
git clone https://github.com/coder/coder -b main --depth 1 coder-docs