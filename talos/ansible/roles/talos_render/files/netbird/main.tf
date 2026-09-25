# Dedicated Talos NetBird access-fabric root — Ansible-managed only (RUNBOOK §1.0b), never Flux.
# Upsert-only: `prevent_destroy` on every resource (delete/replace plans fail closed; no destroy path).
# Staged at build/<cluster>/netbird-tf/ with persistent local state (re-applies upsert — never delete).
# PAT rides NB_PAT env only (never -var); setup key is reusable, expiry/usage 0, plaintext only via
# the sensitive talos_setup_key output; routing via netbird_network_router peer_groups
# (netbird_route unused); admin peer + LAN chains ride two policies (one rule per policy).
# No Service-LB resource (LB VIP owned by the Flux consumer).
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

# Resource groups associate by NAME. Only the network_resource.groups edge is managed here
# (the API mirrors membership back as computed state; managing both edges would cycle).
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

# Reusable setup key: peers minted through it land in the per-cluster nodes group.
# Plaintext leaves ONLY via the sensitive talos_setup_key output.
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

# Routing: per-cluster nodes group serves as routing peers (netbird_network_router peer_groups).
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

# Admin peer policy (peer chain only; LAN resources ride admin_users_lan_access below).
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

# Guest policy: guest-users reach ONLY guest-users-resources on TCP 80/443 (peer chain).
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
