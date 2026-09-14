# external-dns (§9.4)

Single ExternalDNS v0.22.0 instance (chart 1.21.1 + `image.tag` override)
over `home-ops.yansyah.my.id` — the NetBird-only DNS path:

- `external-dns-netbird`: NetBird Custom Zones via the in-repo webhook
  sidecar (`projects/external-dns-netbird`, `:dev`; `NETBIRD_PAT_FILE`
  mounted from the ESO-synced `netbird-pat` secret). No Cloudflare
  provider and no manual `Record`/`DNSEndpoint` CRs ship anywhere in
  `flux/` — the Gateway API HTTPS routes are the only sync source.

TXT ownership: `txtOwnerId`/`txtPrefix` pin the single txt registry
(`home-ops-prd-netbird`/`extdns-nb-`); `policy: sync`, `registry: txt`.

Sources (verified against
`/tmp/home-ops-docs/external-dns-docs/docs/sources/gateway-api.md`):
`service`, `ingress`, `gateway-httproute`, `gateway-grpcroute`,
`gateway-tlsroute`, `crd`.

## Ordering prerequisite runbook

The configs/ `ExternalSecret` resolves through
`ClusterSecretStore/proton-pass` (external-secrets configs/), which needs
the eso-proton-pass webhook Ready plus the out-of-band `proton-pass-pat`
bootstrap (see the external-secrets README "PAT renewal" runbook).
Cross-tenant ordering is owned by tenants/*.yaml (parallel task). On a fresh
cluster expect fail-then-heal: `Ready=False` on the secret, and the
controllers/ `HelmRelease` CrashLooping on the missing synced `netbird-pat`
Secret, until ESO syncs — heals via `refreshInterval` + Flux
`retryInterval`.
Alert past ~10m. Verify: `kubectl get clustersecretstore proton-pass`;
`kubectl -n external-dns get externalsecret,secret`.

Envs are `dev` / `prd` only: dev and prd TXT scope (`txtOwnerId`,
`domainFilters`, webhook `DOMAIN_FILTER`) lives in controllers/{dev,prd};
vault keys live in configs/{dev,prd} — so no second writer can fight prd
over the TXT registry/domain.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | 1 (netbird singleton) | TXT scope `home-ops-dev-netbird`, `domainFilters` + webhook `DOMAIN_FILTER` `homelab-dev.yansyah.my.id`, dev vault key; LB target `192.168.1.249` |
| `prd` | 1 (netbird singleton; no leader election caps `replicaCount` at 1) | TXT scope `home-ops-prd-netbird`, `domainFilters` + webhook `DOMAIN_FILTER` `home-ops.yansyah.my.id`, prd vault key; LB target `192.168.1.199` |

Upstream reference (read-only): `/tmp/home-ops-docs/external-dns-docs`.

## Wildcard record

Sources include Gateway API routes, so the §8 Gateway/HTTPRoutes produce
`*.home-ops.yansyah.my.id → 192.168.1.199` (Gateway `cilium` LB target from
the Cilium `default` LB IP pool — prd `192.168.1.199`, dev
`192.168.1.249`; see `cilium/configs/{base/lb-pool.yaml,dev,prd}`). Per-service
HTTPS `HTTPRoute`s (§§11-13, e.g. `zitadel`, `coder`) attach to the shared
`main` Gateway `https` listener, so each hostname syncs through the webhook
with no file overlap. Seed the vault entry with pass-cli
(`.../external-dns/netbird-pat`).

## Telemetry-off / monitoring / updates

- ExternalDNS reports nothing upstream. Webhook sidecar documents no
  telemetry (`projects/external-dns-netbird/README.md`).
- `ServiceMonitor` disabled until `monitoring.coreos.com` CRDs land.
- Chart + sidecar bumps: `update-policies/external-dns.yaml` → PR automation.
