# external-secrets (§9.1)

External Secrets Operator v2.11.0 + in-cluster Proton Pass webhook
(`projects/eso-proton-pass`, image
`ghcr.io/lazygeniusman/home-ops/projects/eso-proton-pass:dev`).

## Layout

- `controllers/base`: ESO chart (`OCIRepository` + `HelmRelease`).
- `configs/base`: `ClusterSecretStore/proton-pass` (all namespaces),
  `ClusterSecretStore/cosi-{cnpg,dragonfly,clickhouse}` (cross-namespace
  COSI credential bridges — see the cosi README), eso-proton-pass
  `Deployment`/`Service`, PAT bootstrap note.

## Secret addressing

`pass://acme-prd-bdo1-talos-apps-01/{namespace}/{field}`. Per-namespace
`ExternalSecret` objects live in each consuming component (`cert-manager`,
`external-dns`, later §§10-13). Refresh `1h` + `retrySettings`
(maxRetries 5, retryInterval 5m) cover rotation/retry.

## First-sync ordering runbook

`ClusterSecretStore/proton-pass` and the eso-proton-pass webhook live in one
Kustomization by design (see configs/base/kustomization.yaml). On a fresh
cluster expect fail-then-heal: the store (and downstream ExternalSecrets)
report `Ready=False` until the webhook Deployment is Ready; this self-heals
via store `retrySettings` + per-secret `refreshInterval`. Assert the
`proton-pass-pat` bootstrap Secret exists first (verify commands in
configs/base/eso-proton-pass-webhook.yaml). Alert only if `Ready=False`
persists past ~10m.

## Bootstrap (pass-cli, one-time, never committed)

```sh
pass-cli item create login --vault-name 'acme-prd-bdo1-talos-apps-01' --title 'external-secrets/proton-pass-pat'
pass-cli item view 'pass://acme-prd-bdo1-talos-apps-01/external-secrets/proton-pass-pat/pat' \
  | kubectl -n external-secrets create secret generic proton-pass-pat --from-file=pat=/dev/stdin
```

## PAT renewal runbook

The Proton Pass PAT is a plain Kubernetes Secret (`proton-pass-pat` in
`external-secrets`, key `pat`) — there is intentionally no Terraform or
other manager for it; it lives in the vault + this one Secret. Proton PATs
expire after at most 1 year, so renew before expiry (the vault entry carries
an expiry annotation as the reminder):

1. `pass-cli item list 'acme-prd-bdo1-talos-apps-01'` — confirm the
   `external-secrets/proton-pass-pat` entry and its expiry annotation.
2. Create the replacement PAT in Proton Pass (new token, expiry up to 1y
   out). Update the vault entry (`pass-cli item update`) with the
   new `pat` value + new expiry annotation.
3. `kubectl -n external-secrets create secret generic proton-pass-pat
   --from-file=pat=/dev/stdin --dry-run=client -o yaml | kubectl apply -f -`
   (feed the new PAT on stdin — never commit it).
4. `kubectl -n external-secrets rollout restart deploy/eso-proton-pass`
   (the webhook reads the PAT file at startup, so a restart picks it up).
5. Verify: `curl` the webhook `/healthz`, and check every proton-pass
   `ExternalSecret` reports `Ready=True`
   (`kubectl get externalsecrets -A -o wide` — alert on `Ready=False`).

Expiry/401 alerting: alert on a `Ready=False` ExternalSecret (stale or
revoked PAT surfaces there) and on a `401` from the webhook (the pass-cli
login fails first). Rotate at most yearly; an early rotation is just steps
2–5 again.

## Upgrade runbook

- Version source: chart `version:` in
  `controllers/base/externalsecrets.yaml` (v2.11.0) plus the webhook
  image in `configs/base/eso-proton-pass-webhook.yaml` (`:dev`).
- Changelog (chart):
  https://github.com/external-secrets/external-secrets/releases.
  Webhook changelog is in-repo (`projects/eso-proton-pass`).
- Bump: let the ImagePolicy PRs land (markers in both files above,
  `update-policies/external-secrets.yaml`); keep chart and webhook in
  the same PR.
- Verify: `ClusterSecretStore/proton-pass` reports `Ready=True`, every
  `ExternalSecret` re-syncs (`kubectl get externalsecrets -A -o wide`
  — alert on `Ready=False`), and the PAT renewal runbook above still
  applies unchanged.

## Telemetry-off / monitoring / updates

- ESO chart has no usage-reporting values.
- Webhook image forces `PROTON_PASS_DISABLE_TELEMETRY=1` in exec env,
  Dockerfile `ENV`, and unit test (`projects/eso-proton-pass/README.md`).

- Controller + webhook + cert-controller metrics `Service`s on; single
  `ServiceMonitor` with `renderMode: skipIfMissing`.
- Chart bumps: `update-policies/external-secrets.yaml` (+ `:dev` webhook
  image via `infra:eso-proton-pass:tag` policy) → update-cluster PR automation.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | controller HPA 1–2, webhook `eso-proton-pass` 1 (singleton) | controller + cert-controller seeds → 1 in `controllers/dev`; `eso-proton-pass` HPA → 1 / 2 in `configs/dev`; vault refs per env |
| `prd` | controller HPA 2–4, webhook `eso-proton-pass` 1 (stays singleton) | controller + cert-controller seeds → 2 in `controllers/prd`; `eso-proton-pass` HPA → 2 / 4 in `configs/prd`; vault refs per env |

The Proton Pass webhook is a singleton in every env (base `replicas: 1`
is the create-time seed; HPAs own the controller counts). The ESO chart
webhook stays singleton 1 in both env overlays — never scale it.

Upstream reference (read-only): `/tmp/home-ops-docs/external-secret-operator-docs`.
