# Reusable NetBird reverse-proxy root (shipped inside the infra/netbird OCI
# artifact; consumers reference it via cross-namespace sourceRef + their own vars).
# Flat single layer, no child modules: provider auth + guest Service-LB network
# resource + custom-domain registration + Cloudflare wildcard CNAME + the
# reverse-proxy service. Callers pass the fully-rendered FQDN in `domain`.
# Proxy-only: the access fabric (groups, per-cluster network, setup keys, routers,
# LAN resources, access policies) lives in the Talos Ansible Terraform task.
# Upsert-only: every resource carries prevent_destroy, and every consumer CR sets
# destroy/destroyResourcesOnDeletion false — no destroy path via Flux.
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

# Custom-domain registration (create-if-missing, then no-op; import once if
# already registered out-of-band). domain + target_cluster are RequiresReplace
# upstream, so changes fail closed under prevent_destroy — migrate via manual
# import, never via Flux.
resource "netbird_reverse_proxy_domain" "this" {
  count          = var.create_custom_domain ? 1 : 0
  domain         = local.base_domain
  target_cluster = local.target_cluster

  lifecycle {
    prevent_destroy = true
  }
}

# Ownership verification: wildcard CNAME must exist and propagate before the
# one-time dashboard Verify (48h deadline). Skipped when cloudflare_zone_id is
# null or create_custom_domain is false. proxied = false is load-bearing (must
# stay DNS-only).
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

# Guest Service-LB resource only: the guest-users-resources group (one edge managed
# to avoid a group <-> resource cycle) + the Service Load Balancer IP /32 network
# resource on the Talos-owned parent network (data source lookup, never managed).
resource "netbird_group" "guest_users_resources" {
  name = "guest-users-resources"

  lifecycle {
    prevent_destroy = true
  }
}

# Talos-owned parent network — read, never manage.
data "netbird_network" "parent" {
  name = var.network_name
}

# Service Load Balancer IP /32 network resource (pass the bare VIP).
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
  # Shared Service-LB subnet target (callers tune port/protocol/path via vars;
  # empty path = no pin, null keeps the provider default "/").
  service_lb_target = {
    target_id   = netbird_network_resource.service_lb.id
    target_type = "subnet"
    host        = var.service_lb_ip
    port        = var.target_port
    protocol    = var.target_protocol
    path        = var.target_path != "" ? var.target_path : null
  }
}

# Reverse-proxy service: public domain -> shared Service-LB subnet target + extra
# targets. TLS terminates at the proxy for http mode. auth defaults to {} (backends
# needing NetBird identity read the X-NetBird-User / X-NetBird-Groups headers).
# CrowdSec Enforce is a manual dashboard step (provider schema has no CrowdSec field).
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

  # URL resolves once the base domain registration is Active.
  depends_on = [netbird_reverse_proxy_domain.this]

  lifecycle {
    prevent_destroy = true
  }
}
