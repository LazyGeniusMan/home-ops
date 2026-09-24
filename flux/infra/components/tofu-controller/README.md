# tofu-controller

Tofu Controller **v0.16.5** (chart **0.16.5**): Flux-native
Terraform/OpenTofu reconciler that machine-applies `kind: Terraform` objects
(`terraforms.infra.contrib.fluxcd.io`). Unblocks identity-as-code for the
provider-ready Zitadel modules under
`flux/infra/components/zitadel/terraform/` (manually applied; no
`kind: Terraform` objects ship yet — see Layout).

## Chart source

- `oci://ghcr.io/flux-iac/charts/tofu-controller`, tag **0.16.5** (digest
  `sha256:336e9a2690b868781422165bf20550dc7e7093abeb841c498d8bd1dd594fbd27`).
  Chart == app (**v0.16.5** for both `ghcr.io/flux-iac/tofu-controller` and
  `ghcr.io/flux-iac/tf-runner`); on chart bumps set `image.tag` AND
  `runner.image.tag` to the new appVersion together.
- The CRD install policy (`Create`/`CreateReplace`) mirrors upstream
  `release.yaml`.
- Upstream reference: `/tmp/home-ops-docs/flux-tofu-controller-docs/docs/`.

## Compatibility

The controller reaches source-controller and notification-controller over
stable cluster-DNS endpoints (this repo runs Flux 2.x via the Flux
Operator, `FluxInstance` `distribution.version: "2.x"`).

## Layout

Mirrors cert-manager/metrics-server: `controllers/{base,dev,prd}`
(OCIRepository + HelmRelease, env overlays inherit base unchanged) and
`configs/{base,dev,prd}` (empty base — no `kind: Terraform` objects ship
yet). `dev`/`prd` controllers inherit base with no patches.

## Namespace / RBAC

- The controller runs in its **tenant namespace** (`tofu-controller`), NOT
  `flux-system`. The chart parameterises everything on `.Release.Namespace`
  (controller + `tf-runner` ServiceAccounts/token Secrets land there via the
  `allowedNamespaces` helper, always including the release namespace), and the
  controller reaches the Flux stack over cluster DNS, so co-location is
  unnecessary. The tenant auto-creates the Namespace + `flux` ServiceAccount +
  cluster-admin binding per the `infra.yaml` pattern (same as every other
  infra component).
- RBAC the chart itself owns: `ClusterRole`s/`ClusterRoleBinding`s for the
  controller (`tofu-cluster-reconciler-role`, `tofu-manager-role`) plus the
  runner (`tf-runner-role` bound to a `tf-runner` ServiceAccount in every
  allowed namespace), a leader-election `Role`, and the `aws-package`
  OCIRepository (only relevant with `awsPackage.install`, flipped off here).
- Runner reachability: each runner Pod must serve **gRPC on port 30000** to
  the controller (headless discovery Service per allowed namespace), and the
  controller pulls source tarballs from source-controller and posts events to
  notification-controller (both port 80; `--events-addr` defaults to
  `notification-controller.flux-system`).

## Runner namespaces

`runner.serviceAccount.allowedNamespaces` (base values) currently covers:

- `flux-system` (chart default, kept),
- `zitadel`, `clickstack`, `hubble-ui`, `flux-operator-ui`, `headlamp`,
  `coder`, `seaweedfs`, `matrix`, `netbird` — the namespaces for the
  `Terraform` CRs (identity-as-code + SSO + reverse-proxy consumers).

`watchAllNamespaces: true` is stated explicitly so the controller watches
Terraform CRs in every namespace (the default, but load-bearing here since
the CRs live outside the controller's own namespace). **Rule: extend
`allowedNamespaces` whenever a `Terraform` CR lands in a namespace not on
this list** — the chart creates the runner ServiceAccount/token Secret per
listed namespace, and runners can only spawn where those exist.

`allowCrossNamespaceRefs: true` opts in to cross-namespace `sourceRef`s
(upstream default `false` since 0.16.0): tenant-namespace `Terraform` CRs
point at the shared `OCIRepository/infra` source in ns `zitadel` instead of
per-consumer git pins.

## Backend

Default **in-cluster Kubernetes backend**: tfstate is stored as Secrets
(`tfstate-<workspace>-<secretSuffix>`) inside the cluster — no S3 bucket or
external backend is configured or needed. Set an explicit
`.spec.backendConfig` on a `Terraform` object only if a module ever needs to
leave the cluster.

## Telemetry-off / monitoring / updates

- Telemetry: the chart `values.yaml` contains no phone-home, analytics, or
  usage-reporting knobs.
- Unguarded monitors OFF: `metrics.enabled` stays `false` and no
  `ServiceMonitor` is shipped (same §9 deviation).
- Chart bumps flow through `update-policies/tofu-controller.yaml` + PR
  automation (remember the chart↔app lockstep above: bump the chart tag AND
  both image tags together). Branch Planner stays off (`branchPlanner.enabled:
  false`, the chart default).

## Upgrade runbook

- Version source: the `OCIRepository` tag in
  `controllers/base/tofu-controller.yaml` (chart == app v0.16.5) plus
  `image.tag` + `runner.image.tag` in values (same lockstep).
- Changelog: https://github.com/flux-iac/tofu-controller/releases.
- Bump: let the ImagePolicy PR land (marker
  `infra:tofu-controller:tag`, `update-policies/tofu-controller.yaml`),
  then set the chart tag AND both image tags together in the same PR.
- Verify: controller + runner pods `Ready`, then force a re-plan on one
  `Terraform` CR and check it reconciles `Ready=True`.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | `replicaCount: 1` (singleton; leader-elected, safe at 1) | none — inherits `../base` unchanged |
| `prd` | `replicaCount: 1` (singleton; raise to 2–3 only once multi-node HA is wanted) | none — inherits `../base` unchanged |

Runner namespaces and the in-cluster Kubernetes backend are env-independent.

Upstream reference (read-only): `/tmp/home-ops-docs/flux-tofu-controller-docs/docs/`.
