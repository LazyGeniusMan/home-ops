variable "oci_token" {
  description = "GitHub PAT for GHCR access."
  sensitive   = true
  type        = string
  nullable    = false
}

variable "cluster_name" {
  description = "Name of the cluster directory under clusters/ (e.g. acme-prd-bdo1-talos-apps-01, update)."
  type        = string
  nullable    = false
}

variable "cluster_region" {
  description = "Cloud provider region where the cluster runs (e.g. eu-west-2)."
  type        = string
  nullable    = false
}

variable "kubeconfig_path" {
  description = "Path to the kubeconfig file for the target cluster. Defaults to ~/.kube/config for apply; validation-only flows (CI, tofu test) can point at a dummy file since providers never dial out during init/validate/test/plan."
  type        = string
  default     = "~/.kube/config"
  nullable    = false
}

variable "bootstrap_revision" {
  description = "Bump to trigger a new bootstrap run."
  type        = number
  default     = 1
  nullable    = false
}

variable "cilium_k8s_service_host" {
  description = "Talos K8s API VIP (Layer2VIP) for the Cilium prerequisite chart values (k8sServiceHost). Per environment: prd 192.168.1.198, dev 192.168.1.248 — the same values the controllers/<env>/ kustomizations patch into the Flux-reconciled HelmRelease. NOT the LB pool VIP (.199 prd / .249 dev)."
  type        = string
  nullable    = false

  validation {
    condition     = var.cilium_k8s_service_host == "192.168.1.198" || var.cilium_k8s_service_host == "192.168.1.248"
    error_message = "cilium_k8s_service_host must be the Talos API VIP for the target cluster: 192.168.1.198 (prd) or 192.168.1.248 (dev)."
  }
}
