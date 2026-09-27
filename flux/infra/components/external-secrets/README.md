# external-secrets

External Secrets Operator v2.11.0 (`oci://ghcr.io/external-secrets/charts/external-secrets`) + in-cluster Proton Pass webhook (`projects/eso-proton-pass`, image `ghcr.io/lazygeniusman/home-ops/projects/eso-proton-pass:dev`).

## Layout

- `controllers/base`: ESO chart (`OCIRepository` + `HelmRelease`).
- `configs/base`: `ClusterSecretStore/proton-pass` (all namespaces), eso-proton-pass `Deployment`/`Service`.

## Secret addressing

`pass://<cluster>/{namespace}/{field}`. Per-namespace `ExternalSecret` objects live in each consuming component. Refresh `1h` + `retrySettings` (maxRetries 5, retryInterval 5m).

## First-sync ordering

`ClusterSecretStore/proton-pass` and the webhook live in one Kustomization by design. On a fresh cluster expect fail-then-heal (`Ready=False` until the webhook Deployment is Ready; self-heals via store `retrySettings` + per-secret `refreshInterval`). Assert the `proton-pass-pat` bootstrap Secret exists first; alert past ~10m.

## Bootstrap (one-time, never committed)

```sh
pass-cli item create login --vault-name '<cluster>' --title 'external-secrets/proton-pass-pat'
pass-cli item view 'pass://<cluster>/external-secrets/proton-pass-pat/pat' \
  | kubectl -n external-secrets create secret generic proton-pass-pat --from-file=pat=/dev/stdin
```

Proton PATs expire after at most 1 year — renew before expiry (vault entry carries the expiry annotation); see the Talos Ansible RUNBOOK for the renewal procedure.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | controller HPA 1-2, webhook HPA 1-2 | controller + cert-controller seeds -> 1; webhook HPA 1 / 2; vault refs per env |
| `prd` | controller HPA 2-4, webhook HPA 2-4 | controller + cert-controller seeds -> 2; webhook HPA 2 / 4; vault refs per env |

The chart admission webhook stays singleton 1 in both overlays; the `eso-proton-pass` provider webhook is HPA-scaled with a PDB (`configs/base/eso-proton-pass-pdb.yaml`, `minAvailable: 1`).

## Updates

`update-policies/external-secrets.yaml` (chart >=2.11.0 marker `infra:external-secrets:tag` + webhook `:dev` marker `infra:eso-proton-pass:tag`, range >=0.0.0); keep chart and webhook in the same PR. Changelogs: https://github.com/external-secrets/external-secrets/releases (webhook changelog in-repo).
