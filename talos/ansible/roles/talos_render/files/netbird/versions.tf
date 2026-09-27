terraform {
  # Capped to the tested minor line: Flox ships tofu 1.12.6. The staged
  # .terraform.lock.hcl is gitignored at repo root (build/<cluster>/netbird-tf/
  # only), so the exact provider pin below (+ this bound) is the
  # reproducibility mechanism — bump this bound deliberately with a re-lock
  # after testing the new minor (never widen silently).
  required_version = ">= 1.11, < 1.13"
  required_providers {
    netbird = {
      # Exact pin: the NetBird fabric is account-global, so every apply must
      # run the same provider build.
      source  = "netbirdio/netbird"
      version = "0.0.10"
    }
  }
}
