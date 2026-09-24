# talos netbird root — dedicated Talos access fabric (Ansible-managed)

Dedicated Terraform root for the Talos NetBird access fabric. Managed ONLY
by Ansible (`roles/talos_render/tasks/netbird_setup_key.yml` via
`community.general.terraform`, PAT as `NB_PAT` env); never touched by Flux.
The Flux netbird root is proxy-only — no `service_lb_ip` var, no
Service-LB / custom-domain / reverse-proxy resources here (LB VIP is owned
by the Flux consumer).

Per cluster, the role stages a working copy of this dir at
`build/<cluster>/netbird-tf/` (gitignored via `ansible/build/`) and keeps
its local `terraform.tfstate` there ACROSS runs, so re-applies are true
upserts driven by `var.cluster_name` (`acme-dev-bdo1-talos-apps-01` /
`acme-prd-bdo1-talos-apps-01`).

Managed fabric (upsert only, never delete):

- groups `admin-users`, `guest-users`, `<cluster>-nodes`, plus resource
  groups `admin-users-resources` / `guest-users-resources`
- network `<cluster-name>` + `netbird_network_router` routing peer for that
  network to the `<cluster>-nodes` group (`peer_groups`; the
  `netbird_route` resource is unused — see
  `network_router.md`)
- setup key: `netbird_setup_key.talos`, `type = reusable`,
  `expiry_seconds = 0` (never expires), `usage_limit = 0` (unlimited),
  `auto_groups = [<cluster>-nodes]` — plaintext ONLY via the sensitive
  `talos_setup_key` output (Ansible rewrites
  `__TALOS_NETBIRD_SETUP_KEY__` in the rendered `build/<cluster>/patches.yml`,
  `0600`, `no_log` throughout)
- `network_resource` `LAN CIDR` (`192.168.1.0/24`) →
  `admin-users-resources`
- policies: `admin-users-access` (admin-users → all groups, peer chain) +
  `admin-users-lan-access` (forward chain to the LAN resource) — split in
  two because the provider allows one rule per policy and forbids
  `destinations` + `destination_resource` in one rule; plus
  `guest-users-access` (guest-users → `guest-users-resources`, TCP 80+443,
  peer chain only — a guest resource's forward chain rides a separate rule)

Every resource carries `lifecycle { prevent_destroy = true }`: any plan that
would delete or replace a resource fails closed instead of destroying it.
There is no `tofu destroy` path for this root.

State recovery: do NOT delete `build/<cluster>/netbird-tf/` between runs —
a fresh dir means empty state and the next apply would try to CREATE
duplicates and fail closed. After a `rm -rf build/<cluster>`, re-anchor
state with one `tofu import` per resource (same order as `main.tf`), then
re-run day-0 with the staged dir as `-chdir`:

```bash
# Working dir: talos/ansible/ (after day-0 staged build/<cluster>/netbird-tf/)
C=acme-dev-bdo1-talos-apps-01
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

Provider schema entrypoints (`index.md` auth `NB_PAT`, `group.md`,
`network.md`, `setup_key.md`, `network_router.md`, `network_resource.md`,
`policy.md`; `route.md` documents the unused legacy resource).
