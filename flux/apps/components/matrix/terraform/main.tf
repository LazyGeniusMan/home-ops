# Thin root over ./module — shipped inside the apps/matrix OCI artifact
# (flux/apps/components/matrix/terraform/). Consumers (one Terraform CR
# per room, or per team space) reference this root via same-namespace
# sourceRef + their own vars, mirroring the infra/zitadel shared root.
#
# Flat single layer: provider auth + exactly one module call. No child
# modules beyond ./module; per-team variation rides vars, never forks.

provider "matrix" {
  homeserver_url = var.homeserver_url
  access_token   = var.access_token
  user_id        = var.user_id
}

module "room" {
  source = "./module"

  room_name             = var.room_name
  topic                 = var.topic
  preset                = var.preset
  visibility            = var.visibility
  history_visibility    = var.history_visibility
  room_alias_name       = var.room_alias_name
  extra_aliases         = var.extra_aliases
  encryption_enabled    = var.encryption_enabled
  members               = var.members
  power_levels          = var.power_levels
  users_default         = var.users_default
  events_default        = var.events_default
  state_default         = var.state_default
  invite_power          = var.invite_power
  kick_power            = var.kick_power
  ban_power             = var.ban_power
  redact_power          = var.redact_power
  join_rule             = var.join_rule
  allow_spaces          = var.allow_spaces
  create_space          = var.create_space
  space_name            = var.space_name
  space_topic           = var.space_topic
  space_alias_name      = var.space_alias_name
  space_via             = var.space_via
  bot_display_name      = var.bot_display_name
  bot_avatar_url        = var.bot_avatar_url
  bot_room_display_name = var.bot_room_display_name
  bot_room_avatar_url   = var.bot_room_avatar_url
}
