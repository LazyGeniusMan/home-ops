# GitHub Actions (single location)

All executable CI lives here: `.github/workflows/` (12 self-contained
workflows) + `.github/actions/` (13 composite actions, the shared step layer).
There are no `.github/` directories anywhere else in the repo (outside
vendored third-party paths, see below).

## Ownership

Each workflow header states its `Owner:` (the subfolder the pipeline
belongs to), its future dedicated repo name, and numbered migration steps.
Summary:

| Workflow | Owner subfolder | Future repo |
|---|---|---|
| `workflows/flux-fleet-push.yaml` | `flux/fleet` | `home-ops-fleet` |
| `workflows/flux-fleet-release.yaml` | `flux/fleet` | `home-ops-fleet` |
| `workflows/flux-fleet-validate.yaml` | `flux/fleet` | `home-ops-fleet` |
| `workflows/flux-image-updates.yaml` | `flux/fleet` | `home-ops-fleet` |
| `workflows/flux-infra-push.yaml` | `flux/infra` | `home-ops-infra` |
| `workflows/flux-infra-release.yaml` | `flux/infra` | `home-ops-infra` |
| `workflows/flux-infra-validate.yaml` | `flux/infra` | `home-ops-infra` |
| `workflows/flux-apps-push.yaml` | `flux/apps` | `home-ops-apps` |
| `workflows/flux-apps-release.yaml` | `flux/apps` | `home-ops-apps` |
| `workflows/flux-apps-validate.yaml` | `flux/apps` | `home-ops-apps` |
| `workflows/eso-proton-pass.yml` | `projects/eso-proton-pass` | `eso-proton-pass` |
| `workflows/external-dns-netbird.yml` | `projects/external-dns-netbird` | `external-dns-netbird` |

| Composite action | Caller workflow |
|---|---|
| `actions/flux-fleet-oci-push/` | `flux-fleet-push.yaml` |
| `actions/flux-fleet-oci-release/` | `flux-fleet-release.yaml` |
| `actions/flux-fleet-validate-manifests/` | `flux-fleet-validate.yaml` |
| `actions/flux-fleet-validate-terraform/` | `flux-fleet-validate.yaml` |
| `actions/flux-fleet-image-updates/` | `flux-image-updates.yaml` |
| `actions/flux-infra-oci-push/` | `flux-infra-push.yaml` |
| `actions/flux-infra-oci-release/` | `flux-infra-release.yaml` |
| `actions/flux-infra-validate-manifests/` | `flux-infra-validate.yaml` |
| `actions/flux-apps-oci-push/` | `flux-apps-push.yaml` |
| `actions/flux-apps-oci-release/` | `flux-apps-release.yaml` |
| `actions/flux-apps-validate-manifests/` | `flux-apps-validate.yaml` |
| `actions/eso-proton-pass-publish/` | `eso-proton-pass.yml` |
| `actions/external-dns-netbird-publish/` | `external-dns-netbird.yml` |

## Migration playbook

General pattern: copy the workflow file(s) + the composite action dir(s) +
referenced scripts into the new repo root, flip MONOREPO path inputs to
split values, and broaden trigger path filters. Keep SHA pins, permissions,
concurrency, and matrices identical.

### home-ops-fleet (from `flux/fleet`)

Copy into the new repo root:
- `workflows/flux-fleet-push.yaml` -> `.github/workflows/push.yaml`
- `workflows/flux-fleet-release.yaml` -> `.github/workflows/release.yaml`
- `workflows/flux-fleet-validate.yaml` -> `.github/workflows/validate.yaml`
- `workflows/flux-image-updates.yaml` -> `.github/workflows/image-updates.yaml`
- `actions/flux-fleet-oci-push/` -> `.github/actions/oci-push/`
- `actions/flux-fleet-oci-release/` -> `.github/actions/oci-release/`
- `actions/flux-fleet-validate-manifests/` -> `.github/actions/validate-manifests/`
- `actions/flux-fleet-validate-terraform/` -> `.github/actions/validate-terraform/`
- `actions/flux-fleet-image-updates/` -> `.github/actions/image-updates/`
- `flux/scripts/validate.sh` -> `scripts/validate.sh`

Path flips (monorepo -> split):
- `path: "./flux/fleet"` -> `"./"`
- `script: ./flux/scripts/validate.sh` -> `./scripts/validate.sh`
- `validate-dir: flux/fleet` -> `.`
- `working-directory: flux/fleet/terraform` -> `terraform`
- cosign subject: `flux/fleet/clusters/.../flux-instance.yaml` ->
  `clusters/.../flux-instance.yaml` (moves with the area)
- Trigger filters: `paths: ['flux/fleet/**']` -> `['**']` (or drop).
  Keep the `image-updates-*` branch guard, the `create` trigger, and the
  `flux-fleet-v*` tag filter. Optionally restore `workflow_call` inputs for
  cross-repo calls.

### home-ops-infra (from `flux/infra`)

Copy into the new repo root:
- `workflows/flux-infra-push.yaml` -> `.github/workflows/push.yaml`
- `workflows/flux-infra-release.yaml` -> `.github/workflows/release.yaml`
- `workflows/flux-infra-validate.yaml` -> `.github/workflows/validate.yaml`
- `actions/flux-infra-oci-push/` -> `.github/actions/oci-push/`
- `actions/flux-infra-oci-release/` -> `.github/actions/oci-release/`
- `actions/flux-infra-validate-manifests/` -> `.github/actions/validate-manifests/`
- `flux/scripts/validate.sh` -> `scripts/validate.sh`
- `flux/fleet/tenants/infra.yaml` -> `tenants/infra.yaml` (cosign subject pin;
  drop the `flux/fleet/` prefix)

Path flips (monorepo -> split):
- `source-prefix: flux/infra/components` -> `components`
- `script: ./flux/scripts/validate.sh` -> `./scripts/validate.sh`
- `validate-dir: flux/infra` -> `.`
- Trigger filters: `paths: ['flux/infra/**', 'flux/fleet/tenants/infra.yaml']` ->
  `['components/**', 'tenants/infra.yaml']` (or drop). Keep the
  `flux-infra-v*` tag filter. Optionally restore `workflow_call` inputs
  (`repository-prefix`, `source-prefix`; defaults `''`/`'components'`).

### home-ops-apps (from `flux/apps`)

Copy into the new repo root:
- `workflows/flux-apps-push.yaml` -> `.github/workflows/push.yaml`
- `workflows/flux-apps-release.yaml` -> `.github/workflows/release.yaml`
- `workflows/flux-apps-validate.yaml` -> `.github/workflows/validate.yaml`
- `actions/flux-apps-oci-push/` -> `.github/actions/oci-push/`
- `actions/flux-apps-oci-release/` -> `.github/actions/oci-release/`
- `actions/flux-apps-validate-manifests/` -> `.github/actions/validate-manifests/`
- `flux/scripts/validate.sh` -> `scripts/validate.sh`
- `flux/fleet/tenants/apps.yaml` -> `tenants/apps.yaml` (cosign subject pin;
  drop the `flux/fleet/` prefix)

Path flips (monorepo -> split):
- `source-prefix: flux/apps/components` -> `components`
- `script: ./flux/scripts/validate.sh` -> `./scripts/validate.sh`
- `validate-dir: flux/apps` -> `.`
- Trigger filters: `paths: ['flux/apps/**', 'flux/fleet/tenants/apps.yaml']` ->
  `['components/**', 'tenants/apps.yaml']` (or drop). Keep the
  `flux-apps-v*` tag filter. Optionally restore `workflow_call` inputs
  (`repository-prefix`, `source-prefix`; defaults `''`/`'components'`).

### eso-proton-pass (from `projects/eso-proton-pass`)

Copy into the new repo root:
- `workflows/eso-proton-pass.yml` -> `.github/workflows/publish.yml`
- `actions/eso-proton-pass-publish/` -> `.github/actions/publish/`

Path flips (monorepo -> split):
- `context: projects/eso-proton-pass` -> `.`
- `dockerfile: projects/eso-proton-pass/Dockerfile` -> `./Dockerfile`
- Trigger filter: `paths: ['projects/eso-proton-pass/**']` -> `['**']` (or
  drop); keep the `eso-proton-pass-v*` tag filter. Optionally restore
  `workflow_call` inputs (`context`, `dockerfile`; defaults `'.'` /
  `'./Dockerfile'`).

### external-dns-netbird (from `projects/external-dns-netbird`)

Copy into the new repo root:
- `workflows/external-dns-netbird.yml` -> `.github/workflows/publish.yml`
- `actions/external-dns-netbird-publish/` -> `.github/actions/publish/`

Path flips (monorepo -> split):
- `context: projects/external-dns-netbird` -> `.`
- `dockerfile: projects/external-dns-netbird/Dockerfile` -> `./Dockerfile`
- Trigger filter: `paths: ['projects/external-dns-netbird/**']` -> `['**']`
  (or drop); keep the `external-dns-netbird-v*` tag filter. Optionally
  restore `workflow_call` inputs (`context`, `dockerfile`; defaults `'.'` /
  `'./Dockerfile'`).

## Ignored `.github` paths

`find flux projects -type d -name .github` may still report vendored
third-party copies that are NOT owned by this repo and must be left alone,
e.g. `flux/fleet/terraform/.terraform/modules/*/.github/` (downloaded
Terraform modules) and skill fixtures. Only the five first-party subfolder
dirs (`flux/{fleet,infra,apps}/.github/`,
`projects/{eso-proton-pass,external-dns-netbird}/.github/`) were in scope,
and all five are gone.
