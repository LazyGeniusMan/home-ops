#!/usr/bin/env bash
# Fleet bootstrap tofu gate: CI terraform-job mirror for prek hooks.
# Init artifacts (.terraform/) are gitignored; no backend, no live apply.
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
cd "$root/flux/fleet/terraform" || exit 1
tofu init -backend=false
tofu validate
tofu test
