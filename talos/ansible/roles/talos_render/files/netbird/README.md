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
- setup key `netbird_setup_key.talos` (`type = reusable`, `expiry_seconds = 0`,
  `usage_limit = 0`, `auto_groups = [<cluster>-nodes]`) — plaintext ONLY via the
  sensitive `talos_setup_key` output (Ansible rewrites `__TALOS_NETBIRD_SETUP_KEY__`,
  `0600`, `no_log`)
- `network_resource` `LAN CIDR` (`192.168.1.0/24`) → `admin-users-resources`
- policies: `admin-users-access` (peer chain) + `admin-users-lan-access` (LAN forward
  chain; one rule per policy) + `guest-users-access` (TCP 80+443, peer chain)

State recovery: do NOT delete `build/<cluster>/netbird-tf/` between runs (fresh dir =
empty state = duplicate-CREATE failures). After `rm -rf build/<cluster>`, re-anchor
with one `tofu import` per resource (same order as `main.tf`), then re-run day-0:

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

Provider schema entrypoints (`index.md` auth `NB_PAT`, `group.md`,
`network.md`, `setup_key.md`, `network_router.md`, `network_resource.md`,
`policy.md`; `route.md` documents the unused legacy resource).
