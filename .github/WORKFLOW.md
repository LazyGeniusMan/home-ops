# GitHub Actions

14 self-contained workflows in `.github/workflows/` (only pinned external
`owner/repo@sha` actions). Each header states its purpose + path filters.

- Validate: `flux-{infra,apps,fleet}-validate.yaml` (PR + main + dispatch,
  path-gated) — `flux/scripts/validate.sh`; fleet also runs `tofu`
  init/validate/test on `flux/fleet/terraform`.
- Push: `flux-{infra,apps,fleet}-push.yaml` (main pushes, path-gated) —
  OCI `dev` + `dev-<sha>` artifacts, cosign-signed.
- Release: `flux-{infra,apps,fleet}-release.yaml`
  (`flux-{infra,apps,fleet}-v*` tags) — OCI `stable` + version,
  cosign-signed.
- Image updates: `flux-image-updates.yaml` (branch creation + dispatch) —
  opens a PR to main per `image-updates-infra/apps` branch.
- Projects: `apprise-go-api.yml`, `eso-proton-pass.yml`,
  `external-dns-netbird.yml` (test + GHCR publish/sign),
  `helm-rclone-sync.yml` (verify + chart OCI publish/sign).

All `uses:` are SHA-pinned, least-privilege `permissions:`, concurrency
groups, path-gated triggers. Vendored `.github` copies (e.g. under
`flux/**/.terraform/`) are third-party, not owned.
