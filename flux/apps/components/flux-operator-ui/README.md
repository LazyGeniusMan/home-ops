# Flux Operator UI (§11.4)

Standalone Flux Web UI (serverOnly) in the apps tenant, auth-fronted by a
per-instance oauth2-proxy. UI only — bootstrap, fleet sync, and Managed
resources are never touched here (see "Update automation" below).

## Layout (mirrors cert-manager §9 pattern, apps area)

`controllers/base/flux-operator-ui.yaml` (OCIRepository + HelmRelease) +
`controllers/base/oauth2-proxy.yaml` (Deployment + Service) with
`controllers/{production,staging}` overlays (inherit base unchanged), and
`configs/{base,production,staging}` (proxy credentials, wildcard certificate,
HTTPRoutes). Tenant is `apps/flux-operator-ui` via
`flux/apps/update-policies/flux-operator-ui.yaml`.

## Version choice

UI tag **0.59.0**, chart
`oci://ghcr.io/controlplaneio-fluxcd/charts/flux-operator` — pinned to the
Flux Operator release line compatible with bootstrap
**0.59.0** / operator chart **0.59.0** (`operator_chart_version` in
`flux/fleet/terraform/versions.yaml`, the single source: `clusters/*/flux-system/flux-operator.yaml`
consumes the same coordinates via Flux after bootstrap, and
`tests/versions.tftest.hcl` asserts the GitOps↔Terraform mapping). The
standalone install comes from the upstream docs (bounded source:
`/tmp/home-ops-docs/flux-operator-docs/docs/web`, upstream
[flux-operator](https://github.com/controlplaneio-fluxcd/flux-operator) branch
`main`, `docs/web/web-standalone.md`): dedicated Helm release with
`web.serverOnly: true` and `installCRDs: false`, `fullnameOverride:
flux-operator-ui` (Service `flux-operator-ui`, port 9080 `http-web`).
Bootstrap keeps `web.enabled: false` so the two releases never overlap.

## RBAC (read-only-friendly, least-privilege)

The UI backend impersonates the authenticated user for every Kubernetes API
call (upstream `docs/web/web-least-privilege-rbac.md`), so end users need
READ access to the Flux CRs they should see — least-privilege example for a
read-only viewer (adjust subjects to taste; the UI's own service account is
chart-managed, NO extra Role/Binding is created here):

```yaml
# Read-only Flux CR viewer for UI users (illustrative — apply out-of-band).
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: flux-ui-viewer
rules:
  - apiGroups: ["kustomize.toolkit.fluxcd.io", "helm.toolkit.fluxcd.io",
      "source.toolkit.fluxcd.io", "notification.toolkit.fluxcd.io",
      "image.toolkit.fluxcd.io", "fluxcd.controlplane.io"]
    resources: ["*"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["namespaces", "pods", "events"]
    verbs: ["get", "list", "watch"]
```

No Secret/ConfigMap data is ever returned to users by the privileged backend
paths (CronJob pod listing, Flux GVK resolution — see the upstream RBAC doc).

## Auth (locked contract)

Same contract as the other ExternalAuth apps (zitadel README OIDC table —
do not deviate):

| Item | Value |
| --- | --- |
| Issuer | `https://zitadel.home-ops.yansyah.my.id` |
| Client | `oauth2-proxy-shared` (declared in `flux/infra/components/zitadel/terraform` — reference only) |
| Scopes | `openid profile email groups` (groups claim enforced, `admin` group only) |
| Secrets (ESO) | `pass://acme-prd-bdo1-talos-apps-01/flux-operator-ui/oauth2-proxy-*` (client-id, client-secret, cookie-secret) |
| Upstream | `http://flux-operator-ui.<ns>.svc:9080` (namespace-agnostic via `POD_NAMESPACE` env) |
| Callback | `https://flux-operator.home-ops.yansyah.my.id/oauth2/callback` (covered by the shared wildcard redirect `https://*/oauth2/callback` — no client change needed) |

## Routing

Gateway `flux-operator.home-ops.yansyah.my.id`: port 80 carries only the
RequestRedirect filter (§8.1 pattern); 443 terminates with the in-namespace
wildcard Secret and backends to the **oauth2-proxy** Service (4180), never to
the UI directly. TLS via the duplicate in-namespace `Certificate`
(`wildcard-certificate.yaml`, same namespace-local discipline as §11.1 —
cert-manager Secrets cannot cross namespaces).

## Telemetry-off / monitoring / updates

- No usage-reporting flags are set on the release and no metrics port is
  opened (unguarded monitors OFF). UI health via `kube-state-metrics`
  (`kube_deployment_status_replicas_available` for `oauth2-proxy` and the
  `flux-operator-ui` HelmRelease `Ready` condition) once the monitoring stack
  lands; alert on consecutive failures then. No ServiceMonitor here — nothing
  serves metrics yet (monitors guarded, same discipline as cert-manager §9).
- The UI tracks the operator release line via
  `update-policies/flux-operator-ui.yaml` (chart marker
  `apps:flux-operator-ui:tag`, floor `>=0.59.0`). It NEVER touches fleet sync:
  this release is `serverOnly` with `installCRDs: false`, so it owns no CRDs,
  no bootstrap values, and no Managed sync resources — bumps move the UI tag
  and `operator_chart_version` together, nothing else.
