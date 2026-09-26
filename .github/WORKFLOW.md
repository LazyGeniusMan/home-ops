# GitHub Actions

15 self-contained workflows in `.github/workflows/` (only pinned external
`owner/repo@sha` actions). Each header states its purpose + path filters.

- Validate: `flux-{infra,apps,fleet}-validate.yaml` (PR + main + dispatch,
  path-gated) — `flux/scripts/validate.sh`; fleet also runs `tofu`
  init/validate/test on `flux/fleet/terraform`.
- Lint: `lint-shell-ansible.yaml` (PR + main + dispatch, path-gated) —
  shellcheck + yamllint + ansible syntax/lint + talosctl client check
  (day-2 `--check --diff` stays local: needs a live `pass-cli` session).
- Push: `flux-{infra,apps,fleet}-push.yaml` (main pushes, path-gated) —
  OCI `dev` + `dev-<sha>` artifacts, cosign-signed.
- Release: `flux-{infra,apps,fleet}-release.yaml`
  (`flux-{infra,apps,fleet}-v*` tags) — OCI `stable` + version,
  cosign-signed.
- Image updates: `flux-image-updates.yaml` (image-updates-infra/apps branch
  creation + dispatch) —
  opens a PR to main per `image-updates-infra/apps` branch.
- Projects: `apprise-go-api.yml`, `eso-proton-pass.yml`,
  `external-dns-netbird.yml` (test + GHCR publish/sign),
  `helm-rclone.yml` (verify + chart OCI publish/sign).

All `uses:` are SHA-pinned, deny-all `permissions: {}` default with per-job
minimums, concurrency groups, path-gated triggers. Vendored `.github` copies
(e.g. under `flux/**/.terraform/`) are third-party, not owned. Cosign legs pin
the binary via `cosign-release: v3.1.3` (match `.flox`); setup lines carry
`# match .flox ...` parity comments. No dependabot/renovate (forbidden:
automation proposes image/chart updates via `flux-image-updates.yaml`).
