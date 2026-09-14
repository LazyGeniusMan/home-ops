variable "service_name" {
  description = "Reverse-proxy service name (e.g. dashboard, filer-ui) — identifies the service in the NetBird dashboard"
  type        = string
}

variable "domain" {
  description = "Fully-rendered service FQDN (e.g. dashboard.proxy.example.com) — callers render hosts, the root takes no app_host/ui_host vars"
  type        = string
}

variable "base_domain" {
  description = "Custom base domain the service FQDN lives under (defaults to the parent of domain, e.g. proxy.example.com); the custom-domain registration + wildcard CNAME verification record target this domain"
  type        = string
  default     = null
}

variable "create_custom_domain" {
  description = "Whether to register the custom base domain + manage the verification CNAME (false = free/cluster-domain path: no domain resource, no Cloudflare record)"
  type        = bool
  default     = true
}

variable "target_cluster" {
  description = "Proxy cluster address the base domain is validated against (defaults to the first connected cluster from netbird_reverse_proxy_clusters)"
  type        = string
  default     = null
}

variable "cloudflare_zone_id" {
  description = "Cloudflare zone ID holding the base domain (null = DNS self-managed outside Cloudflare, skips the verification CNAME record)"
  type        = string
  default     = null
}

variable "dns_ttl" {
  description = "TTL seconds for the wildcard CNAME verification record"
  type        = number
  default     = 300
}

variable "mode" {
  description = "Service mode: http (L7, TLS termination at the proxy) or tcp/udp/tls (L4 passthrough on a dedicated listen_port)"
  type        = string
  default     = "http"
}

variable "listen_port" {
  description = "Port the proxy listens on (L4/tls modes only; 0 = auto-assign; ignored for http)"
  type        = number
  default     = 0
}

variable "enabled" {
  description = "Whether the service is enabled (toggle off without deleting)"
  type        = bool
  default     = true
}

variable "pass_host_header" {
  description = "When true, the original client Host header is passed through to the backend (http mode)"
  type        = bool
  default     = true
}

variable "rewrite_redirects" {
  description = "When true, Location headers in backend responses are rewritten to the public-facing domain (http mode)"
  type        = bool
  default     = true
}

variable "targets" {
  description = "Backend targets inside the NetBird mesh (peer/host/domain/subnet; port/protocol per target — see README for the HTTP example)"
  type = list(object({
    target_id   = string
    target_type = string
    port        = number
    protocol    = string
    enabled     = optional(bool)
    host        = optional(string)
    path        = optional(string)
    options = optional(object({
      request_timeout      = optional(string)
      session_idle_timeout = optional(string)
      path_rewrite         = optional(string)
      skip_tls_verify      = optional(bool)
      proxy_protocol       = optional(bool)
      custom_headers       = optional(map(string))
    }))
  }))
}

variable "auth" {
  description = "Proxy-level authentication block (default {} = none; backends needing NetBird identity read the X-NetBird-User / X-NetBird-Groups headers instead — see README)"
  type        = any
  default     = {}
}

variable "access_restrictions" {
  description = "Connection-level IP/country allow/block lists (null = unrestricted)"
  type = object({
    allowed_cidrs     = optional(list(string))
    allowed_countries = optional(list(string))
    blocked_cidrs     = optional(list(string))
    blocked_countries = optional(list(string))
  })
  default = null
}

variable "netbird_token" {
  description = "NetBird management PAT with reverse-proxy read/write (controller injects via varsFrom from the ESO-synced <app>-terraform-vars Secret; manual runs pass -var, never commit)"
  type        = string
  sensitive   = true
  default     = null
}

variable "management_url" {
  description = "NetBird management API URL (NetBird Cloud default)"
  type        = string
  default     = "https://api.netbird.io"
}

variable "cloudflare_api_token" {
  description = "Cloudflare API token with DNS edit on the zone (controller injects via varsFrom from the ESO-synced <app>-terraform-vars Secret; manual runs pass -var, never commit)"
  type        = string
  sensitive   = true
  default     = null
}
