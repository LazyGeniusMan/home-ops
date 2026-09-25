# Matrix rooms Terraform — bootstrap + reconcile

Machine-applied by Tofu Controller. Owns ONLY per-team rooms/spaces on the
tuwunel homeserver: the flat root (`main.tf` + `variables.tf` + `outputs.tf`)
plus one consumer Terraform CR per room. Live ops rooms ship in
`../base/rooms.yaml`; `./examples/` files are copy-paste skeletons for TEAM
namespaces only.

Upstream reference (read-only): `/tmp/home-ops-docs/matrix-terraform-provider-docs`.

The provider (`raspbeguy/matrix ~> 0.5`) has **no user/token resources**, so
the bot + token are bootstrapped outside Terraform — by the
`matrix-bot-bootstrap` Job (`../base/matrix-bot-bootstrap.yaml`, kept Secret
`matrix-bot-bootstrap-outputs`). Terraform manages rooms, members, power
levels, join rules, spaces, aliases, and the bot profile — never the token.

Ordering: the consumer CR ships in the app Kustomization (fleet `apps`
dependsOn `infra-configs`), so it reconciles after tuwunel is Ready. No
Terraform `dependsOn` (Terraform-CR-only upstream).

Provider auth (`homeserver_url`, `access_token`, `user_id`) flows from the
kept Secret via same-namespace `varsFrom` (keys land under the root var
names, no `varsKeys` renames, no ESO hop). No token literal in git, no vault
hop. Backend: in-cluster Kubernetes default — no backendConfig. Outputs
(`room_id`, `canonical_alias`, `bot_user_id`) land in `<room>-outputs` via
`writeOutputsToSecret`.

Prerequisite: the bootstrap Job ran per env (vault seed
`matrix/tuwunel-registration-secret` only). Without the kept Secret the CR
retries on interval.

## Bootstrap runbook (bot creation is automated)

Seed the registration secret once per env (the Job does the rest: Synapse-compat
register + token mint + kept Secret):

```shell
pass-cli item create login --vault-name 'acme-<env>-bdo1-talos-apps-01' --title 'matrix/tuwunel-registration-secret'  # 32+ random bytes
kubectl -n matrix get job matrix-bot-bootstrap
kubectl -n matrix get secret matrix-bot-bootstrap-outputs -o jsonpath='{.data}' | jq 'keys'
```

Rerun/rotation: delete the Job + Flux reconcile. Per-env bots: dev
`@apprise-dev:<server>`, prd `@apprise:<server>` — ONE bot per env shared by
all 3 rooms.

## State backend + CR shape

Backend: tofu-controller in-cluster default (no `backendConfig`). Secret refs
`MATRIX_*` → `homeserver_url`/`access_token`/`user_id` (explicit vars win).
CR shape: `sourceRef` (apps/matrix OCI, `path: ./terraform`) + plain `vars` +
`varsFrom` (`matrix-bot-bootstrap-outputs`) + `writeOutputsToSecret`
(`<room>-outputs`). `destroy: false` + `destroyResourcesOnDeletion: false`
(upsert-only). Local check only (no live apply in CI):

```shell
cd flux/apps/components/matrix/terraform
tofu init -backend=false && tofu validate
```

## Room resources (flat root) + contract

One flat root = one room (+ optional parent space): `matrix_room` +
`matrix_room_member` + `matrix_room_power_levels` (bot pinned at 100) +
`matrix_room_join_rules` + optional `matrix_space` (+child) +
`matrix_room_alias` + bot identity (global `matrix_user_profile`, per-room
`_override`). Bot + token come from the bootstrap Job (never managed here).
Power levels are self-lockout safe (root merges the provider account at 100
over `var.power_levels`). Encryption is irreversible (default `true`). No
`matrix_room_server_acl` (federation off; a bad ACL is unfixable).

## Inputs (at least the required nine + friends)

| Var | Purpose |
| --- | ------- |
| `room_name` | Room display name (required). |
| `topic` | Room topic. |
| `preset` | Creation preset (`private_chat` default; creation-only). |
| `visibility` | Directory visibility (`private` default). |
| `room_alias_name` / `extra_aliases` | Canonical alias localpart + extra `#name:server` aliases. |
| `encryption_enabled` | E2EE at creation (default true, irreversible). |
| `members` | `{ mxid = invite\|join\|leave\|ban\|knock }` — declarative. |
| `power_levels` | Extra `{ mxid = level }` merged *under* the bot's pinned 100 + power knobs. |
| `join_rule` / `allow_spaces` | `invite` default; `restricted` / `knock_restricted` gate on space IDs. |
| `create_space` / `space_*` | Optional parent space + `m.space.child` link (`via` required). |
| `bot_display_name` / `bot_avatar_url` | Global bot profile (at most one per provider identity). |
| `bot_room_display_name` / `bot_room_avatar_url` | Per-room bot face (applies after the global profile). |

Local-only call shape lives in `examples/complete/main.tf`. Live CRs
(`../base/rooms.yaml`) render plain vars + inject secrets via `varsFrom`;
`examples/*-terraform.yaml` are copy-paste skeletons.

## Import paths

```shell
tofu import matrix_room.this '!abcDEF:server'
tofu import matrix_room_power_levels.this '!abcDEF:server'
tofu import matrix_room_join_rules.this '!abcDEF:server'
tofu import 'matrix_room_member.this["@alice:server"]' '!abcDEF:server|@alice:server'
tofu import matrix_room_alias.extra['#alt:server'] '#alt:server'
tofu import matrix_space.this[0] '!xyzGHI:server'
tofu import matrix_space_child.this[0] '!xyzGHI:server|!abcDEF:server'
tofu import matrix_user_profile.bot '@bot:server'
tofu import 'matrix_user_profile_override.bot[0]' '!abcDEF:server|@bot:server'
```

`preset` / `initial_invites` have no read-back (first plan shows a no-op in-place
update). Rooms cannot be deleted server-side (`destroy` only makes the bot leave).
On v12+ rooms the creator keeps power without a `users` entry — drop the pin only then.
