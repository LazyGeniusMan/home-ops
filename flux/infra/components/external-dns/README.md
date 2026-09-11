# external-dns (§9.4)

Two synced ExternalDNS v0.22.0 instances (chart 1.21.1 + `image.tag`
override) over `home-ops.yansyah.my.id`:

- `external-dns-cloudflare`: public DNS (`CF_API_TOKEN` from ESO).
- `external-dns-netbird`: NetBird Custom Zones via the in-repo webhook
  sidecar (`projects/external-dns-netbird`, `:dev`; `NETBIRD_PAT_FILE`
  mounted from the ESO-synced `netbird-pat` secret).

TXT ownership: distinct `txtOwnerId`/`txtPrefix` per instance
(`home-ops-prd-cloudflare`/`extdns-cf-`,
`home-ops-prd-netbird`/`extdns-nb-`); `policy: sync` on both.

## Ordering prerequisite runbook

Both configs/ `ExternalSecret`s resolve through
`ClusterSecretStore/proton-pass` (external-secrets configs/), which needs
the eso-proton-pass webhook Ready plus the out-of-band `proton-pass-pat`
bootstrap (see the external-secrets README "PAT renewal" runbook).
Cross-tenant ordering is owned by tenants/*.yaml (parallel task). On a fresh
cluster expect fail-then-heal: `Ready=False` on these secrets, and the
controllers/ `HelmRelease`s CrashLooping on the missing synced Secrets,
until ESO syncs — heals via `refreshInterval` + Flux `retryInterval`.
Alert past ~10m. Verify: `kubectl get clustersecretstore proton-pass`;
`kubectl -n external-dns get externalsecret,secret`.

`stg/` is RESERVED/UNUSED (see the headers in configs/stg and
controllers/stg) — no cluster renders it, so it cannot fight prd over the
TXT registry/domain. Dev and prd TXT scope (`txtOwnerId`, `domainFilters`)
lives in controllers/{dev,prd}; vault keys live in configs/{dev,prd}.

## Wildcard record

Sources include Gateway API routes, so the §8 Gateway/HTTPRoutes produce
`*.home-ops.yansyah.my.id → 192.168.1.199` (Gateway LB target) with no file
overlap. Seed both vault entries with pass-cli
(`.../external-dns/{cloudflare-api-token,netbird-pat}`).

## Telemetry-off / monitoring / updates

- ExternalDNS reports nothing upstream. Webhook sidecar documents no
  telemetry (`projects/external-dns-netbird/README.md`).
- `ServiceMonitor` disabled until `monitoring.coreos.com` CRDs land.
- Chart + sidecar bumps: `update-policies/external-dns.yaml` → PR automation.
