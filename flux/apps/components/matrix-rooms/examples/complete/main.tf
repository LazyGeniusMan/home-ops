# Local-only example: flux-notifications style test room with locked power
# levels. Not applied in CI (no live homeserver); validates the module call
# shape. Real deploys go through the Terraform CR
# (flux-notifications-terraform.yaml) + varsFrom Secret, never literals.
#
#   cd flux/apps/components/matrix-rooms/examples/complete
#   tofu init -backend=false && tofu validate

terraform {
  required_version = ">= 1.11"
  required_providers {
    matrix = {
      source  = "raspbeguy/matrix"
      version = "~> 0.5"
    }
  }
}

provider "matrix" {
  # Reads MATRIX_HOMESERVER_URL / MATRIX_ACCESS_TOKEN / MATRIX_USER_ID.
  # Export dummies for offline validate; real values come from the vault.
}

module "flux_notifications" {
  source = "../../terraform/module"

  room_name       = "flux-notifications"
  topic           = "Flux + Apprise delivery receipts (bot posts, humans read)"
  room_alias_name = "flux-notifications"

  members = {
    "@oncall-lead:tuwunel.matrix.home-ops.yansyah.my.id" = "invite"
  }

  # Locked power levels: bot pinned at 100 by the module; lead at 50;
  # everyone else at users_default 0 (post via events_default 0 only).
  power_levels = {
    "@oncall-lead:tuwunel.matrix.home-ops.yansyah.my.id" = 50
  }

  join_rule = "invite"

  bot_room_display_name = "Flux Notifier"
}

output "room_id" {
  value = module.flux_notifications.room_id
}
