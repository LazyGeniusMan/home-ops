# helm-rclone-sync

Env-only rclone periodic sync as a Kubernetes CronJob. Every rclone remote is
built exclusively from `RCLONE_CONFIG_*` environment variables — this chart
creates **no `rclone.conf` file** (no ConfigMap, Secret, volume, mount, or
`--config` flag for one) and every rendered container sets
`RCLONE_CONFIG=/dev/null`, the documented equivalent of
`rclone --config /dev/null`, so no config file can be read or auto-created.

One generic CronJob template serves **all 10 sync directions** — each direction
is expressed purely via `source.type` / `destination.type`. Shared logic
lives in `templates/_helpers.tpl` behind the `helm-rclone-sync.*` prefix.

Consumed in Flux by six wrappers pinning the exact chart version (no
`$imagepolicy` marker — Helm OCIRepositories are untracked):
`flux/apps/components/{coder,clickstack}/base/rclone-sync-*.yaml`,
`flux/infra/components/{zitadel,clickhouse,cnpg,dragonfly}/configs/base/rclone-sync*.yaml`.

## Install / upgrade

Published as an OCI artifact at
`ghcr.io/lazygeniusman/home-ops/projects/helm-rclone-sync` (chart version = SemVer,
e.g. `0.1.0`):

```bash
helm upgrade --install rclone-nightly \
  oci://ghcr.io/lazygeniusman/home-ops/projects/helm-rclone-sync \
  --version 0.1.0 \
  --values my-sync-values.yaml
```

From a local checkout:

```bash
helm upgrade --install rclone-nightly ./projects/helm-rclone-sync \
  --values my-sync-values.yaml
```

Verify before applying:

```bash
helm lint projects/helm-rclone-sync
bash projects/helm-rclone-sync/ci/verify.sh   # lint + all 10 directions + guards
```

## Sync directions

`source` and `destination` share one shape; only `type` changes:

| # | source | destination | volumes rendered |
|---|--------|-------------|------------------|
| 1 | pvc-rwo | s3 | `src` PVC `existingClaim` |
| 2 | pvc-rwx | s3 | `src` PVC `existingClaim` |
| 3 | pvc-rwo | proton-drive | `src` PVC `existingClaim` |
| 4 | pvc-rwx | proton-drive | `src` PVC `existingClaim` |
| 5 | s3 | proton-drive | none (remote→remote) |
| 6 | s3 | pvc-rwo | `dst` PVC `existingClaim` |
| 7 | s3 | pvc-rwx | `dst` PVC `existingClaim` |
| 8 | proton-drive | pvc-rwo | `dst` PVC `existingClaim` |
| 9 | proton-drive | pvc-rwx | `dst` PVC `existingClaim` |
| 10 | proton-drive | s3 | none (remote→remote) |

Each direction has a proof fixture under `ci/` (`values-direction-01-…​` through
`values-direction-10-…​`); render one with e.g.:

```bash
helm template demo ./projects/helm-rclone-sync \
  -f projects/helm-rclone-sync/ci/values-direction-05-s3-to-proton.yaml
```

## Endpoint model

```yaml
source:                       # same shape for destination
  type: pvc-rwo               # pvc-rwo | pvc-rwx | s3 | proton-drive
  remoteName: ""              # override for RCLONE_CONFIG_<REMOTE>_* (uppercased);
                              # empty defaults to SRC (source) / DST (destination)
  uri: {value: data}          # pvc-*: claim name | s3: bucket/path | proton-drive: path
  credentials: {}             # per-backend map, ignored for pvc-*
```

PVC endpoints mount the **existing** claim by name only; the chart never
creates, resizes, or evicts PVCs. The source mount is `readOnly: true`;
the destination mount is writable.

Remote endpoints render as `REMOTE:path` args (e.g. `DST:my-bucket/backups`)
with one `RCLONE_CONFIG_<REMOTE>_<KEY>` env var per backend option:

- **s3**: `TYPE=s3` (fixed) + required `PROVIDER`, `ACCESS_KEY_ID`,
  `SECRET_ACCESS_KEY`, `REGION`; optional `ENDPOINT`, `ENV_AUTH`.
- **proton-drive**: `TYPE=protondrive` (fixed) + required `USERNAME`,
  `PASSWORD`; optional `MAILBOX_PASSWORD`, `OTP_SECRET_KEY`, `2FA`,
  `CLIENT_UID`, `CLIENT_ACCESS_TOKEN`, `CLIENT_REFRESH_TOKEN`.

## Value sources

Every `uri` and every credential field accepts four sources — a plain-string
literal shorthand, or a map with exactly one of `value`, `secretRef`,
`configMapRef`, `esoRef` (`existingSecret` is an alias):

```yaml
# (a) literal
uri: {value: my-bucket/backups}
uri: my-bucket/backups            # plain-string shorthand, same thing

# (b) Secret key
secretAccessKey:
  secretRef: {name: rclone-s3-credentials, key: secret-access-key}   # -> secretKeyRef

# (c) ConfigMap key
endpoint:
  configMapRef: {name: rclone-s3-config, key: endpoint}              # -> configMapKeyRef

# (d) ESO-synced Secret (consume, NOT create — the chart creates zero
#     ExternalSecret objects; ESO must already sync this Secret)
password:
  esoRef: {name: rclone-proton-eso, key: password}                   # -> secretKeyRef
```

`ci/values-sources-all-four.yaml` proves each source for at least one `uri`
and one credential field in a single render. Ref-sourced remote URIs render via
an intermediate env var (`SRC_PATH`/`DST_PATH`) with Kubernetes `$(VAR)`
expansion in the arg (e.g. `SRC:$(SRC_PATH)`), because `valueFrom` cannot feed
an arg directly.

Two fail-fast rules are enforced at render time with clear `required`/`fail`
messages:

- Missing required credential (e.g. S3 without `secretAccessKey`) aborts the
  render naming the field (`…​ is required`).
- PVC `uri` via `secretRef`/`configMapRef`/`esoRef` aborts: `claimName` cannot
  use `valueFrom`, so PVC claims must be literals naming the existing claim.

## Schedule and version

```yaml
schedule: "0 2 * * *"     # -> CronJob spec.schedule verbatim
timeZone: ""              # e.g. "America/New_York"; needs controller support (>= K8s 1.27)
suspend: false            # true pauses the schedule without uninstalling
image: {repository: rclone/rclone, tag: "1.75.0"}  # exact pin, never "latest"
rclone:
  version: ""             # overrides image.tag when set (exact pin, never "latest")
  operation: sync         # sync (mirror, deletes extras) or copy; nothing long-lived
  extraArgs: []           # e.g. ["--transfers=4", "--stats-one-line"]
```

Job knobs (`concurrencyPolicy`, `restartPolicy`, `backoffLimit`,
history limits, `ttlSecondsAfterFinished`, `activeDeadlineSeconds`)
are configurable in `values.yaml`. Only `sync`/`copy` render; anything
else fails fast.

## Proton obscure step

The ProtonDrive backend rejects plain-text secrets: `password`,
`mailbox_password`, and `otp_secret_key` must be in `rclone obscure` format.
Generate each value (run `rclone` ≥ 1.75 locally or via the chart image) and
store the **obscured** string in your Secret/ESO source:

```bash
rclone obscure 'my-proton-password'        # paste the output into your Secret
rclone obscure 'my-mailbox-password'
```

Smoke-check a live container (exec into a Job Pod created by the CronJob):

```bash
rclone listremotes -vv
rclone about DST: -vv            # or SRC:, or your remoteName: prefix
```

## Overlap safety

- `concurrencyPolicy: Forbid` (default) — a new Job is skipped while the
  previous sync still runs.
- `rclone bisync` is **not** offered: it needs persistent listing state
  a stock CronJob does not provide.

## RWO same-node caveat

For `pvc-rwo`, the claim attaches to one node at a time: the sync Pod must
land on the owner's node or it stays `Pending`, blocking later schedules
under `Forbid`. Prefer `coLocateWith` (follows the owner) over a static
hostname pin:

```yaml
coLocateWith:
  matchLabels: {app: my-db}   # owning workload's pod labels
  # matchExpressions: [...]   # or expressions; optional topologyKey
                              # (default kubernetes.io/hostname) + namespaces
```

This renders a required `podAffinity` term (default
`topologyKey: kubernetes.io/hostname`) **merged** with any user-supplied
`affinity`. Empty `coLocateWith` (default) generates nothing; fall back to
manual pinning (`nodeSelector`/`affinity`/`tolerations`), which goes stale
when the owner moves. `pvc-rwx` has no such constraint; the chart mounts
both types identically and the claim's access mode governs attach.

## Snapshot staging (optional, external)

The chart creates **no** `VolumeSnapshot` objects. An external scheduler
snapshots the live claim and restores to a staging claim; the chart syncs
from staging (`source: {type: pvc-rwx, uri: {value: my-db-snap-staging}}`,
no `coLocateWith`). See `examples/snapshot-staging.yaml`.

Prerequisites (external): snapshot-capable CSI + external-snapshotter +
`VolumeSnapshotClass`. **Not** `local-ssd-nvme` (not snapshottable) —
those stay on the live-claim + `coLocateWith` path. Snapshots are
crash-consistent only unless quiesced; budget ~2x transient storage; the
external scheduler owns the snapshot/restore/cleanup lifecycle.

## values.schema.json

No `values.schema.json`: validation is fail-fast template guards
(`required`/`fail` with field-naming messages).

## Helm merge semantics (read before `--set`)

Helm deep-merges maps at the field level: replace **whole endpoints**
via `--set-json` (a bare omit inherits the demo default), and prefer
whole-file `-f` values (like the `ci/` fixtures) over `--set`. See
`ci/verify.sh` for the merge-semantics proofs.

## Verifying

```bash
helm lint projects/helm-rclone-sync
bash projects/helm-rclone-sync/ci/verify.sh
# env-only proof for all renders:
for f in projects/helm-rclone-sync/ci/values-*.yaml; do
  helm template demo ./projects/helm-rclone-sync -f "$f"
done | grep -ri rclone.conf   # must print nothing
```
