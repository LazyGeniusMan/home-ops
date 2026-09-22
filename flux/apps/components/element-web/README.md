# Element Web standalone (Goal 2d)

Public Matrix web client (stateless SPA) speaking to tuwunel via Zitadel
SSO. Horizontally scalable — pure nginx static file server, no session, no
local state.

## Layout (environment-direct, apps area)

`base/` holds every manifest (`element-web.yaml` workload + config +
Service, `element-httproute.yaml` route, HPA/VPA, DNS-01 solver secret,
wildcard certificate, `element-proxy.yaml` NetBird consumer); env overlays
`{dev,prd}/` patch hostnames, config hosts, vault refs, proxy vars, and HPA
bounds via `resources: [../base]`. Tenant is `apps/element-web` (onboard
via `flux/fleet/tenants/apps.yaml` + the `flux-apps-push.yaml` components
matrix — OUT OF SCOPE for this change, see Non-goals).

## Workload

`base/element-web.yaml` — vendored from the upstream Kubernetes example
(`docs/kubernetes.md` in the element-web sources), image
`vectorim/element-web:v1.12.28` (latest stable at authoring; `$imagepolicy`
marker so update automation owns the tag):

- `element-config` ConfigMap → `config.json` via ConfigMap subPath mount
  at `/app/config.json` (the path the upstream example mounts; the image
  entrypoint copies `/app/config*.json` into the nginx-served staging dir
  at start, so this IS what `/config.json` serves).
- Load order (`docs/config.md`): Element tries `config.$domain.json`
  first, then falls back to `config.json` — we ship ONLY `config.json`
  (no per-domain variant), so every hostname deterministically serves this
  config.
- Service `element-web :80`; probes `GET /` with upstream example values
  (readiness initialDelay 2s / period 3s, liveness initialDelay 10s /
  period 10s). Image `HEALTHCHECK` is `GET /config.json` — covered by the
  same mount.

## Config decisions (all recorded)

| Key | Value | Why |
|---|---|---|
| `default_server_config.m.homeserver.base_url` | `https://tuwunel.matrix.<env>` (placeholder — assumes Goal 2c tuwunel public URL) | PREFERRED explicit-URL method (`docs/config.md`) — no startup `.well-known` lookup. `default_server_name` deliberately NOT set alongside it (docs: Element would try `.well-known` first, extra failure mode). |
| tuwunel `server_name` match | `matrix.<env>` in `room_directory.servers` | MUST match tuwunel `server_name` or login/user-ID routing breaks (placeholder pending Goal 2c). |
| `disable_custom_urls` | `true` | Locks the server picker to our homeserver — users must not be offered arbitrary homeserver URLs. |
| `permalink_prefix` | `https://element.matrix.<env>` | Share-link permalinks point at OUR deployment, not `matrix.to`. |
| `brand` / `default_theme` | `Element` / `light` | Upstream defaults, recorded explicitly. |
| `branding` / `embedded_pages` | `{}` | Stock Element welcome/home pages and assets (no white-labelling; `og:image` stays element.io-hosted as upstream ships it). |
| `room_directory.servers` | `[matrix.<env>]` | Public-room directory queries OUR server, not `matrix.org`. |
| `integrations_*` | `ui: null`, `rest: null`, `widgets: []` | Integration manager DISABLED — no Scalar/vector.im dependency, no widget hosting surface. |
| `jitsi.preferred_domain` | `meet.element.io` | Upstream default, recorded; only used when neither an integration manager nor homeserver `.well-known` supplies Jitsi info. |
| `help_url` / `help_encryption_url` | `element.io` defaults | Recorded; no self-hosted help pages. |
| `default_federate` | `false` | New rooms are created non-federated (federation capability is immutable after creation — safe default for a private deployment). |
| `bug_report_endpoint_url` | omitted | Rageshake/feedback DISABLED (upstream disables when absent — no rageshake server). |

SSO note: Element authenticates via the homeserver's delegated auth
(tuwunel → Zitadel OIDC) — Element itself holds NO OIDC client secret, so
this component ships NO SSO Terraform CR and NO proxy credentials. The
homeserver URL above is public config, not a secret.

## Autoscaling

HPA `element-web` (clickstack precedent: cpu 70 / mem 80): dev min 1/max 2,
prd min 2/max 4 via overlay patches. Base `replicas: 1` is the create-time
seed; HPA owns runtime count. VPA `element-web` is recommender-only
(`updateMode: Off` — never VPA-apply on a resource HPA scales on).

## Exposure — decision: BOTH (Gateway + NetBird)

- **Gateway (public):** `base/element-httproute.yaml` on the shared §8.1
  Gateway (`main`, cross-namespace parentRef, `https` section): hostname
  `element.matrix.<env>`, PathPrefix `/` → `element-web:80`. TLS
  terminates at the Gateway via the in-namespace wildcard `Certificate`
  (`wildcard-certificate.yaml`, same duplicate pattern as hubble-ui —
  cert-manager Secrets are namespace-local). Upstream nginx-snippet headers
  translated to a `ResponseHeaderModifier` filter: `X-Frame-Options
  SAMEORIGIN`, `X-Content-Type-Options nosniff`, `Content-Security-Policy
  "frame-ancestors 'self'"`. `X-XSS-Protection` deliberately DROPPED
  (deprecated, removed from modern browsers — translating it adds a dead
  header).
- **NetBird (private tailnet path):** `base/element-proxy.yaml` follows the
  `consumer-terraform.yaml` skeleton (ESO vars `element-proxy-vars` ←
  Proton Pass, same-namespace `varsFrom`, in-cluster state, outputs in
  `element-proxy-outputs`, upsert-only `destroy: false`, 48h dashboard
  Verify on first apply). Backend is plain `http:80` at the in-cluster
  Service (the SPA is plain HTTP in-cluster; TLS terminates at the edge).
  Env overlays render domain / zone / `network_name` (cluster name) /
  `service_lb_ip` (dev `.249` / prd `.199`, zitadel login-proxy
  convention).
- **Why both:** Element is credential-free (no SSO secret to split across
  two front doors — unlike oauth2-proxy-fronted apps there is no
  callback/cookie-domain cost to a second hostname), tuwunel is itself
  reachable over the tailnet, and the consumer is an upsert-only sidecar
  that never touches Deployment/Service/HTTPRoute. Cost is one extra FQDN
  verify; benefit is an internal path surviving public-DNS/Gateway
  outages. To go Gateway-only later: delete `element-proxy.yaml` (+ vars
  patches) — nothing else references it.

## Secrets

Git holds refs only. Element needs no credentials; the only ESO object
besides the proxy vars is the `cloudflare-api-token` DNS-01 mirror (same
Proton Pass remoteRef as `cert-manager/configs/base/cluster-issuer.yaml`).

## Verification plan

1. Element loads: `GET https://element.matrix.<env>/` → 200 SPA shell
   (internal cluster DNS AND public hostname).
2. `GET https://element.matrix.<env>/config.json` returns the env config
   (`base_url` = tuwunel public URL, `permalink_prefix` = our hostname).
3. Zitadel SSO completes: login in Element redirects through tuwunel
   delegated auth to Zitadel and returns an authenticated session (needs
   tuwunel Goal 2c + Zitadel client wiring — fails closed here if the
   homeserver URL is still a placeholder).
4. Share-links use our hostname: a room Share permalink starts with
   `https://element.matrix.<env>` (not `matrix.to`).
5. HPA 1→2 on dev: load the SPA, watch `kubectl get hpa element-web`
   scale `1→2` (min 1/max 2); prd floor is 2.
6. NetBird outputs Secret: `kubectl get secret element-proxy-outputs`
   contains `service_id` + `service_domain` + `proxy_url` (after the 48h
   Verify completes on first apply).

## Non-goals (not touched)

No tuwunel/bridge changes. `flux/fleet/tenants/apps.yaml` and workflows
untouched (tenant onboarding is a follow-up). `kustomize build` per overlay
+ `kubeconform` clean (see `flux/scripts/validate.sh`).

Upstream reference (read-only): element-web sources
(`/tmp/home-ops-docs/element-web-docs-docs`, `docs/{config,kubernetes}.md`)
— NOT vendored into git.
