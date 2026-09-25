# Flat root vars: provider auth + room inputs. Consumer CR renders plain vars here;
# secrets (homeserver_url/access_token/user_id) arrive via varsFrom (overrides vars on collision).

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
  description = "Room display name (m.room.name), e.g. flux-notifications."
  type        = string
}

variable "topic" {
  description = "Room topic (m.room.topic)."
  type        = string
  default     = null
}

variable "preset" {
  description = "Creation preset: private_chat | trusted_private_chat | public_chat. Creation-only."
  type        = string
  default     = "private_chat"
  validation {
    condition     = contains(["private_chat", "trusted_private_chat", "public_chat"], var.preset)
    error_message = "preset must be one of private_chat, trusted_private_chat, public_chat."
  }
}

variable "visibility" {
  description = "Room directory visibility: private (default) | public."
  type        = string
  default     = "private"
  validation {
    condition     = contains(["private", "public"], var.visibility)
    error_message = "visibility must be private or public."
  }
}

variable "history_visibility" {
  description = "Who can read the timeline: joined | invited | shared | world_readable."
  type        = string
  default     = "shared"
  validation {
    condition     = var.history_visibility == null ? true : contains(["joined", "invited", "shared", "world_readable"], var.history_visibility)
    error_message = "history_visibility must be one of joined, invited, shared, world_readable."
  }
}

variable "room_alias_name" {
  description = "Localpart of the canonical alias set at creation (e.g. flux-notifications). Omit (null) for no canonical alias."
  type        = string
  default     = null
}

variable "extra_aliases" {
  description = "Extra directory aliases (#name:server) pointing at the same room, managed via matrix_room_alias."
  type        = list(string)
  default     = []
}

variable "encryption_enabled" {
  description = "Enable end-to-end encryption at creation time. IRREVERSIBLE: cannot be disabled once set."
  type        = bool
  default     = true
}

variable "members" {
  description = "Membership intents keyed by mxid: invite | join | leave | ban | knock. Bot itself needs no entry."
  type        = map(string)
  default     = {}
  validation {
    condition     = alltrue([for m in values(var.members) : contains(["invite", "join", "leave", "ban", "knock"], m)])
    error_message = "member values must be one of invite, join, leave, ban, knock."
  }
}

variable "power_levels" {
  description = "Extra per-user power overrides keyed by mxid. Bot is ALWAYS pinned at 100 by the root; a declared users map replaces the whole map homeserver-side, so keep this list complete."
  type        = map(number)
  default     = {}
}

variable "users_default" {
  description = "Default power for unlisted users. Keep below state_default."
  type        = number
  default     = 0
}

variable "events_default" {
  description = "Default power to send message events."
  type        = number
  default     = 0
}

variable "state_default" {
  description = "Default power to send state events."
  type        = number
  default     = 50
}

variable "invite_power" {
  description = "Power required to invite (maps to the provider invite field)."
  type        = number
  default     = 50
}

variable "kick_power" {
  description = "Power required to kick."
  type        = number
  default     = 50
}

variable "ban_power" {
  description = "Power required to ban."
  type        = number
  default     = 100
}

variable "redact_power" {
  description = "Power required to redact another user's event."
  type        = number
  default     = 50
}

variable "join_rule" {
  description = "Join rule: invite (default, locked down) | public | knock | restricted | knock_restricted. restricted/knock_restricted require allow_spaces."
  type        = string
  default     = "invite"
  validation {
    condition     = contains(["invite", "public", "knock", "restricted", "knock_restricted"], var.join_rule)
    error_message = "join_rule must be one of invite, public, knock, restricted, knock_restricted."
  }
}

variable "allow_spaces" {
  description = "Space room IDs whose members may join when join_rule is restricted/knock_restricted."
  type        = list(string)
  default     = []
}

variable "create_space" {
  description = "Also create a parent space and link the room under it (m.space.child)."
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
  description = "Localpart of the canonical alias for the space. Omit (null) for none."
  type        = string
  default     = null
}

variable "space_via" {
  description = "Server names for the m.space.child join hint (spec requires via)."
  type        = list(string)
  default     = []
}

variable "bot_display_name" {
  description = "Global bot display name (matrix_user_profile). Null = leave untouched."
  type        = string
  default     = null
}

variable "bot_avatar_url" {
  description = "Global bot avatar mxc:// URI (matrix_user_profile). Null = leave untouched."
  type        = string
  default     = null
}

variable "bot_room_display_name" {
  description = "Per-room bot display name in THIS room (matrix_user_profile_override). Null = no override."
  type        = string
  default     = null
}

variable "bot_room_avatar_url" {
  description = "Per-room bot avatar mxc:// URI in THIS room. Null = no override."
  type        = string
  default     = null
}
