terraform {
  # Tested with tofu 1.12.6; bump + re-lock after testing. Exact provider pin below.
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
