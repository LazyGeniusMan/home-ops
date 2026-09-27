# gateway-api

Gateway API v1.6.1 standard channel (first-party `helm-gateway-api` OCI chart in `crds/base/`: 10 CRDs + 2 ValidatingAdmissionPolicies, standard channel whole), the shared `GatewayClass/cilium` (served by `io.cilium/gateway-controller`), the shared `Gateway/main` (HTTP 80 + HTTPS 443), and base `HTTPRoute` redirects. Per-service routes attach later.

CRDs live in `crds/` and render through the fleet's prune:false `infra-crds` Kustomization; `controllers/base` is an empty Kustomization so `infra-controllers` keeps prune:true.

## TLS: namespace-local wildcard Certificates

A `Gateway` listener only references a Secret in its own namespace, so `configs/base/wildcard-certificate.yaml` mints a duplicate `Certificate/wildcard-home-ops` here (same `ClusterIssuer/letsencrypt`, `secretName: wildcard-home-ops-tls`) with SANs `*.<base>` + `*.zitadel.<base>` (NetBird login host) + `*.coder.<base>` (wildcards match one label only) — plus its own copy of the `cloudflare-api-token` ExternalSecret (`pass://<cluster>/cert-manager/cloudflare-api-token`).

## HTTP->HTTPS

Port 80 is a redirect source only: every base route carries a `RequestRedirect` filter (301 to `https`).

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | data-plane per-node via Cilium (N/A) | TLS `wildcard-home-ops-dev-tls`, hostnames `home-ops-dev.yansyah.my.id` |
| `prd` | data-plane per-node via Cilium (N/A) | TLS `wildcard-home-ops-tls`, hostnames `home-ops.yansyah.my.id` |

Controllers track `../base` with no patches.

## Updates

Chart.yaml + wrapper `ref.tag` together (no ImagePolicy/marker, atomic hand-bump), human merges — paired with the Cilium minor (see `flux/infra/update-policies/cilium.yaml`).
