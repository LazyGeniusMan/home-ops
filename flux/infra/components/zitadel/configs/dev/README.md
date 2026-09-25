# Zitadel dev

Dev identity provider: OIDC issuer on
`https://admin.zitadel.home-ops-dev.yansyah.my.id` (login UI on
`https://login.zitadel.home-ops-dev.yansyah.my.id`, NetBird-exposed) plus
the locked dev client contract below.

Patches against `configs/base`: ESO remoteRefs (dev vault), namespace-local
CNPG Cluster + ScheduledBackup (dev S3 endpoint), namespace-local Dragonfly
+ snapshot credentials (dev S3 host), in-namespace
`wildcard-home-ops-dev-tls`, admin HTTPRoute (`/` -> zitadel:8080 on the
dev Gateway), rclone Proton credentials + destinations (dev cluster
segment), login-proxy vars + domain + mesh attach (dev login host,
`network_name` -> dev cluster, `service_lb_ip` -> `.249`,
`cloudflare_zone_id` null), `org-users.yaml` intent mirror + handoff
mirrors, plus single-instance scaling (DB `instances` -> 1, cache
`replicas` -> 1). Controllers in `controllers/dev`: `ExternalDomain:
admin.zitadel.home-ops-dev.yansyah.my.id` (+ `LoginV2.BaseURI:
https://login.zitadel.home-ops-dev.yansyah.my.id/ui/v2/login`) + API/login
seeds -> 1 with chart-native HPAs 1 / 2.

## OIDC contract (locked)

| Item | Value |
|---|---|
| Issuer (admin host) | `https://admin.zitadel.home-ops-dev.yansyah.my.id` |
| Login UI (NetBird) | `https://login.zitadel.home-ops-dev.yansyah.my.id/ui/v2/login` |
| Org | `home-ops` |
| Users | `admin@home-ops-dev.yansyah.my.id` (super-admin -- bootstrap-owned); non-admin users owned per consumer app |
| Groups | `admin` (admin@ member) -- asserted in the `groups` claim; per-app `users` membership owned by each consumer app |
| Scopes (all clients) | `openid profile email groups` |
| Flow (all clients) | Authorization code + PKCE, refresh tokens on |
| Owner: `coder` -> client `coder` | `https://coder.home-ops-dev.yansyah.my.id/*` |
| Owner: `clickstack` -> client `clickstack` | `https://clickstack.home-ops-dev.yansyah.my.id/*` (covers `/oauth2/callback`) |
| Owner: `hubble-ui` -> client `hubble` | `https://hubble.home-ops-dev.yansyah.my.id/*` (covers `/oauth2/callback`) |
| Owner: `flux-operator-ui` -> client `flux-operator-ui` | `https://flux-operator.home-ops-dev.yansyah.my.id/*` (covers `/oauth2/callback`) |
| Owner: `headlamp` -> client `headlamp` | `https://headlamp.home-ops-dev.yansyah.my.id/*` |
| Owner: `seaweedfs` -> client `seaweedfs` | `https://admin.seaweedfs.home-ops-dev.yansyah.my.id/oauth2/callback` |

Each app owns its own `zitadel_project` + `zitadel_application_oidc` client
in its per-app `terraform/` slice; the central bootstrap owns no clients.
Client `redirect_uris`/`post_logout_redirect_uris` ride each slice's
`app_host`/`ui_host` CR vars. Client secrets read from each app's own state
into the dev vault (`pass://acme-dev-bdo1-talos-apps-01/...`, never Git).

## Credentials

All secrets sync from Proton Pass via ESO (Git holds `remoteRef`s only):

- `pass://acme-dev-bdo1-talos-apps-01/zitadel/masterkey` -- 32-byte
  masterkey, immutable.
- `pass://acme-dev-bdo1-talos-apps-01/zitadel/db-password` -- single
  password source (DSN composed in ESO target.template; `zitadel-db-app-secret`
  consumes the same field).
- `pass://acme-dev-bdo1-talos-apps-01/zitadel/smtp-*` -- relay
  user/password (optional, unwired).
- `pass://acme-dev-bdo1-talos-apps-01/{cnpg,dragonfly}/s3-*` and
  `pass://acme-dev-bdo1-talos-apps-01/cert-manager/cloudflare-api-token` --
  same vault paths as dev cnpg/dragonfly/cert-manager, copied into this
  namespace.
