# talos/ansible — Day 0/1/2 runbook

Operator procedure for the day-0/1/2 automation (concepts: `README.md`).
**Working directory is `talos/ansible/`** unless stated otherwise. Every play runs on
`hosts: localhost` (`connection: local`) — all node contact is `talosctl` over the
Talos API, no SSH. Default cluster is `acme-dev-bdo1-talos-apps-01`;
`<cluster>` below is a real cluster name from the Reference table.

---

## 0. Prereqs (do once per shell)

### 0.1 Enter the flox environment (repo root)

The flox hook provides `talosctl` (version tracks `talos_version` in
`group_vars/all.yml`).

```bash
# Working dir: repo root (where .flox/ lives)
flox activate
talosctl version --client   # must match group_vars talos_version
cd talos/ansible            # all playbook commands run from here
```

### 0.2 Install Ansible collections

```bash
# Working dir: talos/ansible/
ansible-galaxy install -r requirements.yml
```

Provides `community.general` (exact pin in `requirements.yml`: `terraform`
module + `random_string` lookup). Re-run after a fresh checkout or when
`requirements.yml` changes; bump the pin only after re-testing §1.0b.

### 0.3 Authenticate to Proton Pass

Ansible NEVER logs in — run `pass-cli login` in your shell before any play. Every play
probes the session via `pass-cli info -o json` (`rc==0` + JSON mapping = logged in)
and fails fast telling you to log in.

```bash
# Working dir: anywhere
export PROTON_PASS_AGENT_REASON=talos-manual-exec-$(openssl rand -hex 8)
pass-cli login
```

Ansible generates its own per-exec `PROTON_PASS_AGENT_REASON`. A missed login
fails day-0 with `No authenticated pass-cli session...`.

- A stale session surfaces as `pass-cli inject` failures — re-login, re-run.
- Inject errors name the unresolved ref — fix the field name (case-sensitive)
  or the source `pass://` ref, delete the stale `build/<cluster>/` render,
  re-run day-0.

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

Confirm the link name and disk layout match the node patch header (volume
math lives there).

---

## 1. Day 0 — render (`playbooks/day0.yml`)

Generates everything under `build/<cluster>/` (gitignored): injected patches,
per-node schematics + factory IDs, secrets bundle, `talosconfig`, per-node machine
configs. Validates each with `talosctl validate -c <node file> -m metal`.
Changes nothing on the nodes.

Per cluster vault: a `talos` item with `netbird-pat` (§1.0b) **plus** an
`eso-proton-pass` item with `pat` (see `clusters/<cluster>/pat.yml.template`).
Day-0 also auto-stores `secrets.bundle.yml` + `talosconfig` (+ `kubeconfig`
when present) back into the `talos` item `Secrets` section (§6.1).
A missing item/field fails rendering naming the unresolved ref.

Role order (`talos_render`): `mkdir` → login check → inject cluster + node
patches → inject PAT → NetBird plane (§1.0b) → schematic merge/upload/ID-rewrite
→ `gen secrets` → `gen config -t talosconfig` → Proton Pass auto-store (§6.1)
→ per-node `gen config` with `--install-image` → `validate -m metal`.

### 1.0b NetBird setup key (PAT-driven Terraform, never vault-seeded)

The rendered cluster patch carries `NB_SETUP_KEY=<reusable key>` for the netbird
system extension, minted by Terraform (fabric shape: `roles/talos_render/files/netbird/README.md`):

1. **Vault supplies the PAT only** — `talos` item, `netbird-pat` field. Resolved
   via `pass-cli item view`, passed only as `NB_PAT` env (the root declares no
   token variable, so the PAT never lands in TFVARS or state).
2. **Terraform mints the setup key** — staged at `build/<cluster>/netbird-tf/`
   with persistent plaintext state (backup class §6.1; day-0 asserts `0600`).
   Key is reusable but scoped (90d expiry, `usage_limit = 3`, auto-joined to
   `<cluster>-nodes`). Gate: `tofu plan` first — the only expected replace is
   the setup key itself (rotation REPLACES the value, so re-key peers
   afterwards). Never delete `netbird-tf/` between runs (§6.3); both clusters
   share one NetBird account (account-global groups/policies are single-writer —
   the second cluster imports them, see the root README).
3. **Ansible rewrites the placeholder** — `__TALOS_NETBIRD_SETUP_KEY__` →
   resolved key (UUID shape-gated), baked into the machine configs by
   `gen config`.

Rotation (the 90d key expires on its own, so rotate early):

```bash
# Working dir: talos/ansible/
C=<cluster>
tofu -chdir=build/$C/netbird-tf plan    # expect REPLACE on netbird_setup_key.talos only
tofu -chdir=build/$C/netbird-tf apply   # replace mints a NEW key value (re-key every peer afterwards)
rm build/$C/patches.yml build/$C/nodes-*-patches.yml
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=$C   # new key baked in
# Installed cluster: day-2 -e reapply_configs=true pushes the new key (§3.6).
```

Revocation drill (lost key): NetBird console → Setup Keys → revoke, then rotate
as above; joined peers STAY connected, only new joins stop. Watch
`setup_key_expires` before the 90d mark and `setup_key_used_times` /
`setup_key_last_used` for unexpected peer joins after every apply.

### 1.1 Dry run (recommended first)

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=<cluster> --check --diff
```

> Check-mode placeholder: the Image Factory upload is skipped in `--check` mode, so
> `gen config` renders a `pending-schematic-upload` installer ref.
> Check mode proves plumbing, **not** an installable config — always follow with a real run.

### 1.2 Real run

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=<cluster>
```

Expected: `failed=0`; `validate` tasks report `ok`.

### 1.3 What it produces

Per cluster, under `build/<cluster>/` — see the canonical file table (§5.2).
Install images resolve per node as
`--install-image factory.talos.dev/metal-installer/<that-node-ID>:<talos_version>`,
and every node file is validated (`talosctl validate -c <file> -m metal`).

### 1.4 How to verify outputs

```bash
# Working dir: talos/ansible/
C=<cluster>   # e.g. acme-dev-bdo1-talos-apps-01
ls -l build/$C build/$C/nodes/*/                                # all files present, 0600 files / 0700 dirs
cat build/$C/schematic-*.id                                     # 64-hex factory ID per node (no "pending-schematic-upload")
grep -r PLACEHOLDER_SCHEMATIC_ID build/$C && echo STALE || echo "IDs rewritten OK"
grep -o 'factory.talos.dev/metal-installer/[0-9a-f]*' build/$C/nodes-*-patches.yml build/$C/nodes/*/*.yaml | sort -u
talosctl validate -c build/$C/nodes/<node>/controlplane.yaml -m metal
```

### 1.5 Bootstrap-only wipe (explicit intent flag)

Source node patches keep `wipe: true` so a first boot clears a reused OS disk.
Automation makes it bootstrap-only — source is never edited per install:

- Day-0 default rewrites `wipe: true` → `wipe: false` in `build/` and asserts
  no render still carries `wipe: true` before `gen config`.
- Fresh node first render only: day-0 with
  `-e talos_bootstrap_fresh_install=true` keeps `wipe: true` in build/. Day-1
  **requires** the same flag (§2.1).
- Day-2 refresh/reapply ALWAYS forces `wipe: false` and fails closed on
  `wipe: true` (§3.6a, §3.6).

```bash
# Working dir: talos/ansible/ (fresh node only — brand-new install)
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=<cluster> -e talos_bootstrap_fresh_install=true
ansible-playbook playbooks/day1.yml -i localhost, -e talos_cluster=<cluster> -e talos_bootstrap_fresh_install=true
# Every later render omits the flag and defaults safe.
```

### 1.6 NFS server stack (per node)

Each node runs NFS server daemons for LAN clients (ports `2049`, `20048`, `111`).
Three pieces, all in the node sources:

- **Schematic** (node `schematics.yml`): `siderolabs/nfsd` + `nfs-utils` +
  `nfs-server` together.
- **Config** (node `patches.yml`): `exports` (two data-volume LAN-only lines —
  no bare `/var` root; the `/24` CIDR is accepted, access limited by the VLAN)
  + `netconfig` + `nfs-server` service (`RPCNFSDCOUNT`: node-header value —
  dev 32 / prd 64).
- **Backing store**: exports resolve against the `nvme-data` + `sata-data`
  volumes (`<name>` mounts at `/var/mnt/<name>`).

Readiness (after day-1 install):

```bash
# Working dir: talos/ansible/ (authenticated API after install)
C=<cluster>; N=<node-ip>
talosctl --talosconfig build/$C/talosconfig -n $N read /proc/fs/nfsd/threads   # nonzero = running
talosctl --talosconfig build/$C/talosconfig -n $N list /var/mnt/nvme-data      # export path resolves
```

### 1.7 Re-run / reset

Day-0 `creates:` guards skip inject/secrets steps when renders exist; `gen config`
always regenerates + revalidates. When a source changes, delete the stale render
and re-run — see the guard/recovery table (§4.3).

> Deleting `secrets.bundle.yml` mints a **new** cluster PKI bundle — safe before
> first install, orphans a live cluster (§6.3).

---

## 2. Day 1 — bootstrap (`playbooks/day1.yml`)

Installs machine configs onto maintenance-booted nodes, bootstraps etcd on the first
node, fetches the admin kubeconfig. Run **after a successful day-0 real run** for the
same cluster — day-0 must have rendered with the same fresh-install flag.

Role order (`talos_bootstrap`): session check → Proton Pass
pull-restore (§6.2) → intent + wipe asserts →
`apply-config --insecure` per node → TCP wait → `talosctl version` poll on
every node → control-plane assert on `nodes[0]` → `bootstrap` on `nodes[0]` →
`kubeconfig` fetch → markers.

### 2.1 Command

Day-1 is fresh-bootstrap only and fails closed without the flag (§1.5):

```bash
# Working dir: talos/ansible/ (fresh node only — same flag as day-0)
ansible-playbook playbooks/day1.yml -i localhost, -e talos_cluster=<cluster> -e talos_bootstrap_fresh_install=true
# Apply-mode override (default `auto`):
ansible-playbook playbooks/day1.yml -i localhost, -e talos_cluster=<cluster> -e talos_bootstrap_fresh_install=true -e talos_bootstrap_mode=no-reboot
```

### 2.1b PAT source (day-0 render) + one-time interactive `agent create`

Day-1 pulls the Pass-stored trio first (§6.2), then probes the `pass-cli`
session only for auth (§0.3 — it never reads the day-0
`proton-pass-pat` render; that render is day-2 input, §3.7).
If no token exists yet, mint one interactively (one-time):

```bash
pass-cli agent create home-ops-eso --expiration 1y --vault <cluster>
# Save the printed pst_... token, then:
export PROTON_PASS_PERSONAL_ACCESS_TOKEN=pst_...
pass-cli login
```

Expiration enum: `1h, 1d, 1w, 1m, 3m, 6m, 1y` (max `1y` — renew before expiry, §3.7).

### 2.2 What "insecure-first-boot" means

Per node:

```text
talosctl apply-config --talosconfig build/<cluster>/talosconfig \
  --insecure -n <node-ip> \
  -f build/<cluster>/nodes/<node>/<type>.yaml --mode <talos_bootstrap_mode>
```

`--insecure` talks to the maintenance API without PKI auth — only while the node is
uninstalled / in maintenance mode. Day-1 performs no post-bootstrap secure
apply. After a patch change on an installed cluster, re-apply authenticated
(§3.6 / §4.4).

> `nodes[0]` pinning: bootstrap, kubeconfig fetch, and day-2 health `--init-node` /
> etcd queries all target `nodes[0]` (the bootstrap node; each cluster is a
> single control-plane node today).

### 2.3 Marker files + kubeconfig

`creates:` guards make day-1 safe to re-run; each step skips once its marker exists:

| Step | Marker / output |
| --- | --- |
| Per-node apply | `build/<cluster>/.installed-<node>` |
| Etcd bootstrap | `build/<cluster>/.bootstrapped` (never re-bootstraps) |
| Kubeconfig | `build/<cluster>/kubeconfig` |

### 2.4 Verify

```bash
# Working dir: talos/ansible/
C=<cluster>; N=<node-ip>   # see §5.1 for cluster/node/IP mapping
ls -l build/$C/.installed-* build/$C/.bootstrapped build/$C/kubeconfig
talosctl --talosconfig build/$C/talosconfig -n $N -e $N health
kubectl --kubeconfig build/$C/kubeconfig get nodes -o wide   # expect Ready
```

---

## 3. Day 2 — operate (`playbooks/day2.yml`)

Default (no flags) is read-only health/etcd, but still checks the `pass-cli`
session first. Flags opt into regen, secrets refresh, pull-restore (§6.2),
re-apply, upgrades, and the ESO PAT Secret plane. Order: auth probe →
health gate → regen → pull-restore (`restore_secrets=true` only) → secrets
refresh → PAT apply → re-apply (`staged` default) → Talos upgrade → Kubernetes
upgrade (→ PAT renew with `pat_renew=true`, §3.7). Talos always precedes k8s.
Every mutating plane requires the health probe to return `rc==0` this run (or
explicit `-e skip_health=true`, break-glass). The group
`talos_kubernetes_pinned_version` is record-only — Kubernetes upgrades are
explicit `-e kubernetes_version=<ver>` opt-in; a pin bump alone never mutates
the live cluster. Re-apply diffs rendered configs against LIVE node state
(server-side), never build timestamps alone.

### 3.1 Read-only (safe anytime)

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day2.yml -i localhost, -e talos_cluster=<cluster>
```

Runs `talosctl health` + `talosctl etcd members` (report only, never fail). No
changes without a §3.2 flag. Health runs on **every** invocation; skip with
`-e skip_health=true` (break-glass only: it also bypasses the mutating-plane
health gate — use it solely to recover a cluster too sick to pass health,
then re-run without it).

### 3.2 Flags (opt-in upgrades, regen, re-apply, restore)

| Flag | Effect |
| --- | --- |
| `upgrade_image` | Explicit installer per node; wins when both image flags are set |
| `upgrade_talos_version` | Auto-builds installer per node from `build/<cluster>/schematic-<node>.id` |
| `kubernetes_version` | Opt-in only (never defaulted from the group pin): `--dry-run` plan first, then `upgrade-k8s --to`; drift-only |
| `regen_talosconfig` | Rebuilds `talosconfig` from existing secrets bundle (never mints new PKI) |
| `regen_kubeconfig` | Re-fetches admin kubeconfig via `talosctl kubeconfig -f` |
| `refresh_secrets` | Re-renders + regenerates without pushing (vault-rotation pickup; implied by `reapply_configs`) |
| `reapply_configs` (+ `reapply_mode`, default `staged`) | Runs the refresh render path, then pushes via `apply-config --mode` |
| `restore_secrets` | Pulls absent `secrets.bundle.yml`/`talosconfig`/`kubeconfig` from Proton Pass (§6.2); default `false` (day-2 read-only) |
| `pat_apply` | Applies the stored PAT to the ESO Secret (post-Flux only) |
| `pat_renew` (needs `pat_apply=true`) | Renews the agent token first, then applies; blocked sessions skip with the interactive command |
| `pat_name` / `pat_expiration` | Renew identity / expiration enum (defaults `home-ops-eso` / `1y`) |

### 3.3 Talos upgrade (per-node, sequential)

Adjacent minors only (never skip a minor) — enforced by the day-2 gate against
the group pin `talos_version`. Explicit `-e upgrade_image=<installer>` bypasses
the gate (a raw installer ref carries no version to check — the caller owns
the ref).

```bash
# Working dir: talos/ansible/
C=<cluster>; NODE=<node-name>
cat build/$C/schematic-*.id   # 64-hex factory ID per node (never invent one)
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=$C \
  -e upgrade_image=factory.talos.dev/metal-installer/$(cat build/$C/schematic-$NODE.id):<talos_version>
# Or auto-build the same ref per node from the .id files:
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=$C -e upgrade_talos_version=<talos_version>
```

Nodes upgrade **one at a time, in cluster-map order** (upgrade →
`talosctl version` ready-gate → next node). Verify after:

```bash
# Working dir: talos/ansible/
C=<cluster>; N=<node-ip>
talosctl --talosconfig build/$C/talosconfig -n $N -e $N version
kubectl --kubeconfig build/$C/kubeconfig get nodes -o wide
```

### 3.4 Kubernetes upgrade (drift-only)

Compares `-e kubernetes_version=<ver>` (no leading `v`) against the kubelet image in
the rendered machine config; skips on match. Always prints the `--dry-run` plan
before `upgrade-k8s --to`. Targets ALL control-plane nodes.

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=<cluster> -e kubernetes_version=<ver>
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

### 3.6a Secrets refresh without a push (vault rotations)

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=<cluster> -e refresh_secrets=true
```

Re-renders patches + PAT from the vault, rewrites the NetBird placeholder
(§1.0b), refreshes schematic IDs, forces `wipe: false` (§1.5), regenerates +
validates machine configs — but NEVER runs `apply-config`. Inspect the diff,
then push explicitly with §3.6.

### 3.6 Re-apply machine configs (installed cluster)

Day-0 re-render does **not** push to nodes; day-1 insecure apply is maintenance-only.
This is the installed-cluster path: the §3.6a render half + authenticated
`apply-config --mode <reapply_mode>` (default `staged`; diffs live node state
server-side). ALWAYS forces `wipe: false` and fails closed on `wipe: true`
(§1.5). Asserts each node's `.id` is 64-hex (re-run day-0 if missing).

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=<cluster> -e reapply_configs=true
# Mode override:
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=<cluster> -e reapply_configs=true -e reapply_mode=no-reboot
```

### 3.7 ESO PAT Secret apply + renew (post-Flux)

Applies the day-0 rendered PAT to the `external-secrets/proton-pass-pat`
Secret, then restarts `deploy/eso-proton-pass` **only when the Secret changed**.
Post-Flux only — pre-Flux skips with a note.

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
> pass-cli agent renew home-ops-eso --expiration 1y --output json
> ```
>
> Renew never fails the play.

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

- Network/DNS/TLS issue → fix egress, re-run day-0.
- Schematic rejected → check extension names are bare `siderolabs/<name>` entries.
- Check-mode runs never upload.

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
Day-0 must render (and day-1 must bootstrap) with the same
`talos_bootstrap_fresh_install` flag value. Day-0 fails fast with an mtime
preflight when a source patch is newer than its render, naming the `rm` below
(vault rotations stay invisible — still delete + re-run after one).

| Guard file (`build/<cluster>/...`) | Goes stale when… | Recovery |
| --- | --- | --- |
| `patches.yml` | source cluster patch, vault values, or setup key rotates | `rm build/<c>/patches.yml`, re-run day-0 (or day-2 `-e reapply_configs=true`). NetBird plane has NO `creates:` guard — never delete `netbird-tf/` (§1.0b). |
| `nodes-<node>-patches.yml` | source node patch, vault values, or schematic changes | `rm build/<c>/nodes-*-patches.yml`, re-run day-0 |
| `proton-pass-pat` | vault `eso-proton-pass`/`pat` rotated | `rm build/<c>/proton-pass-pat`, re-run day-0 (or day-2 `-e reapply_configs=true`) |
| `schematic-<node>.id` / `.sha256` | re-uploads on content change | none needed |
| `secrets.bundle.yml` | **never** refreshes while present | `rm` + re-run day-0 only pre-install; live cluster pulls from Pass (§6.2) |
| `talosconfig`, `nodes/<n>/*.yaml` | fresh every run (no guard) | n/a |
| `.installed-<node>` | config re-rendered but node never re-applied | `rm` + re-run day-1 (maintenance only) / manual secure apply if installed |
| `.bootstrapped` | never re-run by design | `rm` only to **re-bootstrap a fresh cluster**; never on a live one |
| `kubeconfig` | re-bootstrap / cert rotation | `rm build/<c>/kubeconfig`, re-run day-1 — or day-2 `-e regen_kubeconfig=true` (§3.5) |

### 4.4 Re-apply after a patch change (installed cluster)

Manual equivalent of the §3.6 automation:

```bash
# Working dir: talos/ansible/
C=<cluster>; N=<node-ip>; NODE=<node-name>
rm build/$C/patches.yml build/$C/nodes-*-patches.yml
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=$C
talosctl apply-config --talosconfig build/$C/talosconfig -n $N \
  -f build/$C/nodes/$NODE/controlplane.yaml --mode auto
```

### 4.5 Single-node production posture (accepted)

Both clusters run ONE control-plane node today:

- **Accepted RTO**: any node failure = full outage until the node returns.
  Mitigation is the §6 backup (PKI trio in Pass + `netbird-tf/` gpg state
  off-machine) plus reinstall from the same bundle.
- **Control-plane taint removal is intentional** (`taints: $patch: delete`);
  re-add the taint when scaling past one node.
- **VIP is a single speaker** (no HA) and stays the stable API endpoint across
  the §4.6 scale-up — clients never re-point.
- **Ready-gate between upgrades**: day-2 upgrades one node, polls
  `talosctl version` until `rc==0`, then proceeds to the next.
- **Time + DNS**: dual NTP + public DNS are intentional — no LAN NTP/DNS
  exists yet.
- `nodes[0]` is the bootstrap node, kubeconfig source, health `--init-node`,
  and etcd query target — keep it first. Day-1 apply and day-2 upgrade loop
  over **all** nodes in list order; `role: worker` is handled
  (`worker.yaml` via `--config-patch-worker`).

### 4.6 Scale-up path: single control-plane → 3 control-plane (+ workers)

1. Bump the NetBird setup-key `usage_limit` first (3 covers 1 peer + 2
   headroom), or scale-up joins get rejected.
2. Add two control-plane entries (then optional workers) to
   `talos_clusters[<cluster>].nodes` in `group_vars/all.yml` (keep the current
   bootstrap node at `nodes[0]`), with matching `nodes/<new>/patches.yml` +
   `schematics.yml`.
3. Day-0 render (new nodes get their own schematic IDs + machine configs);
   pre-flight links/disks per node (§0.4). The VIP stays the endpoint — new
   nodes join behind it.
4. Join per new node: `talosctl apply-config --insecure` against the NEW node
   only (maintenance boot), wait for its Talos API, then let it join etcd via
   the VIP (never `talosctl bootstrap` again — one bootstrap per cluster
   lifetime, §6.3).
5. Verify etcd quorum (3 voters), `talosctl health`, `kubectl get nodes`
   (all `Ready`). Restore the control-plane taint and re-check placement.

---

## 5. Reference

### 5.1 Clusters / nodes (from `group_vars/all.yml` — do not invent others)

| Cluster | Endpoint (VIP) | Node | IP | Role | Hardware / NIC / disks |
| --- | --- | --- | --- | --- | --- |
| `acme-prd-bdo1-talos-apps-01` | `https://192.168.1.198:6443` | `bdo-r01-cp-001` | `192.168.1.101` | `controlplane` | Bare metal; link `enp45s0`; install `/dev/nvme0n1` 512 GiB NVMe |
| `acme-dev-bdo1-talos-apps-01` | `https://192.168.1.248:6443` | `bdo-r01-cp-002` | `192.168.1.201` | `controlplane` | QEMU/KVM VM; link `ens18`; `/dev/sda` 20 GiB OS + `/dev/sdb` 960 GB data |

VIP advertises from the control-plane node (single speaker — stable endpoint
across the §4.6 scale-up). Node labels carry `env: dev/prd` per cluster.

### 5.2 `build/<cluster>/` file table (all gitignored)

All files `0600` (`netbird-tf/` dir `0700`).

| Path | Produced by |
| --- | --- |
| `patches.yml` | day-0 inject (cluster) + NetBird rewrite (§1.0b) |
| `nodes-<node>-patches.yml` | day-0 inject (node) + ID-rewrite |
| `schematics-<node>.yml` | day-0 stage (reference copy of resolved schematic) |
| `schematic-<node>.id` / `.sha256` | day-0 factory upload / reuse (64-hex ID) |
| `secrets.bundle.yml` | day-0 `gen secrets` (Cluster PKI bundle — never commit; Pass-backed §6.1) |
| `talosconfig` | day-0 `gen config -t talosconfig` (Pass-backed §6.1) |
| `nodes/<node>/controlplane.yaml` | day-0 `gen config` (control-plane nodes) |
| `nodes/<node>/worker.yaml` | day-0 `gen config` (no worker nodes defined) |
| `kubeconfig` | day-1 `talosctl kubeconfig -f` (Pass-backed §6.1) |
| `proton-pass-pat` | day-0 inject from `pat.yml.template` (day-2 re-render forced on refresh/reapply) |
| `netbird-tf/` (`terraform.tfstate` plaintext) | day-0 NetBird plane (§1.0b), persistent — backup class §6.1 |
| `.installed-<node>` / `.bootstrapped` | day-1 markers |

### 5.3 Extra vars

Pins (`talos_version`, `talos_kubernetes_pinned_version`) live in
`group_vars/all.yml` — never in this table.

| Var | Play | Default | Effect |
| --- | --- | --- | --- |
| Var | Play | Default | Effect |
| --- | --- | --- | --- |
| `talos_cluster` | all | `acme-dev-bdo1-talos-apps-01` | Selects `talos_clusters[<name>]`. |
| `talos_bootstrap_fresh_install` | day-0 + day-1 | `false` (safe) | Bootstrap-only wipe intent (§1.5). Ignored on day-2. |
| `talos_bootstrap_mode` | day-1 | `auto` | `--mode` for insecure `apply-config`. |
| `upgrade_image` | day-2 | `""` (no upgrade) | Explicit installer per node (§3.3). |
| `upgrade_talos_version` | day-2 | `""` (no upgrade) | Auto-builds installer from schematic `.id` (§3.3). |
| `kubernetes_version` | day-2 | `""` (opt-in only) | `--dry-run` plan then `upgrade-k8s --to`; drift-only (§3.4). |
| `regen_talosconfig` / `regen_kubeconfig` | day-2 | `false` | Rebuild talosconfig / re-fetch kubeconfig (§3.5). |
| `refresh_secrets` | day-2 | `false` | Re-render + regenerate without pushing (§3.6a; implied by reapply). |
| `reapply_configs` (+ `reapply_mode`, default `staged`) | day-2 | `false` | Refresh render path, then `apply-config --mode` (§3.6). |
| `restore_secrets` | day-2 | `false` (read-only) | Pull-restore absent files from Proton Pass (§6.2). |
| `skip_health` | day-2 | `false` | Skips the health probe AND the mutating-plane gate (break-glass). |
| `pat_apply` | day-2 | `false` (read-only) | Applies stored PAT to the ESO Secret (§3.7). |
| `pat_renew` | day-2 | `false` (needs `pat_apply=true`) | Renews the agent token first (§3.7). |
| `pat_name` / `pat_expiration` | day-2 | `home-ops-eso` / `1y` | Renew identity / expiration enum. |
---

## 6. Backup / save — surviving a fresh clone

`build/` is gitignored, so a fresh clone starts EMPTY. Day-0 auto-stores the PKI
trio into Proton Pass after every render (§6.1); only the NetBird Terraform
state still needs a manual encrypted backup.

### 6.1 What to back up

Day-0 pushes `secrets.bundle.yml` + `talosconfig` (+ `kubeconfig` when present)
into the cluster vault `talos` item, `Secrets` section, hidden fields —
checksum-driven per-field updates, deep-merge only (repeat updates merge with
no field duplication). Skipped on bootstrapped clusters unless forced
(`-e talos_render_pass_store_force=true`); local files win. Bare
`pass://<vault>/<item>/<field>` refs resolve via `pass-cli item view`
(inject). A missing item is created `custom` from a template; missing vaults
are never auto-created (create the vault first, then re-run); a missing
session fails closed (`pass-cli login` first). A `pass-cli` session backed by
a PAT may lack item-write scope — a refused push fails closed here.

Per-field refs, redacted examples, generating commands, and the remaining
unknowns live beside the role (`roles/talos_render/tasks/pass_store.yml`):

| Field (`pass://<cluster>/talos/<field>`, e.g. `pass://acme-dev-bdo1-talos-apps-01/talos/secrets-bundle`) | Contains (redacted) | Minted by |
| --- | --- | --- |
| `secrets-bundle` | `<cluster PKI bundle YAML>` (redacted) | `talosctl gen secrets -o build/<cluster>/secrets.bundle.yml` |
| `talosconfig` | `<client talosconfig YAML>` (redacted) | `talosctl gen config <cluster> <endpoint> --with-secrets build/<cluster>/secrets.bundle.yml -t talosconfig -o build/<cluster>/talosconfig` |
| `kubeconfig` | `<admin kubeconfig YAML>` (redacted) | `talosctl kubeconfig -f` (day-1 fetch into `build/<cluster>/kubeconfig`) |

MUST-VERIFY (never assumed): real-size bundle whitespace trim behaviour,
server-side field size limits, live `talos` item type (login vs custom) for
qualified writes, PAT-session write allow/deny.

| File (`build/<cluster>/...`) | Class | Why |
| --- | --- | --- |
| `secrets.bundle.yml` | Pass-backed + local cache | Cluster PKI root at `pass://<cluster>/talos/secrets-bundle` — a fresh bundle does NOT match an installed cluster. |
| `talosconfig` | Pass-backed + local cache | At `pass://<cluster>/talos/talosconfig`; also rebuildable via `regen_talosconfig` (§3.2). |
| `kubeconfig` | Pass-backed + local cache | At `pass://<cluster>/talos/kubeconfig` (lands on the next day-0 after day-1 fetches it); may be stale — prefer `regen_kubeconfig` once reachable. |
| `netbird-tf/` (whole dir) | **CRITICAL-local, EXCLUDED from Pass** | Plaintext local state holding the setup-key secret — never stored in Pass; the gpg backup below is its only off-machine copy. |
| `proton-pass-pat` | Re-mintable, EXCLUDED from Pass | Re-renders via day-0 inject; keep a copy for offline `pat_apply` (§6.3). |

Regenerable (no backup needed): rendered patches, staged schematics + `.id`/`.sha256`, node `*.yaml`, markers.

```bash
# Working dir: talos/ansible/
C=<cluster>
tar -czf - build/$C/netbird-tf | \
  gpg --symmetric --cipher-algo AES256 -o ~/talos-$C-netbird-tf.tgz.gpg
# Store OFF this machine. Verify: gpg -d ~/talos-$C-netbird-tf.tgz.gpg | tar -tz
```

### 6.2 Fresh-clone restore

Probe the session, pull the trio from Pass, re-render the rest with day-0, then
continue with day-1 (pre-install) or day-2 (installed). Day-1 pulls
unconditionally; day-2 only with `-e restore_secrets=true` (default `false`
keeps day-2 read-only). Each file restores only when its local copy is absent
(local files win); on a live cluster a file Pass cannot supply fails closed
instead of minting fresh. Per-field refs live beside the role
(`roles/talos_bootstrap/tasks/pass_restore.yml`).

```bash
# Working dir: talos/ansible/
C=<cluster>
pass-cli login   # probe the session first (§0.3)
ansible-playbook playbooks/day2.yml -i localhost, -e talos_cluster=$C -e restore_secrets=true   # pull: restores bundle + talosconfig + kubeconfig from Pass
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=$C   # re-renders patches, .id files, node yamls (netbird-tf/ restored, so the NetBird plane upserts — no imports)
ansible-playbook playbooks/day1.yml -i localhost, -e talos_cluster=$C -e talos_bootstrap_fresh_install=true   # pre-install only: apply + bootstrap + kubeconfig
ansible-playbook playbooks/day2.yml -i localhost, -e talos_cluster=$C   # installed: read-only health first, then opt in (§3.2)
```

`netbird-tf/` is NOT in Pass — restore it from gpg before the day-0 re-render:

```bash
# Working dir: talos/ansible/
C=<cluster>
gpg -d ~/talos-$C-netbird-tf.tgz.gpg | tar -xzf -   # restores netbird-tf/ only
```

### 6.3 What NEVER to do

- **Never commit `build/`** — live PKI, admin configs, vault PAT.
- **Never re-bootstrap a live cluster** — `rm .bootstrapped` + re-run day-1 would
  `talosctl bootstrap` an already-bootstrapped etcd (split-brain).
- **Never mint fresh PKI against live state** — on a fresh clone with a live
  cluster, pull from Pass FIRST (`restore_secrets`), then re-render (§1.7);
  a fresh `gen secrets` orphans the cluster.
- **Never delete `secrets.bundle.yml` on a live cluster** (§1.7) — restore from Pass instead.
- **Never `rm -rf build/<cluster>` on a live cluster without restorable state** —
  re-render needs the SAME bundle from Pass (plus `netbird-tf/` from gpg;
  without it re-anchor with `tofu import` per resource — see the netbird README).
- **Never store `netbird-tf/` in Proton Pass** — gpg backup (§6.1) is its only
  off-machine copy.
- **Never seed `proton-pass-pat` back into the vault** — it is the vault credential;
  it lives in `build/` and flows only to the day-2 ESO Secret apply.
