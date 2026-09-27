# talos/ansible — day-0/1/2 automation (idempotent, `talosctl`-only)

Day-0 renders machine configs + talosconfig + ESO PAT; day-1 applies configs,
bootstraps etcd, fetches kubeconfig; day-2 operates (health, upgrade,
re-apply, PAT apply/renew). All node contact is `talosctl` — no SSH.
`kubectl` is used only by the day-2 PAT Secret plane.

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
  playbooks/day2.yml           # health + etcd checks, upgrade, regen, re-apply, PAT plane (operate)
  roles/talos_render/          # inject + gen config + validate (day-0)
  roles/talos_bootstrap/       # insecure-apply + bootstrap + kubeconfig (day-1)
  roles/talos_operate/         # health/upgrade/patch (day-2)
  build/                       # GITIGNORED rendered output
```

## Variables (group_vars/all.yml)

- `talos_cluster` + `talos_clusters.<name>` (`vault`, `endpoint`,
  `nodes: [{name, ip, role}]`) — node IPs feed `talosctl -n/-e` flags only;
  `talos_machine_roles` maps `role` to the `gen config -t` machine type.
- `talos_talosconfig` — explicit `--talosconfig` on every call (never ambient
  `TALOSCONFIG`); `talosconfig` / `kubeconfig` stay under `build/<cluster>/`.
- Pins live only in `group_vars/all.yml` + `requirements.yml` — never in prose.
- Auth + secrets (procedures: `RUNBOOK.md` §0, §1.0b, §2.1b): `pass-cli login`
  session gate (every play probes via `pass-cli info -o json`); ESO PAT
  renders to `build/<cluster>/proton-pass-pat`; NetBird PAT resolves into
  `NB_PAT` env only, and the Terraform-minted setup key rewrites
  `__TALOS_NETBIRD_SETUP_KEY__`.

## Schematics + machine configs

Day-0 deep-merges three layers per node (base → cluster → node), stages
`build/<cluster>/schematics-<node>.yml`, uploads it to the factory URL from
group_vars, persists `schematic-<node>.id` + `.sha256`, and rewrites
`PLACEHOLDER_SCHEMATIC_ID` to the per-node ID. `gen config` receives
`--install-image factory.talos.dev/metal-installer/<that-node-ID>:<talos_version>`.

One `gen config` per node (plus one `-t talosconfig` per cluster): base +
cluster patches via `--config-patch`, only that node's patch via
`--config-patch-control-plane` / `--config-patch-worker`; output
`build/<cluster>/nodes/<node>/<type>.yaml`, validated
(`talosctl validate -c <node file> -m metal`). Day-1 `apply-config` is
role-aware.

`build/` outputs per cluster are all gitignored — see the canonical table
(`RUNBOOK.md` §5.2); backup rules in §6. The staged `netbird-tf/` dir keeps
persistent plaintext local state so re-applies upsert — backed up encrypted
with the PKI bundle (§6.1); never committed or deleted between runs (see
`RUNBOOK.md` §1.0b). NFS stack (node schematic + exports + netconfig +
nfs-server service) is per node — see `RUNBOOK.md` §1.6.

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
