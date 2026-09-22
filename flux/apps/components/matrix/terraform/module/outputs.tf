output "room_id" {
  description = "Matrix room ID of the managed room (!abc:server)."
  value       = matrix_room.this.id
}

output "canonical_alias" {
  description = "Canonical alias the homeserver set on the room (from room_alias_name)."
  value       = matrix_room.this.canonical_alias
}

output "space_id" {
  description = "Space ID when create_space is true, else empty string."
  value       = var.create_space ? matrix_space.this[0].id : ""
}

output "bot_user_id" {
  description = "mxid of the bot account the provider runs as (matrix_whoami)."
  value       = data.matrix_whoami.me.user_id
}
