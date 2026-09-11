# talos/ansible — Day 0/1/2 runbook

Step-by-step operator runbook for the Talos day-0/1/2 Ansible automation.
Concept reference lives in `README.md`; this file is the copy-pasteable
procedure. **Working directory for every command below is `talos/ansible/`**
unless stated otherwise. Every play runs on `hosts: localhost`
(`connection: local`) — all node contact is `talosctl` over the Talos API,
no SSH.

Default cluster (when `-e talos_cluster=...` is omitted) is
`acme-dev-bdo1-talos-apps-01`.

> Conventions: `<cluster>` is a placeholder for a real cluster name from the
> Reference table. Commands are safe to copy verbatim after substituting it.

---

## 0. Prereqs (do once per shell)

### 0.1 Enter the flox environment (repo root)

The repo-root flox hook installs `talosctl` v1.14.0 into the flox cache
`bin/` (added to `PATH` via the profile script) and `cd`s you back to the
project root.

```bash
# Working dir: repo root (where .flox/ lives)
flox activate
talosctl version --client   # expect v1.14.0
```

Then move into the Ansible tree — **all playbook commands run from here**:

```bash
cd talos/ansible
pwd   # expect .../talos/ansible
```

### 0.2 Install Ansible collections

```bash
# Working dir: talos/ansible/
ansible-galaxy install -r requirements.yml
```

This provides `community.general >= 9.0.0` (used for the
`random_string` lookup behind the per-exec `PROTON_PASS_AGENT_REASON`).
Re-run after a fresh checkout or when `requirements.yml`
changes.

### 0.3 Authenticate to Proton Pass

Secret values are never read from the shell. Login prerequisite:
**Ansible NEVER logs in** — run `pass-cli login` in your
shell before any play. Every play probes the existing session via
`pass-cli info -o json` (`rc==0` + JSON mapping stdout = logged in;
logged-out gives `rc=1` + a non-JSON error) and fails fast telling you
to run `pass-cli login`.

```bash
# Working dir: anywhere
export PROTON_PASS_AGENT_REASON=talos-render-manual-exec-$(openssl rand -hex 8)
pass-cli login
```

Ansible auto-generates a fresh unique `PROTON_PASS_AGENT_REASON` per
`pass-cli` exec during day-0 renders
(`<prefix>-<cluster>[-<node>]-exec-<16 random lowercase hex>`); the export
above is only needed for manual `pass-cli` commands.

If this step is missed, day-0 fails fast with:

```text
No authenticated pass-cli session. Run `pass-cli login` before
rendering secrets, then re-run the play.
```

> ⚠️ **Warning — PAT expiry:** a stale/expired session surfaces as
> `pass-cli inject` failures during day-0 rendering. Run `pass-cli login`
> again, then re-run the play.
>
> Inject failures name the missing secret directly, e.g.
> `Failed to fetch secret for pass://<vault>/talos/<field>` + `Field
> '<field>' not found in item 'talos'` — that means the vault item exists
> but the field is absent/renamed. Add the field to the item in Proton Pass
> (field names are case-sensitive), or fix the `pass://` ref in the source
> `patches.yml`, then delete the stale `build/<cluster>/patches.yml` (or
> `nodes-<node>-patches.yml`) and re-run day-0. Inject tasks intentionally
> carry no `no_log` — `pass-cli inject --out-file` prints no secret values,
> only the unresolved ref, so the error stays actionable.

### 0.4 Network + node state

- Reachability to the target LAN (`192.168.1.0/24`) for every node IP and
  the cluster VIP endpoint (see Reference table).
- `https://factory.talos.dev` reachable (port 443) — day-0 uploads each
  node's schematic there. Failure aborts day-0 (see Troubleshooting).
- Target node(s) booted into **Talos maintenance** (uninstalled Talos media —
  Talos API up, no machine config applied yet). Day-1's
  `apply-config --insecure` only works in this state. Optional pre-flight:

```bash
# Working dir: talos/ansible/ (needs build/<cluster>/talosconfig only after day-0;
# --insecure needs no config). Example for the dev node:
talosctl get links --insecure -n 192.168.1.201
talosctl get disks --insecure -n 192.168.1.201
```

Confirm the link name matches the node patch (`ens18` on dev/QEMU,
`enp45s0` on prd/bare metal) and the disk layout matches (dev: `/dev/sda`
20 GiB OS + `/dev/sdb` 960 GB data; prd: `/dev/nvme0n1` 512 GiB NVMe).

---

## 1. Day 0 — render (`playbooks/day0.yml`)

Generates everything under `build/<cluster>/` (gitignored): injected
patches, per-node schematics + factory IDs, secrets bundle, `talosconfig`,
per-node machine configs. Validates each machine config with
`talosctl validate -m metal`. Changes nothing on the nodes.

Prerequisites (per cluster vault): a `talos` item with a `setup-key` field
(referenced per-cluster as
`pass://<cluster-vault>/talos/netbird-setup-key` in the cluster patch),
**plus** an `eso-proton-pass` item with a `pat` field (referenced as
`pass://<cluster-vault>/eso-proton-pass/pat` in that cluster's
`talos/clusters/<cluster>/pat.yml.template`). A missing item/field fails
rendering naming the unresolved ref.

Role step order (`talos_render`): `mkdir build/<cluster> 0700` → login check
→ `pass-cli inject` cluster base patch → `pass-cli inject` per-node patches
→ `pass-cli inject` PAT render (`pat.yml.template` → `proton-pass-pat`,
`0600`, `creates:` guard; path pinned by the `talos_pat_filename` shared
var) → per-node schematic deep-merge (`_base` → cluster → node) / factory
upload / ID-rewrite → `gen secrets` bundle → `mkdir nodes/<node>` →
`gen config -t talosconfig` → per-node `gen config -t <role>` with
`--install-image` → `validate -m metal`.

### 1.1 Dry run (recommended first)

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=acme-dev-bdo1-talos-apps-01 --check --diff
```

Production variant:

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=acme-prd-bdo1-talos-apps-01 --check --diff
```

> ⚠️ **Warning — check-mode placeholder:** in `--check` mode the Image
> Factory upload is skipped, so no schematic ID is known and `gen config`
> renders `--install-image
> factory.talos.dev/metal-installer/pending-schematic-upload:v1.14.0`.
> Check-mode output proves template/plumbing, **not** an installable config.
> Always follow with a real run below.

### 1.2 Real run

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=acme-dev-bdo1-talos-apps-01
```

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=acme-prd-bdo1-talos-apps-01
```

Expected: play succeeds (`failed=0`); `validate` tasks report `ok`.

### 1.3 What it produces

Per cluster, under `build/<cluster>/` (see Reference for the full file
table):

- `patches.yml` — cluster patch with secrets injected via `pass-cli inject`.
- `nodes-<node>-patches.yml` — per-node patch, injected, then rewritten so
  `PLACEHOLDER_SCHEMATIC_ID` becomes the real factory ID for that node.
- `proton-pass-pat` (`0600`) — ESO webhook PAT, rendered via
  `pass-cli inject` from `talos/clusters/<cluster>/pat.yml.template`
  (double-brace `pass://<own-vault>/eso-proton-pass/pat`). Guarded by
  `creates:` — re-renders only when the file is missing.
- `schematics-<node>.yml` — staged reference copy of the **deep-merged**
  schematic: `_base` (shared) → cluster (env-wide, e.g. netbird) → node
  (node-only, e.g. qemu-guest-agent / intel-ucode + kernel args / nfsd
  stack), merged recursively with dedup list-union. Node files hold
  node-only entries — inherited lines (e.g. netbird) live in the cluster
  layer, not the node file. Merged bytes change → new factory ID on the
  next day-0.
- `schematic-<node>.id` + `schematic-<node>.sha256` — factory response ID
  and content hash. Upload (`POST https://factory.talos.dev/schematics`) is
  idempotent: skipped when the staged schematic hash matches the stored
  `.sha256`; re-uploads only on content change.
- `secrets.bundle.yml` (`0600`) — `talosctl gen secrets` output.
  Created **once**; later runs keep the existing bundle (see Re-run/reset).
- `talosconfig` — cluster admin config (`gen config -t talosconfig`,
  base + cluster patches, carries the cluster endpoint).
- `nodes/<node>/<type>.yaml` — one machine config per node, `-t <type>`
  where `<type>` follows `talos_machine_roles` (`controlplane` →
  `controlplane.yaml`, `worker` → `worker.yaml`). Built from base patch +
  cluster patch (generic `--config-patch`) + **only that node's** patch
  (`--config-patch-control-plane` / `--config-patch-worker`), with
  `--install-image factory.talos.dev/metal-installer/<that-node-ID>:v1.14.0`.
  Each file is validated (`talosctl validate -c <file> -m metal`).

### 1.4 How to verify outputs

```bash
# Working dir: talos/ansible/
# Example with the dev cluster; substitute prd name for production.
C=acme-dev-bdo1-talos-apps-01
ls -l build/$C build/$C/nodes/*/                                # all files present, 0600/0700 modes
cat build/$C/schematic-*.id                                     # 64-hex factory ID per node (no "pending-schematic-upload")
grep -r PLACEHOLDER_SCHEMATIC_ID build/$C && echo STALE || echo "IDs rewritten OK"
grep -o 'factory.talos.dev/metal-installer/[0-9a-f]*' build/$C/nodes-*-patches.yml build/$C/nodes/*/*.yaml | sort -u
talosctl validate -c build/$C/nodes/bdo-r01-cp-002/controlplane.yaml -m metal
```

### 1.6 NFS server stack (per node)

Each node runs NFS server daemons for LAN clients (ports `2049`, `20048`,
`111`). Three pieces, all in the node sources (nothing cluster-wide):

- **Schematic layer** (node `schematics.yml`): the full trio
  `siderolabs/nfsd` (kernel module) + `nfs-utils` (rpcbind/statd) +
  `nfs-server` (daemons) — all three together, per the nfs-server README
  requirements. Merged bytes change the schematic hash → new factory ID on
  the next day-0.
- **Config** (node `patches.yml`): `EtcFileConfig` `exports` serves the
  existing data volumes (LAN-only, default `root_squash` kept) — dev two
  lines (`/var/mnt/nvme-data 192.168.1.0/24(rw,sync,no_subtree_check,
  fsid=0,crossmnt)` + `/var/mnt/sata-data 192.168.1.0/24(rw,sync,
  no_subtree_check)`); prd a single `/var/mnt/nvme-data ...(fsid=0,
  crossmnt)` line (no `sata-data` volume — the host has no SATA disk;
  re-add the volume + its line only when that disk appears). `fsid=0`
  marks the NFSv4 pseudo-root on `nvme-data`; `crossmnt` lets clients
  traverse into the second export. Plus `EtcFileConfig` `netconfig`
  (libtirpc IPv4 server table). No `ExtensionServiceConfig`: the service
  takes no env, so it needs no service config document.
- **Backing store**: none dedicated — no `nfs` volume. The export paths
  resolve against the existing `nvme-data` (+ `sata-data` on dev)
  `UserVolumeConfig` volumes (`<name>` mounts at `/var/mnt/<name>`);
  exports consume no extra disk.

Readiness (after day-1 install) — NFS threads up and ports listening:

```bash
# Working dir: talos/ansible/ (authenticated API after install)
C=acme-dev-bdo1-talos-apps-01
talosctl --talosconfig build/$C/talosconfig -n 192.168.1.201 \
  read /proc/fs/nfsd/threads            # nonzero = server threads running
talosctl --talosconfig build/$C/talosconfig -n 192.168.1.201 \
  list /var/mnt/nvme-data               # export path resolves
```

### 1.7 Re-run / reset

Day-0 is guarded by `creates:` — re-running without changes skips the
inject/secrets steps and regenerates + revalidates configs. A render
freshness preflight compares each source against its render by mtime: when a
source `patches.yml` / `pat.yml.template` is newer than its `build/` output,
day-0 fails fast naming the exact `rm` below instead of rendering +
validating stale configs as if fresh. (Vault value rotations are invisible
to the mtime check — still delete + re-run after a rotation.) To **force** a
re-render after editing a source patch, schematic, or secret reference,
delete the corresponding `build/<cluster>` file(s) and re-run:

```bash
# Working dir: talos/ansible/
# Re-render injected patches after editing source patches.yml or rotating vault values:
C=acme-dev-bdo1-talos-apps-01
rm build/$C/patches.yml build/$C/nodes-*-patches.yml
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=$C
```

```bash
# Working dir: talos/ansible/
# Full day-0 reset for one cluster (keeps kubeconfig/markers out of scope — see Day 1/2 notes):
C=acme-dev-bdo1-talos-apps-01
rm -rf build/$C
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=$C
```

> ⚠️ **Warning — secrets rotation:** deleting `secrets.bundle.yml` mints a
> **new** cluster PKI bundle. Safe before first install; after day-1 it
> orphans the installed cluster (new PKI ≠ installed PKI). Never reset the
> secrets bundle for a live cluster.

---

## 2. Day 1 — bootstrap (`playbooks/day1.yml`)

Installs machine configs onto maintenance-booted nodes, bootstraps etcd on
the first node, and fetches the admin kubeconfig. Run **after a successful
day-0 real run** for the same cluster.

Role step order (`talos_bootstrap`): auth check (PAT env + `pass-cli`
session) → `apply-config --insecure` per node →
readiness wait (TCP 50000 per node, then authenticated `talosctl version`
poll on **every** node) → control-plane assert on `nodes[0]` → `bootstrap`
on `nodes[0]` (retried) → `kubeconfig`
fetch (retried) → marker files. The PAT arrives via the day-0 render
(`proton-pass-pat` is written by `talos_render`, not day-1) — day-1's auth
check stays only as a fail-fast login gate.

> The readiness wait fixes the day-1 bootstrap race: nodes install + reboot
> right after the insecure apply, so bootstrapping immediately fails with
> `FailedPrecondition: bootstrap is not available yet`. Expect 1–2 min of
> waiting on a normal run; gates allow up to ~10 min for slow installs
> (tunables: `talos_bootstrap_*` in `roles/talos_bootstrap/defaults/main.yml`).
Day-1 runs pre-Flux (no `external-secrets` namespace yet). The PAT is the
vault credential itself, so it stays in `build/<cluster>/` (gitignored)
and is never seeded back into the vault it unlocks.

### 2.1 Command

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day1.yml -i localhost, -e talos_cluster=acme-dev-bdo1-talos-apps-01
```

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day1.yml -i localhost, -e talos_cluster=acme-prd-bdo1-talos-apps-01
```

Optional apply mode override (default `auto`):

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day1.yml -i localhost, -e talos_cluster=acme-dev-bdo1-talos-apps-01 -e talos_bootstrap_mode=no-reboot
```

### 2.1b PAT source (day-0 render) + one-time interactive `agent create`

Day-1 probes the `pass-cli` session (same
`info -o json` login check as day-0/day-2) as its FIRST task — before any
`talosctl apply-config --insecure`, bootstrap, or kubeconfig fetch — as a
fail-fast login gate. The PAT *value* is NOT read from the shell: day-0
rendered it via `pass-cli inject` from the cluster's `pat.yml.template`
(vault item `eso-proton-pass`, field `pat`) into
`build/<cluster>/proton-pass-pat` (`0600`, gitignored via
`ansible/build/`). Needs the same `§0.3` login as day-0 — no separate auth.

If no token exists yet, mint one **interactively** first (one-time;
`pass-cli` cannot create agent tokens inside a PAT/agent session, and no
play auto-creates):

```bash
export PROTON_PASS_AGENT_REASON=talos-bootstrap-manual-exec-$(openssl rand -hex 8)
pass-cli agent create home-ops-eso --expiration 1y --vault <cluster>
# e.g. --vault acme-dev-bdo1-talos-apps-01 (always JSON output)
# Save the printed PROTON_PASS_PERSONAL_ACCESS_TOKEN=pst_... token, then:
export PROTON_PASS_PERSONAL_ACCESS_TOKEN=pst_...
pass-cli login
```

Grant the agent access to the cluster vault if needed:

```bash
pass-cli agent access grant home-ops-eso \
  --vault-name <cluster> --role viewer
# e.g. --vault-name acme-dev-bdo1-talos-apps-01
```

Delete an agent by name when retiring it:

```bash
pass-cli agent delete home-ops-eso
```

Then run day-1 — only the `pass-cli login` session gates the play; the
rendered `proton-pass-pat` file carries the webhook value. Expiration enum
for `--expiration`: `1h, 1d, 1w, 1m, 3m, 6m, 1y` (default `1y`, mirroring
`-e pat_expiration=...` only feeds the day-2 renew/example strings, it
never changes the stored file).

### 2.2 What "insecure-first-boot" means

The first task runs, per node:

```text
talosctl apply-config --talosconfig build/<cluster>/talosconfig \
  --insecure -n <node-ip> \
  -f build/<cluster>/nodes/<node>/<type>.yaml --mode <talos_bootstrap_mode>
```

`--insecure` means: talk to the node's maintenance API **without** PKI
auth. This only works while the node is still uninstalled / in maintenance
mode. After the config is applied the node installs, reboots, and requires
authenticated API access — re-running the insecure apply against an
installed node fails (and is skipped anyway once the marker exists).

### 2.3 Marker files + kubeconfig

`creates:` guards make day-1 safe to re-run; each step is skipped once its
marker/output exists:

| Step | Marker / output | Meaning |
| --- | --- | --- |
| Per-node apply | `build/<cluster>/.installed-<node>` | That node has been given its config; re-apply skipped. |
| Etcd bootstrap | `build/<cluster>/.bootstrapped` | `talosctl bootstrap` ran on `nodes[0]`; never re-bootstraps. |
| Kubeconfig | `build/<cluster>/kubeconfig` | Admin kubeconfig fetched via `talosctl kubeconfig -n/-e nodes[0] -f`. |

Day-1 performs no post-bootstrap secure apply — insecure-apply →
bootstrap → kubeconfig only. After a patch change on an installed
cluster, re-apply with an authenticated `talosctl apply-config`
(see §3.6 / Troubleshooting §4.4).

> ⚠️ **Warning — `nodes[0]` pinning:** bootstrap, kubeconfig fetch, and the
> day-2 health `--init-node` / etcd queries all target
> `talos_clusters[talos_cluster].nodes[0]`, not "any healthy control-plane".
> With today's single-node clusters that is the only control-plane; on a
> future multi-node cluster, node order in `group_vars/all.yml` decides who
> bootstraps. Day-1 asserts `nodes[0]` is a control-plane node before
> bootstrapping, and the L2 `talosctl version` poll covers **every** node
> (a non-zero node still installing fails the play instead of bootstrapping
> around it).

### 2.4 Verify

```bash
# Working dir: talos/ansible/
C=acme-dev-bdo1-talos-apps-01
ls -l build/$C/.installed-* build/$C/.bootstrapped build/$C/kubeconfig
talosctl --talosconfig build/$C/talosconfig -n 192.168.1.201 -e 192.168.1.201 health
kubectl --kubeconfig build/$C/kubeconfig get nodes -o wide
```

(prd: substitute `C=acme-prd-bdo1-talos-apps-01` and node IP
`192.168.1.101`.) Expect the node `Ready`, VIP endpoint serving the API.

---

## 3. Day 2 — operate (`playbooks/day2.yml`)

Default (no flags) is read-only health/etcd, but STILL checks the PAT env
var + `pass-cli` session first (cheap fail-fast; re-apply/renew need them).
Flags opt into regen, re-apply, version upgrades, and the ESO PAT Secret
plane. Execution order: auth probe → regen talosconfig/kubeconfig →
re-apply machine configs → Talos upgrade per node → Kubernetes upgrade →
PAT Secret apply/renew (post-Flux, §3.7). Talos always precedes k8s.

### 3.1 Read-only (safe anytime)

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day2.yml -i localhost, -e talos_cluster=acme-dev-bdo1-talos-apps-01
```

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day2.yml -i localhost, -e talos_cluster=acme-prd-bdo1-talos-apps-01
```

This runs `talosctl health` (control-plane nodes + `--init-node nodes[0]`)
and `talosctl etcd members` (against `nodes[0]`), both with
`failed_when: false` — they report via debug output and never fail the play.
No changes are made without one of the §3.2 flags.

Health runs on **every** day-2 invocation; skip explicitly with
`-e skip_health=true`.

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

Talos minor upgrades are **adjacent-minors-only** (e.g. v1.14.x → v1.15.x,
never skip a minor).

Explicit image (read the real ID from the file — never invent one):

```bash
# Working dir: talos/ansible/
C=acme-dev-bdo1-talos-apps-01
cat build/$C/schematic-*.id   # 64-hex factory ID per node
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=$C \
  -e upgrade_image=factory.talos.dev/metal-installer/$(cat build/$C/schematic-bdo-r01-cp-002.id):v1.15.0
```

Version flag (auto-builds the same installer ref per node from the `.id`
files; fails fast naming the node when an ID is missing):

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=acme-dev-bdo1-talos-apps-01 \
  -e upgrade_talos_version=v1.15.0
```

Nodes upgrade **one at a time, in cluster-map order**. Verify after:

```bash
# Working dir: talos/ansible/
C=acme-dev-bdo1-talos-apps-01
talosctl --talosconfig build/$C/talosconfig -n 192.168.1.201 -e 192.168.1.201 version
kubectl --kubeconfig build/$C/kubeconfig get nodes -o wide
```

### 3.4 Kubernetes upgrade (drift-only)

Compares `-e kubernetes_version=<ver>` (no leading `v`) against the kubelet
image recorded in the rendered machine config; skips when they match.
Always prints the `--dry-run` plan before the real `upgrade-k8s --to`.
Targets `nodes[0]` (today's clusters are single control-plane). When the
rendered machine config is missing or unparseable, drift detection warns
loudly (no kubelet version found — re-run day-0) instead of silently
skipping the upgrade.

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=acme-dev-bdo1-talos-apps-01 \
  -e kubernetes_version=1.38.0
```

> ⚠️ **Warning — anti-drift:** `upgrade-k8s` mutates the live cluster
> without re-rendering `build/`. Re-run day-0 afterwards so rendered
> machine configs match the upgraded version.

### 3.5 Regen client configs

Rebuilds from the **existing** secrets bundle — never mints new PKI (fresh
PKI would orphan a live cluster).

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=acme-dev-bdo1-talos-apps-01 \
  -e regen_talosconfig=true -e regen_kubeconfig=true
```

### 3.6 Re-apply machine configs (installed cluster)

Day-0 re-render does **not** push config to nodes, and day-1's insecure
apply is maintenance-only. This is the installed-cluster path: force
re-render of edited patches (needs the PAT, like day-0) **plus** a forced
re-render of the PAT file itself (`pass-cli inject` over
`pat.yml.template` — no `creates:` guard, so vault rotations flow on
re-apply), a live schematic refresh via the day-0 schematic plane
(hash-changed schematics re-upload to the Image Factory and persist fresh
`.id`/`.sha256`, so the installer image never goes stale), regenerate
machine configs, then authenticated
`apply-config --mode <reapply_mode>` (default `staged` — applies without
reboot when the change allows it; one of `auto`, `no-reboot`, `staged`,
`try`). Re-apply also asserts each node's `.id` resolves (re-run day-0 if
one is missing); with an explicit `upgrade_image` the `.id` is not needed.

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=acme-dev-bdo1-talos-apps-01 \
  -e reapply_configs=true
```

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=acme-dev-bdo1-talos-apps-01 \
  -e reapply_configs=true -e reapply_mode=no-reboot
```

### 3.7 ESO PAT Secret apply + renew (post-Flux)

Applies the day-0 rendered PAT (`build/<cluster>/proton-pass-pat`) to the
`external-secrets/proton-pass-pat` Secret (key `pat`) via
`kubectl --kubeconfig ... create secret --dry-run=client -o yaml | apply`,
then restarts `deploy/eso-proton-pass` **only when the Secret data
changed** (the webhook reads `PROTON_PASS_PAT_FILE=/secrets/pat` at
startup). Run after Flux has installed ESO; pre-Flux the plane skips with
a note (missing namespace → refresh kubeconfig via `-e
regen_kubeconfig=true`, or re-run post-Flux). A summary debug reports the
apply result. All PAT-bearing tasks are `no_log`; Ansible auto-generates
`PROTON_PASS_AGENT_REASON=talos-operate-<cluster>-exec-<16 hex>` per
`pass-cli` exec, as on the other planes.

Apply the stored PAT (idempotent; unchanged Secret → no restart):

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=acme-dev-bdo1-talos-apps-01 \
  -e pat_apply=true
```

Renew + apply (opt-in; renewal mints a replacement PAT):

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=acme-dev-bdo1-talos-apps-01 \
  -e pat_apply=true -e pat_renew=true
```

Custom PAT identity / expiration (defaults `home-ops-eso` / `1y`):

```bash
# Working dir: talos/ansible/
ansible-playbook playbooks/day2.yml -i localhost, \
  -e talos_cluster=acme-dev-bdo1-talos-apps-01 \
  -e pat_apply=true -e pat_renew=true \
  -e pat_name=home-ops-eso -e pat_expiration=1y
```

> ⚠️ **Warning — agent-session block:** `pass-cli` cannot create/renew agent
> tokens while logged in with a PAT/agent session (`Cannot manage ...
> personal access tokens while logged in with a personal access token or
> agent session`). The renew step is defensive (`failed_when: false`): on
> that error it skips and prints the exact interactive command instead —
> run it in a user-authenticated shell, export the new `pst_` token, then
> re-run day-2 with `-e pat_apply=true`:
>
> ```bash
> export PROTON_PASS_AGENT_REASON=talos-operate-manual-exec-$(openssl rand -hex 8)
> pass-cli agent renew home-ops-eso \
>   --expiration 1y --output json
> ```
>
> When `rc==0` the role extracts the new token (`.token` from the JSON,
> stripping the `PROTON_PASS_PERSONAL_ACCESS_TOKEN=` prefix, with a `pst_`
> regex fallback) and refreshes the local store; if the output is
> unparseable it keeps the stored PAT and says so. Renew never fails the
> play.

> ⚠️ **Warning — PAT expiry (max 1y):** Proton PATs expire after at most
> one year — renew before expiry or every `ExternalSecret` flips
> `Ready=False` and the webhook answers `401` (see the external-secrets
> component README renewal runbook, referenced — not duplicated — here).
> A stale kubeconfig looks similar (namespace unreachable); refresh first
> with `-e regen_kubeconfig=true` before assuming expiry.

---

## 4. Troubleshooting

### 4.1 Image Factory unreachable / upload rejected

Symptom: day-0 fails at *"Image Factory upload failed for \<node\>"*.

```bash
curl -sS -X POST --data-binary @talos/clusters/_base/schematics.yml https://factory.talos.dev/schematics
```

- Network/DNS/TLS issue → fix egress, re-run day-0 (upload retries; hash
  guard means already-uploaded nodes are skipped).
- Schematic rejected → validate YAML against a known-good cluster
  schematic; check extension names are bare `siderolabs/<name>` entries.
- Check-mode runs never upload (by design) — a real run is required.

### 4.2 PAT expired / `pass-cli` failures

Symptom: day-0 login check fails, or `pass-cli inject` errors mid-render.

```bash
export PROTON_PASS_PERSONAL_ACCESS_TOKEN=pst_...
pass-cli login
# Working dir: talos/ansible/
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=<cluster>
```

Partially rendered `build/` files from the failed run are reused on retry
(`creates:` skips); if the failure was a secrets change, delete the stale
rendered file(s) first (next table).

### 4.3 `creates:`-skip staleness — when to delete what

Every `creates:` guard trades idempotency for staleness: the task never
re-runs while its file exists, even if the **source** changed. Day-0 now
fails fast with an mtime preflight when a source `patches.yml` /
`pat.yml.template` is newer than its render, naming the exact `rm` below —
but vault value rotations stay invisible to it (still delete + re-run after
a rotation).

| Guard file (`build/<cluster>/...`) | Skipped task | Goes stale when… | Recovery |
| --- | --- | --- | --- |
| `patches.yml` | cluster `pass-cli inject` | source cluster `patches.yml` or vault values change | `rm build/<c>/patches.yml`, re-run day-0 |
| `nodes-<node>-patches.yml` | per-node `pass-cli inject` (+ ID-rewrite target) | source node `patches.yml`, vault values, or schematic changes | `rm build/<c>/nodes-*-patches.yml`, re-run day-0 |
| `proton-pass-pat` | PAT `pass-cli inject` (day-0 `creates:`; day-2 re-apply is forced) | vault `eso-proton-pass`/`pat` rotated | `rm build/<c>/proton-pass-pat`, re-run day-0 (or day-2 `-e reapply_configs=true`, which re-renders without deleting) |
| `schematic-<node>.id` / `.sha256` | factory upload (hash-guarded, not `creates:`) | re-uploads automatically on content change | none needed; honest hash comparison. A hash-match with a missing/empty `.id` (or an upload that yields no parseable ID) now fails fast naming the `rm` + re-run day-0 recovery instead of silently falling back to `pending-schematic-upload` |
| `secrets.bundle.yml` | `gen secrets` | **never** refreshes while present | `rm` + re-run day-0 only pre-install (rotation breaks live clusters) |
| `talosconfig`, `nodes/<n>/*.yaml` | `gen config` (no guard — always regenerates) | n/a (fresh every run) | n/a |
| `.installed-<node>` | insecure apply for that node | config re-rendered but node never re-applied | `rm build/<c>/.installed-<node>` + re-run day-1 (maintenance only) / manual secure apply if installed |
| `.bootstrapped` | etcd bootstrap | never re-run by design | `rm` only to **re-bootstrap a fresh cluster**; never on a live one (would split-brain etcd) |
| `kubeconfig` | kubeconfig fetch | cluster re-bootstrapped / certs rotated | `rm build/<c>/kubeconfig`, re-run day-1 — or refresh in place with day-2 `-e regen_kubeconfig=true` (§3.5) |

### 4.4 Re-apply after a patch change (installed cluster)

Day-0 re-render does **not** push config to nodes, and day-1's apply is
insecure-only (skipped once installed). For an installed node:

```bash
# Working dir: talos/ansible/
C=acme-dev-bdo1-talos-apps-01
# 1. Re-render:
rm build/$C/patches.yml build/$C/nodes-*-patches.yml
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=$C
# 2. Push with authenticated API (manual, or automated via §3.6):
talosctl apply-config --talosconfig build/$C/talosconfig -n 192.168.1.201 \
  -f build/$C/nodes/bdo-r01-cp-002/controlplane.yaml --mode auto
```

Automated equivalent (re-renders + pushes in one play, default mode
`staged`): `ansible-playbook playbooks/day2.yml -i localhost,
-e talos_cluster=$C -e reapply_configs=true` (see §3.6).

### 4.5 Multi-node notes (future clusters)

- `nodes[0]` in `group_vars/all.yml` is the bootstrap node, the kubeconfig
  source, the health `--init-node`, and the etcd query target. Keep the
  intended bootstrap node first in the list.
- Day-1 insecure apply loops over **all** nodes; join order is list order.
- Day-2 upgrade loops over **all** nodes sequentially in list order (one at
  a time). Watch each node return `Ready` before the next proceeds
  (`kubectl --kubeconfig build/<c>/kubeconfig get nodes -w`).

### 4.6 Worker caveat

> ⚠️ **Warning — worker path untested:** the roles handle `role: worker`
> (→ `worker.yaml` via `--config-patch-worker`, role-aware day-1 apply,
> upgrade loop covers all roles), but **no cluster map contains a worker
> node today** — both clusters are single control-plane. Treat any future
> worker onboarding as untested until exercised end-to-end.

---

## 5. Reference

### 5.1 Clusters / nodes (from `group_vars/all.yml` — do not invent others)

| Cluster | Endpoint (VIP) | Node | IP | Role | Hardware / NIC / disks |
| --- | --- | --- | --- | --- | --- |
| `acme-prd-bdo1-talos-apps-01` | `https://192.168.1.198:6443` | `bdo-r01-cp-001` | `192.168.1.101` | `controlplane` | Bare metal (MSI Cubi 5); link `enp45s0` (RTL8125); install `/dev/nvme0n1` 512 GiB NVMe |
| `acme-dev-bdo1-talos-apps-01` | `https://192.168.1.248:6443` | `bdo-r01-cp-002` | `192.168.1.201` | `controlplane` | QEMU/KVM VM; link `ens18` (virtio); `/dev/sda` 20 GiB OS + `/dev/sdb` 960 GB data |

Per-node schematics carry extensions (deep-merged base → cluster → node):
prd `intel-ucode, i915, realtek-firmware` (node) + `netbird` (cluster) +
nfsd stack (node); dev `qemu-guest-agent` (node) + `netbird` (cluster) +
nfsd stack (node). VIP advertises from the control-plane node
(`Layer2VIPConfig` link `enp45s0` / `ens18` respectively).

### 5.2 `build/<cluster>/` file table (all gitignored)

| Path | Produced by | Purpose |
| --- | --- | --- |
| `patches.yml` (`0600`) | day-0 `pass-cli inject` (cluster) | Cluster patch, secrets resolved |
| `nodes-<node>-patches.yml` (`0600`) | day-0 `pass-cli inject` (node) + ID-rewrite | Per-node patch, schematic ID rewritten |
| `schematics-<node>.yml` (`0600`) | day-0 stage (fallback winner) | Reference copy of resolved schematic |
| `schematic-<node>.id` (`0600`) | day-0 factory upload / reuse | 64-hex Image Factory schematic ID |
| `schematic-<node>.sha256` (`0600`) | day-0 hash persist | Upload-idempotency hash |
| `secrets.bundle.yml` (`0600`) | day-0 `gen secrets` (once) | Cluster PKI bundle — never commit |
| `talosconfig` | day-0 `gen config -t talosconfig` | Cluster admin Talos API config |
| `nodes/<node>/controlplane.yaml` | day-0 `gen config -t controlplane` | Machine config (control-plane nodes) |
| `nodes/<node>/worker.yaml` | day-0 `gen config -t worker` | Machine config (worker nodes; none today) |
| `kubeconfig` | day-1 `talosctl kubeconfig -f` | Admin kubeconfig |
| `proton-pass-pat` (`0600`) | day-0 `pass-cli inject` from `pat.yml.template` (`creates:`); day-2 re-render is forced on `reapply_configs` | Rendered PAT for the day-2 ESO Secret apply — never committed (see §6 for backup) |
| `.installed-<node>` (`0600`) | day-1 marker | Insecure apply done for that node |
| `.bootstrapped` (`0600`) | day-1 marker | Etcd bootstrap done (nodes[0]) |

### 5.3 Extra vars

| Var | Play | Default | Effect |
| --- | --- | --- | --- |
| `talos_cluster` | all | `acme-dev-bdo1-talos-apps-01` | Selects `talos_clusters[<name>]` (vault, endpoint, nodes). Always pass explicitly except for dev. |
| `talos_bootstrap_mode` | day-1 | `auto` | `--mode` for insecure `apply-config` (`auto` / `no-reboot` / …). |
| `upgrade_image` | day-2 | `""` (no upgrade) | Maps to `talos_operate_upgrade_image`; when set, each node runs `talosctl upgrade -n <ip> -i <image>`. Example: `factory.talos.dev/metal-installer/<id-from-build-schematic-*.id>:v1.14.0`. |
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

`build/` is gitignored (see `talos/.gitignore` — `ansible/build/`), so a
fresh clone starts with an EMPTY `build/<cluster>/`. Most of it
re-renders, but three files cannot be re-created from the repo alone —
back them up off-machine (encrypted) after every successful day-0/day-1.

### 6.1 What to back up

| File (`build/<cluster>/...`) | Class | Why |
| --- | --- | --- |
| `secrets.bundle.yml` | **CRITICAL** | Cluster PKI root. `gen secrets` mints fresh PKI — a new bundle does NOT match an installed cluster (rotation orphans it: `gen config --with-secrets` derives endpoint certs from the bundle). There is no re-derive path on a live cluster. Lose this, lose authenticated API access. |
| `talosconfig` | Convenience | Rebuildable from the saved bundle via `-e regen_talosconfig=true` (§3.5), but keep a copy to skip the dance. |
| `kubeconfig` | Convenience | Re-fetchable via `-e regen_kubeconfig=true` (§3.5), but keep a copy. |
| `proton-pass-pat` | Re-mintable | Re-renders from the vault via day-0 inject (needs `eso-proton-pass`/`pat` in the cluster vault + login). Keep a copy anyway — `pat_apply` needs it offline. |

Regenerable from repo + network (no backup needed): rendered
`patches.yml` / `nodes-<node>-patches.yml` (day-0 inject), staged
`schematics-<node>.yml` + `.id` / `.sha256` (factory upload — same bytes
→ same ID), per-node `nodes/<node>/*.yaml` (day-0 `gen config`,
drift-detect by diffing), markers (`.installed-*`, `.bootstrapped` — but
see the `.bootstrapped` caveat in §6.4).

Back up (example — encrypted archive off-machine, per cluster):

```bash
# Working dir: talos/ansible/
C=acme-dev-bdo1-talos-apps-01
tar -czf - build/$C/secrets.bundle.yml build/$C/talosconfig \
  build/$C/kubeconfig build/$C/proton-pass-pat | \
  gpg --symmetric --cipher-algo AES256 -o ~/talos-$C-backup.tgz.gpg
# Store ~/talos-$C-backup.tgz.gpg OFF this machine (USB / password manager
# file field). Verify: gpg -d ~/talos-$C-backup.tgz.gpg | tar -tz
```

### 6.2 Fresh-clone restore — day-1 (pre-install cluster)

Day-1 needs the day-0 machine configs + `talosconfig`:

| Need | Source |
| --- | --- |
| `build/<cluster>/talosconfig` | Restore from backup **or** regenerate: day-0 re-run needs the bundle below, so restore the bundle first, then `-e regen_talosconfig=true` (§3.5) |
| `build/<cluster>/nodes/<node>/<type>.yaml` | Re-render: needs `secrets.bundle.yml` (restore from backup — never `gen secrets` fresh for an installed cluster; pre-install a fresh bundle is fine) + repo sources + vault login + factory reachability |

Procedure:

```bash
# Working dir: talos/ansible/
C=acme-dev-bdo1-talos-apps-01
gpg -d ~/talos-$C-backup.tgz.gpg | tar -xzf -   # restores build/$C/{secrets.bundle.yml,talosconfig,kubeconfig,proton-pass-pat}
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=$C   # re-renders patches, schematics, node yamls
ansible-playbook playbooks/day1.yml -i localhost, -e talos_cluster=$C   # apply + bootstrap + kubeconfig
```

### 6.3 Fresh-clone restore — day-2 (installed cluster)

Day-2 needs everything day-1 made, plus the factory IDs and rendered
patches:

| Need | Source |
| --- | --- |
| `build/<cluster>/talosconfig` | Restore from backup, or `-e regen_talosconfig=true` (rebuilds from the restored bundle — never mints new PKI) |
| `build/<cluster>/kubeconfig` | Restore from backup, or `-e regen_kubeconfig=true` |
| `build/<cluster>/schematic-<node>.id` | **Hard requirement** unless passing explicit `-e upgrade_image=...`: day-2 `upgrade_talos_version` and `reapply_configs` read the `.id` per node (slurp, never re-uploads). Re-creatable: day-0 re-run re-uploads (same bytes → same ID), but that needs factory reachability — restore from backup when offline. |
| `build/<cluster>/secrets.bundle.yml` | Restore from backup. Then `-e reapply_configs=true` re-renders patches (`--with-secrets` reuses the bundle, never rotates). |
| Rendered `patches.yml` + `nodes-<node>-patches.yml` | Re-render via day-0 (needs vault login) or day-2 `-e reapply_configs=true` (forced re-render, bypasses `creates:`). |
| `build/<cluster>/nodes/<node>/*.yaml` | Re-render via day-0; diff against the live config for drift detection before any re-apply. |
| `build/<cluster>/proton-pass-pat` | Re-render via day-0 inject (needs vault login), or restore — `pat_apply` only reads the file. |

Procedure:

```bash
# Working dir: talos/ansible/
C=acme-dev-bdo1-talos-apps-01
gpg -d ~/talos-$C-backup.tgz.gpg | tar -xzf -   # restores bundle + talosconfig + kubeconfig + PAT
ansible-playbook playbooks/day0.yml -i localhost, -e talos_cluster=$C   # re-renders patches, .id files, node yamls
ansible-playbook playbooks/day2.yml -i localhost, -e talos_cluster=$C   # read-only health check first
# Then opt in: -e reapply_configs=true / -e pat_apply=true / upgrades (§3.2)
```

### 6.4 What NEVER to do

- **Never commit `build/`** — it holds live PKI, admin configs, and the
  vault PAT. Gitignored by design; keep it that way.
- **Never re-bootstrap a live cluster** — deleting
  `build/<cluster>/.bootstrapped` and re-running day-1 would run
  `talosctl bootstrap` against an already-bootstrapped etcd (split-brain).
  The marker restore is informational only.
- **Never delete `secrets.bundle.yml` on a live cluster** — a fresh
  `gen secrets` mints a NEW PKI bundle that does not match the installed
  nodes (see §1.7 rotation warning). Restore from backup instead.
- **Never `rm -rf build/<cluster>` on a live cluster without a backup** —
  the re-render path needs the SAME bundle; without it day-0 mints fresh
  PKI and every authenticated call fails.
- **Never seed `proton-pass-pat` back into the vault** — it is the vault
  credential itself; it lives in `build/` (gitignored) and flows only to
  the day-2 ESO Secret apply.
