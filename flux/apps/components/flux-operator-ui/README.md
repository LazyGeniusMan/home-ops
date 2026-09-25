# Flux Operator UI

Standalone Flux Web UI (serverOnly) in the apps tenant, auth-fronted by a
per-instance oauth2-proxy. UI only — bootstrap, fleet sync, and Managed
resources are never touched here.

## Layout

`base/` holds every manifest (`flux-operator-ui.yaml` OCIRepository +
HelmRelease, `oauth2-proxy.yaml` OCIRepository + HelmRelease, proxy
credentials, wildcard certificate, HTTPRoutes); env overlays `{dev,prd}/`
patch hostnames, vault refs, and proxy values via `resources: [../base]`.
Tenant is `apps/flux-operator-ui` via
`flux/apps/update-policies/flux-operator-ui.yaml`.

## Pins

UI chart `oci://ghcr.io/controlplaneio-fluxcd/charts/flux-operator` tag
**0.60.0**, in lockstep with `operator_chart_version` in
`flux/fleet/terraform/versions.yaml`. Dedicated Helm release with
`web.serverOnly: true`, `installCRDs: false`, `fullnameOverride:
flux-operator-ui` (Service `flux-operator-ui`, port 9080). Bootstrap keeps
`web.enabled: false` so the two releases never overlap.

## Sync-path guardrail

The UI must never own CRDs or the fleet sync path — the bootstrap
`flux-operator` owns both. Enforced in `base/flux-operator-ui.yaml` via
`web.serverOnly: true`, `installCRDs: false`, and the
`home-ops.yansyah.my.id/sync-path-guardrail` HelmRelease annotation
(validation-visible marker of the same rule).

## RBAC

The UI backend impersonates the authenticated user, so end users need READ
access to the Flux CRs they should see. The UI's own SA is chart-managed;
grant users a read-only ClusterRole (`get`/`list`/`watch` on the Flux CRD
groups `kustomize/helm/source/notification/image.toolkit.fluxcd.io` +
`fluxcd.controlplane.io`, plus `namespaces`/`pods`/`events`) out-of-band —
Flux never manages user RBAC here.

## Auth

| Item | Value |
| --- | --- |
| Issuer | `https://admin.zitadel.home-ops.yansyah.my.id` |
| Client | `flux-operator-ui` (owned by the `flux-operator-ui-sso` Terraform CR) |
| Scopes | `openid profile email groups` (`flux-operator-ui-admin` group only) |
| Secrets (ESO) | `oauth2-proxy-oidc` syncs client-id + client-secret from `flux-operator-ui-sso-outputs` via the in-cluster `flux-operator-ui-k8s` SecretStore (no pass:// seeding for OIDC creds); `oauth2-proxy-cookie` syncs the cookie-secret from `pass://acme-<env>-bdo1-talos-apps-01/flux-operator-ui/oauth2-proxy-cookie-secret` |
| Upstream | `http://flux-operator-ui.<ns>.svc:9080` (namespace-agnostic via `POD_NAMESPACE` env) |
| Callback | `https://flux-operator.home-ops.yansyah.my.id/oauth2/callback` (covered by the shared wildcard redirect) |

## Routing

Gateway `flux-operator.home-ops.yansyah.my.id`: port 80 carries only the
RequestRedirect filter; 443 terminates with the in-namespace wildcard Secret
and backends to the **oauth2-proxy** Service (4180), never to the UI
directly. TLS via the duplicate in-namespace `Certificate`
(cert-manager Secrets cannot cross namespaces).

## Telemetry / monitoring / updates

No usage-reporting flags, no metrics port, no ServiceMonitor. Tracks the
operator release line via `update-policies/flux-operator-ui.yaml` (UI marker
`apps:flux-operator-ui:tag`, floor `>=0.60.0`; proxy markers
`apps:oauth2-proxy-chart` + `apps:oauth2-proxy` shared with clickstack +
hubble-ui). Never touches fleet sync (`serverOnly` + `installCRDs: false`).
Version source: chart tag in `base/flux-operator-ui.yaml` (UI 0.60.0) in
lockstep with `operator_chart_version` in `flux/fleet/terraform/versions.yaml`.
Changelog: https://github.com/controlplaneio-fluxcd/flux-operator/releases.

## Environments

| Env | Replicas | Patches |
| --- | --- | --- |
| `dev` | UI chart default 1, oauth2-proxy 1 | hostnames, vault refs, proxy args |
| `prd` | UI chart default 1, oauth2-proxy 1 (singleton in every env) | hostnames, vault refs, proxy args |

oauth2-proxy stays a singleton (1) in every env — never scale it.

Upstream reference (read-only): `/tmp/home-ops-docs/flux-operator-docs/docs/web`.
