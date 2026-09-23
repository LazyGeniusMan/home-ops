# Provider configuration: both providers read the SAME kubeconfig file so
# validation-only flows (`tofu plan` with a dummy kubeconfig, `tofu test`
# with mocks) never need a live cluster. Point kubeconfig_path at the real
# cluster kubeconfig for apply.
provider "kubernetes" {
  config_path = var.kubeconfig_path
}

provider "helm" {
  kubernetes = {
    config_path = var.kubeconfig_path
  }
}
