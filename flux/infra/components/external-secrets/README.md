# external-secrets (§9.1)

External Secrets Operator v2.10.0 + in-cluster Proton Pass webhook
(`projects/eso-proton-pass`, image
`ghcr.io/lazygeniusman/home-ops/infra/eso-proton-pass:dev`).

## Layout

- `controllers/base`: ESO chart (`OCIRepository` + `HelmRelease`).
- `configs/base`: `ClusterSecretStore/proton-pass` (all namespaces),
  eso-proton-pass `Deployment`/`Service`, PAT bootstrap note.

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
