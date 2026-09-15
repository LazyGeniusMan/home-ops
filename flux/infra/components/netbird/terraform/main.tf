# Reusable NetBird reverse-proxy root — shipped inside the infra/netbird OCI artifact
# (flux/infra/components/netbird/terraform/). Consumers reference this root
# via cross-namespace sourceRef + their own vars.
#
# Flat single layer: provider auth + PAT-driven access fabric (groups,
# per-cluster network + routing peer, Talos setup key, LAN / Service-LB
# network resources, admin/guest policies) + custom-domain registration +
# Cloudflare wildcard CNAME for NetBird ownership verification + the
# reverse-proxy service itself — no child modules. Callers pass the
# fully-rendered service FQDN in `domain` (this root takes no
# app_host/ui_host vars), mirroring the zitadel shared root.
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

# PAT-driven access fabric (admin/guest segmentation + per-cluster Network).
# Talos nodes join via the reusable setup key below (no manual Proton Pass
# setup-key seeding per node); the per-cluster Network carries the LAN CIDR
# resource (admin-only) and the Service-LB resource (guest path into the
# mesh). Groups/policies follow the Networks product model: a resource is
# reachable only when an access policy allows a source group to reach it.
resource "netbird_group" "admin_users" {
  name = "admin-users"

  lifecycle {
    prevent_destroy = true
  }
}

resource "netbird_group" "guest_users" {
  name = "guest-users"

  lifecycle {
    prevent_destroy = true
  }
}

resource "netbird_group" "cluster_nodes" {
  name = "${var.cluster_name}-nodes"

  lifecycle {
    prevent_destroy = true
  }
}

# Resource groups associate by NAME with the resources below: the resource's
# `groups` field references the group ID (network_resource -> group edge),
# and the API mirrors the membership back onto the group's `resources`
# (group -> resource edge). Managing both edges in TF would cycle
# (group <-> resource), so only the network_resource.groups edge is managed
# here; the group reads the mirrored membership back as computed state.
resource "netbird_group" "admin_users_resources" {
  name = "admin-users-resources"

  lifecycle {
    prevent_destroy = true
  }
}

resource "netbird_group" "guest_users_resources" {
  name = "guest-users-resources"

  lifecycle {
    prevent_destroy = true
  }
}

# Built-in catch-all group (exists on every account) — read, never manage.
data "netbird_group" "all" {
  name = "All"
}

resource "netbird_network" "cluster" {
  name        = var.cluster_name
  description = "Cluster LAN fabric for ${var.cluster_name} (Talos nodes join via the ${var.cluster_name} setup key; routing peers forward into the LAN)"

  lifecycle {
    prevent_destroy = true
  }
}

# Unlimited reusable setup key for Talos nodes: expiry_seconds = 0 (never
# expires) + usage_limit = 0 (unlimited uses) + type reusable. Peers minted
# through this key land in the per-cluster nodes group automatically. The
# plaintext key is exposed ONLY via the sensitive talos_setup_key output
# (never echoed, never logged — no local-exec anywhere in this root).
resource "netbird_setup_key" "talos" {
  name           = var.cluster_name
  type           = "reusable"
  expiry_seconds = 0
  usage_limit    = 0
  auto_groups    = [netbird_group.cluster_nodes.id]

  lifecycle {
    prevent_destroy = true
  }
}

# Routing: the per-cluster nodes group serves as routing peers for the
# per-cluster Network (new Networks model — netbird_network_router with
# peer_groups; the legacy netbird_route resource is intentionally unused).
# Masquerade stays on (LAN needs no awareness of the overlay); metric is the
# provider default (single routing group — no primary/failover split).
resource "netbird_network_router" "cluster" {
  network_id  = netbird_network.cluster.id
  peer_groups = [netbird_group.cluster_nodes.id]
  masquerade  = true
  enabled     = true

  lifecycle {
    prevent_destroy = true
  }
}

resource "netbird_network_resource" "lan" {
  network_id  = netbird_network.cluster.id
  name        = "LAN CIDR"
  description = "Cluster LAN reachable by the admin-users group only"
  address     = var.lan_cidr
  groups      = [netbird_group.admin_users_resources.id]
  enabled     = true

  lifecycle {
    prevent_destroy = true
  }
}

# Service Load Balancer IP as a /32 network resource: the single stable
# entrypoint guests use to reach in-cluster Services through the mesh
# (routing peer forwards onto the LAN toward the Cilium LB VIP). The address
# is always the /32 host form — pass the bare VIP in var.service_lb_ip.
resource "netbird_network_resource" "service_lb" {
  network_id  = netbird_network.cluster.id
  name        = "Service Load Balancer IP"
  description = "Cilium Service LB VIP for ${var.cluster_name} (guest path into the mesh)"
  address     = "${var.service_lb_ip}/32"
  groups      = [netbird_group.guest_users_resources.id]
  enabled     = true

  lifecycle {
    prevent_destroy = true
  }
}

# Admin policies: admin-users may reach every group (node input-chain +
# peer-to-peer reachability across the mesh) and, separately, the LAN CIDR
# resource behind the routing peer (forward chain — peer-to-peer
# destinations alone do NOT cover resources behind the peer). The provider
# schema allows exactly ONE rule per policy AND forbids destinations +
# destination_resource in one rule (both mutually exclusive), so the two
# chains ride two policies.
resource "netbird_policy" "admin_users_access" {
  name        = "admin-users-access"
  description = "Admin users reach all mesh groups (peer-to-peer / input chain)"
  enabled     = true

  rule {
    name         = "admin-users-peers"
    action       = "accept"
    protocol     = "all"
    enabled      = true
    sources      = [netbird_group.admin_users.id]
    destinations = [data.netbird_group.all.id, netbird_group.admin_users.id, netbird_group.guest_users.id, netbird_group.cluster_nodes.id]
  }

  lifecycle {
    prevent_destroy = true
  }
}

resource "netbird_policy" "admin_users_lan_access" {
  name        = "admin-users-lan-access"
  description = "Admin users reach the cluster LAN resource (forward chain through the routing peer)"
  enabled     = true

  rule {
    name     = "admin-users-lan"
    action   = "accept"
    protocol = "all"
    enabled  = true
    sources  = [netbird_group.admin_users.id]

    destination_resource = {
      id   = netbird_network_resource.lan.id
      type = "subnet"
    }
  }

  lifecycle {
    prevent_destroy = true
  }
}

# Guest policy: guest-users reach ONLY the Service-LB resource (the LB VIP
# the reverse-proxy subnet target forwards to), on the Gateway TCP
# listeners 80/443. No destinations list — destination_resource (type subnet)
# is mutually exclusive with destinations per the provider schema.
resource "netbird_policy" "guest_users_access" {
  name        = "guest-users-access"
  description = "Guest users reach the Service Load Balancer IP on HTTP/HTTPS only"
  enabled     = true

  rule {
    name     = "guest-users-access"
    action   = "accept"
    protocol = "tcp"
    enabled  = true
    sources  = [netbird_group.guest_users.id]
    ports    = ["80", "443"]

    destination_resource = {
      id   = netbird_network_resource.service_lb.id
      type = "subnet"
    }
  }

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
