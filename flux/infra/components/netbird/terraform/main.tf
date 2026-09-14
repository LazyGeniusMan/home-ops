# Reusable NetBird reverse-proxy root — shipped inside the infra/netbird OCI artifact
# (flux/infra/components/netbird/terraform/). Consumers reference this root
# via cross-namespace sourceRef + their own vars.
#
# Flat single layer: provider auth + custom-domain registration + Cloudflare
# wildcard CNAME for NetBird ownership verification + the reverse-proxy
# service itself — no child modules. Callers pass the fully-rendered service
# FQDN in `domain` (this root takes no app_host/ui_host vars), mirroring the
# zitadel shared root.
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

# Reverse-proxy service: public `domain` (full FQDN) -> caller-rendered
# `targets` inside the NetBird mesh. No open ports or firewall rules on the
# backends; TLS terminates at the proxy for `http` mode. `auth` defaults to
# `{}` (no proxy-level auth — backends that need NetBird identity read the
# `X-NetBird-User` / `X-NetBird-Groups` headers the proxy stamps).
resource "netbird_reverse_proxy_service" "this" {
  name              = var.service_name
  domain            = var.domain
  mode              = var.mode
  listen_port       = var.mode == "http" ? null : var.listen_port
  enabled           = var.enabled
  pass_host_header  = var.pass_host_header
  rewrite_redirects = var.rewrite_redirects
  targets           = var.targets
  auth              = var.auth

  access_restrictions = var.access_restrictions

  # The service URL only resolves once the base domain registration is
  # Active (wildcard CNAME propagated + dashboard Verify done).
  depends_on = [netbird_reverse_proxy_domain.this]

  lifecycle {
    prevent_destroy = true
  }
}
