variable "oci_token" {
  description = "GitHub PAT for GHCR access."
  sensitive   = true
  type        = string
  nullable    = false
}

variable "github_token" {
  description = "GitHub token with contents read+write on home-ops (classic PAT repo scope, or fine-grained contents read+write), seeded as flux-system/github-auth for ImageUpdateAutomation push access. Null (default) seeds nothing — set it when bootstrapping the update cluster."
  sensitive   = true
  type        = string
  default     = null
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
  description = "Path to the kubeconfig file for the target cluster (dummy file OK for validation-only flows)."
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
  description = "Talos K8s API VIP for the Cilium k8sServiceHost value: prd 192.168.1.198, dev 192.168.1.248 (NOT the LB pool VIP .199/.249)."
  type        = string
  nullable    = false

  validation {
    condition     = var.cilium_k8s_service_host == "192.168.1.198" || var.cilium_k8s_service_host == "192.168.1.248"
    error_message = "cilium_k8s_service_host must be the Talos API VIP for the target cluster: 192.168.1.198 (prd) or 192.168.1.248 (dev)."
  }
}
