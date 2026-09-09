# talos/ansible — day-0/1/2 automation (idempotent, talosctl-only)

Day-0 renders per-node machine configs + cluster talosconfig; day-1 applies
each node's own config, bootstraps etcd, and fetches kubeconfig; day-2 is
ongoing operate (health, upgrade, config patch). All node contact is
`talosctl` over the Talos API — no SSH, no kubectl in this tree.

> Operator? Start with [`RUNBOOK.md`](RUNBOOK.md) — step-by-step Day 0/1/2
> commands, verification, troubleshooting, and reference tables. This README
> is the concept index; the runbook is the procedure.

## Layout

```text
ansible/
  ansible.cfg                  # roles_path, transport=local, diff output
  requirements.yml             # collections (community.general for key-value lookups)
  inventory.example            # COMMITTED localhost-only stub (nodes live in group_vars, not inventory)
  group_vars/all.yml           # PAT env passthrough, talos version, per-cluster map
  group_vars/vault.template.yml# double-brace pass:// refs (render via pass-cli inject)
  playbooks/day0.yml           # render secrets/configs, gen + validate, install
  playbooks/day1.yml           # bootstrap etcd, kubeconfig, apply node patches
  playbooks/day2.yml           # health, upgrade, patch, VIP/etcd checks (operate)
  roles/talos_render/          # inject + gen config + validate (day-0)
  roles/talos_bootstrap/       # bootstrap + kubeconfig + apply (day-1)
  roles/talos_operate/         # health/upgrade/patch (day-2)
  build/                       # GITIGNORED rendered output
```

## Variables (group_vars/all.yml)

- `proton_pass_pat_env: PROTON_PASS_PERSONAL_ACCESS_TOKEN` — pass-cli PAT
  arrives via env only, never in files. Authenticate:
  `export PROTON_PASS_PERSONAL_ACCESS_TOKEN=pst_... ; pass-cli login`
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
  `worker`; `talos_machine_roles` maps it to the Talos machine type of the
  same name (`controlplane` — inventory spelling — maps to `controlplane`).
- `talos_talosconfig` — explicit `--talosconfig` path
  (`build/<cluster>/talosconfig`) passed on every bootstrap/operate
  `talosctl` call instead of an ambient `TALOSCONFIG` or `~/.talos/config`.
  `talosconfig`/`kubeconfig` stay under `build/<cluster>/` (gitignored,
  existing convention) — they are NOT written into `talos/clusters/`.
- Secrets use `no_log: true` on every task that touches them and are only
  ever resolved through `pass-cli item view "pass://<vault>/talos/<field>"`
  or `pass-cli inject` on double-brace templates.

## Schematics (Image Factory upload + --install-image)

Day-0 resolves each node's installer schematic before `gen config` with
fallback order (first existing file wins; node is AUTHORITATIVE):

1. `talos/clusters/<cluster>/nodes/<node>/schematics.yml`
2. `talos/clusters/<cluster>/schematics.yml`
3. `talos/clusters/_base/schematics.yml` (vanilla `customization: {}`)

For each node the role (`roles/talos_render/tasks/schematic.yml`) stages a
reference copy at `build/<cluster>/schematics-<node>.yml`, uploads it via
`POST https://factory.talos.dev/schematics` (JSON `.id`, raw-ID fallback),
and persists `schematic-<node>.id` + `schematic-<node>.sha256`. Upload is
idempotent: skipped when the schematic hash is unchanged (re-upload only on
content change). The rendered node patch copies under
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
`patches.yml`, `nodes-<node>-patches.yml`, `nodes/<node>/*.yaml`,
`schematics-<node>.yml`, `schematic-<node>.id`,
`schematic-<node>.sha256`.

## Inventory (local-only — no node inventory)

Every playbook runs on `hosts: localhost` with `connection: local` (plus
`transport = local` in `ansible.cfg`); all node contact is `talosctl` over
the Talos API — no SSH. There are no node entries in any inventory:
`inventory.example` is an unused localhost-only stub, and
`group_vars/all.yml` (`talos_clusters`) remains the single IP source.

```bash
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=acme-dev-bdo1-talos-apps-01 --check --diff
```

FQCN (`ansible.builtin.*`, `community.general.*`) is enforced (ansible-lint
`fqcn` rule). `false`/`true` are lowercase YAML booleans everywhere.
