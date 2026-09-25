# talos/ansible — Day 0/1/2 runbook

Operator procedure for the day-0/1/2 automation (concepts: `README.md`).
**Working directory is `talos/ansible/`** unless stated otherwise. Every play runs on
`hosts: localhost` (`connection: local`) — all node contact is `talosctl` over the
Talos API, no SSH. Default cluster is `acme-dev-bdo1-talos-apps-01`;
`<cluster>` below is a real cluster name from the Reference table.

---

## 0. Prereqs (do once per shell)

### 0.1 Enter the flox environment (repo root)

The flox hook provides `talosctl` v1.15.0-alpha.0.

```bash
# Working dir: repo root (where .flox/ lives)
flox activate
talosctl version --client   # expect v1.15.0-alpha.0
cd talos/ansible            # all playbook commands run from here
```

### 0.2 Install Ansible collections

```bash
# Working dir: talos/ansible/
ansible-galaxy install -r requirements.yml
```

Provides `community.general >= 9.0.0` (§1.0b `terraform` module + `random_string`
lookup). Re-run after a fresh checkout or when `requirements.yml` changes.

### 0.3 Authenticate to Proton Pass

Ansible NEVER logs in — run `pass-cli login` in your shell before any play. Every play
probes the session via `pass-cli info -o json` (`rc==0` + JSON mapping = logged in)
and fails fast telling you to log in.

```bash
# Working dir: anywhere
export PROTON_PASS_AGENT_REASON=talos-manual-exec-$(openssl rand -hex 8)
pass-cli login
```

Ansible generates its own per-exec `PROTON_PASS_AGENT_REASON`
(`<prefix>-<cluster>[-<node>]-exec-<16 hex>`); the export above is only for manual
commands. A missed login fails day-0 with `No authenticated pass-cli session...`.

- A stale session surfaces as `pass-cli inject` failures — re-login, re-run.
- Inject errors name the unresolved ref (`pass://<vault>/talos/<field>` + field
  message): fix the field name (case-sensitive) or the source `pass://` ref, delete
  the stale `build/<cluster>/` render, re-run day-0. Inject tasks carry no `no_log`
  (`--out-file` mode prints no values, only the ref).

### 0.4 Network + node state

- LAN `192.168.1.0/24` reachability to every node IP and the cluster VIP (see Reference).
- `https://factory.talos.dev:443` reachable (day-0 schematic upload; failure aborts day-0).
- `https://api.netbird.io:443` reachable (day-0 NetBird plane, §1.0b).
- Target nodes booted into **Talos maintenance** (uninstalled media, API up, no config).
  Day-1 `--insecure` apply only works in this state. Pre-flight:

```bash
# Working dir: talos/ansible/ (--insecure needs no config)
talosctl get links --insecure -n 192.168.1.201
talosctl get disks --insecure -n 192.168.1.201
```

Confirm the link name matches the node patch (`ens18` dev/QEMU, `enp45s0` prd/bare metal)
and the disk layout matches (dev: `/dev/sda` 20 GiB OS + `/dev/sdb` 960 GB data;
prd: `/dev/nvme0n1` 512 GiB NVMe).

---

## 1. Day 0 — render (`playbooks/day0.yml`)

Generates everything under `build/<cluster>/` (gitignored): injected patches,
per-node schematics + factory IDs, secrets bundle, `talosconfig`, per-node machine
configs. Validates each with `talosctl validate -c <node file> -m metal`.
Changes nothing on the nodes.

Per cluster vault: a `talos` item with `netbird-pat` (NetBird management PAT, §1.0b)
**plus** an `eso-proton-pass` item with `pat` (referenced as
`pass://<cluster-vault>/eso-proton-pass/pat` in
`talos/clusters/<cluster>/pat.yml.template`). A missing item/field fails rendering
naming the unresolved ref.

Role order (`talos_render`): `mkdir build/<cluster> 0700` → login check →
`pass-cli inject` cluster + per-node patches → `pass-cli inject` PAT render
(`pat.yml.template` → `proton-pass-pat`, `0600`, `creates:` guard; path pinned by
`talos_pat_filename`) → NetBird plane (§1.0b) → schematic merge/upload/ID-rewrite →
`gen secrets` → `mkdir nodes/<node>` → `gen config -t talosconfig` →
per-node `gen config -t <role>` with `--install-image` → `validate -m metal`.

### 1.0b NetBird setup key (PAT-driven Terraform, never vault-seeded)

The rendered cluster patch carries `NB_SETUP_KEY=<reusable key>` for the netbird
system extension, minted by Terraform:

1. **Proton Pass supplies the PAT only** — `talos` item, `netbird-pat` field, own
   cluster vault (`pass://<cluster-vault>/talos/netbird-pat`). Resolved via
   `pass-cli item view` (`no_log`, never on disk), passed only as `NB_PAT` env on the
   `community.general.terraform` call (never `-var`).
2. **Terraform mints the setup key** — module (`state: present`, `force_init: true`,
   only `cluster_name` var) applies the dedicated root
   (`roles/talos_render/files/netbird/`, Ansible-managed, never Flux) staged at
   `build/<cluster>/netbird-tf/` (gitignored; local state kept ACROSS runs so
   re-applies upsert; `prevent_destroy` on every resource). Never delete that dir
   between runs (fresh dir = empty state = duplicate-CREATE failures; recovery is
   `tofu import` per resource — see `roles/talos_render/files/netbird/README.md`).
   Fabric: groups `admin-users` / `guest-users` / `<cluster>-nodes` /
   `admin-users-resources` / `guest-users-resources`, network `<cluster-name>`,
   `netbird_network_router` peer routing to the nodes group, `LAN CIDR`
   `192.168.1.0/24` resource → `admin-users-resources`, policies
   `admin-users-access` (peer chain) + `admin-users-lan-access` (LAN forward chain;
   one rule per policy) + `guest-users-access` (TCP 80+443). Key:
   `type = reusable`, `expiry_seconds = 0`, `usage_limit = 0`,
   `auto_groups = [<cluster>-nodes]`; returned as sensitive `talos_setup_key`
   output (never `tofu output -raw`).
3. **Ansible rewrites the placeholder** — `NB_SETUP_KEY=__TALOS_NETBIRD_SETUP_KEY__`
   → resolved key (`ansible.builtin.replace`, `no_log`, `0600`), UUID shape-gated
   (non-match fails naming the PAT source, never the value), baked into
   `build/<cluster>/nodes/*/*.yaml` by `gen config`.

No raw `tofu apply` shell on this plane. Day-2 re-apply (`-e reapply_configs=true`)
reuses it via `include_role` (forced re-render restores the placeholder first).
Provider schema entrypoints: `index.md` (auth `NB_PAT`), `group.md`, `network.md`,
`setup_key.md`, `network_router.md`, `network_resource.md`, `policy.md`
(`scripts/fetch-references.sh` → `/tmp/home-ops-docs/netbird-terraform-provider-docs/`).

Rotation: replace `netbird_setup_key.talos`, then re-run day-0
(`rm build/<c>/patches.yml` first) or day-2 `-e reapply_configs=true`.

### 1.1 Dry run (recommended first)

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=<cluster> --check --diff
```

> Check-mode placeholder: the Image Factory upload is skipped in `--check` mode, so
> `gen config` renders `--install-image
> factory.talos.dev/metal-installer/pending-schematic-upload:v1.15.0-alpha.0`.
> Check mode proves plumbing, **not** an installable config — always follow with a real run.

### 1.2 Real run

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=<cluster>
```

Expected: `failed=0`; `validate` tasks report `ok`.

### 1.3 What it produces

Per cluster, under `build/<cluster>/` (see Reference for the full file
table):

- `patches.yml` — cluster patch with secrets injected via `pass-cli inject`
  **plus** the NetBird placeholder rewritten from the Terraform
  `talos_setup_key` output (§1.0b; `NB_SETUP_KEY=<reusable key>`, `0600`).
- `nodes-<node>-patches.yml` — per-node patch, injected, then rewritten so
  `PLACEHOLDER_SCHEMATIC_ID` becomes the real factory ID for that node.
- `proton-pass-pat` (`0600`) — ESO webhook PAT, rendered via
  `pass-cli inject` from `talos/clusters/<cluster>/pat.yml.template`
  (double-brace `pass://<own-vault>/eso-proton-pass/pat`). Guarded by
  `creates:` — re-renders only when the file is missing.
- `schematics-<node>.yml` — staged deep-merged schematic (`_base` → cluster → node,
  recursive `combine` with dedup list-union; node files hold node-only entries).
  Changed bytes → new factory ID next day-0.
- `schematic-<node>.id` + `.sha256` — factory ID + content hash; upload
  (`POST https://factory.talos.dev/schematics`) runs only on content change.
- `secrets.bundle.yml` (`0600`) — `talosctl gen secrets` output, created **once**
  (see Re-run/reset).
- `talosconfig` — `gen config -t talosconfig` (base + cluster patches, cluster endpoint).
- `nodes/<node>/<type>.yaml` — one machine config per node, `-t <type>` per
  `talos_machine_roles`; base + cluster patches via `--config-patch`, only that
  node's patch via `--config-patch-control-plane` / `--config-patch-worker`;
  `--install-image factory.talos.dev/metal-installer/<that-node-ID>:v1.15.0-alpha.0`.
  Validated (`talosctl validate -c <file> -m metal`).

### 1.4 How to verify outputs

```bash
# Working dir: talos/ansible/
C=<cluster>   # e.g. acme-dev-bdo1-talos-apps-01
ls -l build/$C build/$C/nodes/*/                                # all files present, 0600/0700 modes
cat build/$C/schematic-*.id                                     # 64-hex factory ID per node (no "pending-schematic-upload")
grep -r PLACEHOLDER_SCHEMATIC_ID build/$C && echo STALE || echo "IDs rewritten OK"
grep -o 'factory.talos.dev/metal-installer/[0-9a-f]*' build/$C/nodes-*-patches.yml build/$C/nodes/*/*.yaml | sort -u
talosctl validate -c build/$C/nodes/<node>/controlplane.yaml -m metal
```

### 1.6 NFS server stack (per node)

Each node runs NFS server daemons for LAN clients (ports `2049`, `20048`, `111`).
Three pieces, all in the node sources:

- **Schematic** (node `schematics.yml`): `siderolabs/nfsd` + `nfs-utils` +
  `nfs-server` together (changed bytes → new factory ID next day-0).
- **Config** (node `patches.yml`): `EtcFileConfig` `exports` (three LAN-only
  `192.168.1.0/24` `all_squash` lines, `fsid=0/1/2`) + `EtcFileConfig` `netconfig`
  + `ExtensionServiceConfig` `nfs-server` (`RPCNFSDCOUNT=32`).
- **Backing store**: none dedicated — exports resolve against the `nvme-data` +
  `sata-data` `UserVolumeConfig` volumes (`<name>` mounts at `/var/mnt/<name>`).

Readiness (after day-1 install):

```bash
# Working dir: talos/ansible/ (authenticated API after install)
C=<cluster>; N=<node-ip>
talosctl --talosconfig build/$C/talosconfig -n $N read /proc/fs/nfsd/threads   # nonzero = running
talosctl --talosconfig build/$C/talosconfig -n $N list /var/mnt/nvme-data      # export path resolves
```

### 1.7 Re-run / reset

Day-0 `creates:` guards skip inject/secrets steps when renders exist; `gen config`
always regenerates + revalidates. An mtime preflight fails fast naming the `rm`
below when a source is newer than its render (vault rotations are invisible to it —
still delete + re-run after a rotation). Force re-render after editing a source:

```bash
# Working dir: talos/ansible/
# Re-render injected patches after editing source patches.yml or rotating vault values:
C=<cluster>
rm build/$C/patches.yml build/$C/nodes-*-patches.yml
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=$C
```

```bash
# Working dir: talos/ansible/
# Full day-0 reset for one cluster:
C=<cluster>
rm -rf build/$C
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=$C
```

> Deleting `secrets.bundle.yml` mints a **new** cluster PKI bundle. Safe before first
> install; after day-1 it orphans the installed cluster. Never reset the bundle on a
> live cluster.

---

## 2. Day 1 — bootstrap (`playbooks/day1.yml`)

Installs machine configs onto maintenance-booted nodes, bootstraps etcd on the first
node, fetches the admin kubeconfig. Run **after a successful day-0 real run** for the
same cluster.

Role order (`talos_bootstrap`): session check → `apply-config --insecure` per node →
TCP 50000 wait per node → authenticated `talosctl version` poll on **every** node →
control-plane assert on `nodes[0]` → `bootstrap` on `nodes[0]` (retried) →
`kubeconfig` fetch (retried) → markers. The PAT value comes from the day-0 render
(`proton-pass-pat`); day-1 only gates on the login session. Pre-Flux (no
`external-secrets` namespace yet): the PAT stays in `build/<cluster>/`, never seeded
back into the vault it unlocks.

### 2.1 Command

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day1.yml -i localhost, -e talos_cluster=<cluster>
# Apply-mode override (default `auto`):
ansible-playbook playbooks/day1.yml -i localhost, -e talos_cluster=<cluster> -e talos_bootstrap_mode=no-reboot
```

### 2.1b PAT source (day-0 render) + one-time interactive `agent create`

Day-1 probes the `pass-cli` session FIRST (fail-fast login gate, same §0.3 login as
day-0). The PAT value is the day-0 render (`build/<cluster>/proton-pass-pat`, `0600`)
from `pat.yml.template` (`eso-proton-pass`/`pat` in the own vault).

If no token exists yet, mint one **interactively** first (one-time; `pass-cli` cannot
create agent tokens inside a PAT/agent session):

```bash
export PROTON_PASS_AGENT_REASON=talos-bootstrap-manual-exec-$(openssl rand -hex 8)
pass-cli agent create home-ops-eso --expiration 1y --vault <cluster>
# Save the printed PROTON_PASS_PERSONAL_ACCESS_TOKEN=pst_... token, then:
export PROTON_PASS_PERSONAL_ACCESS_TOKEN=pst_...
pass-cli login
```

```bash
pass-cli agent access grant home-ops-eso --vault-name <cluster> --role viewer   # if needed
pass-cli agent delete home-ops-eso                                              # when retiring
```

Expiration enum: `1h, 1d, 1w, 1m, 3m, 6m, 1y` (default `1y`).

### 2.2 What "insecure-first-boot" means

Per node:

```text
talosctl apply-config --talosconfig build/<cluster>/talosconfig \
  --insecure -n <node-ip> \
  -f build/<cluster>/nodes/<node>/<type>.yaml --mode <talos_bootstrap_mode>
```

`--insecure` talks to the maintenance API without PKI auth — only while the node is
uninstalled / in maintenance mode. After apply the node installs, reboots, and needs
authenticated access; re-running insecure apply against an installed node fails (skipped
via marker anyway).

Day-1 performs no post-bootstrap secure apply — insecure-apply → bootstrap →
kubeconfig only. After a patch change on an installed cluster, re-apply with
authenticated `talosctl apply-config` (§3.6 / §4.4).

> `nodes[0]` pinning: bootstrap, kubeconfig fetch, and day-2 health `--init-node` /
> etcd queries all target `talos_clusters[talos_cluster].nodes[0]`. Each cluster is a
> single control-plane node, so `nodes[0]` is the bootstrap node. Day-1 asserts
> `nodes[0]` is control-plane; the `talosctl version` poll covers **every** node.

### 2.3 Marker files + kubeconfig

`creates:` guards make day-1 safe to re-run; each step skips once its marker exists:

| Step | Marker / output | Meaning |
| --- | --- | --- |
| Per-node apply | `build/<cluster>/.installed-<node>` | Config given to that node; re-apply skipped. |
| Etcd bootstrap | `build/<cluster>/.bootstrapped` | `talosctl bootstrap` ran on `nodes[0]`; never re-bootstraps. |
| Kubeconfig | `build/<cluster>/kubeconfig` | Admin kubeconfig (`talosctl kubeconfig -n/-e nodes[0] -f`). |

### 2.4 Verify

```bash
# Working dir: talos/ansible/
C=<cluster>; N=<node-ip>   # dev: acme-dev-bdo1-talos-apps-01 / 192.168.1.201; prd: acme-prd-bdo1-talos-apps-01 / 192.168.1.101
ls -l build/$C/.installed-* build/$C/.bootstrapped build/$C/kubeconfig
talosctl --talosconfig build/$C/talosconfig -n $N -e $N health
kubectl --kubeconfig build/$C/kubeconfig get nodes -o wide
```

Expect the node `Ready`, VIP endpoint serving the API.

---

## 3. Day 2 — operate (`playbooks/day2.yml`)

Default (no flags) is read-only health/etcd, but still checks the `pass-cli`
session first (re-apply/renew need it). Flags opt into regen, re-apply, upgrades,
and the ESO PAT Secret plane. Order: auth probe → regen → re-apply → Talos upgrade
→ Kubernetes upgrade → PAT apply/renew (§3.7). Talos always precedes k8s.

### 3.1 Read-only (safe anytime)

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day2.yml -i localhost, -e talos_cluster=<cluster>
```

Runs `talosctl health` (control-plane nodes + `--init-node nodes[0]`) and
`talosctl etcd members` (against `nodes[0]`), both `failed_when: false` — report only,
never fail. No changes without a §3.2 flag. Health runs on **every** invocation;
skip with `-e skip_health=true`.

### 3.2 Flags (opt-in upgrades, regen, re-apply)

| Flag | Effect | Example |
| --- | --- | --- |
| `upgrade_image` | Explicit installer per node; wins when both image flags are set | `-e upgrade_image=factory.talos.dev/metal-installer/<id>:v1.15.0` |
| `upgrade_talos_version` | Auto-builds installer per node from `build/<cluster>/schematic-<node>.id` (slurp, never re-uploads) | `-e upgrade_talos_version=v1.15.0` |
| `kubernetes_version` | `--dry-run` plan first, then `upgrade-k8s --to`; runs only on drift vs rendered kubelet image | `-e kubernetes_version=1.38.0` |
| `regen_talosconfig` | Rebuilds `talosconfig` from existing secrets bundle (never mints new PKI) | `-e regen_talosconfig=true` |
| `regen_kubeconfig` | Re-fetches admin kubeconfig via `talosctl kubeconfig -f` | `-e regen_kubeconfig=true` |
| `reapply_configs` (+ `reapply_mode`, default `staged`) | Re-renders patches (bypasses day-0 `creates:` guards), regenerates machine configs, `apply-config --mode <reapply_mode>` | `-e reapply_configs=true` |
| `pat_apply` | Applies the stored PAT to the ESO Secret (post-Flux only; skips + explains when the `external-secrets` ns is missing) | `-e pat_apply=true` |
| `pat_renew` (needs `pat_apply=true`) | Attempts `pass-cli ... renew` first, refreshes the local store when a new `pst_` token parses; blocked PAT/agent sessions skip with the interactive command | `-e pat_apply=true -e pat_renew=true` |
| `pat_name` | PAT identity for renew (default `home-ops-eso`) | `-e pat_name=home-ops-eso` |
| `pat_expiration` | Expiration enum for renew (`1h,1d,1w,1m,3m,6m,1y`; default `1y`) | `-e pat_expiration=1y` |

### 3.3 Talos upgrade (per-node, sequential)

Adjacent minors only (e.g. v1.15.x → v1.16.x, never skip a minor).

```bash
# Working dir: talos/ansible/
C=<cluster>
cat build/$C/schematic-*.id   # 64-hex factory ID per node (never invent one)
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=$C \
  -e upgrade_image=factory.talos.dev/metal-installer/$(cat build/$C/schematic-<node>.id):v1.15.0
# Or auto-build the same ref per node from the .id files:
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=$C -e upgrade_talos_version=v1.15.0
```

Nodes upgrade **one at a time, in cluster-map order**. Verify after:

```bash
# Working dir: talos/ansible/
C=<cluster>; N=<node-ip>
talosctl --talosconfig build/$C/talosconfig -n $N -e $N version
kubectl --kubeconfig build/$C/kubeconfig get nodes -o wide
```

### 3.4 Kubernetes upgrade (drift-only)

Compares `-e kubernetes_version=<ver>` (no leading `v`) against the kubelet image in
the rendered machine config; skips on match. Always prints the `--dry-run` plan
before `upgrade-k8s --to`. Targets `nodes[0]`. A missing/unparseable rendered config
warns (re-run day-0) instead of silently skipping.

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=<cluster> -e kubernetes_version=1.38.0
```

> `upgrade-k8s` mutates the live cluster without re-rendering `build/`. Re-run day-0
> afterwards so rendered configs match.

### 3.5 Regen client configs

Rebuilds from the **existing** secrets bundle — never mints new PKI.

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=<cluster> -e regen_talosconfig=true -e regen_kubeconfig=true
```

### 3.6 Re-apply machine configs (installed cluster)

Day-0 re-render does **not** push to nodes; day-1 insecure apply is maintenance-only.
This is the installed-cluster path: forced re-render of edited patches (needs the PAT,
like day-0) + forced PAT re-render (vault rotations flow) + NetBird rewrite (§1.0b) +
live schematic refresh (changed schematics re-upload) + regen + authenticated
`apply-config --mode <reapply_mode>` (default `staged`; one of `auto`, `no-reboot`,
`staged`, `try`). Asserts each node's `.id` resolves (re-run day-0 if missing).

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=<cluster> -e reapply_configs=true
# Mode override:
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=<cluster> -e reapply_configs=true -e reapply_mode=no-reboot
```

### 3.7 ESO PAT Secret apply + renew (post-Flux)

Applies the day-0 rendered PAT (`build/<cluster>/proton-pass-pat`) to the
`external-secrets/proton-pass-pat` Secret (key `pat`), then restarts
`deploy/eso-proton-pass` **only when the Secret changed** (webhook reads
`PROTON_PASS_PAT_FILE=/secrets/pat` at startup). Post-Flux only — pre-Flux skips
with a note. PAT-bearing tasks are `no_log`.

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=<cluster> -e pat_apply=true
# Renew + apply (mints a replacement PAT):
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=<cluster> -e pat_apply=true -e pat_renew=true
# Custom identity / expiration (defaults home-ops-eso / 1y):
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=<cluster> -e pat_apply=true -e pat_renew=true \
  -e pat_name=home-ops-eso -e pat_expiration=1y
```

> `pass-cli` cannot create/renew agent tokens inside a PAT/agent session. The renew
> step is `failed_when: false`: on that error it skips and prints the interactive
> command — run it in a user-authenticated shell, export the new `pst_` token, then
> re-run with `-e pat_apply=true`:
>
> ```bash
> export PROTON_PASS_AGENT_REASON=talos-operate-manual-exec-$(openssl rand -hex 8)
> pass-cli agent renew home-ops-eso --expiration 1y --output json
> ```
>
> `rc==0` refreshes the local store; unparseable output keeps the stored PAT. Renew
> never fails the play.

> Renew before expiry (max 1y) or every `ExternalSecret` flips `Ready=False` and the
> webhook answers `401`. A stale kubeconfig looks similar — refresh with
> `-e regen_kubeconfig=true` before assuming expiry.

---

## 4. Troubleshooting

### 4.1 Image Factory unreachable / upload rejected

Symptom: day-0 fails at *"Image Factory upload failed for \<node\>"*.

```bash
curl -sS -X POST --data-binary @talos/clusters/_base/schematics.yml https://factory.talos.dev/schematics
```

- Network/DNS/TLS issue → fix egress, re-run day-0 (already-uploaded nodes skip via
  hash guard).
- Schematic rejected → check extension names are bare `siderolabs/<name>` entries.
- Check-mode runs never upload — a real run is required.

### 4.2 PAT expired / `pass-cli` failures

Symptom: day-0 login check fails, or `pass-cli inject` errors mid-render.

```bash
export PROTON_PASS_PERSONAL_ACCESS_TOKEN=pst_...
pass-cli login
# Working dir: talos/ansible/
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=<cluster>
```

Partial renders are reused on retry (`creates:` skips); after a secrets change, delete
the stale render first (next table).

### 4.3 `creates:`-skip staleness — when to delete what

A `creates:` guard never re-runs while its file exists, even if the **source** changed.
Day-0 fails fast with an mtime preflight when a source is newer than its render, naming
the `rm` below (vault rotations stay invisible — still delete + re-run after one).

| Guard file (`build/<cluster>/...`) | Goes stale when… | Recovery |
| --- | --- | --- |
| `patches.yml` | source cluster patch, vault values, or setup key rotates | `rm build/<c>/patches.yml`, re-run day-0 (or day-2 `-e reapply_configs=true`). NetBird plane has NO `creates:` guard — re-applies every run (needs vault PAT + `api.netbird.io`); never `rm -rf build/<c>/netbird-tf` alone (recovery is `tofu import` per resource). |
| `nodes-<node>-patches.yml` | source node patch, vault values, or schematic changes | `rm build/<c>/nodes-*-patches.yml`, re-run day-0 |
| `proton-pass-pat` | vault `eso-proton-pass`/`pat` rotated | `rm build/<c>/proton-pass-pat`, re-run day-0 (or day-2 `-e reapply_configs=true`) |
| `schematic-<node>.id` / `.sha256` | re-uploads on content change | none needed. Hash-match with missing/empty `.id` fails fast naming the `rm` + re-run day-0 recovery |
| `secrets.bundle.yml` | **never** refreshes while present | `rm` + re-run day-0 only pre-install (rotation breaks live clusters) |
| `talosconfig`, `nodes/<n>/*.yaml` | fresh every run (no guard) | n/a |
| `.installed-<node>` | config re-rendered but node never re-applied | `rm build/<c>/.installed-<node>` + re-run day-1 (maintenance only) / manual secure apply if installed |
| `.bootstrapped` | never re-run by design | `rm` only to **re-bootstrap a fresh cluster**; never on a live one (would split-brain etcd) |
| `kubeconfig` | re-bootstrap / cert rotation | `rm build/<c>/kubeconfig`, re-run day-1 — or day-2 `-e regen_kubeconfig=true` (§3.5) |

### 4.4 Re-apply after a patch change (installed cluster)

Day-0 re-render does **not** push to nodes; day-1 apply is insecure-only (skipped once
installed). For an installed node:

```bash
# Working dir: talos/ansible/
C=<cluster>; N=<node-ip>; NODE=<node-name>
# 1. Re-render:
rm build/$C/patches.yml build/$C/nodes-*-patches.yml
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=$C
# 2. Push with authenticated API (manual, or automated via §3.6):
talosctl apply-config --talosconfig build/$C/talosconfig -n $N \
  -f build/$C/nodes/$NODE/controlplane.yaml --mode auto
```

Automated equivalent (default mode `staged`): `ansible-playbook playbooks/day2.yml
-i localhost, -e talos_cluster=$C -e reapply_configs=true` (§3.6).

### 4.5 Multi-node notes

- `nodes[0]` is the bootstrap node, kubeconfig source, health `--init-node`, and etcd
  query target. Keep the intended bootstrap node first in the list.
- Day-1 insecure apply loops over **all** nodes in list order.
- Day-2 upgrade loops over **all** nodes sequentially in list order. Watch each return
  `Ready` before the next proceeds (`kubectl --kubeconfig build/<c>/kubeconfig get nodes -w`).
- Both clusters are single control-plane; `role: worker` is handled (→ `worker.yaml`
  via `--config-patch-worker`, role-aware apply, upgrade loop covers all roles).

---

## 5. Reference

### 5.1 Clusters / nodes (from `group_vars/all.yml` — do not invent others)

| Cluster | Endpoint (VIP) | Node | IP | Role | Hardware / NIC / disks |
| --- | --- | --- | --- | --- | --- |
| `acme-prd-bdo1-talos-apps-01` | `https://192.168.1.198:6443` | `bdo-r01-cp-001` | `192.168.1.101` | `controlplane` | Bare metal (MSI Cubi 5); link `enp45s0` (RTL8125); install `/dev/nvme0n1` 512 GiB NVMe |
| `acme-dev-bdo1-talos-apps-01` | `https://192.168.1.248:6443` | `bdo-r01-cp-002` | `192.168.1.201` | `controlplane` | QEMU/KVM VM; link `ens18` (virtio); `/dev/sda` 20 GiB OS + `/dev/sdb` 960 GB data |

Per-node extensions (base → cluster → node): prd `intel-ucode, i915,
realtek-firmware` + `netbird` + nfsd stack; dev `qemu-guest-agent` + `netbird` +
nfsd stack. VIP advertises from the control-plane node (`Layer2VIPConfig`).

### 5.2 `build/<cluster>/` file table (all gitignored)

| Path | Produced by | Purpose |
| --- | --- | --- |
| `patches.yml` (`0600`) | day-0 `pass-cli inject` (cluster) + NetBird rewrite (§1.0b) | Cluster patch, secrets resolved, `__TALOS_NETBIRD_SETUP_KEY__` replaced with the Terraform key |
| `nodes-<node>-patches.yml` (`0600`) | day-0 `pass-cli inject` (node) + ID-rewrite | Per-node patch, schematic ID rewritten |
| `schematics-<node>.yml` (`0600`) | day-0 stage | Reference copy of resolved schematic |
| `schematic-<node>.id` (`0600`) | day-0 factory upload / reuse | 64-hex Image Factory schematic ID |
| `schematic-<node>.sha256` (`0600`) | day-0 hash persist | Upload-idempotency hash |
| `secrets.bundle.yml` (`0600`) | day-0 `gen secrets` (once) | Cluster PKI bundle — never commit |
| `talosconfig` | day-0 `gen config -t talosconfig` | Cluster admin Talos API config |
| `nodes/<node>/controlplane.yaml` | day-0 `gen config -t controlplane` | Machine config (control-plane nodes) |
| `nodes/<node>/worker.yaml` | day-0 `gen config -t worker` | Machine config (worker nodes; no worker nodes defined) |
| `kubeconfig` | day-1 `talosctl kubeconfig -f` | Admin kubeconfig |
| `proton-pass-pat` (`0600`) | day-0 `pass-cli inject` from `pat.yml.template` (`creates:`); day-2 re-render is forced on `reapply_configs` | Rendered PAT for the day-2 ESO Secret apply — never committed (see §6 for backup) |
| `.installed-<node>` (`0600`) | day-1 marker | Insecure apply done for that node |
| `.bootstrapped` (`0600`) | day-1 marker | Etcd bootstrap done (nodes[0]) |

### 5.3 Extra vars

| Var | Play | Default | Effect |
| --- | --- | --- | --- |
| `talos_cluster` | all | `acme-dev-bdo1-talos-apps-01` | Selects `talos_clusters[<name>]` (vault, endpoint, nodes). |
| `talos_bootstrap_mode` | day-1 | `auto` | `--mode` for insecure `apply-config` (`auto` / `no-reboot` / …). |
| `upgrade_image` | day-2 | `""` (no upgrade) | Maps to `talos_operate_upgrade_image`; when set, each node runs `talosctl upgrade -n <ip> -i <image>`. Example: `factory.talos.dev/metal-installer/<id-from-build-schematic-*.id>:v1.15.0-alpha.0`. |
| `upgrade_talos_version` | day-2 | `""` (no upgrade) | Auto-builds installer per node from `build/<cluster>/schematic-<node>.id`. Adjacent minors only. Example: `v1.15.0` (see §3.3). |
| `kubernetes_version` | day-2 | `1.37.0` (group default) | `--dry-run` plan then `upgrade-k8s --to`; drift-only. Example: `1.38.0` (see §3.4). |
| `regen_talosconfig` | day-2 | `false` | Rebuilds talosconfig from existing secrets bundle (see §3.5). |
| `regen_kubeconfig` | day-2 | `false` | Re-fetches admin kubeconfig (see §3.5). |
| `reapply_configs` (+ `reapply_mode`, default `staged`) | day-2 | `false` | Re-renders patches + pushes via `apply-config --mode` (see §3.6). |
| `skip_health` | day-2 | `false` | Skips the always-on `talosctl health` probe. |
| `pat_apply` | day-2 | `false` (stays read-only) | Applies stored PAT to `external-secrets/proton-pass-pat` + conditional webhook restart (see §3.7). |
| `pat_renew` | day-2 | `false` (needs `pat_apply=true`) | Attempts `pass-cli agent renew` first; blocked agent sessions skip with the interactive command (see §3.7). |
| `pat_name` | day-2 | `home-ops-eso` | PAT identity for renew. |
| `pat_expiration` | day-2 | `1y` | Expiration enum (`1h,1d,1w,1m,3m,6m,1y`) for the renew command. |
---

## 6. Backup / save — surviving a fresh clone

`build/` is gitignored, so a fresh clone starts EMPTY. Most re-renders, but the PKI
bundle cannot be re-created — back up off-machine (encrypted) after every day-0/day-1.

### 6.1 What to back up

| File (`build/<cluster>/...`) | Class | Why |
| --- | --- | --- |
| `secrets.bundle.yml` | **CRITICAL** | Cluster PKI root. A fresh bundle does NOT match an installed cluster (rotation orphans it). |
| `talosconfig` | Convenience | Rebuildable via `-e regen_talosconfig=true` (§3.5); keep a copy anyway. |
| `kubeconfig` | Convenience | Re-fetchable via `-e regen_kubeconfig=true` (§3.5); keep a copy anyway. |
| `proton-pass-pat` | Re-mintable | Re-renders via day-0 inject; keep a copy for offline `pat_apply`. |

Regenerable (no backup needed): rendered patches, staged schematics + `.id`/`.sha256`
(same bytes → same ID), node `*.yaml`, markers (`.installed-*`, `.bootstrapped` —
see §6.4 caveat).

```bash
# Working dir: talos/ansible/
C=<cluster>
tar -czf - build/$C/secrets.bundle.yml build/$C/talosconfig \
  build/$C/kubeconfig build/$C/proton-pass-pat | \
  gpg --symmetric --cipher-algo AES256 -o ~/talos-$C-backup.tgz.gpg
# Store OFF this machine. Verify: gpg -d ~/talos-$C-backup.tgz.gpg | tar -tz
```

### 6.2 Fresh-clone restore — day-1 (pre-install cluster)

Day-1 needs the day-0 machine configs + `talosconfig`:

| Need | Source |
| --- | --- |
| `build/<cluster>/talosconfig` | Restore from backup, or restore the bundle first then `-e regen_talosconfig=true` (§3.5) |
| `build/<cluster>/nodes/<node>/<type>.yaml` | Re-render: needs `secrets.bundle.yml` (restore from backup; pre-install a fresh bundle is fine) + repo sources + vault login + factory reachability |

```bash
# Working dir: talos/ansible/
C=<cluster>
gpg -d ~/talos-$C-backup.tgz.gpg | tar -xzf -   # restores bundle + talosconfig + kubeconfig + PAT
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=$C   # re-renders patches, schematics, node yamls
ansible-playbook playbooks/day1.yml -i localhost, -e talos_cluster=$C   # apply + bootstrap + kubeconfig
```

### 6.3 Fresh-clone restore — day-2 (installed cluster)

| Need | Source |
| --- | --- |
| `build/<cluster>/talosconfig` | Restore, or `-e regen_talosconfig=true` (never mints new PKI) |
| `build/<cluster>/kubeconfig` | Restore, or `-e regen_kubeconfig=true` |
| `build/<cluster>/schematic-<node>.id` | **Required** unless `-e upgrade_image=...`: day-2 reads the `.id` per node (never re-uploads). Re-creatable via day-0 re-run (needs factory); restore when offline. |
| `build/<cluster>/secrets.bundle.yml` | Restore. Then `-e reapply_configs=true` re-renders patches (reuses the bundle, never rotates). |
| Rendered `patches.yml` + `nodes-<node>-patches.yml` | Re-render via day-0 or day-2 `-e reapply_configs=true` (forced; needs vault login + NetBird PAT). |
| `build/<cluster>/nodes/<node>/*.yaml` | Re-render via day-0; diff against live config before re-apply. |
| `build/<cluster>/proton-pass-pat` | Re-render via day-0 inject, or restore — `pat_apply` only reads the file. |

```bash
# Working dir: talos/ansible/
C=<cluster>
gpg -d ~/talos-$C-backup.tgz.gpg | tar -xzf -   # restores bundle + talosconfig + kubeconfig + PAT
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=$C   # re-renders patches, .id files, node yamls
ansible-playbook playbooks/day2.yml -i localhost, -e talos_cluster=$C   # read-only health check first
# Then opt in: -e reapply_configs=true / -e pat_apply=true / upgrades (§3.2)
```

### 6.4 What NEVER to do

- **Never commit `build/`** — live PKI, admin configs, vault PAT. Gitignored by design.
- **Never re-bootstrap a live cluster** — `rm .bootstrapped` + re-run day-1 would
  `talosctl bootstrap` an already-bootstrapped etcd (split-brain).
- **Never delete `secrets.bundle.yml` on a live cluster** — fresh PKI does not match
  installed nodes (§1.7). Restore from backup instead.
- **Never `rm -rf build/<cluster>` on a live cluster without a backup** — re-render
  needs the SAME bundle (also wipes staged `netbird-tf/` state; re-anchor with
  `tofu import` per resource after restore — see `roles/talos_render/files/netbird/README.md`).
- **Never seed `proton-pass-pat` back into the vault** — it is the vault credential;
  it lives in `build/` and flows only to the day-2 ESO Secret apply.
