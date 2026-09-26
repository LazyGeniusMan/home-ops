terraform {
  # Capped floor: Flox ships tofu 1.12.6. The staged .terraform.lock.hcl is
  # gitignored at repo root (build/<cluster>/netbird-tf/ only), so the exact
  # provider pin below (+ this bound) is the reproducibility mechanism —
  # re-lock explicitly when bumping.
  required_version = ">= 1.11, < 2.0"
  required_providers {
    netbird = {
      # Exact pin: the NetBird fabric is account-global, so every apply must
      # run the same provider build.
      source  = "netbirdio/netbird"
      version = "0.0.10"
    }
  }
}
