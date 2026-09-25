# Local-only example: notifier-room shape (flux-notifications + tofu-runs). Not applied in
# CI (no live homeserver); validates the flat root call shape. Real deploys go through the
# Terraform CRs + varsFrom Secret, never literals. Notification-only (events_default 50) +
# PLAINTEXT (encryption_enabled false — E2EE unreadable to the stateless notifier).
#
#   cd flux/apps/components/matrix/terraform/examples/complete
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
  # Reads MATRIX_HOMESERVER_URL / MATRIX_ACCESS_TOKEN / MATRIX_USER_ID (dummies for offline validate).
}

module "flux_notifications" {
  source = "../.."

  room_name          = "flux-notifications"
  topic              = "Flux + Apprise delivery receipts (bot posts, humans read)"
  room_alias_name    = "flux-notifications"
  encryption_enabled = false
  events_default     = 50

  members = {
    "@oncall-lead:tuwunel.matrix.home-ops.yansyah.my.id" = "invite"
  }

  # Bot pinned at 100 by the root; lead at 50; rest read-only.
  power_levels = {
    "@oncall-lead:tuwunel.matrix.home-ops.yansyah.my.id" = 50
  }

  join_rule = "invite"

  bot_room_display_name = "Flux Notifier"
}

module "tofu_runs" {
  source = "../.."

  room_name          = "tofu-runs"
  topic              = "Terraform/tofu per-run log (bot posts, humans read)"
  room_alias_name    = "tofu-runs"
  encryption_enabled = false
  events_default     = 50

  members = {
    "@oncall-lead:tuwunel.matrix.home-ops.yansyah.my.id" = "invite"
  }

  power_levels = {
    "@oncall-lead:tuwunel.matrix.home-ops.yansyah.my.id" = 50
  }

  join_rule = "invite"

  bot_room_display_name = "Tofu Runner"
}

output "room_id" {
  value = module.flux_notifications.room_id
}

output "tofu_runs_room_id" {
  value = module.tofu_runs.room_id
}
