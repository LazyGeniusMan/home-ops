# Flat single-layer root (shipped inside the apps/matrix OCI artifact). Consumers (one
# Terraform CR per room) reference it via same-namespace sourceRef + their own vars.
# Single layer: provider auth + room resources directly, no child modules.

provider "matrix" {
  homeserver_url = var.homeserver_url
  access_token   = var.access_token
  user_id        = var.user_id
}

# Reusable per-team room — locked-down defaults, self-lockout safe.
# Wraps matrix_room + member + power_levels (bot always pinned at 100) + join_rules +
# optional space (+child) + alias + bot profile (_override). Never matrix_room_server_acl
# (federation stays off; a bad ACL is unfixable).
# Power semantics: a declared `users` map REPLACES the whole map homeserver-side, so this
# root always merges the caller var with the provider account at 100. On v12+ rooms the
# creator keeps power without a `users` entry — drop the pin only then (see README).

data "matrix_whoami" "me" {}

resource "matrix_room" "this" {
  name               = var.room_name
  topic              = var.topic
  preset             = var.preset
  visibility         = var.visibility
  history_visibility = var.history_visibility
  room_alias_name    = var.room_alias_name
  encryption_enabled = var.encryption_enabled

  lifecycle {
    # Encryption is irreversible (true -> false must fail closed).
    prevent_destroy = false
  }
}

resource "matrix_room_member" "this" {
  for_each   = var.members
  room_id    = matrix_room.this.id
  user_id    = each.key
  membership = each.value
}

resource "matrix_room_power_levels" "this" {
  room_id        = matrix_room.this.id
  users_default  = var.users_default
  events_default = var.events_default
  state_default  = var.state_default
  invite         = var.invite_power
  kick           = var.kick_power
  ban            = var.ban_power
  redact         = var.redact_power

  # Caller overrides + bot pinned at 100 (bot wins on collision).
  users = merge(
    var.power_levels,
    { (data.matrix_whoami.me.user_id) = 100 },
  )
}

resource "matrix_room_join_rules" "this" {
  room_id   = matrix_room.this.id
  join_rule = var.join_rule
  # restricted/knock_restricted gate on space membership; others take no allow list.
  allow_rooms = contains(["restricted", "knock_restricted"], var.join_rule) ? toset(var.allow_spaces) : null
}

# Optional parent space + child link (via required by spec; a link with no
# via/order/suggested reads as removed on refresh).
resource "matrix_space" "this" {
  count           = var.create_space ? 1 : 0
  name            = var.space_name
  topic           = var.space_topic
  preset          = var.preset
  visibility      = var.visibility
  room_alias_name = var.space_alias_name
}

resource "matrix_space_child" "this" {
  count           = var.create_space ? 1 : 0
  parent_space_id = matrix_space.this[0].id
  child_room_id   = matrix_room.this.id
  suggested       = true
  via             = toset(var.space_via)
}

# Extra directory aliases (canonical alias rides room_alias_name).
resource "matrix_room_alias" "extra" {
  for_each = toset(var.extra_aliases)
  alias    = each.value
  room_id  = matrix_room.this.id
}

# Bot identity: at most ONE matrix_user_profile per provider identity (gate on nulls so
# per-room-only rooms do not fight the global profile). Destroy leaves the profile as-is.
resource "matrix_user_profile" "bot" {
  count        = var.bot_display_name != null || var.bot_avatar_url != null ? 1 : 0
  display_name = var.bot_display_name
  avatar_url   = var.bot_avatar_url
}

# Per-room bot override. depends_on the global profile (Synapse propagates global changes
# over member events and would wipe this override if it applied last).
resource "matrix_user_profile_override" "bot" {
  count        = var.bot_room_display_name != null || var.bot_room_avatar_url != null ? 1 : 0
  room_id      = matrix_room.this.id
  user_id      = data.matrix_whoami.me.user_id
  display_name = var.bot_room_display_name
  avatar_url   = var.bot_room_avatar_url

  depends_on = [matrix_user_profile.bot]
}
