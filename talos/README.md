# talos/ — Talos Linux cluster configs (Talos v1.15.0-alpha.0, `talosctl`-only)

Declarative machine configuration for all Talos clusters. No CNI, no CoreDNS,
and no bootstrap manifests ship from this tree — Cilium, DNS, and workloads
arrive later via GitOps. No telemetry, SideroLink, or discovery config ships
anywhere in this tree (cluster discovery is deleted in `_base/patches.yml`),
so nodes never auto-join anything. Verify: `grep -rni
'telemetry\|siderolink\|TelemetryConfig' talos/clusters/` returns nothing.

## Layout convention

```text
talos/
  README.md                          # this file (convention + workflow)
  .gitignore                         # build/, secrets bundles, kubeconfigs
  clusters/
    _base/
      patches.yml                    # shared barebone patch (Flannel CNI deleted, CoreDNS off, discovery off, kube-proxy iptables)
      schematics.yml                 # vanilla Image Factory schematic (no extensions)
    <cluster-name>/                   # e.g. acme-prd-bdo1-talos-apps-01
      patches.yml                    # multi-doc split-doc kinds (all docs carry explicit kind)
      schematics.yml                 # cluster-shared schematic (e.g. netbird)
      secrets.yml.template           # committed; intentionally field-free manual `talosctl gen secrets` notes (output never committed)
      secrets.yml                    # GITIGNORED manual output (day-0 automation writes ansible/build/<cluster>/secrets.bundle.yml)
      nodes/<node-name>/
        patches.yml                  # node multi-doc: hostname, LinkConfig, install, volumes
        schematics.yml               # AUTHORITATIVE schematic for this node's installer image
  ansible/                           # day-0/1/2 automation (see ansible/README.md)
```

`patches.yml` files are multi-document YAML where every document is a split-doc
kind with explicit `apiVersion`/`kind` (base:
`KubeFlannelCNIConfig`, `KubeCoreDNSConfig`, `KubeProxyConfig`,
`DiscoveryServiceConfig`, `TimeSyncConfig`; clusters add `KubeClusterConfig`,
`KubeNodeConfig`, `Layer2VIPConfig`, `RegistryAuthConfig`, `ResolverConfig`,
`ExtensionServiceConfig`; nodes add `HostnameConfig`, `LinkConfig`,
`UnattendedInstallConfig`, `VolumeConfig`, `UserVolumeConfig`,
`SysctlConfig`, `KernelModuleConfig`, `EtcFileConfig`, `ExtensionServiceConfig`
— including the netbird + nfs-server `ExtensionServiceConfig` docs). The base
ships zero manifests (no `KubeInlineManifestConfig` /
`KubeExternalManifestConfig`).

## Secrets (no plaintext, ever)

- Committed files contain only double-brace Proton Pass references, e.g.
  `{{ pass://acme-prd-bdo1-talos-apps-01/talos/docker-password }}`.
  Only `pass-cli inject` / `pass-cli item view` resolve them — bare `pass://`
  URIs are never dereferenced by Talos or Ansible directly.
- Render via `pass-cli inject` on double-brace templates (cluster + per-node
  patches, PAT from `pat.yml.template`); NetBird PAT via `pass-cli item view`
  as `NB_PAT` env, setup key Terraform-minted — scoped 90d / usage 3, never
  unlimited (procedures: `ansible/RUNBOOK.md` §0.3, §1.0b). Authenticate first:
  `pass-cli login` (gate: `pass-cli info`; Ansible probes it via `pass-cli
  info -o json` before every play).
- `secrets.bundle.yml`, `talosconfig`, `kubeconfig`, rendered `ansible/build/`
  output are gitignored. Binary is `pass-cli` (not `proton-pass-cli`).

## Workflow (talosctl only — no kubectl here)

```bash
# 1. Upload the node's merged schematic, note the schematic ID
curl -X POST --data-binary @ansible/build/<cluster>/schematics-<node>.yml \
  https://factory.talos.dev/schematics
# 2. Day-0 rewrites PLACEHOLDER_SCHEMATIC_ID to that ID, then generates:
talosctl gen secrets -o ansible/build/<cluster>/secrets.bundle.yml
talosctl gen config <cluster> https://<VIP>:6443 \
  --with-secrets ansible/build/<cluster>/secrets.bundle.yml \
  --config-patch @clusters/_base/patches.yml \
  --config-patch @ansible/build/<cluster>/patches.yml \
  -t talosconfig \
  -o ansible/build/<cluster>/talosconfig
talosctl gen config <cluster> https://<VIP>:6443 \
  --with-secrets ansible/build/<cluster>/secrets.bundle.yml \
  --config-patch @clusters/_base/patches.yml \
  --config-patch @ansible/build/<cluster>/patches.yml \
  --config-patch-control-plane @ansible/build/<cluster>/nodes-<node>-patches.yml \
  --install-image factory.talos.dev/metal-installer/<ID>:v1.15.0-alpha.0 \
  -t controlplane \
  -o ansible/build/<cluster>/nodes/<node>/controlplane.yaml
talosctl validate -c ansible/build/<cluster>/nodes/<node>/controlplane.yaml -m metal
# 3. Or run the Ansible day-0 playbook (does all of the above idempotently).
```

Per-node `schematics.yml` is authoritative for that node's installer image;
cluster `schematics.yml` holds extensions shared by all nodes in the cluster.
Schematics list bare `siderolabs/<name>` entries — the factory pins versions
to the Talos release. Kernel args live in schematics (`extraKernelArgs`).

## Storage (homelab posture)

Disks are unencrypted — accepted for this homelab (physical access is trusted).
Real production must add `SystemDiskEncryption` (TPM2/KMS or an ESO-held key)
with documented key custody + recovery before storing non-replaceable data.

Node `UnattendedInstallConfig` `wipe: true` is bootstrap-only: sources keep it
for first boot, and automation forces `wipe: false` in `ansible/build/`
unless day-0 renders with `-e talos_bootstrap_fresh_install=true` (day-1
requires the same flag; day-2 always forces safe). Never edit the source per
install — see `ansible/RUNBOOK.md` §1.5.

## RPCNFSDCOUNT (single source)

Thread counts live in the node headers: dev 32 / prd 64 slots/threads, matching
`[nfsd] threads` in `nfs.conf` and `RPCNFSDCOUNT` in the `nfs-server`
`ExtensionServiceConfig` beside them.
