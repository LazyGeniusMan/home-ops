# external-secrets

External Secrets Operator v2.11.0
(`oci://ghcr.io/external-secrets/charts/external-secrets`) + in-cluster
Proton Pass webhook (`projects/eso-proton-pass`, image
`ghcr.io/lazygeniusman/home-ops/projects/eso-proton-pass:dev`).

## Layout

- `controllers/base`: ESO chart (`OCIRepository` + `HelmRelease`).
- `configs/base`: `ClusterSecretStore/proton-pass` (all namespaces),
  eso-proton-pass `Deployment`/`Service`.

## Secret addressing

`pass://<cluster>/{namespace}/{field}`. Per-namespace `ExternalSecret`
objects live in each consuming component. Refresh `1h` + `retrySettings`
(maxRetries 5, retryInterval 5m).

## First-sync ordering

`ClusterSecretStore/proton-pass` and the webhook live in one Kustomization
by design. On a fresh cluster expect fail-then-heal (`Ready=False` until
the webhook Deployment is Ready; self-heals via store `retrySettings` +
per-secret `refreshInterval`). Assert the `proton-pass-pat` bootstrap
Secret exists first. Alert past ~10m.

## Bootstrap (one-time, never committed)

```sh
pass-cli item create login --vault-name '<cluster>' --title 'external-secrets/proton-pass-pat'
pass-cli item view 'pass://<cluster>/external-secrets/proton-pass-pat/pat' \
  | kubectl -n external-secrets create secret generic proton-pass-pat --from-file=pat=/dev/stdin
```

## PAT renewal

The PAT is a plain Kubernetes Secret (`proton-pass-pat` in
`external-secrets`, key `pat`); it lives in the vault + this one Secret.
Proton PATs expire after at most 1 year, so renew before expiry (the vault
entry carries an expiry annotation as the reminder):

1. Confirm the `external-secrets/proton-pass-pat` entry and its expiry.
2. Create the replacement PAT, update the vault entry (`pat` + expiry).
3. Recreate the Secret from stdin (never commit it).
4. `rollout restart deploy/eso-proton-pass` (the webhook reads the PAT file
   at startup).
5. Verify: webhook `/healthz` plus every proton-pass `ExternalSecret`
   `Ready=True`.

Alert on a `Ready=False` ExternalSecret and on a `401` from the webhook.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | controller HPA 1-2, webhook `eso-proton-pass` 1 (singleton) | controller + cert-controller seeds -> 1; webhook HPA 1 / 2; vault refs per env |
| `prd` | controller HPA 2-4, webhook `eso-proton-pass` 1 (singleton) | controller + cert-controller seeds -> 2; webhook HPA 2 / 4; vault refs per env |

The ESO chart webhook stays singleton 1 in both overlays -- never scale it.

## Telemetry / monitoring / updates

ESO chart has no usage-reporting values. Webhook forces
`PROTON_PASS_DISABLE_TELEMETRY=1` in exec env, Dockerfile `ENV`, and unit
test. Controller + webhook + cert-controller metrics Services on; single
`ServiceMonitor` with `renderMode: skipIfMissing`. Bumps:
`update-policies/external-secrets.yaml` (chart >=2.11.0 marker
`infra:external-secrets:tag` + webhook `:dev` marker
`infra:eso-proton-pass:tag`, range >=0.0.0) -> PR automation; keep chart
and webhook in the same PR.
Changelogs: https://github.com/external-secrets/external-secrets/releases
(webhook changelog in-repo).
