# Provider configuration for local clusters (KinD, Talos dev).
# For cloud clusters, point these at the cluster module outputs instead
# (see the flux-operator-bootstrap module docs for an EKS example).
provider "kubernetes" {
  config_path = "~/.kube/config"
}

provider "helm" {
  kubernetes = {
    config_path = "~/.kube/config"
  }
}
