# Local-only example: notifier-room shape (flux-notifications + tofu-runs)
# with locked power levels. Not applied in CI (no live homeserver); validates
# the flat root call shape. Real deploys go through the Terraform CRs
# (flux-notifications-terraform.yaml / tofu-runs-terraform.yaml /
# team-terraform.yaml) + varsFrom Secret, never literals.
#
# Notifier rooms are notification-only (events_default 50: only the bot at
# 100 posts, members read) and PLAINTEXT (encryption_enabled false — the
# stateless apprise notifier has no Olm persistence, so E2EE rooms are
# unreadable to it; irreversible at creation).
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
  # Reads MATRIX_HOMESERVER_URL / MATRIX_ACCESS_TOKEN / MATRIX_USER_ID.
  # Export dummies for offline validate; real values come from the vault.
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

  # Locked power levels: bot pinned at 100 by the root; lead at 50;
  # everyone else at users_default 0 + events_default 50 (read-only).
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
