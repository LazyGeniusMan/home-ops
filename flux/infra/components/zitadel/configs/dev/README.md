# Zitadel (§14 dev)

Dev identity provider: the OIDC issuer for
`https://zitadel.homelab-dev.yansyah.my.id` plus the locked dev client
contract the §14 app writers build against (table below — do not deviate).
`kustomization.yaml` here references `../base` plus kustomize patches
carrying the §14 locked deltas: dev domain, DEV Proton Pass vault
(`pass://acme-dev-bdo1-talos-apps-01/...`), dev SeaweedFS S3 endpoints,
same DB/cache classes (3-instance CNPG, 3-replica Dragonfly,
`local-ssd-nvme`). No prd references outside comments.

## Layout

Patches against `configs/base`: ESO remoteRefs (dev vault),
namespace-local CNPG Cluster + ScheduledBackup (dev S3 endpoint),
namespace-local Dragonfly + snapshot credentials (dev S3 host),
in-namespace `wildcard-homelab-dev-tls`, HTTPRoutes (`/` → zitadel:8080,
`/ui/v2/login` → zitadel-login:3000 on the shared §14 DEV Gateway),
`org-users.yaml` intent (human-readable mirror) + `zitadel-bootstrap-handoff.yaml`
(ESO credential/asset mirrors, machine-applied).
Controllers live in `controllers/dev` (`../base` + patch setting
`ExternalDomain: zitadel.homelab-dev.yansyah.my.id`).

## OIDC contract (LOCKED for §14 app writers)

App writers build against THIS table — do not deviate.

| Item | Value |
|---|---|
| Issuer | `https://zitadel.homelab-dev.yansyah.my.id` |
| Org | `home-ops` |
| Users | `admin@homelab-dev.yansyah.my.id` (super-admin, `admin` group + role — bootstrap-owned); non-admin users are owned per consumer app, not by this bootstrap |
| Groups | `admin` (admin@ member) — asserted in the `groups` claim; per-app `users` membership is owned by each consumer app |
| Scopes (all clients) | `openid profile email groups` |
| Flow (all clients) | Authorization code + PKCE, refresh tokens on |
| Owner: `coder` → client `coder` | `https://coder.homelab-dev.yansyah.my.id/*` (post-logout → `https://coder.homelab-dev.yansyah.my.id/`) |
| Owner: `clickstack` → client `clickstack` | `https://clickstack.homelab-dev.yansyah.my.id/*` (covers the per-instance oauth2-proxy callback under `/oauth2/callback`) |
| Owner: `hubble-ui` → client `hubble` | `https://hubble.homelab-dev.yansyah.my.id/*` (covers the per-instance oauth2-proxy callback under `/oauth2/callback`) |
| Owner: `flux-operator-ui` → client `flux-operator-ui` | `https://flux-operator.homelab-dev.yansyah.my.id/*` (covers the per-instance oauth2-proxy callback under `/oauth2/callback`) |
| Owner: `headlamp` → client `headlamp` | `https://headlamp.homelab-dev.yansyah.my.id/*` |
| Owner: `seaweedfs` → client `seaweedfs` | `https://ui.seaweedfs.homelab-dev.yansyah.my.id/oauth2/callback` (serves the filer-UI proxy) |

Each app owns its own `zitadel_project` + `zitadel_application_oidc` client
in its per-app `terraform/` slice (own project roles/grants assert the
`groups` claim); the central bootstrap slice owns no clients. Post-logout
redirects point at each app's root (`https://<app>…/`).

## Identity bootstrap (Helm FirstInstance) — DEV note

Machine-applied like base: the chart's `FirstInstance` stanza (inherited
from `controllers/base`, plus this overlay's `ExternalDomain:
zitadel.homelab-dev.yansyah.my.id` patch) creates org `home-ops` + the
`zitadel-bootstrap-sa` machine user; `zitadel-bootstrap-handoff.yaml`
mirrors its key/PAT into `zitadel-bootstrap-credentials` (no DEV vault refs
for bootstrap — the setup Job mints the key) plus the `zitadel-assets` S3
mirror (dev internal S3 endpoint patch in this overlay's `kustomization.yaml`).

- Client `redirect_uris`/`post_logout_redirect_uris`: each per-app
  `terraform/` slice rides its own `app_host`/`ui_host` CR vars (same shapes
  as the table above) — no shared client remains.
- Read each app's generated client secret from its own per-app state into
  the DEV Proton Pass vault
  (`pass://acme-dev-bdo1-talos-apps-01/...`, never Git) — each §14 app's dev
  ESO secrets reference those entries.

## Credentials

All secrets sync from Proton Pass via ESO (Git holds `remoteRef`s only).
Seed each vault entry with pass-cli (never commit):

- `pass://acme-dev-bdo1-talos-apps-01/zitadel/masterkey` — 32-byte masterkey.
  IMMUTABLE: Zitadel cannot re-key; loss means loss of all encrypted data.
- `pass://acme-dev-bdo1-talos-apps-01/zitadel/db-password` — SINGLE password
  source: the `zitadel-db-credentials` DSN is composed in ESO target.template
  from literal parts (user/host/port/db + `sslmode=require`) plus this field,
  and `zitadel-db-app-secret` consumes the same field (no dual-write; rotate
  in one place).
- `pass://acme-dev-bdo1-talos-apps-01/zitadel/smtp-*` — relay user/password
  (optional; unwired until a relay exists).
- `pass://acme-dev-bdo1-talos-apps-01/{cnpg,dragonfly}/s3-*` and
  `pass://acme-dev-bdo1-talos-apps-01/cert-manager/cloudflare-api-token` —
  same vault paths as the §14 DEV §§9–10, copied so the Barman/snapshot/DNS-01
  secrets exist in this namespace too.
