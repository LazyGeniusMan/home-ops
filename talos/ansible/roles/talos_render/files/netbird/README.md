# talos netbird root — dedicated Talos access fabric (Ansible-managed)

Dedicated Terraform root for the Talos NetBird access fabric. Managed ONLY
by Ansible (`roles/talos_render/tasks/netbird_setup_key.yml` via the
`community.general.terraform` module, PAT from Proton Pass as `NB_PAT` env);
never touched by Flux. The Flux netbird infra root is proxy-only and MUST
NOT be referenced or imported here — no `service_lb_ip` var, no Service-LB
resource, no custom-domain / Cloudflare / reverse-proxy service in this
root (the LB VIP is a proxy-only concern owned by the Flux consumer).

Per cluster, the role stages a working copy of this dir at
`build/<cluster>/netbird-tf/` (gitignored via `ansible/build/`) and keeps
its local `terraform.tfstate` there ACROSS runs, so re-applies are true
upserts driven by `var.cluster_name` (`acme-dev-bdo1-talos-apps-01` /
`acme-prd-bdo1-talos-apps-01`).

Managed fabric (greenfield root — upsert only, never delete):

- groups `admin-users`, `guest-users`, `<cluster>-nodes`, plus resource
  groups `admin-users-resources` / `guest-users-resources`
- network `<cluster-name>` + `netbird_network_router` routing peer for that
  network to the `<cluster>-nodes` group (`peer_groups`; the
  `netbird_route` resource is intentionally unused — see
  `network_router.md` vs `route.md`)
- setup key: `netbird_setup_key.talos`, `type = reusable`,
  `expiry_seconds = 0` (never expires), `usage_limit = 0` (unlimited),
  `auto_groups = [<cluster>-nodes]` — plaintext ONLY via the sensitive
  `talos_setup_key` output (Ansible rewrites
  `__TALOS_NETBIRD_SETUP_KEY__` in the rendered `build/<cluster>/patches.yml`,
  `0600`, `no_log` throughout)
- `network_resource` `LAN CIDR` (`192.168.1.0/24`) →
  `admin-users-resources`
- policies: `admin-users-access` (admin-users → all groups, all protocols)
  split into TWO policies because the provider schema allows exactly ONE
  rule per policy AND forbids `destinations` + `destination_resource` in
  one rule (both mutually exclusive): `admin-users-access` (peer chain) +
  `admin-users-lan-access` (forward chain to the LAN resource); plus
  `guest-users-access` (guest-users → `guest-users-resources`, TCP 80+443,
  peer chain only — no guest `network_resource` exists in this root, so a
  guest-facing resource's forward chain rides a separate
  `destination_resource` rule, never merged into this rule)

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

Provider schema entrypoints (fetch via `scripts/fetch-references.sh` into
`/tmp/home-ops-docs/netbird-terraform-provider-docs/`): `index.md` (auth
`NB_PAT`), `group.md`, `network.md`, `setup_key.md`,
`network_router.md` (+ `route.md` for the unused resource),
`network_resource.md`, `policy.md`.
