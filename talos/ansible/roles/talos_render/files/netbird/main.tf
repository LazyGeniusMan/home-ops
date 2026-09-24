# Dedicated Talos NetBird access-fabric root — managed ONLY by Ansible
# (roles/talos_render/tasks/netbird_setup_key.yml via the
# `community.general.terraform` module), never by Flux. The Flux netbird
# infra root is proxy-only and MUST NOT be referenced or imported here.
#
# Upsert-only: every managed resource below carries
# `lifecycle { prevent_destroy = true }`, so any plan that would delete or
# replace a resource fails instead of destroying it. There is no
# `tofu destroy` path for this root; drift detection stays on.
#
# State: the module stages a working copy of this root at
# build/<cluster>/netbird-tf/ (gitignored via ansible/build/) and keeps its
# local `terraform.tfstate` there ACROSS runs, so re-applies are true
# upserts. Do NOT delete that dir between runs: a fresh dir means empty
# state, and the next apply would try to CREATE duplicates of the groups /
# network / key below and fail closed under `prevent_destroy`. Recovery
# after a `rm -rf build/<cluster>` is `tofu import` per resource (see
# README.md in this dir), never a blind re-apply.
#
# Provider schema reference (entrypoints; fetch via
# scripts/fetch-references.sh into /tmp/home-ops-docs/):
# - index.md ................ provider auth: `token` may ride the NB_PAT env
#   var (config value wins over env, so Ansible passes the PAT ONLY as
#   NB_PAT env, never as -var netbird_token).
# - group.md ................ netbird_group (name identifier; resource-group
#   membership mirrors back as computed state — see the comment on the
#   resource groups below).
# - network.md .............. netbird_network (per-cluster fabric).
# - setup_key.md ............ netbird_setup_key (reusable, expiry_seconds 0
#   = never expires, usage_limit 0 = unlimited; plaintext ONLY via the
#   sensitive talos_setup_key output).
# - network_router.md ....... netbird_network_router (Networks-model routing
#   peers via peer_groups). route.md documents the LEGACY per-network route
#   resource (netbird_route), which is intentionally UNUSED here.
# - network_resource.md ..... netbird_network_resource (address = subnet
#   CIDR; groups edge managed here only).
# - policy.md ............... netbird_policy: `destinations` and
#   `destination_resource` are mutually exclusive per rule, and the provider
#   accepts exactly ONE rule per policy — so the admin peer chain and the
#   admin LAN-resource chain ride TWO policies.
#
# PAT-driven access fabric (admin/guest segmentation + per-cluster Network).
# Talos nodes join via the reusable setup key below (no per-node Proton Pass
# seeding); the per-cluster Network carries the LAN CIDR resource
# (admin-only). No Service-LB resource lives here — the LB VIP is a
# proxy-only concern owned by the Flux consumer, so this root takes no
# service_lb_ip var. Groups/policies follow the Networks product model: a
# resource is reachable only when an access policy allows a source group to
# reach it.
provider "netbird" {
  token          = var.netbird_token
  management_url = var.management_url
}

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

# Admin peer policy: admin-users may reach every group (node input-chain +
# peer-to-peer reachability across the mesh), on all protocols. This covers
# the peer chain only — resources behind the routing peer need the
# destination_resource chain in admin_users_lan_access below (peer-to-peer
# destinations alone do NOT cover resources behind the peer). The provider
# schema allows exactly ONE rule per policy AND forbids destinations +
# destination_resource in one rule (both mutually exclusive), so the peer
# chain and the LAN-resource chain ride TWO policies — admin-users-access
# here plus admin-users-lan-access below.
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

# Guest policy: guest-users reach ONLY the guest-users-resources group, on
# TCP 80/443. This is the peer/input chain; no guest network_resource exists
# in this root (the Service-LB resource stays owned by the proxy-only Flux
# consumer), so there is no destination_resource to point at here. When a
# guest-facing network_resource attaches to guest-users-resources, its
# forward-chain access rides a separate destination_resource rule (same
# one-rule-per-policy split as the admin pair above) — do NOT merge it into
# this rule (destinations ⊕ destination_resource are mutually exclusive).
resource "netbird_policy" "guest_users_access" {
  name        = "guest-users-access"
  description = "Guest users reach the guest-users-resources group on HTTP/HTTPS only"
  enabled     = true

  rule {
    name         = "guest-users-access"
    action       = "accept"
    protocol     = "tcp"
    enabled      = true
    sources      = [netbird_group.guest_users.id]
    ports        = ["80", "443"]
    destinations = [netbird_group.guest_users_resources.id]
  }

  lifecycle {
    prevent_destroy = true
  }
}
