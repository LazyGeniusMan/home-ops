terraform {
  required_version = ">= 1.11"
  required_providers {
    zitadel = {
      source  = "zitadel/zitadel"
      version = "~> 3.3"
    }
  }
}
