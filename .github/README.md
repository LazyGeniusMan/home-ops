# GitHub Actions

All executable CI lives in `.github/workflows/` (12 self-contained
workflows). Each workflow inlines every step it runs — only pinned
external `owner/repo@sha` actions are referenced.

## Ownership

Each workflow header states its `Owner:` (the subfolder the pipeline
belongs to). Summary:

| Workflow | Owner subfolder |
|---|---|
| `workflows/flux-fleet-push.yaml` | `flux/fleet` |
| `workflows/flux-fleet-release.yaml` | `flux/fleet` |
| `workflows/flux-fleet-validate.yaml` | `flux/fleet` |
| `workflows/flux-image-updates.yaml` | `flux/fleet` |
| `workflows/flux-infra-push.yaml` | `flux/infra` |
| `workflows/flux-infra-release.yaml` | `flux/infra` |
| `workflows/flux-infra-validate.yaml` | `flux/infra` |
| `workflows/flux-apps-push.yaml` | `flux/apps` |
| `workflows/flux-apps-release.yaml` | `flux/apps` |
| `workflows/flux-apps-validate.yaml` | `flux/apps` |
| `workflows/eso-proton-pass.yml` | `projects/eso-proton-pass` |
| `workflows/external-dns-netbird.yml` | `projects/external-dns-netbird` |

## Ignored `.github` paths

`find flux projects -type d -name .github` may still report vendored
third-party copies that are NOT owned by this repo and must be left alone,
e.g. `flux/fleet/terraform/.terraform/modules/*/.github/` (downloaded
Terraform modules) and skill fixtures.
