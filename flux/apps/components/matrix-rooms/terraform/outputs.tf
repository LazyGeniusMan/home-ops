output "room_id" {
  description = "Matrix room ID of the managed room."
  value       = module.room.room_id
}

output "canonical_alias" {
  description = "Canonical alias the homeserver set on the room."
  value       = module.room.canonical_alias
}

output "space_id" {
  description = "Space ID when create_space is true, else empty string."
  value       = module.room.space_id
}

output "bot_user_id" {
  description = "mxid of the bot account the provider runs as."
  value       = module.room.bot_user_id
}
