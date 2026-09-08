# talos/ansible — day-0/1/2 automation (idempotent, talosctl-only)

Day-0 renders + generates configs and installs Talos; day-1 bootstraps etcd /
fetches kubeconfig and applies node patches; day-2 is ongoing operate
(health, upgrade, config patch). All node contact is `talosctl` over the
Talos API — no SSH, no kubectl in this tree.

## Layout

```text
ansible/
  ansible.cfg                  # roles_path, no cows, diff output
  requirements.yml             # collections (community.general for key-value lookups)
  inventory.example            # COMMITTED example (bdo-r01-cp-001 + dev node)
  inventory/                   # GITIGNORED real inventory (copy from the example)
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
- `talos_cluster` — active cluster name (override with `-e talos_cluster=...`).
- `talos_clusters.<name>` — per-cluster map: `vault` (Proton Pass vault),
  `endpoint` (VIP URL), `nodes: [{name, ip, role}]`.
- Secrets use `no_log: true` on every task that touches them and are only
  ever resolved through `pass-cli item view "pass://<vault>/talos/<field>"`
  or `pass-cli inject` on double-brace templates.

## Inventory

```bash
cp inventory.example inventory/bdo1.yml   # real inventory is gitignored
ansible-playbook playbooks/day0.yml -i inventory/bdo1.yml -e talos_cluster=acme-dev-bdo1-talos-apps-01 --check --diff
```

FQCN (`ansible.builtin.*`, `community.general.*`) is enforced (ansible-lint
`fqcn` rule). `false`/`true` are lowercase YAML booleans everywhere.
