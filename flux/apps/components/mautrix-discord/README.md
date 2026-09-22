# Mautrix Discord bridge

Matrix↔Discord puppeting bridge (`@discordbot:<server>`) at
`dock.mau.dev/mautrix/discord:v0.7.7` (pinned; GH release
`mautrix/discord` is the alternative source). Single-replica singleton,
own Postgres database, all tokens via ESO/Proton Pass — Git holds
remoteRefs only, never values.

Upstream references (read-only):
`/tmp/home-ops-docs/matrix-mautrix-bridge-docs/bridges/{general,go/discord,go/setup.md}`,
`/tmp/home-ops-docs/matrix-mautrix-discord-bridge-docs/{README.md,example-config.yaml}`,
`config/bridge.go` (legacy arch).

## Layout (environment-direct, apps area)

`base/` holds every manifest (workload + entrypoint ConfigMaps, secrets,
CNPG Cluster, bucket claims, VPA); env overlays `{dev,prd}/` patch vault
refs, homeserver link, admin identity, S3 endpoint, and replica counts
via `resources: [../base]`. Tenant is `apps/mautrix-discord` (wired by the
fleet tenant file, not here — no tenant/workflow edits in this change).

| Env | Bridge | `mautrix-discord-db` Cluster | Homeserver link |
| --- | --- | --- | --- |
| `dev` | `replicas` 1 | `instances` 1 | `http://tuwunel.matrix-dev.svc:8008`, domain `tuwunel.matrix.home-ops.yansyah.my.id` |
| `prd` | `replicas` 1 | `instances` 3 | `http://tuwunel.matrix.svc:8008`, domain `tuwunel.matrix.home-ops.yansyah.my.id` |

Namespace per env is the component name (`mautrix-discord`) via the Fleet
`apps` Kustomization's `targetNamespace`; the `matrix`/`matrix-dev`
segments in the URLs above are the *tuwunel* namespaces (owned by the
parallel tuwunel task — if that task renames tuwunel's namespace, update
the `HS_ADDRESS` patches in `{dev,prd}/kustomization.yaml` to match;
nothing else blocks on it).

## Singleton workload (why restarts never re-register)

`base/mautrix-discord.yaml` — Deployment `replicas: 1`, `strategy:
Recreate`, **no HPA** (none exists and none may be added — see below):

- Fixed appservice listener: `hostname: 0.0.0.0`, `port: 29334`;
  `portal_message_buffer: 128` is in-memory per portal;
  `async_transactions: false` keeps ordered delivery. Two writers would
  fight over the port and corrupt bridge rows, so scaling is vertical
  only (VPA below).
- `Recreate` (not `RollingUpdate`): with `replicas: 1` the default
  RollingUpdate would briefly run TWO bridges (maxSurge) during every
  rollout.
- Stability chain: as_token/hs_token are vault-seeded ONCE via ESO (same
  value every boot) → entrypoint writes `/data/config.yaml` from them →
  `./mautrix-discord -g` generates `/data/registration.yaml` ONLY if
  absent → both live on the `mautrix-discord-data` PVC (2Gi,
  `local-ssd-nvme`). A restart therefore reuses the same tokens and the
  same registration: it never re-registers, never rotates tokens, and
  tuwunel needs no restart. To rotate: change the vault fields, delete
  `/data/registration.yaml` (exec into the pod or delete the PVC —
  deleting the PVC also wipes bridge state, prefer deleting the single
  file), let the entrypoint regenerate, then re-run the registration
  runbook below.
- Config mechanics (legacy arch): this revision has NO `-e` flag (setup
  docs: "Discord is still using the legacy architecture which doesn't
  have the `-e` flag, so just manually copy `example-config.yaml` from
  the repo to `config.yaml`"), so the full `config.yaml` is authored in
  the `mautrix-discord-config-tmpl` ConfigMap (from `example-config.yaml`
  at v0.7.7) with `__PLACEHOLDER__` secret/env fields. The
  `mautrix-discord-entrypoint` script (same image — it ships `yq-go`)
  copies the template to `/data/config.yaml` and overwrites every secret
  and environment-wired field from env via `yq ... strenv(...)`. The
  Discord **bot token is never in config**: auth is `login-token bot
  <token>` at runtime (see Auth below); the vault `bot-token` field is
  where the operator copies it from.
- Probes are `tcpSocket` on the appservice port: this revision exposes
  no dedicated health endpoint on that listener.

## VPA (apply) / HPA (none)

`base/mautrix-discord-vpa.yaml` — `updateMode: Initial` (VPA evicts +
recreates the lone pod with the recommended request once the recommender
has learned; `Recreate` strategy keeps at-most-one-writer during the
eviction). Read the recommendation with
`kubectl describe vpa mautrix-discord -n mautrix-discord` after a week of
bridge traffic and fold it back into the Deployment requests if the
recommender disagrees persistently. No `HorizontalPodAutoscaler` exists
in this component and none may be added: HPA + a fixed appservice
`hostname:port` + in-memory `portal_message_buffer` + non-ordered-safe
multi-writer state is an unsupported combination
(`known-limitations.md`: never VPA-apply on a resource HPA scales on —
trivially satisfied here).

## Image

`dock.mau.dev/mautrix/discord:v0.7.7` (pinned; upstream tags per release,
`:latest` tracks commits — never use it). No `$imagepolicy` marker and
no `update-policies/mautrix-discord.yaml` ship with this component (out
of scope) — bump the tag manually. The image already bundles `ffmpeg` +
`lottieconverter` (its Dockerfile: `apk add ffmpeg ... lottieconverter`),
so voice-message conversion and animated-sticker conversion (`target:
webp`, 320×320@25fps) work with no sidecar. Postgres requirement per
setup docs: **v16+**; own database `discord` on the colocated
`mautrix-discord-db` Cluster (shared *instance* is fine, shared
*database* is not — this component never shares).

## Database

`base/mautrix-discord-db.yaml` — namespace-local instantiation of the
§10.1 `cluster-base` template (3 instances, sync quorum 1,
`local-ssd-nvme`, continuous WAL + daily base backup to SeaweedFS S3
under `s3://cnpg-backups/mautrix-discord/`). Adjusted: dbname/owner
`discord`. The bridge reads the ESO-composed `connection-url`
(`sslmode=require`, same self-signed trade-off as coder/zitadel) via
`DATABASE_URL`. S3 keys are COSI-minted (`bucketclaims.yaml` +
`cosi-keys.yaml`, same pattern as coder).

## Credentials

`base/mautrix-discord-secrets.yaml` — Git holds refs only, never values.
One Proton Pass prefix per env,
`pass://acme-<env>-bdo1-talos-apps-01/mautrix-discord/<field>`:

| Vault field | Consumed as | Notes |
| --- | --- | --- |
| `bot-token` | manual `login-token bot <token>` | session credential, never in config |
| `as-token` / `hs-token` | `appservice.as_token/hs_token` | seeded once, STABLE forever |
| `avatar-proxy-key` | `bridge.avatar_proxy_key` | HMAC for the relay avatar endpoint |
| `direct-media-server-key` | `bridge.direct_media.server_key` | federation signing key (synapse `.signing.key` format) |
| `provisioning-shared-secret` | `bridge.provisioning.shared_secret` | provisioning API auth |
| `double-puppet-shared-secret` | `bridge.login_shared_secret_map[<domain>]` | legacy auto-double-puppet (see below) |
| `db-password` | `connection-url` + `mautrix-discord-db-app-secret` | single password source, no dual-write |

Seed each entry with pass-cli before first install. The Deployment pends
until ESO syncs them and Flux retries.

## Homeserver link

`homeserver.address` = tuwunel internal URL (`HS_ADDRESS`,
`http://tuwunel.<ns>.svc:8008`-style per-env value above);
`homeserver.domain` = tuwunel `server_name` (`HS_DOMAIN`,
`tuwunel.matrix.home-ops.yansyah.my.id` in both envs — the parallel task
owns tuwunel's value; if it picks a different `server_name`, update the
`HS_DOMAIN` patches here to match).

## Appservice block (registration-sensitive — changing any of these
requires regenerating `registration.yaml`)

| Key | Value | Why |
| --- | --- | --- |
| `address` | `http://mautrix-discord.mautrix-discord.svc:29334` | what TUWUNEL dials: the in-cluster Service DNS name, never `localhost` (`localhost` would resolve inside the tuwunel pod) |
| `hostname` / `port` | `0.0.0.0` / `29334` | fixed listener (singleton contract) |
| `id` | `discord` | registration ID |
| `bot.username` | `discordbot` | management-room bot `@discordbot:<server>` |
| `ephemeral_events` | `true` | MSC2409 receipt support |
| `async_transactions` | `false` | ordered delivery (must stay false) |
| `as_token` / `hs_token` | vault-seeded, stable | restart must not re-register |

## Registration runbook (first install + token rotation)

1. Deploy the component; the entrypoint writes `/data/config.yaml` and
   generates `/data/registration.yaml` inside the pod (first boot only).
2. Copy it out:
   `kubectl cp mautrix-discord/<pod>:/data/registration.yaml ./discord-registration.yaml`
3. Install it on tuwunel and restart tuwunel. Which mechanism depends on
   the tuwunel task's outcome (both upstream-confirmed):
   - **Synapse-style** (`app_service_config_files` in the homeserver
     config + restart): mount/copy the file where tuwunel reads it, add
     the path, restart.
   - **Continuwuity-style** (`register_appservice` admin-room command —
     Conduit-family servers have no config file): paste the registration
     contents into the admin command, no file mount needed.
4. Verify: bridge pod logs show a clean startup, and inviting
   `@discordbot:<server>` to a Matrix room (or opening a DM with it)
   gets the welcome message.

## Auth (bot login)

Per the Discord bridge authentication docs — bot login, NOT user-token:

1. Create an application at <https://discord.com/developers/applications>,
   add a bot, copy the token into the vault `bot-token` field.
2. Enable **Privileged Gateway Intents**: **Server Members Intent** +
   **Message Content Intent**.
3. Open a DM with `@discordbot:<server>` and send
   `login-token bot <token>`.
4. Invite the bot to guilds via OAuth2 → URL Generator, scope **`bot`
   ONLY** (any other scope forces a redirect URI; with `bot` alone none
   is needed). Minimum permissions: Send Messages, Create Public
   Threads, Send Messages in Threads, Read Message History, Add
   Reactions — plus Manage Webhooks for `!discord set-relay --create`.
   Guild admins can grant Administrator instead.
5. Relay without a personal login: log the bot in on a **dedicated
   Matrix relay account** (closest to a native "bridge bot" on Discord),
   then `!discord set-relay --create [name]` (needs Manage Webhooks) or
   `--url <webhook-url>` per room. The bridge itself is never logged in
   — at least one login must exist for relaying to work.
6. Permissions (entrypoint-rendered):
   `{"*": relay, "<HS_DOMAIN>": user, "<ADMIN_MXID>": admin}` —
   strangers relay-only, local users full bridge, one admin.

## public_address + direct_media decisions

Both default OFF; enabling either needs a public Gateway hostname
routed to the `mautrix-discord` Service (none ships here — the Service
is ClusterIP-only):

- `public_address: null` → relay avatars are NOT bridged. To enable:
  add a Gateway HTTPRoute `<host>` → `mautrix-discord:29334`, set the
  hostname as `public_address` (no trailing slash — the bridge appends
  `/mautrix-discord/avatar/{server}/{id}/{hash}`), keep the vault
  `avatar-proxy-key` (already plumbed).
- `direct_media` (MSC3860 custom `mxc://` URIs): `enabled: false`. To
  enable: the entrypoint derives `server_name` as
  `discord-media.<HS_DOMAIN>` at boot; override that derivation (or
  delegate a custom name via `.well-known`/Gateway proxy) and keep the
  vault `server_key` (already plumbed, synapse `.signing.key` format).

## Double-puppet decision (pinned to this Discord revision)

At v0.7.7 this bridge is **legacy arch**: `config/bridge.go` embeds
`bridgeconfig.DoublePuppetConfig` inline (only
`double_puppet_server_map` + `double_puppet_allow_discovery` appear in
`example-config.yaml`, and `config.go` reads
`DoublePuppetConfig.SharedSecretMap`) — there is **no**
`double_puppet.secrets` block and no separate `doublepuppet.yaml` at
this revision. So this component uses the **legacy
`bridge.login_shared_secret_map`** (`{<HS_DOMAIN>: <vault secret>}`,
entrypoint-rendered): it auto-enables double puppeting IF the
homeserver runs a shared-secret-auth password provider; otherwise it is
inert and per-user manual `login-matrix <access-token>` (password or
SSO-token flow per the double-puppeting docs) always works. When the
bridge is eventually rewritten to the new arch, migrate to
`double_puppet.secrets: {<domain>: "as_token:<token>"}` + a
`doublepuppet.yaml` appservice registration (distinct ID/tokens, `url:
null`, non-exclusive `@.*:<domain>` user namespace) installed on the
homeserver — do NOT mix both maps.

## Environments

| Env | Bridge | DB | Patches |
| --- | --- | --- | --- |
| `dev` | `replicas` 1 | `instances` 1 | vault refs, HS link, admin MXID, S3 endpoint |
| `prd` | `replicas` 1 | `instances` 3 | vault refs, HS link, admin MXID, S3 endpoint |

## Verification plan

- [ ] `kustomize build flux/apps/components/mautrix-discord/{dev,prd} --load-restrictor=LoadRestrictionsNone | kubeconform -strict -ignore-missing-schemas` clean (also `./flux/scripts/validate.sh -d flux/apps`).
- [ ] After ESO sync + first install + registration runbook: pod Running,
  logs show homeserver connectivity (no appservice auth errors).
- [ ] DM `@discordbot:<server>` → welcome message (proves HS→bridge path).
- [ ] `login-token bot <token>` succeeds; `guilds` lists guilds; bridge
  one guild channel and confirm **both directions** (Matrix→Discord and
  Discord→Matrix).
- [ ] Restart the Deployment (`kubectl rollout restart`): pod comes back
  with the SAME tokens (`/data/registration.yaml` untouched — compare
  checksums before/after), no tuwunel restart needed, bridge resumes
  without re-invite.
- [ ] `kubectl describe vpa mautrix-discord` shows a recommendation
  after traffic; confirm NO `HorizontalPodAutoscaler` targets the
  Deployment (`kubectl get hpa -n mautrix-discord` empty).
