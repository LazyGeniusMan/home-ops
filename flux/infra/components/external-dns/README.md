# external-dns

Single ExternalDNS chart 1.22.0 + app v0.22.0
(`oci://ghcr.io/controlplaneio-fluxcd/charts/external-dns`, `image.tag`
override) over `home-ops.yansyah.my.id` -- the NetBird-only DNS path:

- `external-dns-netbird`: NetBird Custom Zones via the in-repo webhook
  sidecar (`projects/external-dns-netbird`, `:dev`; `NETBIRD_PAT_FILE`
  from the ESO-synced `netbird-pat` secret). No Cloudflare provider and no
  `Record`/`DNSEndpoint` CRs in `flux/` -- Gateway API HTTPS routes are the
  only sync source.

TXT ownership: `txtOwnerId`/`txtPrefix` pin the single txt registry
(`home-ops-prd-netbird`/`extdns-nb-`); `policy: sync`, `registry: txt`.
Sources: `service`, `ingress`, `gateway-httproute`, `gateway-grpcroute`,
`gateway-tlsroute`, `crd`.

## Ordering

The configs `ExternalSecret` resolves through
`ClusterSecretStore/proton-pass` (external-secrets configs/), which needs
the eso-proton-pass webhook Ready plus the `proton-pass-pat` bootstrap.
Cross-tenant ordering is owned by tenants/*.yaml. On a fresh cluster expect
fail-then-heal (`Ready=False` secret, CrashLooping HelmRelease) until ESO
syncs -- heals via `refreshInterval` + Flux `retryInterval`. Alert past
~10m.

Dev and prd TXT scope (`txtOwnerId`, `domainFilters`, webhook
`DOMAIN_FILTER`) lives in controllers/{dev,prd}; vault keys live in
configs/{dev,prd} -- no second writer fights prd over the TXT registry.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | 1 (singleton; chart schema caps `replicaCount` at 1) | TXT scope `home-ops-dev-netbird`, domain `home-ops-dev.yansyah.my.id`, dev vault key |
| `prd` | 1 (singleton; chart schema caps `replicaCount` at 1) | TXT scope `home-ops-prd-netbird`, domain `home-ops.yansyah.my.id`, prd vault key |

## Wildcard record

Gateway API routes are sync sources, so the Gateway/HTTPRoutes produce
`*.home-ops.yansyah.my.id` -> LB VIP (prd `.199`, dev `.249`). Per-service
HTTPS HTTPRoutes attach to the shared `main` Gateway `https` listener, so
each hostname syncs with no file overlap. Seed the vault entry
(`.../external-dns/netbird-pat`) with pass-cli.

## Telemetry / monitoring / updates

ExternalDNS reports nothing upstream; the sidecar documents no telemetry.
`ServiceMonitor` off. Bumps: `update-policies/external-dns.yaml`
(chart >=1.22.0 marker `infra:external-dns:tag` + sidecar `:dev` marker
`infra:external-dns-netbird:tag`, range >=0.0.0) -> PR automation; keep
chart and sidecar in the same PR.
Changelog: https://github.com/kubernetes-sigs/external-dns/releases
(sidecar changelog in-repo).
