# talos netbird root — dedicated Talos access fabric (Ansible-managed)

Dedicated Terraform root for the Talos NetBird access fabric. Managed ONLY by Ansible
(`roles/talos_render/tasks/netbird_setup_key.yml` via `community.general.terraform`,
PAT as `NB_PAT` env); never Flux. No `service_lb_ip` var, no Service-LB resources
(LB VIP is owned by the Flux consumer).

Per cluster, staged at `build/<cluster>/netbird-tf/` (gitignored) with local
`terraform.tfstate` kept ACROSS runs, so re-applies are true upserts driven by
`var.cluster_name`.

Managed fabric (upsert only, never delete; `lifecycle { prevent_destroy = true }` on
every resource — delete/replace plans fail closed; no `tofu destroy` path):

- groups `admin-users`, `guest-users`, `<cluster>-nodes`, plus resource groups
  `admin-users-resources` / `guest-users-resources`
- network `<cluster-name>` + `netbird_network_router` routing peer to the
  `<cluster>-nodes` group (`peer_groups`; `netbird_route` is unused)
- setup key `netbird_setup_key.talos` (`type = reusable`, `expiry_seconds =
  7776000` (90d), `usage_limit = 3`, `auto_groups = [<cluster>-nodes]`; scoped
  per the credential rule, never `0`/unlimited) — plaintext ONLY via the
  sensitive `talos_setup_key` output (Ansible rewrites `__TALOS_NETBIRD_SETUP_KEY__`,
  `0600`, `no_log`); audit via the `setup_key_expires` / `setup_key_used_times` /
  `setup_key_last_used` outputs
- `network_resource` `LAN CIDR` (`192.168.1.0/24`) → `admin-users-resources`
- policies: `admin-users-access` (peer chain) + `admin-users-lan-access` (LAN forward
  chain; one rule per policy) + `guest-users-access` (TCP 80+443, peer chain)

State: `terraform.tfstate` is plaintext LOCAL state under gitignored
`build/<cluster>/netbird-tf/` — it holds the setup-key secret, so it shares the
`secrets.bundle.yml` backup class (see RUNBOOK §6.1: include the whole
`netbird-tf/` dir in the encrypted off-machine backup; restore before re-running
day-0). There is no remote/encrypted backend on purpose — one local operator at
a time (single-writer). Never commit, copy, or `tofu output -raw` it.

Single-writer rule: both clusters share ONE NetBird account, so the
account-global groups (`admin-users`, `guest-users`, `admin-users-resources`,
`guest-users-resources`) and policies are owned by exactly one cluster root at
a time (whichever applied first). A second cluster's first apply plans CREATEs
for those same names and fails on duplicates. Give the second cluster the
globals by importing them into its own `build/<cluster>/netbird-tf/` state
(GlobalIDs from the first cluster's `tofu show -json`, or the NetBird admin
console), then apply — only the per-cluster network/router/setup-key stay
owned per cluster.

State recovery: do NOT delete `build/<cluster>/netbird-tf/` between runs (fresh dir =
empty state = duplicate-CREATE failures). After `rm -rf build/<cluster>` WITHOUT a
backup, re-anchor with one `tofu import` per resource (same order as `main.tf`),
then re-run day-0 (with a backup, restore `netbird-tf/` instead — no imports needed):

```bash
# Working dir: talos/ansible/ (after day-0 staged build/<cluster>/netbird-tf/)
C=<cluster>
tofu -chdir=build/$C/netbird-tf import 'netbird_group.admin_users' <id>
tofu -chdir=build/$C/netbird-tf import 'netbird_group.guest_users' <id>
tofu -chdir=build/$C/netbird-tf import 'netbird_group.cluster_nodes' <id>
tofu -chdir=build/$C/netbird-tf import 'netbird_group.admin_users_resources' <id>
tofu -chdir=build/$C/netbird-tf import 'netbird_group.guest_users_resources' <id>
tofu -chdir=build/$C/netbird-tf import 'netbird_network.cluster' <id>
tofu -chdir=build/$C/netbird-tf import 'netbird_setup_key.talos' <id>
tofu -chdir=build/$C/netbird-tf import 'netbird_network_router.cluster' <network_id>/<router_id>
tofu -chdir=build/$C/netbird-tf import 'netbird_network_resource.lan' <network_id>/<resource_id>
tofu -chdir=build/$C/netbird-tf import 'netbird_policy.admin_users_access' <id>
tofu -chdir=build/$C/netbird-tf import 'netbird_policy.admin_users_lan_access' <id>
tofu -chdir=build/$C/netbird-tf import 'netbird_policy.guest_users_access' <id>
```
For a SECOND cluster sharing the account, import ONLY the four account-global
groups + three policies (the lines above minus `cluster_nodes`, network,
router, setup key, LAN resource), then apply — its own per-cluster resources
CREATE fresh.

Rotation (90d scoped key — credential rule): the key expires on its own, so
rotate well before `setup_key_expires`:

```bash
# Working dir: talos/ansible/ (day-0 staged build/<cluster>/netbird-tf/)
C=<cluster>
tofu -chdir=build/$C/netbird-tf plan   # expect REPLACE on netbird_setup_key.talos only
tofu -chdir=build/$C/netbird-tf apply  # replace mints a NEW key value (re-key every peer)
rm build/$C/patches.yml build/$C/nodes-*-patches.yml     # force placeholder rewrite
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=$C   # re-renders with the new key
# Installed cluster instead: day-2 re-apply pushes the new key to nodes (§3.6).
# Revocation drill (lost key): NetBird admin console → Setup Keys → revoke
# (`revoked = true` in config also revokes but REPLACES the key — same re-key
# cost), then rotate as above; joined peers STAY connected, only new joins stop.
```

Watch `setup_key_used_times` / `setup_key_last_used` after every apply —
unexpected growth means an unplanned peer joined with the key.

Provider schema entrypoints (`index.md` auth `NB_PAT`, `group.md`,
`network.md`, `setup_key.md`, `network_router.md`, `network_resource.md`,
`policy.md`; `route.md` documents the unused legacy resource).
