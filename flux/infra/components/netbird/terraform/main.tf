# Reusable NetBird reverse-proxy root — shipped inside the infra/netbird OCI artifact
# (flux/infra/components/netbird/terraform/). Consumers reference this root
# via cross-namespace sourceRef + their own vars.
#
# Flat single layer: provider auth + the guest Service-LB network resource +
# custom-domain registration + Cloudflare wildcard CNAME for NetBird ownership
# verification + the reverse-proxy service itself — no child modules. Callers
# pass the fully-rendered service FQDN in `domain` (this root takes no
# app_host/ui_host vars), mirroring the zitadel shared root.
#
# Proxy-only: the access fabric (admin/guest/node groups, per-cluster
# network, setup keys, routers, LAN resources, access policies) lives in the
# Talos Ansible Terraform task — NOT here. This root owns ONLY the
# `guest-users-resources` group, the `Service Load Balancer IP` network
# resource (attached to the Talos-owned parent network via data-source
# lookup, never a managed network), the custom-domain registration +
# verification CNAME, and the reverse-proxy service itself.
#
# Upsert-only: every managed resource below carries
# `lifecycle { prevent_destroy = true }`, so any plan that would delete or
# replace a resource fails instead of destroying it. Together with the
# explicit `destroy: false` + `destroyResourcesOnDeletion: false` on every
# consumer Terraform CR, there is no `tofu destroy` path via Flux; drift
# detection stays on and outputs are unchanged.
provider "netbird" {
  token          = var.netbird_token
  management_url = var.management_url
}

provider "cloudflare" {
  api_token = var.cloudflare_api_token
}

# Proxy clusters currently connected to the management server. Supplies the
# default `target_cluster` (first cluster) and the CNAME target for the
# Cloudflare verification record when the caller does not pin one.
data "netbird_reverse_proxy_clusters" "all" {}

locals {
  # Base custom domain under which the service FQDN lives. Explicit
  # `base_domain` wins; otherwise the parent of `domain` (e.g. service
  # `dashboard.proxy.example.com` -> base `proxy.example.com`).
  base_domain = coalesce(
    var.base_domain,
    join(".", slice(split(".", var.domain), 1, length(split(".", var.domain))))
  )
  # Cluster address the base domain is validated against (explicit pin or
  # first connected cluster).
  target_cluster = coalesce(
    var.target_cluster,
    data.netbird_reverse_proxy_clusters.all.clusters[0].address
  )
}

# Custom-domain registration (create-if-missing, then steady-state no-op).
# If the base domain was already registered out-of-band (dashboard), import
# it once instead of creating (see README.md); afterwards this resource is
# an upsert-only no-op. Both `domain` and `target_cluster` are RequiresReplace
# upstream, so changing them fails closed under `prevent_destroy` by design —
# migrate via a manual import-state dance, never via Flux.
resource "netbird_reverse_proxy_domain" "this" {
  count          = var.create_custom_domain ? 1 : 0
  domain         = local.base_domain
  target_cluster = local.target_cluster

  lifecycle {
    prevent_destroy = true
  }
}

# Ownership verification: NetBird verifies `*.<base-domain>` with a CNAME
# lookup against the target cluster, so this wildcard CNAME must exist (and
# propagate) before the one-time Verify click in the dashboard (48h deadline
# once the registration exists; see README.md). Skipped when
# `cloudflare_zone_id` is null (DNS self-managed outside Cloudflare) or when
# the caller sets `create_custom_domain = false` (free/cluster domain path).
# `proxied = false` is load-bearing: the record must stay DNS-only or the
# NetBird CNAME lookup (and ZeroSSL issuance) sees Cloudflare edge IPs
# instead of the proxy cluster address.
resource "cloudflare_dns_record" "validation" {
  count   = var.create_custom_domain && var.cloudflare_zone_id != null ? 1 : 0
  zone_id = var.cloudflare_zone_id
  name    = "*.${local.base_domain}"
  type    = "CNAME"
  content = local.target_cluster
  ttl     = var.dns_ttl
  proxied = false

  lifecycle {
    prevent_destroy = true
  }
}

# Guest Service-LB resource only (proxy-only root). The full access fabric
# (admin/guest/node groups, per-cluster network, setup keys, routers, LAN
# resources, access policies) lives in the Talos Ansible Terraform task.
# This root manages ONLY:
#   - the `guest-users-resources` group (the resource's `groups` edge; the
#     API mirrors membership back onto the group as computed state, so only
#     one edge is managed to avoid a group <-> resource cycle),
#   - the `Service Load Balancer IP` /32 network resource (the single stable
#     entrypoint guests use to reach in-cluster Services through the mesh;
#     the Talos-side routing peer forwards onto the LAN toward the Cilium
#     LB VIP), attached to the Talos-owned parent network looked up by name
#     via `var.network_name` (data source — never a managed network here).
resource "netbird_group" "guest_users_resources" {
  name = "guest-users-resources"

  lifecycle {
    prevent_destroy = true
  }
}

# Talos-owned parent network (created by the Talos Ansible Terraform task) —
# read, never manage. `network_id` is schema-required on every
# `netbird_network_resource`, so the LB resource below points at this lookup.
data "netbird_network" "parent" {
  name = var.network_name
}

# Service Load Balancer IP as a /32 network resource. The address is always
# the /32 host form — pass the bare VIP in var.service_lb_ip.
resource "netbird_network_resource" "service_lb" {
  network_id  = data.netbird_network.parent.id
  name        = "Service Load Balancer IP"
  description = "Cilium Service LB VIP in network ${var.network_name} (guest path into the mesh)"
  address     = "${var.service_lb_ip}/32"
  groups      = [netbird_group.guest_users_resources.id]
  enabled     = true

  lifecycle {
    prevent_destroy = true
  }
}

locals {
  # Shared Service-LB subnet target: the reverse-proxy forwards THROUGH the
  # mesh to the network resource (target_id = resource ID, target_type
  # subnet) and the routing peer delivers it to the LB VIP (host). Callers
  # tune port/protocol/path via vars; extra peer/host/domain targets ride
  # var.targets unchanged. An empty path means no path pin (null keeps the
  # provider default of "/").
  service_lb_target = {
    target_id   = netbird_network_resource.service_lb.id
    target_type = "subnet"
    host        = var.service_lb_ip
    port        = var.target_port
    protocol    = var.target_protocol
    path        = var.target_path != "" ? var.target_path : null
  }
}

# Reverse-proxy service: public `domain` (full FQDN) -> shared Service-LB
# subnet target + caller-rendered extra `targets` inside the NetBird mesh.
# No open ports or firewall rules on the backends; TLS terminates at the
# proxy for `http` mode. `auth` defaults to `{}` (no proxy-level auth —
# backends that need NetBird identity read the `X-NetBird-User` /
# `X-NetBird-Groups` headers the proxy stamps).
# CrowdSec note: the provider schema (0.0.10, latest) has NO CrowdSec field
# (only access_restrictions for CIDR/country lists), and the product API
# exposes CrowdSec mode dashboard-side only — so Enforce is a manual
# dashboard step (Reverse Proxy > Services > Access Control), documented in
# README.md. Do NOT repurpose access_restrictions to fake it.
resource "netbird_reverse_proxy_service" "this" {
  name              = var.service_name
  domain            = var.domain
  mode              = var.mode
  listen_port       = var.mode == "http" ? null : var.listen_port
  enabled           = var.enabled
  pass_host_header  = var.pass_host_header
  rewrite_redirects = var.rewrite_redirects
  targets           = concat([local.service_lb_target], var.targets)
  auth              = var.auth

  access_restrictions = var.access_restrictions

  # The service URL only resolves once the base domain registration is
  # Active (wildcard CNAME propagated + dashboard Verify done).
  depends_on = [netbird_reverse_proxy_domain.this]

  lifecycle {
    prevent_destroy = true
  }
}
