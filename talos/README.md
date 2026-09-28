# talos/ — Talos Linux machine configs (`talosctl`-only)

Declarative machine configuration for all Talos clusters. No CNI, no CoreDNS,
no bootstrap manifests, no telemetry/SideroLink/discovery — Cilium, DNS, and
workloads arrive later via GitOps. Nodes never auto-join anything (discovery
is deleted in `_base/patches.yml`). Verify: `grep -rni
'telemetry\|siderolink\|TelemetryConfig' talos/clusters/` returns nothing.

Pins live in `ansible/group_vars/all.yml` (Talos version) and
`ansible/requirements.yml` (collections) — never in prose.

## Layout

```text
talos/
  README.md                          # this file (convention + layout)
  .gitignore                         # build/, secrets bundles, kubeconfigs
  clusters/
    _base/
      patches.yml                    # shared barebone patch (applied first)
      schematics.yml                 # vanilla Image Factory schematic
    <cluster-name>/                   # e.g. acme-prd-bdo1-talos-apps-01
      patches.yml                    # cluster multi-doc patch (split-doc kinds)
      schematics.yml                 # cluster-shared schematic layer
      pat.yml.template               # ESO PAT render source (committed)
      secrets.yml.template           # manual `talosctl gen secrets` notes (committed, field-free)
      secrets.yml                    # GITIGNORED manual output (automation uses build/<cluster>/secrets.bundle.yml instead)
      nodes/<node-name>/
        patches.yml                  # node multi-doc: hostname, network, install, volumes
        schematics.yml               # AUTHORITATIVE schematic for this node's installer image
  ansible/                           # day-0/1/2 automation (see ansible/README.md)
```

`patches.yml` files are multi-document YAML with explicit `apiVersion`/`kind`
per document; the base ships zero manifests. Per-node `schematics.yml` is
authoritative for that node's installer image; cluster `schematics.yml` holds
extensions shared by all nodes. Schematics list bare `siderolabs/<name>`
entries (the factory pins versions to the release); kernel args live in
schematics (`extraKernelArgs`).

## Secrets (no plaintext, ever)

- Committed files contain only double-brace Proton Pass refs, e.g.
  `{{ pass://acme-prd-bdo1-talos-apps-01/talos/docker-password }}` — resolved
  only by `pass-cli inject` / `pass-cli item view`. Gate: `pass-cli info`.
- Day-0 renders `ansible/build/<cluster>/` (`secrets.bundle.yml`,
  `talosconfig`, `proton-pass-pat`); day-1 adds `kubeconfig` — all gitignored.
  Binary is `pass-cli` (not `proton-pass-cli`).

## Workflow (talosctl only — no kubectl here)

The day-0 playbook does all of this idempotently (`ansible/RUNBOOK.md` §1);
the manual equivalent:

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
  --config-patch-control-plane @ansible/build/<cluster>/nodes-<node>-patches.yml \
  --install-image factory.talos.dev/metal-installer/<ID>:<talos_version> \
  -t controlplane \
  -o ansible/build/<cluster>/nodes/<node>/controlplane.yaml
talosctl validate -c ansible/build/<cluster>/nodes/<node>/controlplane.yaml -m metal
# 3. Or run the Ansible day-0 playbook (does all of the above idempotently).
```

## Storage + NFS posture (node headers are the single source)

- Disks unencrypted (accepted homelab posture).
- Node `wipe: true` is bootstrap-only (automation forces `wipe: false` in
  `build/` unless day-0 renders with `-e talos_bootstrap_fresh_install=true`).
  Never edit the source per install — see `ansible/RUNBOOK.md` §1.5.
- NFS thread counts live in the node headers (dev 32 / prd 64); exports are
  LAN-only with VLAN-scoped access.
