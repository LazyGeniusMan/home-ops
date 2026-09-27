# Matrix rooms Terraform — bootstrap + reconcile

Machine-applied by Tofu Controller. Owns ONLY per-team rooms/spaces on the tuwunel homeserver: the flat root (`main.tf` + `variables.tf` + `outputs.tf`) plus one consumer Terraform CR per room. Live ops rooms ship in `../base/rooms.yaml`; `./examples/` files are copy-paste skeletons for team namespaces.

The provider (`raspbeguy/matrix ~> 0.5`) has no user/token resources, so the bot + token are bootstrapped by the `matrix-bot-bootstrap` Job (kept Secret `matrix-bot-bootstrap-outputs`). Terraform manages rooms, members, power levels, join rules, spaces, aliases, and the bot profile — never the token.

## Contract

- Auth: `homeserver_url`/`access_token`/`user_id` from the kept Secret via same-namespace `varsFrom` (no token literal in git). Backend: in-cluster Kubernetes default.
- Outputs (`room_id`, `canonical_alias`, `bot_user_id`) land in `<room>-outputs` via `writeOutputsToSecret`.
- `destroy: false` + `destroyResourcesOnDeletion: false` (upsert-only). Rooms cannot be deleted server-side (`destroy` only makes the bot leave).
- One flat root = one room (+ optional parent space): `matrix_room` + `matrix_room_member` + `matrix_room_power_levels` (bot pinned at 100, self-lockout safe) + `matrix_room_join_rules` + optional `matrix_space` (+child) + `matrix_room_alias`. Encryption is irreversible (default `true`). No `matrix_room_server_acl` (federation off; a bad ACL is unfixable).
- Inputs: nine required groups (`room_name`, `topic`, `preset`, `visibility`, aliases, encryption, members, power levels, join rule/spaces, bot profile) — see `variables.tf`. Import shape in `main.tf`; `preset`/`initial_invites` have no read-back.

## Usage

Seed per env (one vault field), then Flux reconciles declaratively. Per-env bots: `@apprise-dev` (dev) / `@apprise` (prd). Rerun = delete the Job + reconcile.

```shell
pass-cli item create login --vault-name 'acme-<env>-bdo1-talos-apps-01' --title 'matrix/tuwunel-registration-secret'  # 32+ random bytes
```

```shell
cd flux/apps/components/matrix/terraform
tofu init -backend=false && tofu validate
```
