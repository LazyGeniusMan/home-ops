# Reusable NetBird reverse-proxy root

Single reusable Terraform root for every reverse-proxy slice, shipped inside
the `infra/netbird` OCI artifact. Consumers reference this root via
cross-namespace `sourceRef` + their own vars. Proxy-only: the access fabric
(groups, networks, setup keys, routers, LAN resources, policies) lives in the
Talos Ansible Terraform task, not here. `netbird_dns_zone` (internal MagicDNS
custom zones) is not in this root.

## Layout (flat — no child modules)

- `main.tf` — `netbird` + `cloudflare` provider blocks,
  `netbird_reverse_proxy_clusters` lookup, the `guest-users-resources` group +
  `Service Load Balancer IP` `/32` network resource (attached to the
  Talos-owned parent network via the `netbird_network` data source — never a
  managed network here), one `netbird_reverse_proxy_domain` (count-gated), one
  Cloudflare wildcard CNAME (count-gated, DNS-only `proxied = false`), one
  `netbird_reverse_proxy_service` (shared Service-LB subnet target + caller
  extras). Zero `module` blocks.
- `variables.tf` — full contract (service identity, domain, DNS zone,
  mode/targets/auth, provider tokens, plus `network_name`, `service_lb_ip`,
  `target_port`/`target_protocol`/`target_path`). No `app_host`/`ui_host` vars:
  callers pass the fully-rendered `domain` FQDN. `targets` defaults to `[]`
  (extra backends only — the LB target is always on). See `variables.tf` for
  the variable table (`versions.tf`: `required_version >= 1.11`,
  `netbirdio/netbird ~> 0.0.10`, `cloudflare/cloudflare ~> 5.0`).
- `outputs.tf` — `service_id`, `service_domain`, `proxy_url`, `proxy_cluster`,
  `domain_id`, `domain_validated`, `dns_record_name`, plus
  `parent_network_id`, `guest_users_resources_group_id`,
  `service_lb_resource_id`. Names match what consumer `writeOutputsToSecret`
  expects. See `outputs.tf` for the output table.

## What the root does

Registers the custom base domain (unless `create_custom_domain = false`),
manages the wildcard CNAME `*.<base-domain>` → proxy cluster address in
Cloudflare (unless `cloudflare_zone_id` is `null`), manages the guest
entrypoint (group + `/32` resource on the Talos-owned parent network), and
wires the reverse-proxy service: public `domain` FQDN → the shared Service-LB
subnet target plus caller extras in `targets`. `http` mode terminates TLS at
the proxy; `tcp` / `udp` / `tls` listen on `listen_port` (0 = auto-assign).
`domain` and `target_cluster` are `RequiresReplace` upstream.

## First-run ordering (custom-domain path)

Apply the consumer Terraform CR — registration lands **Pending Verification**
and the wildcard CNAME is created. Wait for DNS propagation, confirm CAA
authorizes `sectigo.com` for `issue` + `issuewild` only where CAA records
exist, then click **Verify Domain** in the dashboard (**Reverse Proxy >
Custom Domains**) within 48h — unverified registrations expire. Once
**Active**, later applies are no-ops. Domains already registered out-of-band
are imported into the consumer's state once, then reconciled (see `tofu import`
in `examples/consumer-terraform.yaml` for the shape).

## Fabric ownership

Setup keys, networks, routers, LAN resources, and access policies live in the
Talos Ansible Terraform task, which applies first (the parent network
`var.network_name` must exist for the `netbird_network` data-source lookup).

## CrowdSec (dashboard-only — no TF field)

Provider `0.0.10` exposes no CrowdSec attribute on
`netbird_reverse_proxy_service`. After the first apply, set per service:
**Reverse Proxy > Services > <service> > Access Control > CrowdSec → Enforce**
(default Off). Re-apply if the service is recreated.

## Consumer usage (cross-namespace sourceRef)

Copy `examples/consumer-terraform.yaml` into the app's `base/` directory
(`<app>-terraform-vars` ExternalSecret + `<app>-proxy` Terraform CR — provider
tokens via Proton Pass through the cluster-scoped `proton-pass`
ClusterSecretStore, keys under the module var names so `varsFrom` needs no
renames), then add per-env overlay patches for hosts/zone IDs (positional
`/spec/vars/*`; keep the `vars` order). Provider tokens flow from Proton Pass
via the ESO-synced `<app>-terraform-vars` Secret — never commit secrets, never
hardcode IDs. Backend: in-cluster Kubernetes default — no backendConfig needed.
Four shapes: shared-LB HTTP service (zitadel-login shape), HTTP with an extra
peer backend (oauth2-proxy SSO front), free-domain path (no custom domain, no
Cloudflare), and TCP passthrough with access restrictions — see
`examples/consumer-terraform.yaml` comments and `variables.tf`.

## Secure defaults

- Upsert-only: every managed resource carries
  `lifecycle { prevent_destroy = true }`, and every consumer Terraform CR
  sets explicit `destroy: false` + `destroyResourcesOnDeletion: false`. No
  `tofu destroy` path via Flux; drift detection stays on.
- Verification CNAME is DNS-only (`proxied = false`): orange-clouding it
  would hand NetBird and ZeroSSL Cloudflare edge IPs instead of the proxy
  cluster address.
- No secrets in git: the NetBird PAT and the Cloudflare API token flow
  through ESO mirrors and the CR vars Secret; service outputs land in the
  `<app>-proxy-outputs` Secret via `writeOutputsToSecret`. The root
  contains no `local-exec` and exposes no sensitive outputs.
- CrowdSec stays Off in TF (no provider field); the manual Enforce step per
  service is the compensating control.
- Backends must trust the proxy range `100.64.0.0/10` to read the real client
  IP from `X-Forwarded-For`; never hardcode a single NetBird IP.
  Proxy-stamped `X-NetBird-User` / `X-NetBird-Groups` headers are trustworthy
  only if the backend is reachable solely through the service.
