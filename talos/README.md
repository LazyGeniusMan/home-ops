# talos/ — Talos Linux cluster configs (Talos v1.15.0-alpha.0, `talosctl`-only)

Declarative machine configuration for all Talos clusters. No CNI, no CoreDNS,
and no bootstrap manifests ship from this tree — Cilium, DNS, and workloads
arrive later via GitOps. Telemetry is off and cluster discovery is disabled in `_base/patches.yml`,
so nodes never auto-join anything.

## Layout convention

```text
talos/
  README.md                          # this file (convention + workflow)
  .gitignore                         # build/, secrets bundles, kubeconfigs
  clusters/
    _base/
      patches.yml                    # shared barebone patch (CNI none, CoreDNS off, discovery off, kube-proxy iptables)
      schematics.yml                 # vanilla Image Factory schematic (no extensions)
    <cluster-name>/                   # e.g. acme-prd-bdo1-talos-apps-01
      patches.yml                    # multi-doc: v1alpha1 SMP + split-doc kinds
      schematics.yml                 # cluster-shared schematic (e.g. netbird)
      secrets.yml.template           # committed; manual `talosctl gen secrets` notes (output never committed)
      secrets.yml                    # GITIGNORED manual output (day-0 automation writes ansible/build/<cluster>/secrets.bundle.yml)
      nodes/<node-name>/
        patches.yml                  # node multi-doc: hostname, LinkConfig, install, volumes
        schematics.yml               # AUTHORITATIVE schematic for this node's installer image
  ansible/                           # day-0/1/2 automation (see ansible/README.md)
```

`patches.yml` files are multi-document YAML: document 1 is a `v1alpha1`
strategic-merge fragment; subsequent documents are split-doc kinds
(`Layer2VIPConfig`, `UserVolumeConfig`, `RegistryAuthConfig`, `HostnameConfig`,
`LinkConfig`, `TimeSyncConfig`, `ResolverConfig`, `KubeNodeConfig`,
`UnattendedInstallConfig`). The base ships zero manifests (no
`KubeInlineManifestConfig` / `KubeExternalManifestConfig`).

## Secrets (no plaintext, ever)

- Committed files contain only double-brace Proton Pass references, e.g.
  `{{ pass://acme-prd-bdo1-talos-apps-01/talos/docker-password }}`.
  Only `pass-cli inject` / `pass-cli item view` resolve them — bare `pass://`
  URIs are never dereferenced by Talos or Ansible directly.
- Render: `pass-cli inject --in-file clusters/<cluster>/patches.yml --out-file ansible/build/<cluster>/patches.yml` (per-node: `clusters/<cluster>/nodes/<node>/patches.yml` → `ansible/build/<cluster>/nodes-<node>-patches.yml`), then the NetBird plane rewrites `NB_SETUP_KEY=__TALOS_NETBIRD_SETUP_KEY__` from the Terraform `talos_setup_key` output (see `ansible/RUNBOOK.md` §1.0b).
- NetBird PAT comes from Proton Pass (`pass://<cluster-vault>/talos/netbird-pat`, via `pass-cli item view` as `NB_PAT` env to the `community.general.terraform` module applying `ansible/roles/talos_render/files/netbird/` — Ansible-managed, never Flux); the reusable setup key is Terraform-minted (sensitive `talos_setup_key` output, `no_log`) and never lives in the vault.
- Authenticate: `export PROTON_PASS_PERSONAL_ACCESS_TOKEN=pst_...` (`pass-cli login`).
- `secrets.bundle.yml`, `talosconfig`, `kubeconfig`, rendered `ansible/build/` output are gitignored.
- Binary is `pass-cli` (not `proton-pass-cli`).

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
