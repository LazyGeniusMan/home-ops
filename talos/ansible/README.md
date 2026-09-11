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
  requirements.yml             # collections (community.general for key-value lookups)
  group_vars/all.yml           # talos version, per-cluster map, shared filenames
  playbooks/day0.yml           # render secrets/configs, gen + validate, install
  playbooks/day1.yml           # insecure-apply, wait gates, bootstrap etcd, kubeconfig
  playbooks/day2.yml           # health, upgrade, patch, VIP/etcd checks (operate)
  roles/talos_render/          # inject + gen config + validate (day-0)
  roles/talos_bootstrap/       # insecure-apply + bootstrap + kubeconfig (day-1)
  roles/talos_operate/         # health/upgrade/patch (day-2)
  build/                       # GITIGNORED rendered output
```

## Variables (group_vars/all.yml)

- `pass-cli login` session gate. Run `pass-cli login` before any play,
  because Ansible NEVER logs in. The PAT *value* used by the ESO webhook
  never flows through the shell: day-0 renders it via `pass-cli inject`
  from the cluster's own `talos/clusters/<cluster>/pat.yml.template`
  (double-brace ref to `pass://<own-vault>/eso-proton-pass/pat`) into
  `build/<cluster>/proton-pass-pat` (`0600`, `talos_pat_filename` shared
  var keeps render/store/apply in sync), and day-2 reads/renews it there.
  Every play probes the session via `pass-cli info -o json` (`rc==0` +
  JSON mapping stdout = logged in; logged-out gives `rc=1` + a non-JSON
  error) and fails fast telling you to run `pass-cli login`. Day-2
  checks unconditionally (even read-only runs); day-1 checks before
  `apply-config --insecure`.
  (manual commands can also
  `export PROTON_PASS_AGENT_REASON=talos-render-manual-exec-<16 hex>` for
  audit attribution). Ansible auto-generates a fresh unique
  `PROTON_PASS_AGENT_REASON` per `pass-cli` exec
  (`<prefix>-<cluster>[-<node>]-exec-<16 random lowercase hex>`,
  `no_log: true` keeps it out of logs).
- `talos_cluster` — active cluster name (override with `-e talos_cluster=...`).
- `talos_clusters.<name>` — per-cluster map: `vault` (Proton Pass vault),
  `endpoint` (VIP URL), `nodes: [{name, ip, role}]`. This map is the single
  source of truth for node IPs — they are data for `talosctl -n/-e` flags
  only, never Ansible connection targets. `role` is `controlplane` or
  `worker`; `talos_machine_roles` maps it to the `talosctl gen config -t`
  machine type of the same name.
- `talos_talosconfig` — explicit `--talosconfig` path
  (`build/<cluster>/talosconfig`) passed on every bootstrap/operate
  `talosctl` call instead of an ambient `TALOSCONFIG` or `~/.talos/config`.
  `talosconfig`/`kubeconfig` stay under `build/<cluster>/` (gitignored,
  existing convention) — they are NOT written into `talos/clusters/`.
- Secrets are only ever resolved through
  `pass-cli item view "pass://<vault>/talos/<field>"`
  or `pass-cli inject` on double-brace templates. The day-0 `pass-cli
  inject` / `talosctl gen secrets` tasks intentionally carry NO `no_log`:
  in `--out-file` mode neither tool prints secret values, so keeping output
  visible means failures name the unresolved `pass://` ref instead of
  showing "censored".

## Schematics (Image Factory upload + --install-image)

Day-0 resolves each node's installer schematic before `gen config` by
deep-merging all three layers (base → cluster → node, recursive
`combine` with dedup list-union — node holds node-only entries, never
copies of inherited ones):

1. `talos/clusters/_base/schematics.yml` (shared; vanilla on its own)
2. `talos/clusters/<cluster>/schematics.yml` (env-wide, e.g. netbird)
3. `talos/clusters/<cluster>/nodes/<node>/schematics.yml` (node-only,
   e.g. qemu-guest-agent / intel-ucode + kernel args / nfsd stack)

For each node the role (`roles/talos_render/tasks/schematic.yml`) slurps
all three levels, merges them, then stages a reference copy at
`build/<cluster>/schematics-<node>.yml`, uploads it via
`POST https://factory.talos.dev/schematics` (JSON `.id`, raw-ID fallback),
and persists `schematic-<node>.id` + `schematic-<node>.sha256`. Merged
bytes change → new factory ID on the next day-0 (hash-gated). Upload is
idempotent: skipped when the schematic hash is unchanged (re-upload only on
content change). A hash-match with a missing/empty `.id`, or an upload that
yields no parseable ID, fails fast naming the `rm` + re-run day-0 recovery
instead of silently rendering `pending-schematic-upload`. The rendered node
patch copies under
`build/<cluster>/nodes-<node>-patches.yml` get their
`PLACEHOLDER_SCHEMATIC_ID` rewritten to the resolved per-node ID, so each
node's `UnattendedInstallConfig.installer.image` is correct.

Each node's `talosctl gen config` receives
`--install-image factory.talos.dev/metal-installer/<that-node-ID>:<talos_version>`
(`talos_version` already carries the leading `v`, e.g. `v1.14.0`, so the
ref has exactly one `v`).
Schematic IDs are not secrets but task output is kept tidy.

## Machine configs (per-node, role-based)

Day-0 runs ONE `gen config` per node (plus one `-t talosconfig` run per
cluster), so each node gets only its own patches, scoped to its role:

- output: `build/<cluster>/nodes/<node>/<type>.yaml` where `<type>` is the
  `talos_machine_roles` mapping of the node's `role`
  (`controlplane` -> `controlplane.yaml`, `worker` -> `worker.yaml`).
- invocation: `-t <type> -o build/<cluster>/nodes/<node>/<type>.yaml` with
  base + cluster patches via generic `--config-patch` and ONLY that node's
  patch via `--config-patch-control-plane` (role `controlplane`) or
  `--config-patch-worker` (role `worker`).
- `talosconfig` is generated once per cluster into
  `build/<cluster>/talosconfig` (base + cluster patches, carries the
  cluster endpoint) and reused via `talos_talosconfig`.
- each generated node file is validated
  (`talosctl validate -c <node file> -m metal`).
- day-1 `apply-config` is role-aware: node `<name>` gets
  `build/<cluster>/nodes/<name>/<type>.yaml` for its own role.

`build/` outputs per cluster (all gitignored via `talos/.gitignore`
`ansible/build/`): `secrets.bundle.yml`, `talosconfig`, `kubeconfig`,
`proton-pass-pat` (day-0 `pass-cli inject` from the cluster
`pat.yml.template`; see the backup/save guide in `RUNBOOK.md` §6),
`patches.yml`, `nodes-<node>-patches.yml`, `nodes/<node>/*.yaml`,
`schematics-<node>.yml`, `schematic-<node>.id`,
`schematic-<node>.sha256`.

Each node also runs an NFS server stack: the node schematic layer adds
`siderolabs/nfsd` + `nfs-utils` + `nfs-server`, configured by
`EtcFileConfig` `exports` (existing `nvme-data` (+ `sata-data` on dev —
prd has no `sata-data`: the host has no SATA disk, so the unsatisfiable
selector was removed) volumes, LAN-only `192.168.1.0/24`, `root_squash`,
`fsid=0` pseudo-root on `nvme-data`) + `netconfig` — no dedicated volume
(see `RUNBOOK.md` §1.6).

## Inventory (local-only — no node inventory)

Every playbook runs on `hosts: localhost` with `connection: local` (plus
`transport = local` in `ansible.cfg`); all node contact is `talosctl` over
the Talos API — no SSH (`kubectl --kubeconfig build/<cluster>/kubeconfig`
only for the day-2 ESO PAT Secret plane). Run with an inline localhost
inventory (`-i localhost,`); no inventory files are needed:
`group_vars/all.yml` (`talos_clusters`) remains the single IP source.

```bash
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=acme-dev-bdo1-talos-apps-01 --check --diff
```

FQCN (`ansible.builtin.*`, `community.general.*`) is enforced (ansible-lint
`fqcn` rule). `false`/`true` are lowercase YAML booleans everywhere.
