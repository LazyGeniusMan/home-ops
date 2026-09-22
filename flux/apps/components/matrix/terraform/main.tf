# Flat single-layer root — shipped inside the apps/matrix OCI artifact
# (flux/apps/components/matrix/terraform/). Consumers (one Terraform CR
# per room, or per team space) reference this root via same-namespace
# sourceRef + their own vars, mirroring the infra/zitadel shared root.
#
# Single layer: provider auth + room resources directly. No child
# modules; per-team variation rides vars, never forks.

provider "matrix" {
  homeserver_url = var.homeserver_url
  access_token   = var.access_token
  user_id        = var.user_id
}

# Reusable per-team Matrix room — locked-down defaults, self-lockout safe.
#
# Wraps matrix_room (private_chat + private visibility) + matrix_room_member +
# matrix_room_power_levels (bot always pinned at 100) + matrix_room_join_rules
# (invite default, restricted gated on allow_spaces) + optional matrix_space
# (+child) + matrix_room_alias + matrix_user_profile(_override) for the bot
# identity. Never matrix_room_server_acl: federation stays off on the tuwunel
# homeserver, and a bad ACL is irreversible (locks federation permanently).
#
# Power-level semantics (provider wholesale-map): a declared `users` map
# REPLACES the whole map homeserver-side, so this root always merges the
# caller's var.power_levels with the provider account at 100. Self-lockout
# rules enforced here:
#   1. bot (data.matrix_whoami.me.user_id) is always present at 100 —
#      omitting it would drop the bot to users_default, below state_default,
#      after which it can no longer change power levels (destroy undoes
#      nothing: power levels cannot be deleted);
#   2. room version 12+ caveat: the room CREATOR keeps power without a users
#      entry and the homeserver REJECTS a power event listing a creator. The
#      bot creates these rooms, so on a v12 homeserver the pinned entry may be
#      rejected at plan/apply time — drop the pin only then (see README).

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
    # Encryption is irreversible: a flip from true to false must fail closed
    # instead of silently drifting. Name/alias renames stay allowed.
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

  # Wholesale-map merge: caller overrides + bot pinned at 100. The bot entry
  # wins on collision so no caller can (accidentally) demote the provider.
  users = merge(
    var.power_levels,
    { (data.matrix_whoami.me.user_id) = 100 },
  )
}

resource "matrix_room_join_rules" "this" {
  room_id   = matrix_room.this.id
  join_rule = var.join_rule
  # restricted/knock_restricted gate on space membership; invite/public/knock
  # take no allow list (null = untouched).
  allow_rooms = contains(["restricted", "knock_restricted"], var.join_rule) ? toset(var.allow_spaces) : null
}

# Optional parent space + child link. via is required by the Matrix spec and
# load-bearing here: a link with no via/order/suggested reads as removed and
# disappears from state on refresh.
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

# Extra directory aliases on the room (canonical alias rides room_alias_name).
resource "matrix_room_alias" "extra" {
  for_each = toset(var.extra_aliases)
  alias    = each.value
  room_id  = matrix_room.this.id
}

# Bot identity: at most ONE matrix_user_profile per provider identity (every
# instance resolves to the caller's mxid and races last-writer-wins). Gate on
# nulls so rooms that only set a per-room override do not fight the global
# profile. Destroy drops state but leaves the profile as-is (no protocol
# delete; clearing would render the bot as a raw mxid).
resource "matrix_user_profile" "bot" {
  count        = var.bot_display_name != null || var.bot_avatar_url != null ? 1 : 0
  display_name = var.bot_display_name
  avatar_url   = var.bot_avatar_url
}

# Per-room bot override (different face in this room). Membership must exist
# first (the bot joins by creating the room); depends_on the global profile
# because Synapse propagates global changes over member events and would wipe
# this override if it applied last (perpetual drift without the edge).
resource "matrix_user_profile_override" "bot" {
  count        = var.bot_room_display_name != null || var.bot_room_avatar_url != null ? 1 : 0
  room_id      = matrix_room.this.id
  user_id      = data.matrix_whoami.me.user_id
  display_name = var.bot_room_display_name
  avatar_url   = var.bot_room_avatar_url

  depends_on = [matrix_user_profile.bot]
}
