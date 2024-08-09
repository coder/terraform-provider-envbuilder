terraform {
  required_providers {
    envbuilder = {
      source = "coder/envbuilder"
    }
  }
}

// The provider currently requires no additional configuration.
provider "envbuilder" {}

