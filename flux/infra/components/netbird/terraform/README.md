# Reusable NetBird reverse-proxy root

Single reusable Terraform root for every reverse-proxy slice, shipped inside
the `infra/netbird` OCI artifact. Consumers reference this root via
cross-namespace `sourceRef` + their own vars.

## Layout (flat — no child modules)

- `main.tf` — `netbird` + `cloudflare` provider blocks, `netbird_reverse_proxy_clusters`
  lookup, the PAT-driven access fabric (groups, per-cluster network +
  routing peer, Talos setup key, LAN / Service-LB network resources,
  admin/guest policies), one `netbird_reverse_proxy_domain` (count-gated),
  one Cloudflare wildcard CNAME `cloudflare_dns_record` (count-gated), one
  `netbird_reverse_proxy_service` (shared Service-LB subnet target +
  caller extras). Zero `module` blocks.
- `variables.tf` — full contract (service identity, domain, DNS zone,
  mode/targets/auth, provider tokens, plus fabric knobs `cluster_name`,
  `service_lb_ip`, `target_port`/`target_protocol`/`target_path`,
  `lan_cidr`). No `app_host`/`ui_host` vars: callers pass the
  fully-rendered `domain` FQDN, mirroring the zitadel root. `targets`
  defaults to `[]` (extra backends only — the LB target is always on).
- `outputs.tf` — `service_id`, `service_domain`, `proxy_url`, `proxy_cluster`,
  `domain_id`, `domain_validated`, `dns_record_name`, plus fabric outputs
  `talos_setup_key` (sensitive), `cluster_network_id`,
  `cluster_nodes_group_id`, `service_lb_resource_id`, `lan_resource_id`.
  Names match what consumer `writeOutputsToSecret` expects.
- `versions.tf` — `required_version >= 1.11`, `netbirdio/netbird ~> 0.0.10`,
  `cloudflare/cloudflare ~> 5.0`. 0.0.10 is the latest registry release, so
  no bump is available for CrowdSec (see below).

## What the root does

1. Registers the custom base domain (`netbird_reverse_proxy_domain`) unless
   `create_custom_domain = false` (free/cluster-domain path). Create-once,
   then a steady-state no-op: both `domain` and `target_cluster` are
   `RequiresReplace` upstream, so changing either fails closed under
   `prevent_destroy` by design.
2. Manages the wildcard CNAME `*.<base-domain>` → proxy cluster address in
   Cloudflare unless `cloudflare_zone_id` is `null` (DNS self-managed). The
   record is `proxied = false` (DNS-only) — Cloudflare proxying would hide
   the cluster address from NetBird's ownership lookup and break ZeroSSL
   issuance.
3. Builds the PAT-driven access fabric: `admin-users` / `guest-users` /
   `<cluster>-nodes` groups (+ `admin-users-resources` /
   `guest-users-resources` resource groups), the per-cluster
   `netbird_network` (`var.cluster_name`), the unlimited reusable Talos
   setup key (`expiry_seconds = 0`, `usage_limit = 0`, auto-joins
   `<cluster>-nodes`; plaintext leaves only via the sensitive
   `talos_setup_key` output), the `netbird_network_router` (nodes group as
   routing peers, masquerade on — new Networks model; the legacy
   `netbird_route` resource is intentionally unused), the `LAN CIDR`
   resource (`var.lan_cidr`, admin-only) and the `Service Load Balancer IP`
   `/32` resource (`var.service_lb_ip`, guest path), plus the
   `admin-users-access` / `admin-users-lan-access` (peers rule uses
   `destinations` incl. the built-in `All` group; LAN rule uses
   `destination_resource` — the two are mutually exclusive per rule and a
   policy holds exactly one rule, hence two policies) and
   `guest-users-access` (TCP 80+443 to the LB resource via
   `destination_resource type = subnet`) policies.
4. Wires the reverse-proxy service: public `domain` FQDN → the shared
   Service-LB subnet target (`target_id` = LB resource ID, `target_type`
   `subnet`, `host` = `var.service_lb_ip`, port/protocol/path via
   `target_port`/`target_protocol`/`target_path`) plus caller extras in
   `targets`. `http` mode terminates TLS at the proxy; `tcp` /
   `udp` / `tls` listen on `listen_port` (0 = auto-assign).

`netbird_dns_zone` (internal MagicDNS custom zones) is deliberately NOT in
this root: it governs in-mesh name resolution, not proxy verification.

## First-run ordering (custom-domain path)

Terraform only performs the *registration* (step 0). Ownership verification
is a one-time manual confirmation — NetBird has no API to trigger it:

0. Apply the consumer Terraform CR — registers the domain (status **Pending
   Verification**) and creates the wildcard CNAME.
1. Wait for DNS propagation: `dig CNAME *.<base-domain> +short` must return
   the proxy cluster address.
2. Check CAA: `dig CAA <base-domain> +short` and each parent. If records
   exist they must authorize `sectigo.com` for both `issue` and `issuewild`
   (NetBird Cloud issues via ZeroSSL/Sectigo); domains with no CAA records
   need no action.
3. In the dashboard (**Reverse Proxy > Custom Domains**) click **Verify
   Domain** next to the domain within 48h of registration — unverified
   registrations expire (removed at management startup and every 60 min) and
   must be re-added. Once **Active**, the domain stays usable and later
   applies are no-ops.

If the base domain was already registered out-of-band, import it once into
the consumer's state instead of creating (same for the CNAME record), then
reconcile:

```bash
tofu import 'netbird_reverse_proxy_domain.this[0]' '<domain-id>'
tofu import 'cloudflare_dns_record.validation[0]' '<zone-id>/<record-id>'
```

## Talos bootstrap (setup key, no manual per-node seeding)

Talos nodes join the mesh with the `talos_setup_key` output — no Proton
Pass setup-key seeding per node (the PAT in `netbird_token` is the only
vault secret). Read the key from the consumer's outputs Secret (it is
sensitive and never lands in git):

```bash
kubectl -n <consumer-ns> get secret <app>-proxy-outputs \
  -o jsonpath='{.data.talos_setup_key}' | base64 -d
```

then hand it to the talos/ cluster task as the node's `--setup-key`
(see that task's contract — this root only mints and exposes the key).
Key properties: `type = reusable`, `expiry_seconds = 0` (never expires),
`usage_limit = 0` (unlimited uses), `auto_groups = [<cluster>-nodes]`.
Rotating means replacing the key resource — plan first; `prevent_destroy`
fails closed rather than silently revoking node membership.

## CrowdSec (dashboard-only — no TF field)

CrowdSec Enforce is NOT Terraform-representable: provider `0.0.10` (the
latest registry release — verified, no bump available) exposes no CrowdSec
attribute on `netbird_reverse_proxy_service` (only `access_restrictions`
for CIDR/country lists), and the management API surfaces CrowdSec mode
dashboard-side only. Do NOT repurpose `access_restrictions` to fake it.
After the first apply, set it manually per service: **Reverse Proxy >
Services > <service> > Access Control > CrowdSec → Enforce** (default
Off; Observe only logs). Re-apply this step if the service is ever
recreated. `main.tf` carries the same note at the service resource.

## Consumer usage (cross-namespace sourceRef)

```yaml
---
# Zero-UI vars mirror: provider tokens only (plain values ride `vars`
# below). Unlike the zitadel handoff there is no chart-minted central secret
# to mirror cross-namespace — the NetBird PAT and the Cloudflare API token
# are seeded once in Proton Pass and read directly through the existing
# cluster-scoped `proton-pass` ClusterSecretStore (same shape as the
# coder-db-credentials ExternalSecret). Keys land under the module var names
# so `varsFrom` below needs no varsKeys renames. Per-env overlays replace
# the `pass://...` keys with the cluster vault path
# (`pass://<cluster>/<app>/netbird-pat`, same convention as the coder dev
# overlay).
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: <app>-terraform-vars
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: proton-pass
    kind: ClusterSecretStore
  target:
    name: <app>-terraform-vars
  data:
    - secretKey: netbird_token
      remoteRef:
        key: __PROTON_PASS_BASE__/<app>/netbird-pat
    - secretKey: cloudflare_api_token
      remoteRef:
        key: __PROTON_PASS_BASE__/<app>/cloudflare-api-token
---
apiVersion: infra.contrib.fluxcd.io/v1alpha2
kind: Terraform
metadata:
  name: <app>-proxy
spec:
  interval: 30m
  approvePlan: auto
  # Upsert-only guard (tofu-controller 0.16.5): the v1alpha2 CRD has no
  # preventDestroy/destroyPlan field — `destroy` (destroy plan on reconcile)
  # and `destroyResourcesOnDeletion` (destroy plan on CR deletion) are the
  # only destroy knobs, both default false. Stated explicitly so no `tofu
  # destroy` path exists via Flux; drift detection stays on (default).
  destroy: false
  destroyResourcesOnDeletion: false
  # Shared reusable reverse-proxy root: the infra/netbird OCI artifact
  # (namespace `netbird`) packs flux/infra/components/netbird/terraform/ at
  # ./terraform. Cross-namespace sourceRef is allowed by
  # allowCrossNamespaceRefs=true on the tofu-controller — no per-app
  # terraform/ root, no git-SHA module pin.
  sourceRef:
    kind: OCIRepository
    name: infra
    namespace: netbird
  path: ./terraform
  # Plain (non-sensitive) per-env values; secrets (netbird_token,
  # cloudflare_api_token) come from varsFrom below. varsFrom overrides vars
  # on key collision (upstream GenerateVarsForTF).
  # The service FQDN is fully rendered here (the shared root takes no
  # app_host var): `__SERVICE_HOST__` is the per-env hostname, replaced by
  # the dev/prd overlay patches. The backend rides the shared Service-LB
  # subnet target the root appends automatically: pass the per-env
  # cluster_name + service_lb_ip and tune port/protocol/path via target_*.
  # Keep targets [] (extra backends only) unless the slice needs more.
  # POSITIONAL: the dev/prd overlays patch /spec/vars/0 (domain) by index,
  # so this order must not change without updating the overlays.
  vars:
    - name: domain
      value: __SERVICE_HOST__
    - name: service_name
      value: <app>
    - name: cloudflare_zone_id
      value: __CLOUDFLARE_ZONE_ID__
    - name: cluster_name
      value: __CLUSTER_NAME__
    - name: service_lb_ip
      value: __SERVICE_LB_IP__
    - name: target_port
      value: 8080
    - name: target_protocol
      value: https
  varsFrom:
    - kind: Secret
      name: <app>-terraform-vars
  writeOutputsToSecret:
    name: <app>-proxy-outputs
    outputs:
      - service_id
      - service_domain
      - proxy_url
      - talos_setup_key
      - service_lb_resource_id
```

Provider tokens (`netbird_token`, `cloudflare_api_token`) flow from Proton
Pass via the ESO-synced `<app>-terraform-vars` Secret — never commit
secrets, never hardcode IDs. The controller also accepts
`NB_PAT`/`CLOUDFLARE_API_TOKEN` env vars on the runner as a fallback, but
`varsFrom` is the in-repo mechanism. Backend: in-cluster Kubernetes default
(state Secrets in the consumer namespace) — no backendConfig needed.

## Variables

| Name | Type | Required | Default | Description |
| ---- | ---- | -------- | ------- | ----------- |
| `service_name` | `string` | yes | — | Reverse-proxy service name (dashboard label) |
| `domain` | `string` | yes | — | Fully-rendered service FQDN — callers render hosts; the root takes no `app_host`/`ui_host` vars |
| `base_domain` | `string` | no | parent of `domain` | Custom base domain for the registration + verification CNAME |
| `create_custom_domain` | `bool` | no | `true` | Register the custom domain (`false` = free/cluster-domain path) |
| `target_cluster` | `string` | no | first connected cluster | Proxy cluster address the base domain validates against |
| `cloudflare_zone_id` | `string` | no | `null` | Cloudflare zone ID (`null` = DNS self-managed, skip CNAME) |
| `dns_ttl` | `number` | no | `300` | TTL for the wildcard CNAME verification record |
| `mode` | `string` | no | `http` | `http` (L7, TLS at proxy) or `tcp`/`udp`/`tls` (L4 passthrough) |
| `listen_port` | `number` | no | `0` | Proxy listen port for L4/tls modes (0 = auto-assign; ignored for `http`) |
| `enabled` | `bool` | no | `true` | Service toggle (off without deleting) |
| `pass_host_header` | `bool` | no | `true` | Pass the client Host header through (http mode) |
| `rewrite_redirects` | `bool` | no | `true` | Rewrite backend `Location` headers to the public domain (http mode) |
| `targets` | `list(object)` | no | `[]` | Extra mesh backends (`target_id`/`target_type`/`port`/`protocol` + optional `host`/`path`/`enabled`/`options`) — the shared Service-LB subnet target is always appended by the root |
| `auth` | `any` | no | `{}` | Proxy-level auth block (`{}` = none; NetBird identity via `X-NetBird-User`/`X-NetBird-Groups` headers) |
| `access_restrictions` | `object` | no | `null` | IP/country allow/block lists (`null` = unrestricted) |
| `netbird_token` | `string` (sensitive) | no | `null` | NetBird management PAT (controller injects via `varsFrom`; manual runs pass `-var`, never commit) |
| `management_url` | `string` | no | `https://api.netbird.io` | NetBird management API URL |
| `cloudflare_api_token` | `string` (sensitive) | no | `null` | Cloudflare API token with DNS edit (controller injects via `varsFrom`; manual runs pass `-var`, never commit) |
| `cluster_name` | `string` | no | `talos-apps` | Per-cluster fabric name (network, `<cluster>-nodes` group, setup key); consumer passes the full Talos name (e.g. `acme-dev-bdo1-talos-apps-01`) |
| `service_lb_ip` | `string` | no | `192.168.1.199` | Cilium LB VIP for the shared subnet target + Service-LB resource (consumer passes per env: dev `.249` / prd `.199`) |
| `target_port` | `number` | no | `3000` | Backend port of the shared Service-LB subnet target |
| `target_protocol` | `string` | no | `http` | Backend protocol of the shared Service-LB subnet target |
| `target_path` | `string` | no | `""` | URL path prefix on the shared target (`""` = no pin) |
| `lan_cidr` | `string` | no | `192.168.1.0/24` | LAN CIDR exposed via the admin-only network resource |

`base_domain`, `target_cluster`, `cloudflare_zone_id`, `netbird_token`, and
`cloudflare_api_token` default to `null` (Terraform forbids referencing
other values in a variable default); `main.tf` resolves them with
`coalesce` to the values shown above.

## Examples

HTTP service behind the shared Service-LB subnet target (zitadel-login
shape — public TLS at NetBird, Gateway terminates cluster TLS behind the
LB VIP; the root appends the subnet target automatically):

```hcl
# consumer Terraform CR vars (path: ./terraform):
service_name       = "zitadel-login"
domain             = "login.zitadel.proxy.example.com"
cloudflare_zone_id = "<zone-id>"
cluster_name       = "acme-dev-bdo1-talos-apps-01"
service_lb_ip      = "192.168.1.249"
target_path        = "/ui/v2/login"
```

HTTP service with an extra peer backend alongside the LB target
(oauth2-proxy-style SSO front — public TLS at NetBird, extra backend
terminates its own session; port/protocol/path via target_* for the LB
leg, raw target block for the extra leg):

```hcl
# consumer Terraform CR vars (path: ./terraform):
service_name       = "filer-ui"
domain             = "ui.seaweedfs.proxy.example.com"
cloudflare_zone_id = "<zone-id>"
cluster_name       = "acme-dev-bdo1-talos-apps-01"
service_lb_ip      = "192.168.1.249"
target_port        = 4180
targets = [{
  target_type = "peer"
  target_id   = "<netbird-peer-id>"
  port        = 4180
  protocol    = "http"
}]
```

Free-domain path (no custom domain, no Cloudflare — NetBird Cloud):

```hcl
# consumer Terraform CR vars (path: ./terraform):
service_name         = "dashboard"
domain               = "dashboard.abc123.eu.proxy.netbird.io"
create_custom_domain = false
cluster_name         = "acme-dev-bdo1-talos-apps-01"
service_lb_ip        = "192.168.1.249"
```

TCP passthrough with access restrictions (L4 modes take no `auth` — use
`{}`):

```hcl
# consumer Terraform CR vars (path: ./terraform):
service_name = "postgres"
domain       = "pg.proxy.example.com"
mode         = "tcp"
listen_port  = 15432
targets = [{
  target_type = "subnet"
  target_id   = "<network-resource-id>"
  host        = "10.0.0.5"
  port        = 5432
  protocol    = "tcp"
}]
auth = {}
access_restrictions = {
  allowed_countries = ["US", "DE"]
}
```

## Outputs

| Name | Sensitive | Description |
| ---- | --------- | ----------- |
| `service_id` | no | ID of the reverse-proxy service owned by this slice |
| `service_domain` | no | Service FQDN the proxy answers on |
| `proxy_url` | no | Public `https://` URL (`""` for L4 modes) |
| `proxy_cluster` | no | Proxy cluster handling this service (derived from domain) |
| `domain_id` | no | Custom-domain registration ID (`""` when `create_custom_domain` is `false`) |
| `domain_validated` | no | Whether the custom domain is validated (`null` when disabled) |
| `dns_record_name` | no | Wildcard CNAME record name (`""` when no Cloudflare record is managed) |
| `talos_setup_key` | **yes** | Reusable Talos setup key plaintext (unlimited, never expires; auto-joins `<cluster>-nodes`) |
| `cluster_network_id` | no | Per-cluster network ID (`var.cluster_name`) |
| `cluster_nodes_group_id` | no | Per-cluster nodes group ID (also the routing-peer group) |
| `service_lb_resource_id` | no | Service-LB network resource ID (backs the shared subnet target) |
| `lan_resource_id` | no | LAN CIDR network resource ID (admin path) |

## Secure defaults

- Upsert-only: every managed resource carries
  `lifecycle { prevent_destroy = true }`, and every consumer Terraform CR
  sets explicit `destroy: false` + `destroyResourcesOnDeletion: false`
  (the only destroy knobs in tofu-controller 0.16.5 v1alpha2 — there is no
  `preventDestroy`/`destroyPlan` field). No `tofu destroy` path via Flux;
  drift detection stays on.
- Verification CNAME is DNS-only (`proxied = false`): orange-clouding it
  would hand NetBird and ZeroSSL Cloudflare edge IPs instead of the proxy
  cluster address.
- No secrets in git: the NetBird PAT and the Cloudflare API token flow
  through ESO mirrors and the CR vars Secret; service outputs land in the
  `<app>-proxy-outputs` Secret via `writeOutputsToSecret`. The
  `talos_setup_key` output is `sensitive = true` and the root contains no
  `local-exec` (nothing ever echoes the key); consumers must add the key
  to `writeOutputsToSecret.outputs` (never to `vars`) so it lands in the
  Secret, not the controller logs.
- CrowdSec stays Off in TF (no provider field — see above); the manual
  Enforce step per service is the compensating control until the provider
  supports it.
- Backends must trust the proxy range `100.64.0.0/10` (the WireGuard source
  the proxy connects from) to read the real client IP from `X-Forwarded-For`;
  never hardcode a single NetBird IP. Proxy-stamped `X-NetBird-User` /
  `X-NetBird-Groups` headers are trustworthy only if the backend is reachable
  solely through the service — treat their absence as unauthenticated.
