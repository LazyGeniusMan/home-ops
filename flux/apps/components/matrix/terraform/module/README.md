# matrix-room module (raspbeguy/matrix ~> 0.5)

Reusable locked-down per-team room. One module call = one room (+ optional
parent space): `matrix_room` (private_chat, private visibility) +
`matrix_room_member` + `matrix_room_power_levels` (bot pinned at 100) +
`matrix_room_join_rules` (invite default, restricted gated on spaces) +
optional `matrix_space` (+child) + `matrix_room_alias` + bot identity
(`matrix_user_profile` global, `matrix_user_profile_override` per room).

## Why the module looks like this

- **Bot + token are NOT manageable by the provider** (no user/token
  resources), so bootstrap happens once outside Terraform — see
  `../README.md`. This module assumes the bot + token already exist.
- **Power levels are self-lockout safe.** A declared `users` map *replaces*
  the whole map homeserver-side; omitting the provider account drops it to
  `users_default` (below `state_default` = no more power-level writes, and
  `destroy` undoes nothing). The module always merges
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

## Usage

```hcl
module "flux_notifications" {
  source = "./module" # thin root; direct module users pin the registry path

  room_name         = "flux-notifications"
  topic             = "Flux + Apprise delivery receipts"
  room_alias_name   = "flux-notifications"
  members           = { "@oncall-lead:tuwunel.matrix.home-ops.yansyah.my.id" = "invite" }
  power_levels      = { "@oncall-lead:tuwunel.matrix.home-ops.yansyah.my.id" = 50 }
  join_rule         = "invite"
  bot_room_display_name = "Flux Notifier"
}
```

The thin root (`../`) exposes the same vars plus `homeserver_url` /
`access_token` / `user_id` provider auth; the live consumer CRs
(`../../base/rooms.yaml` — `flux-notifications` + `tofu-runs`) render the
plain vars and inject the secrets via `varsFrom` (the
`../examples/*-terraform.yaml` files are copy-paste skeletons).

## Import paths

Adopt rooms the bot already created without recreating them:

```shell
tofu import module.room.matrix_room.this '!abcDEF:server'
tofu import module.room.matrix_room_power_levels.this '!abcDEF:server'
tofu import module.room.matrix_room_join_rules.this '!abcDEF:server'
tofu import 'module.room.matrix_room_member.this["@alice:server"]' '!abcDEF:server|@alice:server'
tofu import module.room.matrix_room_alias.extra['#alt:server'] '#alt:server'
tofu import module.room.matrix_space.this[0] '!xyzGHI:server'
tofu import module.room.matrix_space_child.this[0] '!xyzGHI:server|!abcDEF:server'
tofu import module.room.matrix_user_profile.bot '@bot:server'            # must equal the caller's mxid
tofu import 'module.room.matrix_user_profile_override.bot[0]' '!abcDEF:server|@bot:server'
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
