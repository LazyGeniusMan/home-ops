# tofu-controller

Tofu Controller **v0.16.5** (chart **0.16.5**): Flux-native
Terraform/OpenTofu reconciler that machine-applies `kind: Terraform` objects
(`terraforms.infra.contrib.fluxcd.io`). Unblocks identity-as-code: the
provider-ready Zitadel modules under
`flux/infra/components/zitadel/terraform/` are manually applied today; once
this controller is live, later tasks can ship `Terraform` CRs that apply them
(and future infra modules) in-cluster.

## Chart source

OCI is the upstream source of truth, verified by pull:

- `oci://ghcr.io/flux-iac/charts/tofu-controller`, tag **0.16.5** (digest
  `sha256:336e9a2690b868781422165bf20550dc7e7093abeb841c498d8bd1dd594fbd27`,
  `helm template` + full-values render pass locally). Chart == app
  (**v0.16.5** for both `ghcr.io/flux-iac/tofu-controller` and
  `ghcr.io/flux-iac/tf-runner`); on chart bumps set `image.tag` AND
  `runner.image.tag` to the new appVersion together (same divergence note
  pattern as Zitadel).
- No classic `HelmRepository` fallback: unlike coredns (no OCI mirror, hence
  the classic precedent), this chart publishes to GHCR on every release, so
  the OCI-first convention (cert-manager/zitadel/dragonfly) applies directly.
- Upstream reference: `/tmp/home-ops-docs/flux-tofu-controller-docs/docs/`
  (`index.md`, `getting_started.md`, `release.yaml`); the CRD install policy
  (`Create`/`CreateReplace`) mirrors upstream `release.yaml`.

## Compatibility note

Upstream support matrix lists **v0.16 <-> Flux v2.6.x** (source controller
v1.7.x, Terraform v1.5.7); this repo runs **Flux v2.9.4**. No v0.16.x chart
restriction on the Flux version was found in the pulled values/templates
(controller talks to source/notification over stable cluster-DNS endpoints),
but if reconciliation misbehaves after install, check the Flux changelogs for
source/notification API drift first.

## Layout

Mirrors cert-manager/metrics-server: `controllers/{base,dev,prd,stg}`
(OCIRepository + HelmRelease, env overlays inherit base unchanged) and
`configs/{base,dev,prd,stg}` (empty base for now — **no `kind: Terraform`
objects ship yet**; later tasks add them here once the controller is live).
`dev`/`prd`/`stg` controllers currently inherit base with no patches; per-env
divergence (replica counts, runner namespaces) lands with the first real
divergence, not here.

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
- `zitadel` + `clickstack`, `hubble-ui`, `flux-operator-ui`, `headlamp`,
  `coder` — the namespaces where the first `Terraform` CRs will live
  (identity-as-code + SSO app clients).

`watchAllNamespaces: true` is stated explicitly so the controller watches
Terraform CRs in every namespace (the default, but load-bearing here since
the CRs live outside the controller's own namespace). **Rule: extend
`allowedNamespaces` whenever a `Terraform` CR lands in a namespace not on
this list** — the chart creates the runner ServiceAccount/token Secret per
listed namespace, and runners can only spawn where those exist.

## Backend

Default **in-cluster Kubernetes backend**: tfstate is stored as Secrets
(`tfstate-<workspace>-<secretSuffix>`) inside the cluster — no S3 bucket or
external backend is configured or needed. Set an explicit
`.spec.backendConfig` on a `Terraform` object only if a module ever needs to
leave the cluster.

## Telemetry-off / monitoring / updates

- Telemetry evidence: the pulled chart `values.yaml` contains no phone-home,
  analytics, or usage-reporting knobs (checked at authoring time; a grep for
  `telemetry|usageReport|phoneHome|analytics|tracking|segment|sentry` returns
  nothing).
- Unguarded monitors OFF: `metrics.enabled` stays `false` and no
  `ServiceMonitor` is shipped until `monitoring.coreos.com` CRDs land (same
  §9 deviation — flip: set `metrics.enabled: true` plus
  `metrics.serviceMonitor.enabled: true` once the monitoring stack exists).
- Chart bumps flow through `update-policies/tofu-controller.yaml` + PR
  automation (remember the chart↔app lockstep above: bump the chart tag AND
  both image tags together). Branch Planner stays off (`branchPlanner.enabled:
  false`, the chart default).

## Environments

`dev`, `prd`, and `stg` currently inherit `../base` unchanged (same shape as
cert-manager before per-env divergence).
