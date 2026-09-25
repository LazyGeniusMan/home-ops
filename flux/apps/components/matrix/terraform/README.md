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
The consumer CR ships in the app Kustomization (fleet `apps` dependsOn
`infra-configs`), so it reconciles after tuwunel is Ready.

Provider auth (`homeserver_url`, `access_token`, `user_id`) flows from the
kept Secret via same-namespace `varsFrom` (no ESO hop, no token literal in
git). Backend: in-cluster Kubernetes default — no backendConfig. Outputs
(`room_id`, `canonical_alias`, `bot_user_id`) land in `<room>-outputs` via
`writeOutputsToSecret`. Prerequisite: the bootstrap Job ran per env (vault seed
`matrix/tuwunel-registration-secret` only); without the kept Secret the CR
retries on interval.

## Bootstrap (bot creation is automated)

Seed the registration secret once per env, then Flux reconciles declaratively:
the Job registers + mints the token into the kept Secret, and the consumer CRs
reconcile rooms against it. Per-env bots: dev `@apprise-dev:<server>`, prd
`@apprise:<server>` — ONE bot per env shared by all 3 rooms. Rerun/rotation:
delete the Job + Flux reconcile. Seed shape (one per env, run by hand):

```shell
pass-cli item create login --vault-name 'acme-<env>-bdo1-talos-apps-01' --title 'matrix/tuwunel-registration-secret'  # 32+ random bytes
```

## State backend + CR shape

Secret refs `MATRIX_*` → `homeserver_url`/`access_token`/`user_id` (explicit
vars win). CR shape: `sourceRef` (apps/matrix OCI, `path: ./terraform`) + plain
`vars` + `varsFrom` (`matrix-bot-bootstrap-outputs`) + `writeOutputsToSecret`
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
`_override`). Power levels are self-lockout safe (root merges the provider
account at 100 over `var.power_levels`). Encryption is irreversible (default
`true`). No `matrix_room_server_acl` (federation off; a bad ACL is unfixable).

## Inputs

Required nine + friends (`room_name`, `topic`, `preset`, `visibility`,
`room_alias_name`/`extra_aliases`, `encryption_enabled`, `members`,
`power_levels`, `join_rule`/`allow_spaces`, `create_space`/`space_*`,
`bot_display_name`/`bot_avatar_url`, `bot_room_display_name`/
`bot_room_avatar_url`) — see `variables.tf` for the full table. Local-only
call shape lives in `examples/complete/main.tf`. Live CRs
(`../base/rooms.yaml`) render plain vars + inject secrets via `varsFrom`;
`examples/*-terraform.yaml` are copy-paste skeletons.

## Import paths

Import shape lives alongside the root (see `main.tf` resource addresses);
`preset` / `initial_invites` have no read-back (first plan shows a no-op
in-place update). Rooms cannot be deleted server-side (`destroy` only makes the
bot leave). On v12+ rooms the creator keeps power without a `users` entry —
drop the pin only then.
