# GitHub Actions

25 self-contained workflows in `.github/workflows/` (only pinned external
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
- First-party fetch-time charts: `helm-{cosi,gateway-api,kubevirt,multus,otel-monitoring-crds}.yml`
  (push-to-main + PR legs; PR runs verify/test only, publish runs on main) +
  `helm-*-check.yml` (daily schedule + dispatch; compares Chart.yaml against
  upstream, opens version-bump PRs with push scope).

Push/release/check/update workflows use deny-all `permissions: {}` default
with per-job minimums; publish jobs additionally gate on
`github.event_name != 'pull_request'` (test-only PR legs) and
`github.ref == 'refs/heads/main'` (dev artifacts only from main, tag legs
excepted). Validate/lint workflows grant top-level `contents: read`.
Concurrency groups, path-gated triggers throughout. Each validate/test/lint leg
above also runs locally before commit via prek (`.pre-commit-config.yaml` at
the repo root mirrors these path filters per hook; pre-commit = fast gates
(go-fast is vet + gofmt + tidy + build only — test -race/lint/vuln stay at
pre-push), pre-push = slow whole-scope gates, manual = day-2 `--check --diff`;
flux scope validates and fetch-time chart verifies need GitHub network at
pre-commit like CI does; push, release, sign, and bot workflows stay
CI-only). Vendored copies (e.g. under `.terraform/` or `.agents/`) are third-party, not owned. Cosign legs pin
the binary via `cosign-release: v3.1.3` (match `.flox`); setup lines carry
`# match .flox ...` parity comments. No dependabot/renovate (forbidden:
automation proposes image/chart updates via `flux-image-updates.yaml`).
`.terraform.lock.hcl` files are gitignored by policy (single-writer: the
bootstrapping operator regenerates them; never committed).
