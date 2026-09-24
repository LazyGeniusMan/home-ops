# Matrix rooms Terraform — bootstrap + reconcile

Machine-applied by Tofu Controller. Owns ONLY per-team rooms/spaces on the
tuwunel homeserver (`tuwunel.matrix.home-ops.yansyah.my.id`, sibling task):
the flat root here (`main.tf` + `variables.tf` + `outputs.tf`) plus one consumer Terraform CR
per room. Live ops rooms ship in `../base/rooms.yaml`
(`flux-notifications` + `tofu-runs` + `coder-notifications`); the `./examples/` files
(`flux-notifications-terraform.yaml`, `tofu-runs-terraform.yaml`,
`team-terraform.yaml`) are copy-paste skeletons for TEAM namespaces only.

Upstream reference (read-only): `/tmp/home-ops-docs/matrix-terraform-provider-docs`.

The provider (`raspbeguy/matrix ~> 0.5`) has **no user/token resources**, so
the bot + token are bootstrapped ONCE outside Terraform — by the
`matrix-bot-bootstrap` Job (`../base/matrix-bot-bootstrap.yaml`, kept Secret
`matrix-bot-bootstrap-outputs`), NOT by hand (runbook below is first-seed-only).
Terraform then manages rooms, members, power levels, join rules, spaces,
aliases, and the bot profile — never the token.

Ordering: the consumer CR ships in the app Kustomization (fleet `apps`
dependsOn `infra-configs`), so it reconciles after the tuwunel HelmRelease
is Ready. Terraform `dependsOn` is Terraform-CR-only upstream, so NO
dependsOn entry — the fleet ordering above is the mechanism.

Upstream wiring: provider auth (`homeserver_url`, `access_token`, `user_id`)
flows from the bootstrap Job's KEPT Secret `matrix-bot-bootstrap-outputs`
(same-namespace `varsFrom` in the consumer CR; keys land under the
root var names so no `varsKeys` renames; NO ESO mirror hop — same namespace
needs none). NO token literal in git, NO manual per-env fill, NO vault hop.
Backend: in-cluster Kubernetes default (state Secrets in the
consumer team's namespace) — no backendConfig needed. Drift detection stays
on (default). Outputs (`room_id`, `canonical_alias`, `bot_user_id`) land in
`<room>-outputs` via `writeOutputsToSecret`.

First-run prerequisite: the bootstrap Job ran once per env (vault first-seed
`matrix/tuwunel-registration-secret` ONLY — the runbook below). Without the
kept Secret the CR retries on interval — no bot vault seeding, no console step.

## Bootstrap runbook (first-seed ONLY — bot creation is automated)

The `matrix-bot-bootstrap` Job (`../base/matrix-bot-bootstrap.yaml`) owns
bot creation end-to-end: it registers the bot via the Synapse-compat admin
API + mints the token + writes the KEPT Secret
`matrix-bot-bootstrap-outputs` the room CRs read. The ONLY manual step left
is seeding the registration secret ONCE per env (the Job handles
nonce/HMAC/register/login/token minting):

```shell
pass-cli item create login --vault-name 'acme-<env>-bdo1-talos-apps-01' --title 'matrix/tuwunel-registration-secret'  # 32+ random bytes
```

Then verify (no console step, no token handling):

```shell
kubectl -n matrix get job matrix-bot-bootstrap
kubectl -n matrix get secret matrix-bot-bootstrap-outputs -o jsonpath='{.data}' | jq 'keys'
```

Rerun/rotation: `kubectl -n matrix delete job matrix-bot-bootstrap` +
Flux reconcile — the Job re-registers (M_USER_IN_USE path fails LOUD with
the server-side delete recovery step; see matrix-bot-bootstrap.yaml) and
rotates the token (kept Secret patched -> ESO/varsFrom propagate within
`refreshInterval`).

Per-env bots (no shared prod/dev bot): dev `@apprise-dev:<server>`,
prd `@apprise:<server>` — the Job mints the SAME identities (ONE bot per
env shared by all 3 rooms). The Job registers via the Synapse-compatible
admin API + mints the token server-side; there is no manual HMAC/nonce/vault
path.

## Token strategy

The Job mints the token server-side and writes the kept Secret
`matrix-bot-bootstrap-outputs`; the CRs consume it via `varsFrom`, the
apprise ES via the in-cluster store. Rotation = delete Job + reconcile.
Separate mxids per env (`@apprise-dev` dev, `@apprise` prd) so dev applies
never post into prod rooms.

## State backend + CR shape

- Backend: tofu-controller in-cluster Kubernetes default — state lives in
  Secrets in the consumer team's namespace; no `backendConfig`, no remote
  state, no cross-slice reads.
- Secret refs: `MATRIX_HOMESERVER_URL` → `homeserver_url`,
  `MATRIX_ACCESS_TOKEN` → `access_token`, `MATRIX_USER_ID` → `user_id`
  (provider env fallbacks; explicit CR vars win on collision).
- CR shape: `sourceRef` (apps/matrix OCI artifact, `path:
  ./terraform`) + plain `vars` (room config) + `varsFrom`
  (`matrix-bot-bootstrap-outputs`, the Job's kept Secret) +
  `writeOutputsToSecret`
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
  resources), so the bootstrap Job creates them once outside Terraform —
  see the runbook above. This root assumes the bot + token already exist
  (kept Secret `matrix-bot-bootstrap-outputs`).
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
