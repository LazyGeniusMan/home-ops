# external-secrets (§9.1)

External Secrets Operator v2.10.0 + in-cluster Proton Pass webhook
(`projects/eso-proton-pass`, image
`ghcr.io/lazygeniusman/home-ops/infra/eso-proton-pass:dev`).

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

## Bootstrap (pass-cli, one-time, never committed)

```sh
pass insert 'acme-prd-bdo1-talos-apps-01/external-secrets/proton-pass-pat'
pass show --field=pat 'acme-prd-bdo1-talos-apps-01/external-secrets/proton-pass-pat' \
  | kubectl -n external-secrets create secret generic proton-pass-pat --from-file=pat=/dev/stdin
```

## PAT renewal runbook (no terraform move)

The Proton Pass PAT is a plain Kubernetes Secret (`proton-pass-pat` in
`external-secrets`, key `pat`) — there is intentionally no Terraform or
other manager for it; it lives in the vault + this one Secret. Proton PATs
expire after at most 1 year, so renew before expiry (the vault entry carries
an expiry annotation as the reminder):

1. `pass list` — confirm the
   `acme-prd-bdo1-talos-apps-01/external-secrets/proton-pass-pat` entry and
   its expiry annotation.
2. `pass renew` (create the replacement PAT in Proton Pass — new token,
   expiry up to 1y out). Update the vault entry (`pass insert`) with the
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

## Telemetry-off evidence

- ESO chart has no usage-reporting values (verified: no `telemetry` key in
  chart values).
- Webhook image forces `PROTON_PASS_DISABLE_TELEMETRY=1` in exec env,
  Dockerfile `ENV`, and unit test (`projects/eso-proton-pass/README.md`).

## Monitoring / updates

- Controller + webhook + cert-controller metrics `Service`s on; single
  `ServiceMonitor` with `renderMode: skipIfMissing` (safe pre-Prometheus).
- Chart bumps: `update-policies/external-secrets.yaml` (+ `:dev` webhook
  image via `infra:eso-proton-pass` policy) → update-cluster PR automation.
