# Zitadel dev

Dev identity provider: OIDC issuer on `https://admin.zitadel.home-ops-dev.yansyah.my.id` (login UI on `https://login.zitadel.home-ops-dev.yansyah.my.id`, NetBird-exposed). OIDC contract, SSO slice shapes, and bootstrap handoff are owned by the parent `zitadel/README.md` — this overlay documents only the dev delta.

Dev delta against `configs/base`: ESO remoteRefs (dev vault), namespace-local CNPG Cluster + ScheduledBackup (dev S3 endpoint), namespace-local Dragonfly + snapshot credentials (dev S3 host), in-namespace `wildcard-home-ops-dev-tls`, admin HTTPRoute on the dev Gateway, rclone Proton destinations (dev cluster segment), login-proxy vars (dev login host, `network_name` -> dev cluster, `service_lb_ip` -> `.249`, `cloudflare_zone_id` null), `org-users.yaml` intent mirror + handoff mirrors, single-instance scaling (DB `instances` -> 1, cache `replicas` -> 1). Controllers: `ExternalDomain: admin.zitadel.home-ops-dev.yansyah.my.id` (+ `LoginV2.BaseURI: https://login.zitadel.home-ops-dev.yansyah.my.id/ui/v2/login`), API/login seeds -> 1 with chart-native HPAs 1/2.

## Dev clients

Same locked contract as the parent (issuer, org `home-ops`, scopes `openid profile email groups`, code + PKCE, refresh on) with the dev domain: `coder`, `clickstack`, `hubble-ui`, `flux-operator-ui`, `headlamp` at `https://<app>.home-ops-dev.yansyah.my.id/*`, `seaweedfs` at `https://admin.seaweedfs.home-ops-dev.yansyah.my.id/oauth2/callback`, super-admin `admin@home-ops-dev.yansyah.my.id`.

## Credentials

All secrets sync from Proton Pass via ESO (Git holds `remoteRef`s only): `pass://acme-dev-bdo1-talos-apps-01/zitadel/masterkey` (immutable), `pass://acme-dev-bdo1-talos-apps-01/zitadel/db-password`, `pass://acme-dev-bdo1-talos-apps-01/zitadel/smtp-*` (optional, unwired); S3 + Cloudflare paths mirror the dev cnpg/dragonfly/cert-manager vault paths into this namespace.
