# GitHub Actions

All executable CI lives in `.github/workflows/` (12 self-contained
workflows). Each workflow inlines every step it runs — only pinned
external `owner/repo@sha` actions are referenced.

## Ownership

Each workflow header states its `Owner:` (the subfolder the pipeline
belongs to), its future dedicated repo name, a self-contained note, and
numbered migration steps. Summary:

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

## Migration playbook

General pattern: copy the single workflow file + referenced scripts into
the new repo root, flip monorepo path values to split values, and broaden
trigger path filters. No local actions dir is involved. Keep SHA pins,
permissions, concurrency, and matrices identical.

### home-ops-fleet (from `flux/fleet`)

Copy into the new repo root:
- `workflows/flux-fleet-push.yaml` -> `.github/workflows/push.yaml`
- `workflows/flux-fleet-release.yaml` -> `.github/workflows/release.yaml`
- `workflows/flux-fleet-validate.yaml` -> `.github/workflows/validate.yaml`
- `workflows/flux-image-updates.yaml` -> `.github/workflows/image-updates.yaml`
- `flux/scripts/validate.sh` -> `scripts/validate.sh`

Path flips (monorepo -> split):
- `path: "./flux/fleet"` -> `"./"`
- `run: ./flux/scripts/validate.sh` -> `./scripts/validate.sh`
- `run: ... -d flux/fleet` -> `-d .`
- `working-directory: flux/fleet/terraform` -> `terraform`
- cosign subject: `flux/fleet/clusters/.../flux-instance.yaml` ->
  `clusters/.../flux-instance.yaml` (moves with the area)
- Trigger filters: `paths: ['flux/fleet/**']` -> `['**']` (or drop).
  Keep the `image-updates-*` branch guard, the `create` trigger, and the
  `flux-fleet-v*` tag filter.

### home-ops-infra (from `flux/infra`)

Copy into the new repo root:
- `workflows/flux-infra-push.yaml` -> `.github/workflows/push.yaml`
- `workflows/flux-infra-release.yaml` -> `.github/workflows/release.yaml`
- `workflows/flux-infra-validate.yaml` -> `.github/workflows/validate.yaml`
- `flux/scripts/validate.sh` -> `scripts/validate.sh`
- `flux/fleet/tenants/infra.yaml` -> `tenants/infra.yaml` (cosign subject pin;
  drop the `flux/fleet/` prefix)

Path flips (monorepo -> split):
- `path: flux/infra/components/<component>` -> `components/<component>`
- `run: ./flux/scripts/validate.sh` -> `./scripts/validate.sh`
- `run: ... -d flux/infra` -> `-d .`
- Trigger filters: `paths: ['flux/infra/**', 'flux/fleet/tenants/infra.yaml']` ->
  `['components/**', 'tenants/infra.yaml']` (or drop). Keep the
  `flux-infra-v*` tag filter.

### home-ops-apps (from `flux/apps`)

Copy into the new repo root:
- `workflows/flux-apps-push.yaml` -> `.github/workflows/push.yaml`
- `workflows/flux-apps-release.yaml` -> `.github/workflows/release.yaml`
- `workflows/flux-apps-validate.yaml` -> `.github/workflows/validate.yaml`
- `flux/scripts/validate.sh` -> `scripts/validate.sh`
- `flux/fleet/tenants/apps.yaml` -> `tenants/apps.yaml` (cosign subject pin;
  drop the `flux/fleet/` prefix)

Path flips (monorepo -> split):
- `path: flux/apps/components/<component>` -> `components/<component>`
- `run: ./flux/scripts/validate.sh` -> `./scripts/validate.sh`
- `run: ... -d flux/apps` -> `-d .`
- Trigger filters: `paths: ['flux/apps/**', 'flux/fleet/tenants/apps.yaml']` ->
  `['components/**', 'tenants/apps.yaml']` (or drop). Keep the
  `flux-apps-v*` tag filter.

### eso-proton-pass (from `projects/eso-proton-pass`)

Copy into the new repo root:
- `workflows/eso-proton-pass.yml` -> `.github/workflows/publish.yml`

Path flips (monorepo -> split):
- `context: projects/eso-proton-pass` -> `.`
- `file: projects/eso-proton-pass/Dockerfile` -> `./Dockerfile`
- Trigger filter: `paths: ['projects/eso-proton-pass/**']` -> `['**']` (or
  drop); keep the `eso-proton-pass-v*` tag filter.

### external-dns-netbird (from `projects/external-dns-netbird`)

Copy into the new repo root:
- `workflows/external-dns-netbird.yml` -> `.github/workflows/publish.yml`

Path flips (monorepo -> split):
- `context: projects/external-dns-netbird` -> `.`
- `file: projects/external-dns-netbird/Dockerfile` -> `./Dockerfile`
- Trigger filter: `paths: ['projects/external-dns-netbird/**']` -> `['**']`
  (or drop); keep the `external-dns-netbird-v*` tag filter.

## Ignored `.github` paths

`find flux projects -type d -name .github` may still report vendored
third-party copies that are NOT owned by this repo and must be left alone,
e.g. `flux/fleet/terraform/.terraform/modules/*/.github/` (downloaded
Terraform modules) and skill fixtures.
