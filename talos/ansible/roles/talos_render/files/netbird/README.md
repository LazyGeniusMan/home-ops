# talos netbird root — dedicated Talos access fabric (Ansible-managed)

Dedicated Terraform root for the Talos NetBird access fabric. Managed ONLY by Ansible
(`roles/talos_render/tasks/netbird_setup_key.yml` via `community.general.terraform`,
PAT as `NB_PAT` env); never Flux. No `service_lb_ip` var, no Service-LB resources
(LB VIP is owned by the Flux consumer). Fabric shape lives in `main.tf`
(groups, network, router, policies), vars in `variables.tf`, key + audit outputs
in `outputs.tf` — read them there, not here.

Per cluster, staged at `build/<cluster>/netbird-tf/` (gitignored) with local
`terraform.tfstate` kept ACROSS runs, so re-applies are true upserts driven by
`var.cluster_name`.

- Setup key (`netbird_setup_key.talos` in `main.tf`): reusable but scoped (90d,
  `usage_limit = 3`, `auto_groups = [<cluster>-nodes`) — plaintext ONLY via the
  sensitive `talos_setup_key` output (Ansible rewrites `__TALOS_NETBIRD_SETUP_KEY__`,
  `0600`, `no_log`); audit via `setup_key_expires` / `setup_key_used_times` /
  `setup_key_last_used`. No `prevent_destroy` on the key — replacement is the
  sanctioned rotation path; every other resource keeps it (fail closed, no
  `tofu destroy` path).
- State: `terraform.tfstate` is plaintext LOCAL state — it holds the setup-key
  secret, so it shares the `secrets.bundle.yml` backup class (RUNBOOK §6.1:
  include the whole `netbird-tf/` dir in the encrypted off-machine backup;
  restore before re-running day-0). No remote backend on purpose (single-writer).
  Never commit, copy, or `tofu output -raw` it. Day-0 asserts it stays `0600`.
- Single-writer: both clusters share ONE NetBird account, so the account-global
  groups/policies are owned by exactly one cluster root at a time (whichever
  applied first). A second cluster's first apply plans CREATEs for those names
  and fails on duplicates — import the globals into its own state first (IDs
  from the first cluster's `tofu show -json` or the NetBird console), then apply;
  only the per-cluster network/router/setup-key stay owned per cluster.

State recovery: do NOT delete `build/<cluster>/netbird-tf/` between runs. After
`rm -rf build/<cluster>` WITHOUT a backup, re-anchor with one `tofu import` per
resource (same order as `main.tf`), then re-run day-0 (with a backup, restore
`netbird-tf/` instead — no imports needed):

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
router, setup key, LAN resource), then apply.

Rotation: RUNBOOK §1.0b is canonical (rotate/revoke procedure + join monitoring).
