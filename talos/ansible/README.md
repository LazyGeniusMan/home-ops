# talos/ansible — day-0/1/2 automation (idempotent, talosctl + kubectl)

Day-0 renders per-node machine configs + cluster talosconfig, and renders
the ESO webhook PAT from the vault (`build/<cluster>/proton-pass-pat`);
day-1 applies each node's own config, bootstraps etcd, and fetches
kubeconfig; day-2 is ongoing operate (health, upgrade, config re-apply,
ESO PAT Secret apply/renew).
All node contact is `talosctl` over the Talos API — no SSH. `kubectl` is
used only by the day-2 PAT Secret plane (post-Flux apply + webhook restart).

> Operator? Start with [`RUNBOOK.md`](RUNBOOK.md) — step-by-step Day 0/1/2
> commands, verification, troubleshooting, and reference tables. This README
> is the concept index; the runbook is the procedure.

## Layout

```text
ansible/
  ansible.cfg                  # roles_path, transport=local, diff output
  requirements.yml             # collections (community.general for the terraform module + random_string lookup)
  group_vars/all.yml           # talos version, per-cluster map, shared filenames
  playbooks/day0.yml           # render secrets/configs, gen + validate machine configs
  playbooks/day1.yml           # insecure-apply, wait gates, bootstrap etcd, kubeconfig
  playbooks/day2.yml           # health, upgrade, patch, VIP/etcd checks (operate)
  roles/talos_render/          # inject + gen config + validate (day-0)
  roles/talos_bootstrap/       # insecure-apply + bootstrap + kubeconfig (day-1)
  roles/talos_operate/         # health/upgrade/patch (day-2)
  build/                       # GITIGNORED rendered output
```

## Variables (group_vars/all.yml)

- `talos_cluster` (default `acme-dev-bdo1-talos-apps-01`) + `talos_clusters.<name>`
  (`vault`, `endpoint`, `nodes: [{name, ip, role}]`) — node IPs only feed
  `talosctl -n/-e` flags; `talos_machine_roles` maps `role` to the
  `gen config -t` machine type.
- `talos_talosconfig` — explicit `--talosconfig` path on every call (never ambient
  `TALOSCONFIG`); `talosconfig` / `kubeconfig` stay under `build/<cluster>/`.
- Auth + secrets (procedures: `RUNBOOK.md` §0.3, §1.0b, §2.1b): `pass-cli login`
  session gate (Ansible never logs in; every play probes via `pass-cli info -o json`);
  ESO PAT renders via `pass-cli inject` to `build/<cluster>/proton-pass-pat`
  (`0600`, pinned by `talos_pat_filename`); NetBird PAT resolves via
  `pass-cli item view` into `NB_PAT` env only, and the Terraform-minted setup key
  rewrites `__TALOS_NETBIRD_SETUP_KEY__`. Inject tasks carry no `no_log` in
  `--out-file` mode so failures name the unresolved `pass://` ref.

## Schematics (Image Factory upload + --install-image)

Day-0 deep-merges three layers per node (base → cluster → node, recursive `combine`
with dedup list-union; node files hold node-only entries):

1. `talos/clusters/_base/schematics.yml` (shared; vanilla)
2. `talos/clusters/<cluster>/schematics.yml` (env-wide, e.g. netbird)
3. `talos/clusters/<cluster>/nodes/<node>/schematics.yml` (node-only)

The role stages `build/<cluster>/schematics-<node>.yml`, uploads it via
`POST https://factory.talos.dev/schematics`, and persists
`schematic-<node>.id` + `.sha256` (upload only on content change; missing/empty `.id`
or unparseable upload fails fast naming the `rm` + re-run day-0 recovery).
`PLACEHOLDER_SCHEMATIC_ID` in `nodes-<node>-patches.yml` is rewritten to the per-node
ID. `gen config` receives
`--install-image factory.talos.dev/metal-installer/<that-node-ID>:<talos_version>`
(`talos_version` carries the leading `v`, e.g. `v1.15.0-alpha.0`).

## Machine configs (per-node, role-based)

One `gen config` per node (plus one `-t talosconfig` per cluster), each node scoped to
its role:

- output: `build/<cluster>/nodes/<node>/<type>.yaml` (`talos_machine_roles`: role →
  `controlplane.yaml` / `worker.yaml`).
- invocation: base + cluster patches via `--config-patch`, only that node's patch via
  `--config-patch-control-plane` / `--config-patch-worker`.
- `talosconfig`: once per cluster (`build/<cluster>/talosconfig`, base + cluster
  patches, cluster endpoint), reused via `talos_talosconfig`.
- every node file validated (`talosctl validate -c <node file> -m metal`); day-1
  `apply-config` is role-aware.

`build/` outputs per cluster (all gitignored) — see the canonical table
(`RUNBOOK.md` §5.2); backup rules in §6. The staged `netbird-tf/` dir keeps
persistent state so re-applies upsert — never backed up, committed, or deleted
between runs (see `RUNBOOK.md` §1.0b).

NFS server stack per node: `siderolabs/nfsd` + `nfs-utils` + `nfs-server` in the node
schematic, `EtcFileConfig` `exports` (three LAN-only `192.168.1.0/24` `all_squash`
lines, `fsid=0/1/2`) + `netconfig` + `ExtensionServiceConfig` `nfs-server`
(`RPCNFSDCOUNT=32`); no dedicated volume (`RUNBOOK.md` §1.6).

## Inventory (local-only — no node inventory)

Every playbook runs on `hosts: localhost` with `connection: local` (plus
`transport = local` in `ansible.cfg`); all node contact is `talosctl` — no SSH
(`kubectl --kubeconfig build/<cluster>/kubeconfig` only for the day-2 ESO PAT plane).
Inline localhost inventory (`-i localhost,`); `talos_clusters` is the single IP source.

```bash
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=<cluster> --check --diff
```

FQCN (`ansible.builtin.*`, `community.general.*`) is enforced (ansible-lint `fqcn`
rule). `false`/`true` are lowercase YAML booleans everywhere.
