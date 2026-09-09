# Rclone (§12.2)

Scheduled vault sync: SeaweedFS S3 bucket → Proton Drive, image
`rclone/rclone:1.75.1`, as a customizable CronJob template.

## Layout (environment-direct, apps area)

`base/` holds every manifest (CronJob `rclone-vault-sync` + secrets
ExternalSecret); env overlays `{dev,stg,prd}/` patch vault refs
and the S3 endpoint via `resources: [../base]`. Tenant is `apps/rclone` via
`flux/apps/update-policies/rclone.yaml`.

## Job shape (parameterized knobs)

Four plain env vars define each sync — patch them per environment or per
new job without touching the image args:

| Var | Base | Prd | Stg |
| --- | --- | --- | --- |
| `SOURCE` | `sw:rclone-vault` | same | same |
| `DEST` | `drive:/home-ops-vault` | same | same |
| `SYNC_FLAGS` | `--dry-run --verbose` | `--verbose --transfers=4 --checkers=8` (live) | dry-run (base) |
| `RETENTION_DAYS` | `30` (`--max-age`) | `30` | `30` |

Schedule: base hourly; prd nightly `0 3 * * *`; stg
`suspend: true` (manual drill: un-suspend + `kubectl create job`).
`concurrencyPolicy: Forbid`, `backoffLimit: 2`, 1h deadline.
Copy the CronJob block for new buckets and change
`SOURCE`/`DEST`/`schedule` — that is the intended extension pattern.

## Env-only config (NO rclone.conf)

Remotes are pure `RCLONE_CONFIG_*` env vars — no config file is mounted:

- `sw` remote: `type=s3 provider=Other endpoint=http://
  seaweed-main-s3.seaweedfs.svc.cluster.local:8333 force_path_style=true`,
  keys from the `rclone-credentials` Secret (§12.1 in-cluster endpoint,
  FQDN).
- `drive` remote: `type=protondrive`, credentials from the same Secret.
  Key names (`drive-username/password/token`) must match the rclone
  protondrive backend options — run `rclone config` once locally and
  align any drift before go-live.

## Credentials

Two ExternalSecrets target the SAME `rclone-credentials` Secret (one
store per ES):

- `rclone-cosi-s3`: the S3 pair (`sw-access-key`/`sw-secret-key`), synced
  by ESO from the COSI-minted BucketInfo JSON (Secret
  `rclone-vault-cosi-creds`, key `BucketInfo`) through the in-namespace
  `rclone-cosi` SecretStore — GJSON `property` extracts
  `spec.secretS3.accessKeyID/accessSecretKey`.
- `rclone-credentials`: the Drive triple (`drive-username`,
  `drive-password`, `drive-token`) still from Proton Pass
  (`pass://acme-prd-bdo1-talos-apps-01/rclone/*`); seed those vault
  entries with pass-cli.

Decision: COSI, not the operator S3 CRDs. The operator S3 CRDs were
evaluated and NOT created — they would mint a SECOND identity/keypair on
the same `rclone-vault` bucket, duplicating the identity COSI already
provides for no benefit. COSI stays the single S3-identity source here.

## Environments

Base ships dry-run so a mis-applied overlay can only log. Prd
flips to live flags + nightly schedule. Stg is suspended dry-run.
PVC mode: the CronJob carries a commented `volumeMounts`/`volumes`
stanza — uncomment, point `SOURCE` at `/data`, set `claimName` to back
up a volume instead of a bucket.

## Dry-run + scheduled sync shape

First deploy is always dry-run: check CronJob logs
(`rclone sync … --dry-run`), confirm the file list, then flip
`SYNC_FLAGS` live. `--max-age="${RETENTION_DAYS}d` bounds each run to
recent changes for the vault use case; drop it for full mirrors.

## Telemetry-off / monitoring / updates

- rclone has no phone-home/telemetry; `--rc` is not enabled (unguarded
  monitors OFF). Evidence: rclone docs expose no usage-reporting flag;
  this job sets none and opens no metrics port.
- Job health via `kube-state-metrics` (`kube_cronjob_*`,
  `kube_job_failed`) once the monitoring stack lands; alert on
  consecutive `rclone-vault-sync` failures and missed schedules then.
  No ServiceMonitor here — nothing serves metrics yet.
- Image auto-tracks via `update-policies/rclone.yaml`
  (`rclone/rclone:1.75.1` marker).
