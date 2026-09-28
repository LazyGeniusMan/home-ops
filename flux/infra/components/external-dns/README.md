# external-dns

ExternalDNS chart 1.22.0 + app v0.22.0 (`oci://ghcr.io/controlplaneio-fluxcd/charts/external-dns`, `image.tag` override) over `home-ops.yansyah.my.id` — the NetBird-only DNS path: `external-dns-netbird` webhook sidecar (`projects/external-dns-netbird`, `:dev`) via NetBird Custom Zones. No Cloudflare provider and no `Record`/`DNSEndpoint` CRs in `flux/` — Gateway API HTTPS routes are the only sync source.

TXT ownership: `txtOwnerId`/`txtPrefix` pin the single txt registry (`home-ops-prd-netbird`/`extdns-nb-`); `policy: sync`, `registry: txt`. Sources: `service`, `ingress`, `gateway-httproute`, `crd` (grpcroute/tlsroute dropped — no such routes in `flux/`). Gateway/HTTPRoutes produce `*.home-ops.yansyah.my.id` -> LB VIP (prd `.199`, dev `.249`).

## Ordering

The configs `ExternalSecret` resolves through `ClusterSecretStore/proton-pass` (external-secrets configs/), which needs the eso-proton-pass webhook Ready plus the `proton-pass-pat` bootstrap. On a fresh cluster expect fail-then-heal until ESO syncs (see the external-secrets README); alert past ~10m. Dev and prd TXT scope lives in `controllers/{dev,prd}`; vault keys live in `configs/{dev,prd}` — no second writer fights prd over the TXT registry.

## Credentials

`NETBIRD_PAT_FILE` from the ESO-synced `netbird-pat` secret; seed `pass://<cluster>/external-dns/netbird-pat` with pass-cli.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | 1 (singleton; chart schema caps `replicaCount` at 1) | TXT scope `home-ops-dev-netbird`, domain `home-ops-dev.yansyah.my.id`, dev vault key |
| `prd` | 1 (singleton; chart schema caps `replicaCount` at 1) | TXT scope `home-ops-prd-netbird`, domain `home-ops.yansyah.my.id`, prd vault key |

## Updates

`update-policies/external-dns.yaml` (chart >=1.22.0 marker `infra:external-dns:tag` + runtime app marker `infra:external-dns-app:tag` + sidecar `:dev` marker `infra:external-dns-netbird:tag`, range >=0.0.0); keep chart, app, and sidecar in the same PR. Changelog: https://github.com/kubernetes-sigs/external-dns/releases (sidecar changelog in-repo).
