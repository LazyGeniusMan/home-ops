# Matrix rooms Terraform — bootstrap + reconcile

Machine-applied by Tofu Controller. Owns ONLY per-team rooms/spaces on the
tuwunel homeserver (`tuwunel.matrix.home-ops.yansyah.my.id`, sibling task):
the flat root here (`main.tf` + `variables.tf` + `outputs.tf`) plus one consumer Terraform CR
per room. Live ops rooms ship in `../base/rooms.yaml`
(`flux-notifications` + `tofu-runs`); the `./examples/` files
(`flux-notifications-terraform.yaml`, `tofu-runs-terraform.yaml`,
`team-terraform.yaml`) are copy-paste skeletons for TEAM namespaces only.

The provider (`raspbeguy/matrix ~> 0.5`) has **no user/token resources**, so
the bot + token are bootstrapped ONCE outside Terraform (runbook below).
Terraform then manages rooms, members, power levels, join rules, spaces,
aliases, and the bot profile — never the token.

Ordering: the consumer CR ships in the app Kustomization (fleet `apps`
dependsOn `infra-configs`), so it reconciles after the tuwunel HelmRelease
is Ready. Terraform `dependsOn` is Terraform-CR-only upstream, so NO
dependsOn entry — the fleet ordering above is the mechanism.

Upstream wiring: provider auth (`homeserver_url`, `access_token`, `user_id`)
flows from the vault via the ESO-synced `matrix-rooms-terraform-vars`
Secret (same-namespace `varsFrom` in the consumer CR; keys land under the
root var names so no `varsKeys` renames). NO token literal in git, NO
manual per-env fill. Same-namespace `varsFrom` CANNOT cross namespaces (no
namespace field) — direct vault read through the cluster-scoped
`proton-pass` ClusterSecretStore, same pattern as the netbird consumer
example. Backend: in-cluster Kubernetes default (state Secrets in the
consumer team's namespace) — no backendConfig needed. Drift detection stays
on (default). Outputs (`room_id`, `canonical_alias`, `bot_user_id`) land in
`<room>-outputs` via `writeOutputsToSecret`.

First-run prerequisite: the bootstrap runbook below minted the bot + token
once per env. Without the `matrix-rooms-terraform-vars` Secret the CR
retries on interval — no vault seeding, no console step.

## Bootstrap runbook (once per env, outside Terraform)

Prerequisites: tuwunel is Ready; you hold the `registration_shared_secret`
(≥32 random bytes, server-side `registration_shared_secret_file`, served
only on a **trusted network path** — never commit it, never log it). The
secret enables the Synapse-compatible admin registration API, which tuwunel
serves (GET/POST `/_synapse/admin/v1/register`); it is NOT served when MAS
is active (then provision users in MAS instead and skip to token minting).

Pick distinct bot + alias per env (no shared prod/dev bot):
dev `@apprise-dev:<server>`, prd `@apprise:<server>`.

1. **Fetch a nonce** (unauthenticated, trusted network only):

   ```shell
   NONCE=$(curl -s https://tuwunel.matrix.home-ops.yansyah.my.id/_synapse/admin/v1/register | jq -r .nonce)
   ```

2. **Compute the HMAC** (`nonce\x00user\x00password\x00admin\x00?` —
   Synapse shared-secret register joins `nonce`, `user`, `password`,
   `admin` (`admin`/`notadmin`), and optionally `user_type`, with NUL bytes;
   key = the registration shared secret, SHA-1 hex):

   ```shell
   printf '%s\0%s\0%s\0%s' "$NONCE" "apprise" "$BOT_PASSWORD" "notadmin" \
     | openssl dgst -sha1 -hmac "$REG_SECRET" -hex | awk '{print $NF}'
   ```

3. **Register the bot** (admin=false — the bot needs no server admin):

   ```shell
   curl -s -X POST https://tuwunel.matrix.home-ops.yansyah.my.id/_synapse/admin/v1/register \
     -H 'Content-Type: application/json' \
     -d '{"nonce":"'"$NONCE"'","username":"apprise","password":"'"$BOT_PASSWORD"'","mac":"'"$MAC"'","admin":false}'
   ```

   Alternative without curl/HMAC: `tuwunel --execute 'users create_user
   "apprise" "password…" false'` (or `make_user_admin` for an admin), run
   against the server config on trusted infra.

4. **Mint the long-lived access token.** Either log in once
   (`POST /_matrix/client/v3/login` with the bot password → `access_token`)
   or mint server-side without a password round-trip
   (`POST /_synapse/admin/v1/users/<mxid>/login` with an admin bearer →
   `access_token`). Store the token per the strategy below; the password can
   then be rotated/discarded.

5. **Seed the vault** (one-time per env; the ESO ExternalSecret in the
   consumer CR reads these — Terraform never sees a literal):

   ```shell
   pass insert __PROTON_PASS_BASE__/matrix-rooms/homeserver-url    # https://tuwunel.matrix.home-ops.yansyah.my.id
   pass insert __PROTON_PASS_BASE__/matrix-rooms/bot-access-token  # from step 4 (sensitive)
   pass insert __PROTON_PASS_BASE__/matrix-rooms/bot-user-id       # @apprise(-dev):<server>
   ```

   Env vars for manual runs (never commit): `MATRIX_HOMESERVER_URL`,
   `MATRIX_ACCESS_TOKEN` (sensitive), `MATRIX_USER_ID` (optional, inferred
   from `/whoami`).

## Token strategy (chosen: manual vault v1)

- **(a) Vault → ESO manual (CHOSEN v1).** The token minted above lives in
  the vault; ESO mirrors it into `matrix-rooms-terraform-vars`; the CR
  consumes it via `varsFrom`. Rotation = re-login, update the vault entry,
  ESO re-syncs within `refreshInterval`. No in-cluster writer, no password
  in git — matches the netbird consumer pattern.
- **(b) CronJob re-login → Secret (NOT chosen).** A CronJob holding the bot
  *password* re-logs-in and writes the Secret. Rejected v1: stores a second
  long-lived credential (the password) to protect the first, for no gain —
  access tokens do not expire on tuwunel, so scheduled rotation buys
  nothing. Revisit only if the homeserver starts expiring tokens.
- **(c) Per-env bot + alias (ADOPTED alongside (a)).** Separate mxids (and
  room aliases) per env — `@apprise-dev` vs `@apprise` — so dev applies can
  never post into prod rooms. This is scoping, not storage: it composes
  with (a).

Hard rules: no token in Git (varsFrom-only), no shared prod/dev bot, no
`login_with_password=true` equivalent in Terraform (the provider takes a
token, never a password).

## State backend + CR shape

- Backend: tofu-controller in-cluster Kubernetes default — state lives in
  Secrets in the consumer team's namespace; no `backendConfig`, no remote
  state, no cross-slice reads.
- Secret refs: `MATRIX_HOMESERVER_URL` → `homeserver_url`,
  `MATRIX_ACCESS_TOKEN` → `access_token`, `MATRIX_USER_ID` → `user_id`
  (provider env fallbacks; explicit CR vars win on collision).
- CR shape: `sourceRef` (apps/matrix OCI artifact, `path:
  ./terraform`) + plain `vars` (room config) + `varsFrom`
  (`matrix-rooms-terraform-vars`) + `writeOutputsToSecret`
  (`<room>-outputs`: `room_id`, `canonical_alias`, `bot_user_id`).
  `destroy: false` + `destroyResourcesOnDeletion: false` (upsert-only).

## Usage (manual, no live apply in CI)

```shell
cd flux/apps/components/matrix/terraform
tofu init -backend=false
tofu validate
```

Manual plan needs the three provider vars (env or `-var`); CI runs
init + validate only.

## Room resources (flat root)

One flat root = one room (+ optional parent space): `matrix_room`
(`private_chat`, private visibility) + `matrix_room_member` +
`matrix_room_power_levels` (bot pinned at 100) +
`matrix_room_join_rules` (invite default, restricted gated on spaces) +
optional `matrix_space` (+child) + `matrix_room_alias` + bot identity
(`matrix_user_profile` global, `matrix_user_profile_override` per room).

## Why the root looks like this

- **Bot + token are NOT manageable by the provider** (no user/token
  resources), so bootstrap happens once outside Terraform — see the runbook
  above. This root assumes the bot + token already exist.
- **Power levels are self-lockout safe.** A declared `users` map *replaces*
  the whole map homeserver-side; omitting the provider account drops it to
  `users_default` (below `state_default` = no more power-level writes, and
  `destroy` undoes nothing). The root always merges
  `(data.matrix_whoami.me.user_id) = 100` over `var.power_levels`, so no
  caller can demote the bot by accident.
- **Encryption is irreversible** (`encryption_enabled` cannot go true ->
  false). Default `true`; pick it at creation and never flip it.
- **No `matrix_room_server_acl`.** Federation stays off on the tuwunel
  homeserver, and a bad ACL is unfixable (remote servers reject the room's
  events *and* the corrective ACL). Never add one here.

## Inputs (at least the required nine + friends)

| Var | Purpose |
| --- | ------- |
| `room_name` | Room display name (required). |
| `topic` | Room topic. |
| `preset` | Creation preset (`private_chat` default; creation-only). |
| `visibility` | Directory visibility (`private` default; `public` only where the homeserver allows publication). |
| `room_alias_name` / `extra_aliases` | Canonical alias localpart + extra `#name:server` aliases. |
| `encryption_enabled` | E2EE at creation (default true, irreversible). |
| `members` | `{ mxid = invite\|join\|leave\|ban\|knock }` — declarative; `invite` re-fires after a later leave. |
| `power_levels` | Extra `{ mxid = level }` merged *under* the bot's pinned 100 + `users_default` / `events_default` / `state_default` / `invite_power` / `kick_power` / `ban_power` / `redact_power` knobs. |
| `join_rule` / `allow_spaces` | `invite` default; `restricted` / `knock_restricted` gate on space IDs. |
| `create_space` / `space_*` | Optional parent space + `m.space.child` link (`via` required by spec). |
| `bot_display_name` / `bot_avatar_url` | Global bot profile (at most one `matrix_user_profile` per provider identity). |
| `bot_room_display_name` / `bot_room_avatar_url` | Per-room bot face in this room (applies after the global profile). |

Local-only call shape lives in `examples/complete/main.tf` (two rooms
sourcing `../..`, the flat root). The live consumer CRs
(`../base/rooms.yaml` — `flux-notifications` + `tofu-runs` +
`coder-notifications`) render the plain vars and inject the secrets via
`varsFrom` (the `examples/*-terraform.yaml` files are copy-paste
skeletons).

## Import paths

Adopt rooms the bot already created without recreating them:

```shell
tofu import matrix_room.this '!abcDEF:server'
tofu import matrix_room_power_levels.this '!abcDEF:server'
tofu import matrix_room_join_rules.this '!abcDEF:server'
tofu import 'matrix_room_member.this["@alice:server"]' '!abcDEF:server|@alice:server'
tofu import matrix_room_alias.extra['#alt:server'] '#alt:server'
tofu import matrix_space.this[0] '!xyzGHI:server'
tofu import matrix_space_child.this[0] '!xyzGHI:server|!abcDEF:server'
tofu import matrix_user_profile.bot '@bot:server'            # must equal the caller's mxid
tofu import 'matrix_user_profile_override.bot[0]' '!abcDEF:server|@bot:server'
```

`preset` / `initial_invites` have no read-back endpoint: the first plan
after import shows an in-place update that changes nothing homeserver-side
(it records the declared values; later plans are clean). Rooms cannot be
deleted server-side — `destroy` only makes the bot leave; the room lingers.

## Room version 12 caveat

The creator keeps power without a `users` entry on v12+ rooms, and the
homeserver *rejects* a power event listing a creator. The bot creates these
rooms, so if the homeserver is v12 and the pinned bot entry is rejected at
plan/apply time, that is the cause — drop the pin only then (and keep the
bot's power via creator status).
