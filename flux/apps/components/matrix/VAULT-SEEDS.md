# matrix tenant — vault seed checklist

One-time `pass-cli item create login` per env BEFORE first install. Git holds `remoteRef` keys only, never values. Overlays patch every `remoteRef.key` from `__PROTON_PASS_BASE__` to the per-env path. Vault naming: `pass://acme-<env>-bdo1-talos-apps-01/<path>` (`dev`/`prd`).

**14** fields per env. Bot credentials are minted in-cluster by the bootstrap Job; COSI/Terraform/CNPG/kept-Secret outputs need no seeding.

## Per-env seed table

Same 14 rows for `dev` and `prd`; only values differ per env.

| # | Vault field (`<prefix>/<path>`) | ExternalSecret → Secret (key) | Consumed by | Value notes |
|---|---|---|---|---|
| 1 | `cert-manager/cloudflare-api-token` | `cloudflare-api-token` (`api-token`) | in-namespace wildcard `Certificate` DNS-01 | Zone-scoped, same value both envs |
| 2 | `matrix/tuwunel-registration-secret` | `tuwunel-registration-secret` (`shared-secret`) | tuwunel mount + bootstrap Job env (one source) | Random ≥32 bytes; seed once, then stable |
| 3 | `mautrix-discord/bot-token` | `mautrix-discord` (`bot-token`) | Bridge `login-token bot` runtime auth | Discord developer portal |
| 4 | `mautrix-discord/as-token` | (same, `as-token`) | Bridge `registration.yaml` + tuwunel appservice `as_token` | Random ≥64ch; stable (PVC persists registration) |
| 5 | `mautrix-discord/hs-token` | (same, `hs-token`) | Bridge `registration.yaml` + tuwunel appservice `hs_token` | Same stability contract as `as-token` |
| 6 | `mautrix-discord/avatar-proxy-key` | (same, `avatar-proxy-key`) | Bridge avatar relay HMAC | Random ≥32ch |
| 7 | `mautrix-discord/direct-media-server-key` | (same, `direct-media-server-key`) | Bridge media signing (synapse `.signing.key` format) | Generate per synapse format |
| 8 | `mautrix-discord/provisioning-shared-secret` | (same, `provisioning-shared-secret`) | Bridge provisioning API auth | Random ≥32ch |
| 9 | `mautrix-discord/double-puppet-shared-secret` | (same, `double-puppet-shared-secret`) | Legacy double-puppet value | Random ≥32ch |
| 10 | `mautrix-discord/db-password` | `mautrix-discord-db-credentials` + `mautrix-discord-db-app-secret` | Bridge `DATABASE_URL` + CNPG initdb owner password | Random ≥32ch; one field feeds both Secrets |
| 11 | `element-web/netbird-pat` | `element-proxy-vars` (`netbird_token`) | Terraform `element-proxy` CR via `varsFrom` | Per-env NetBird PAT |
| 12 | `element-web/cloudflare-api-token` | (same, `cloudflare_api_token`) | (same CR, Cloudflare side) | Separate field from #1; same upstream value is fine |
| 13 | `rclone/proton-username` | `rclone-proton-credentials` (`username`) | rclone Proton Drive remote | Shared with the other six rclone legs |
| 14 | `rclone/proton-password` | (same, `password`) | (same remote) | Shared with the other six rclone legs |

Seed commands (dev shown; repeat with `acme-prd-bdo1-talos-apps-01` for prd):

```shell
for t in cert-manager/cloudflare-api-token matrix/tuwunel-registration-secret mautrix-discord/bot-token mautrix-discord/as-token mautrix-discord/hs-token mautrix-discord/avatar-proxy-key mautrix-discord/direct-media-server-key mautrix-discord/provisioning-shared-secret mautrix-discord/double-puppet-shared-secret mautrix-discord/db-password element-web/netbird-pat element-web/cloudflare-api-token rclone/proton-username rclone/proton-password; do
  pass-cli item create login --vault-name 'acme-dev-bdo1-talos-apps-01' --title "$t"
done
```

## Not vault-seeded (in-cluster minted)

Tuwunel SSO outputs (`tuwunel-sso-outputs`), COSI S3 keys (`tuwunel-media`, `mautrix-discord-cosi`), NetBird proxy outputs (`element-proxy-outputs`), bot token (bootstrap Job → kept Secret), coder notifier fields (coder ES reads the kept Secret cross-namespace), in-namespace plumbing (`eso-k8s-reader` RBAC, `kube-root-ca.crt`, wildcard TLS Secret).
