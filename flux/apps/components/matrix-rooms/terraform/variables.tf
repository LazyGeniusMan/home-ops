# Thin root: provider auth + one passthrough per module input. The consumer
# Terraform CR (examples/flux-notifications-terraform.yaml) renders plain
# (non-sensitive) vars here; secrets (homeserver_url/access_token/user_id)
# arrive via varsFrom from the ESO-synced matrix-rooms-terraform-vars Secret —
# varsFrom overrides vars on key collision (upstream GenerateVarsForTF).

variable "homeserver_url" {
  description = "Matrix homeserver base URL (e.g. https://tuwunel.matrix.home-ops.yansyah.my.id). Also settable via MATRIX_HOMESERVER_URL."
  type        = string
  default     = null
}

variable "access_token" {
  description = "Bot access token (controller injects via varsFrom from the ESO-synced Secret; manual runs pass -var, never commit). Also settable via MATRIX_ACCESS_TOKEN."
  type        = string
  sensitive   = true
  default     = null
}

variable "user_id" {
  description = "Bot mxid the provider runs as (e.g. @apprise:server). Optional: inferred from /whoami. Also settable via MATRIX_USER_ID."
  type        = string
  default     = null
}

variable "room_name" {
  description = "Room display name (m.room.name)."
  type        = string
}

variable "topic" {
  description = "Room topic (m.room.topic)."
  type        = string
  default     = null
}

variable "preset" {
  description = "Creation preset: private_chat | trusted_private_chat | public_chat."
  type        = string
  default     = "private_chat"
}

variable "visibility" {
  description = "Room directory visibility: private | public."
  type        = string
  default     = "private"
}

variable "history_visibility" {
  description = "Who can read the timeline: joined | invited | shared | world_readable."
  type        = string
  default     = "shared"
}

variable "room_alias_name" {
  description = "Localpart of the canonical alias set at creation."
  type        = string
  default     = null
}

variable "extra_aliases" {
  description = "Extra directory aliases (#name:server) pointing at the same room."
  type        = list(string)
  default     = []
}

variable "encryption_enabled" {
  description = "Enable end-to-end encryption at creation. IRREVERSIBLE."
  type        = bool
  default     = true
}

variable "members" {
  description = "Membership intents keyed by mxid."
  type        = map(string)
  default     = {}
}

variable "power_levels" {
  description = "Extra per-user power overrides keyed by mxid (bot pinned at 100 by the module)."
  type        = map(number)
  default     = {}
}

variable "users_default" {
  type        = number
  default     = 0
  description = "Default power for unlisted users."
}

variable "events_default" {
  type        = number
  default     = 0
  description = "Default power to send message events."
}

variable "state_default" {
  type        = number
  default     = 50
  description = "Default power to send state events."
}

variable "invite_power" {
  type        = number
  default     = 50
  description = "Power required to invite."
}

variable "kick_power" {
  type        = number
  default     = 50
  description = "Power required to kick."
}

variable "ban_power" {
  type        = number
  default     = 100
  description = "Power required to ban."
}

variable "redact_power" {
  type        = number
  default     = 50
  description = "Power required to redact another user's event."
}

variable "join_rule" {
  description = "Join rule: invite | public | knock | restricted | knock_restricted."
  type        = string
  default     = "invite"
}

variable "allow_spaces" {
  description = "Space IDs gating restricted/knock_restricted joins."
  type        = list(string)
  default     = []
}

variable "create_space" {
  description = "Also create a parent space and link the room under it."
  type        = bool
  default     = false
}

variable "space_name" {
  description = "Space display name (required when create_space is true)."
  type        = string
  default     = null
}

variable "space_topic" {
  description = "Space topic."
  type        = string
  default     = null
}

variable "space_alias_name" {
  description = "Localpart of the canonical alias for the space."
  type        = string
  default     = null
}

variable "space_via" {
  description = "Server names for the m.space.child join hint."
  type        = list(string)
  default     = []
}

variable "bot_display_name" {
  description = "Global bot display name (matrix_user_profile). Null = untouched."
  type        = string
  default     = null
}

variable "bot_avatar_url" {
  description = "Global bot avatar mxc:// URI. Null = untouched."
  type        = string
  default     = null
}

variable "bot_room_display_name" {
  description = "Per-room bot display name in this room. Null = no override."
  type        = string
  default     = null
}

variable "bot_room_avatar_url" {
  description = "Per-room bot avatar mxc:// URI in this room. Null = no override."
  type        = string
  default     = null
}
