# gateway-api (§8.1)

Gateway API v1.6.1 standard channel (`standard-install.yaml` CRDs), the
shared `GatewayClass/cilium` (served by `io.cilium/gateway-controller` —
§8.2 runs Cilium with Gateway API enabled), the shared `Gateway/main`
(HTTP 80 + HTTPS 443 for `home-ops.yansyah.my.id`), and base `HTTPRoute`
redirects. Per-service routes attach later (§§11-13).

## TLS: why a second wildcard Certificate

cert-manager Secrets are namespace-local: a `Gateway` listener can only
reference a Secret in its own namespace, so the §9 `Certificate` (which
writes `wildcard-home-ops-tls` into the `cert-manager` namespace) cannot
feed this Gateway. `configs/base/wildcard-certificate.yaml` therefore mints
a duplicate `Certificate/wildcard-home-ops` here — same
`ClusterIssuer/letsencrypt`, same `dnsNames: ["*.home-ops.yansyah.my.id"]`,
same `secretName: wildcard-home-ops-tls` — plus its own copy of the
`cloudflare-api-token` ExternalSecret (same Proton Pass remoteRef as
`cert-manager/configs/base/cluster-issuer.yaml`,
`pass://acme-prd-bdo1-talos-apps-01/cert-manager/cloudflare-api-token`,
token needs Zone:Read + DNS:Edit). Let's Encrypt permits duplicate wildcard
orders, so both Certificates stay valid side by side.

## HTTP→HTTPS

Port 80 exists only as a redirect source: EVERY base route carries a
`RequestRedirect` filter (301 to `https`). The `*.home-ops.yansyah.my.id`
hostname matches dots to the left per the Gateway API spec, covering both
`<service>.home-ops.yansyah.my.id` and nested
`<subservice>.<service>.home-ops.yansyah.my.id` shapes.

## Environments

`production` and `staging` overlays both track `../base` with no patches;
the `Certificate` uses the shared `ClusterIssuer/letsencrypt` whose ACME
server is set per environment by the cert-manager overlays.

## Telemetry-off / monitoring / updates

- No telemetry knobs exist in the upstream standard-install manifests
  (raw CRDs + admission webhook, no reporting flags) — nothing to disable.
- No ServiceMonitors ship in raw CRD manifests; Gateway data-plane metrics
  come via Cilium (§8.2).
- Version bumps via `update-policies/gateway-api.yaml` → PR automation (the
  `$imagepolicy` marker is the version comment in
  `controllers/base/standard-install.yaml`).
