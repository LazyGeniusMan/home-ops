# gateway-api

Gateway API v1.6.1 standard channel (`crds/base/standard-install.yaml`: 10
CRDs + 2 ValidatingAdmissionPolicies, vendored whole from the upstream
release asset -- experimental not adopted), the shared
`GatewayClass/cilium` (served by `io.cilium/gateway-controller`), the
shared `Gateway/main` (HTTP 80 + HTTPS 443), and base `HTTPRoute`
redirects. Per-service routes attach later.

CRD delivery: the bundle lives in `crds/` and renders through the fleet's
prune:false `infra-crds` Kustomization -- a removed CRD file never
cascade-deletes CRs. `controllers/base` is an empty Kustomization so
`infra-controllers` keeps prune:true; bumps re-vendor the whole file
(marker + policy range move together, see the header in
`crds/base/standard-install.yaml` and
`flux/infra/update-policies/gateway-api.yaml` >=1.6.1).

## TLS: namespace-local wildcard Certificates

A `Gateway` listener only references a Secret in its own namespace, so the
cert-manager-namespace `wildcard-home-ops-tls` Secret cannot feed this
Gateway. `configs/base/wildcard-certificate.yaml` mints a duplicate
`Certificate/wildcard-home-ops` here -- same `ClusterIssuer/letsencrypt`,
same `secretName: wildcard-home-ops-tls` -- with SANs `*.<base>` +
`*.zitadel.<base>` (NetBird login host) + `*.coder.<base>` (coder
workspaces; wildcards match one label only, so each nested shape needs its
own SAN) -- plus its own copy of the `cloudflare-api-token` ExternalSecret
(same Proton Pass remoteRef,
`pass://<cluster>/cert-manager/cloudflare-api-token`, Zone:Read + DNS:Edit).

## HTTP->HTTPS

Port 80 is a redirect source only: every base route carries a
`RequestRedirect` filter (301 to `https`). Hostname matches dots to the
left per spec, covering `<service>.home-ops...` and nested
`<subservice>.<service>.home-ops...` shapes.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | data-plane per-node via Cilium (N/A) | TLS `wildcard-home-ops-dev-tls`, hostnames `home-ops-dev.yansyah.my.id` |
| `prd` | data-plane per-node via Cilium (N/A) | TLS `wildcard-home-ops-tls`, hostnames `home-ops.yansyah.my.id` |

The `Certificate` uses the shared `ClusterIssuer/letsencrypt` whose ACME
server is set per environment by the cert-manager overlays. Controllers
track `../base` with no patches.

## Telemetry / monitoring / updates

No telemetry knobs in the upstream manifests. No ServiceMonitors ship;
Gateway data-plane metrics come via Cilium. Bumps via
`update-policies/gateway-api.yaml` -> PR automation (the `$imagepolicy`
marker is the version comment in `crds/base/standard-install.yaml`).
