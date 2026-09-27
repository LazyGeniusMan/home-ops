# Dedicated Talos NetBird root — Ansible-managed only (RUNBOOK §1.0b), never Flux.
# Upsert-only: `prevent_destroy` everywhere except the setup key (replacement is
# the sanctioned rotation path). Staged at build/<cluster>/netbird-tf/.
# PAT rides NB_PAT env only (no token variable); setup key is reusable but scoped
# (90d, usage_limit 3 — rotate before expiry); routing via peer_groups.
# No Service-LB resource (LB VIP owned by the Flux consumer).
# Single-writer: both clusters share one account — import the globals into the
# second cluster's state first (see README.md).
provider "netbird" {
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

# Resource groups associate by NAME (only the network_resource.groups edge is managed).
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

# Built-in catch-all group — read, never manage.
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

# Reusable setup key, scoped (90d + usage_limit 3); plaintext only via the
# sensitive talos_setup_key output. No prevent_destroy: replacement is the
# sanctioned rotation path (RUNBOOK §1.0b).
resource "netbird_setup_key" "talos" {
  name           = var.cluster_name
  type           = "reusable"
  expiry_seconds = 7776000
  usage_limit    = 3
  auto_groups    = [netbird_group.cluster_nodes.id]
}

# Routing: per-cluster nodes group serves as routing peers.
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

# Admin peer policy (peer chain; LAN rides admin_users_lan_access).
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

# Guest policy: guest-users reach guest-users-resources on TCP 80/443 only.
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
