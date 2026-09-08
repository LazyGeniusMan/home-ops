# talos/ — Talos Linux cluster configs (Talos v1.14, `talosctl`-only)

Declarative machine configuration for all Talos clusters. No CNI, no CoreDNS,
and no bootstrap manifests ship from this tree — Cilium, DNS, and workloads
arrive later via GitOps. Telemetry: Talos v1.14 has no telemetry field (zero
hits in `config.schema.json`); cluster discovery is explicitly disabled in
`_base/patches.yml` so nodes never auto-join anything.

## Layout convention

```text
talos/
  README.md                          # this file (convention + workflow)
  .gitignore                         # build/, secrets bundles, kubeconfigs
  clusters/
    _base/
      patches.yml                    # shared barebone patch (CNI none, CoreDNS off, discovery off)
      kube-proxy-override.yml        # opt-in patch: disable kube-proxy (Cilium clusters)
      schematics.yml                 # vanilla Image Factory schematic (no extensions)
    <cluster-name>/                   # e.g. acme-prd-bdo1-talos-apps-01
      patches.yml                    # multi-doc: v1alpha1 SMP + split-doc kinds
      schematics.yml                 # cluster-shared schematic (e.g. netbird)
      secrets.yml.template           # committed; render to secrets.yml via pass-cli inject
      secrets.yml                    # GITIGNORED: `talosctl gen secrets` output + rendered creds
      nodes/<node-name>/
        patches.yml                  # node multi-doc: hostname, LinkConfig, install, volumes
        schematics.yml               # AUTHORITATIVE schematic for this node's installer image
  ansible/                           # day-0/1/2 automation (see ansible/README.md)
```

`patches.yml` files are multi-document YAML (supported per
`reference/configuration/overview.mdx`: "a multi-document configuration that
may contain multiple YAML documents, separated by `---`"). Document 1 is a
`v1alpha1` strategic-merge fragment; subsequent documents are split-doc kinds
(`Layer2VIPConfig`, `UserVolumeConfig`, `RegistryAuthConfig`, `HostnameConfig`,
`LinkConfig`, `TimeSyncConfig`, `ResolverConfig`, `KubeNodeConfig`,
`UnattendedInstallConfig`) validated against
`/tmp/home-ops-docs/talos-docs/public/talos/v1.14/schemas/config.schema.json`.
There are intentionally no `KubeInlineManifestConfig` / `KubeExternalManifestConfig`
documents — the base ships zero manifests.

## Secrets (no plaintext, ever)

- Committed files contain only double-brace Proton Pass references, e.g.
  `{{ pass://acme-prd-bdo1-talos-apps-01/talos/docker-password }}`.
  Only `pass-cli inject` / `pass-cli item view` resolve them — bare `pass://`
  URIs are never dereferenced by Talos or Ansible directly.
- Render: `pass-cli inject --in-file patches.yml --out-file build/patches.yml`
- Authenticate: `export PROTON_PASS_PERSONAL_ACCESS_TOKEN=pst_...` (`pass-cli login`).
- `secrets.yml`, `talosconfig`, `kubeconfig`, rendered `build/` output are gitignored.
- Binary is `pass-cli` (not `proton-pass-cli`).

## Workflow (talosctl only — no kubectl here)

```bash
# 1. Upload node schematic, note the schematic ID
curl -X POST --data-binary @clusters/<cluster>/nodes/<node>/schematics.yml \
  https://factory.talos.dev/schematics
# 2. Put factory.talos.dev/metal-installer/<ID>:v1.14.0 in the node's
#    UnattendedInstallConfig installer.image, then generate:
talosctl gen secrets -o clusters/<cluster>/secrets.yml
talosctl gen config <cluster> https://<VIP>:6443 \
  --with-secrets clusters/<cluster>/secrets.yml \
  --config-patch @clusters/_base/patches.yml \
  --config-patch @clusters/<cluster>/patches.yml \
  --config-patch @clusters/<cluster>/nodes/<node>/patches.yml \
  -o /tmp/<cluster>
talosctl validate -c /tmp/<cluster>/controlplane.yaml -m metal
# 3. Or run the Ansible day-0 playbook (does all of the above idempotently).
```

Per-node `schematics.yml` is authoritative for that node's installer image;
cluster `schematics.yml` holds extensions shared by all nodes in the cluster.
Schematics list bare `siderolabs/<name>` entries — the factory pins versions
to the Talos release. Kernel args live in schematics (`extraKernelArgs`);
`machine.kernel.args` does not exist in v1alpha1.
