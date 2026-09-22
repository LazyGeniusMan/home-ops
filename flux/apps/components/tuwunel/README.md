# Tuwunel (Matrix homeserver)

Fresh-install Matrix homeserver at
`https://tuwunel.matrix.home-ops.yansyah.my.id` (prd) /
`https://tuwunel.matrix.homelab-dev.yansyah.my.id` (dev), app `v1.9.2`
(`ghcr.io/matrix-construct/tuwunel`; docs snapshot Cargo.toml 1.9.2).
Upstream reference (read-only): `/tmp/home-ops-docs/tuwumel-docs`
(docs + `tuwunel-example.toml`, 4314 lines — every env var below traces to
a commented key there).

## Layout (environment-direct, apps area)

`base/` holds every manifest (Deployment + PVC + Service, secrets, COSI
claims, wildcard Certificate, HTTPRoutes, VPA); env overlays `{dev,prd}/`
patch hostnames, vault refs, and identity values via
`resources: [../base]`. Tenant is `apps/tuwunel` (wired by the fleet tenant
file, not here — no tenant/workflow edits in this change).

## Decisions (recorded per brief)

### 1. Bare-domain delegation: NOT delegated

`server_name` == the serving host (`tuwunel.matrix.<env-domain>`) — no
`matrix.example.com` hosted / `example.com` delegated split
(`docs/deploying/root-domain-delegation.md` pattern rejected). User IDs are
`@user:tuwunel.matrix.<env-domain>`, not `@user:<env-domain>`. Rationale:
delegation adds a second well-known serving point (bare domain) with zero
benefit for a fresh namespace; the nested hostname is already covered by the
in-namespace `*.<env-domain>` wildcard cert (Gateway API `*.` matches dots
to the left).

### 2. Well-known: served by tuwunel (config, not Gateway)

`TUWUNEL_WELL_KNOWN__CLIENT=https://<matrix-host>` (brief: mandatory for
OIDC start — without it `/.well-known/matrix/client` 404s and IdP-brokered
login never starts). Tuwunel generates and serves both
`/.well-known/matrix/{client,server}` entries itself; the Gateway
`tuwunel-tls` route carries `/.well-known/matrix/` to the backend (see
Routing). No static Gateway response, no bare-domain serving point.

### 3. VPA mode: Auto (modern Recreate) + minAllowed + minReplicas 1

`tuwunel-vpa.yaml`: `updateMode: Auto` (the brief's "Auto (or Initial+
Recreate if disruption risk)" — Auto IS the modern Recreate path; in-place
needs VPA 1.x, which the pinned 0.12.0 chart does not ship) with
`minReplicas: 1` (updater default `--min-replicas=2` never evicts
singletons otherwise — same rationale as the tofu-controller and
external-dns-netbird singleton VPAs) and `minAllowed` `{cpu: 100m,
memory: 512Mi}` guarding the RocksDB page-cache / LRU budget
(`db_cache_capacity_mb` default "varies by system" per
tuwunel-example.toml — the floor, not the ceiling, is what matters).
Disruption risk accepted deliberately: the Deployment strategy is already
Recreate and no PDB gates the eviction — a resize IS a restart, same as any
rollout. No HPA (singleton; RocksDB single-writer).

### 4. S3 projection: explicit secretKeyRef envs (NOT envFrom)

Claim `tuwunel-media` + access `tuwunel-media-access` (BucketClass/
`seaweedfs`, KEY auth) mint `tuwunel-media-cosi-creds` (BucketInfo JSON).
ESO `tuwunel-s3` extracts `BucketInfo.spec.bucketName` (LIVE
controller-generated `bc-<uuid>` — no per-env bucket literal in git) +
`accessKeyID`/`accessSecretKey` into flat keys (`bucket`,
`access-key-id`, `secret-access-key`); the Deployment maps them 1:1 onto
`TUWUNEL_STORAGE_PROVIDER__MEDIA_ON_S3__S3__{BUCKET,KEY,SECRET}` (form
verified in `docs/media/storage.md`), plus literals
`REGION=us-east-1` (SeaweedFS ignores it), `ENDPOINT=` the INTERNAL S3
(`http://seaweed-main-s3.seaweedfs.svc.cluster.local:8333` — same as
CNPG/Dragonfly/Zitadel), `BASE_PATH=tuwunel-media` (never share the
cnpg/dragonfly backup buckets), `USE_VHOST_REQUEST=false` (SeaweedFS S3 is
path-style). `media_storage_providers='["media", "media_on_s3"]'` fresh
install (local fallback retained — brief contract) with
`store_media_on_providers='["media_on_s3"]'` (new writes go to S3) and
`media_startup_check=true`.

envFrom vs files: envFrom is NOT used — the ESO keys contain dashes
(`access-key-id`), which are invalid env names and would be silently
skipped by kubelet. S3 + client_id use explicit secretKeyRef; only
client_secret uses a file (`CLIENT_SECRET_FILE`, read at startup and on
each OAuth exchange per tuwunel-example.toml).

### 5. SSO: Zitadel IdP, password/guest/native paths all closed

Single `[[global.identity_provider]]` (`__0__` index form verified in
`docs/authentication/providers/{keycloak,mas}.md`): `brand=zitadel`
(`trusted=true` — self-hosted, so user matching may associate existing
accounts), `client_id` immutable per-registration via
`$(TUWUNEL_SSO_CLIENT_ID)` k8s expansion (never in git) +
`client_secret_file` via ESO, `issuer_url=https://admin.zitadel.<env>`,
`callback_url=https://<matrix>/_matrix/client/unstable/login/sso/
callback/$(TUWUNEL_SSO_CLIENT_ID)` (strict format per tuwunel-example.toml),
`userid_claims=["preferred_username", "email"]`,
`unique_id_fallbacks=false` (private server — random fallback IDs are an
error, not a feature), `registration=true` (IdP is the ONLY registration
path: `allow_registration=false` + `login_with_password=false` +
`login_via_token=true` (brief contract — token login stays on for session
uplift), `allow_guest_registration=false`, SMTP untouched).
`grant_admin_to_first_user=true` + `create_admin_room=true`
(first IdP login claims admin); `server_user_localpart=conduit` untouched.
The companion `tuwunel-sso` Terraform CR (Zitadel project + `tuwunel` OIDC
client, outputs `client_id`/`client_secret` → `tuwunel-sso-outputs`) ships
in the follow-up tofu task, not here.

Zitadel-side app requirements (staging-first): create the `tuwunel` OIDC
client in DEV first (`admin.zitadel.homelab-dev.yansyah.my.id`), verify the
full login round-trip there, then promote the same shape to prd. Required:
redirect URI `https://<matrix>/_matrix/client/unstable/login/sso/
callback/<client_id>` registered EXACTLY (tuwunel formats it strictly —
placeholder `<client_id>` replaced with the generated id, same value as the
Deployment's expanded callback); allowed origins `https://<matrix-host>`;
scopes `openid email profile` (default empty-array scope); `preferred_
username` claim present (primary userid claim — without it every login
falls back to `email`, and without either, registration errors since
`unique_id_fallbacks=false`).

### 6. Federation: off before first room (immutable history risk)

`allow_federation=false` + `federate_created_rooms=false` +
`federate_admin_room=false`, plus asserted
`allow_public_room_directory_over_federation=false` +
`allow_device_name_federation=false` (both inherently false with
federation off; set explicitly so a later federation flip cannot silently
open them). `m.federate` pin: rooms FREEZE the federate setting at
creation (`federate_created_rooms` docs) — every room created while
federation is off stays non-federating forever even if the server later
federates. Re-federating later means NEW rooms only; old rooms never join.

## Routing

Hand-written HTTPRoutes on the shared gateway-api Gateway (`main`,
cross-namespace parentRef); reverse-proxy mode behind Gateway + Cilium:

- `tuwunel-tls` (`https` listener, `tuwunel.matrix.<env>`): one rule per
  required prefix → `tuwunel:8008`: `/_matrix/` (client + federation +
  SSO callback), `/_tuwunel/` (server admin console),
  `/_synapse/admin/` (Synapse-admin compat), `/.well-known/matrix/`
  (client auto-discovery, served by tuwunel itself).
- `tuwunel-redirect`: HTTP→HTTPS 301 on the `http` listener (port 80 is
  redirect-only, same shape as gateway-api `redirect-services`).
- X-Forwarded-For: preserved end-to-end — NO RequestHeaderModifier filter
  (stripping/rewriting XFF at the route would destroy the chain tuwunel
  trusts); Cilium Gateway appends the downstream peer itself and tuwunel
  resolves the client from `rightmost_x_forwarded_for`
  (`TUWUNEL_IP_SOURCE`; nginx/Caddy-equivalent semantics per
  tuwunel-example.toml — correct behind a Gateway that appends XFF).

## TLS + DNS

One in-namespace wildcard Certificate (`wildcard-<env>` →
`wildcard-<env>-tls`, `ClusterIssuer/letsencrypt` DNS-01 via the
`cloudflare-api-token` ExternalSecret mirroring the cert-manager remoteRef;
same namespace-local pattern as hubble-ui/flux-operator-ui) covering the
nested `tuwunel.matrix.<env>` leaf. TLS terminates at the Gateway. No
manual DNS: issuance uses DNS-01 TXT, A records ride external-dns.

## Storage (RocksDB)

`StatefulSet`-free on purpose: Deployment `replicas: 1` + `strategy:
Recreate` (a second replica must never open the same `database_path`),
single RWO PVC `tuwunel-data` (`local-ssd-nvme`, 10Gi) at
`/var/lib/tuwunel`. Probes all `exec ["tuwunel", "--health-check"]` with
startup `failureThreshold: 180` (30 min migration budget — liveness/
readiness are gated while startup fails, so migrations are never killed)
and `terminationGracePeriodSeconds: 600` (a mid-migration kill leaves
RocksDB half-migrated — per upstream `docs/deploying/kubernetes.md`).

## Credentials

`ExternalSecret/tuwunel-sso-client` (Zitadel `client_id` + `client_secret`
from stored tofu outputs via the `tuwunel-k8s` SecretStore — NO pass://
seeding), `tuwunel-s3` (COSI-minted media keys via the `tuwunel-cosi`
SecretStore — dedicated claim `tuwunel-media`, see `bucketclaims.yaml`),
`cloudflare-api-token` (§9 vault path). Seed the Proton Pass
`cert-manager/cloudflare-api-token` entry per env with pass-cli.

## Environments

| Env | Hosts | Patches |
| --- | --- | --- |
| `dev` | `tuwunel.matrix.homelab-dev.yansyah.my.id`, Zitadel `admin.zitadel.homelab-dev.yansyah.my.id` | vault refs, wildcard cert, hostnames, SERVER_NAME + well-known + issuer + callback |
| `prd` | `tuwunel.matrix.home-ops.yansyah.my.id`, Zitadel `admin.zitadel.home-ops.yansyah.my.id` | same |

`server_name` is IMMUTABLE (database wipe to change) — the per-env values
above are set before first boot and must never be patched afterwards.

## Assumptions (per brief — scaffolded anyway)

Gateway `main`/cilium, `ClusterIssuer/letsencrypt` + Cloudflare token,
COSI `seaweedfs`/`seaweedfs-key` classes, and VPA CRDs are assumed present
per the explore report. If any is missing, first sync will report it —
nothing here fabricates them.

## Follow-ups (out of scope — NOT in this change)

- Companion `tuwunel-sso` Terraform CR (separate tofu task) + tofu
  bootstrap execution / reusable room module (separate task).
- `update-policies/tuwunel.yaml` (`ghcr.io/matrix-construct/tuwunel`
  ImageRepository + ImagePolicy; image pinned `v1.9.2` here, no
  `$imagepolicy` marker yet) + fleet wiring (`tenants/apps.yaml` untouched
  per brief — wire `apps/tuwunel` there when promoting).
- Bridge / Element-Web (non-goal).
