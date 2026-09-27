# Reusable NetBird reverse-proxy root

Single reusable Terraform root for every reverse-proxy slice, shipped inside the `infra/netbird` OCI artifact. Consumers reference this root via cross-namespace `sourceRef` + their own vars. Proxy-only: the access fabric (groups, networks, setup keys, routers, LAN resources, policies) lives in the Talos Ansible Terraform task, not here; `netbird_dns_zone` (internal MagicDNS custom zones) is not in this root.

## Layout (flat — no child modules)

- `main.tf` — `netbird` + `cloudflare` provider blocks, `netbird_reverse_proxy_clusters` lookup, the `guest-users-resources` group + `Service Load Balancer IP` `/32` network resource (attached to the Talos-owned parent network via the `netbird_network` data source), one `netbird_reverse_proxy_domain` (count-gated), one Cloudflare wildcard CNAME (count-gated, DNS-only `proxied = false`), one `netbird_reverse_proxy_service` (shared Service-LB subnet target + caller extras). Zero `module` blocks.
- `variables.tf` — full contract (service identity, domain, DNS zone, mode/targets/auth, provider tokens, plus `network_name`, `service_lb_ip`, `target_port`/`target_protocol`/`target_path`). Callers pass the fully-rendered `domain` FQDN. `service_lb_ip` has NO default (required IPv4; dev 192.168.1.249, prd 192.168.1.199). `target_port` defaults to 3000; `targets` defaults to `[]` (extra backends only — the LB target is always on). `versions.tf`: `required_version >= 1.11`, `netbirdio/netbird ~> 0.0.10`, `cloudflare/cloudflare ~> 5.0`.
- `outputs.tf` — `service_id`, `service_domain`, `proxy_url`, `proxy_cluster`, `domain_id`, `domain_validated`, `dns_record_name`, plus `parent_network_id`, `guest_users_resources_group_id`, `service_lb_resource_id` (names match what consumer `writeOutputsToSecret` expects).

## What the root does

Registers the custom base domain (unless `create_custom_domain = false`), manages the wildcard CNAME `*.<base-domain>` in Cloudflare, manages the guest entrypoint (group + `/32` resource on the Talos-owned parent network), and wires the reverse-proxy service (public `domain` FQDN -> shared Service-LB subnet target plus caller extras). `http` mode terminates TLS at the proxy; `tcp` / `udp` / `tls` listen on `listen_port` (0 = auto-assign). `domain` and `target_cluster` are `RequiresReplace` upstream.

## First-run ordering (custom-domain path)

Apply the consumer Terraform CR — registration lands **Pending Verification** and the wildcard CNAME is created. Confirm DNS propagation, then click **Verify Domain** in the dashboard (**Reverse Proxy > Custom Domains**) within 48h; once **Active**, later applies are no-ops. Domains already registered out-of-band are imported into the consumer's state once, then reconciled (see `tofu import` in `examples/consumer-terraform.yaml`).

## Consumer usage (cross-namespace sourceRef)

Copy `examples/consumer-terraform.yaml` into the app's `base/` (`<app>-terraform-vars` ExternalSecret + `<app>-proxy` Terraform CR — provider tokens via Proton Pass through the cluster-scoped `proton-pass` ClusterSecretStore, keys under the module var names so `varsFrom` needs no renames), then add per-env overlay patches for hosts/zone IDs. Backend: in-cluster Kubernetes default — no backendConfig needed. Four shapes: shared-LB HTTP service, HTTP with an extra peer backend, free-domain path, TCP passthrough — see `examples/consumer-terraform.yaml` comments and `variables.tf`.

## Secure defaults

- Upsert-only: every managed resource carries `lifecycle { prevent_destroy = true }`, and every consumer Terraform CR sets `destroy: false` + `destroyResourcesOnDeletion: false`.
- Verification CNAME is DNS-only (`proxied = false`).
- No secrets in git: PAT + Cloudflare token flow through ESO mirrors; service outputs land in `<app>-proxy-outputs` via `writeOutputsToSecret`. No `local-exec`, no sensitive outputs.
- CrowdSec has no provider field — set **Enforce** per service in the dashboard after the first apply (re-apply if the service is recreated).
- Backends must trust the proxy range `100.64.0.0/10` for the real client IP from `X-Forwarded-For`; `X-NetBird-User` / `X-NetBird-Groups` headers are trustworthy only if the backend is reachable solely through the service.
