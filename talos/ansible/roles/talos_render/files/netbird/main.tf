# Dedicated Talos NetBird access-fabric root — managed ONLY by Ansible
# (roles/talos_render/tasks/netbird_setup_key.yml via
# `community.general.terraform`), never by Flux (Flux root is proxy-only).
#
# Upsert-only: every resource carries `lifecycle { prevent_destroy = true }`,
# so a delete/replace plan fails closed. No `tofu destroy` path.
#
# State: staged at build/<cluster>/netbird-tf/ (gitignored) with local
# `terraform.tfstate` kept ACROSS runs so re-applies upsert. Do NOT delete
# that dir between runs (fresh dir = empty state = duplicate-CREATE attempts
# failing closed; recovery is `tofu import` per resource — see README.md).
#
# Provider schema: PAT rides the NB_PAT env only (never -var netbird_token);
# setup key is reusable, expiry 0 = never, usage 0 = unlimited, plaintext
# only via the sensitive talos_setup_key output; routing uses
# netbird_network_router peer_groups (the legacy netbird_route resource is
# unused); admin peer + LAN-resource chains ride TWO policies (one rule per
# policy; `destinations` and `destination_resource` are mutually exclusive).
#
# Talos nodes join via the reusable setup key (no per-node vault seeding);
# the per-cluster Network carries the admin-only LAN CIDR resource. No
# Service-LB resource here (LB VIP is owned by the Flux consumer, so no
# service_lb_ip var).
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

# Resource groups associate by NAME with the resources below. Only the
# network_resource.groups edge is managed here — the API mirrors membership
# back onto the group's `resources` as computed state (managing both edges
# would cycle).
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

# Reusable setup key for Talos nodes: peers minted through it land in the
# per-cluster nodes group. Plaintext leaves ONLY via the sensitive
# talos_setup_key output (no local-exec anywhere in this root).
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
# per-cluster Network (netbird_network_router with peer_groups; the legacy
# netbird_route resource is unused). Single routing group, masquerade on.
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

# Admin peer policy: admin-users reach every group on all protocols (peer
# chain only — resources behind the routing peer need the
# destination_resource chain in admin_users_lan_access below).
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

# Guest policy: guest-users reach ONLY guest-users-resources on TCP 80/443
# (peer chain; no guest network_resource exists here, so no
# destination_resource — a future guest resource's forward chain rides a
# separate rule, never merged into this one).
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
