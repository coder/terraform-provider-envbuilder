terraform {
  required_providers {
    envbuilder = {
      source = "coder/envbuilder"
    }
  }
}

// The provider currently requires no additional configuration.
provider "envbuilder" {}

resource "envbuilder_cached_image" "example" {
  builder_image = "ghcr.io/coder/envbuilder:latest"
  git_url       = "https://github.com/coder/envbuilder-starter-devcontainer"
  cache_repo    = "localhost:5000"
}

output "builder_image" {
  value = envbuilder_cached_image.example.builder_image
}

output "exists" {
  value = envbuilder_cached_image.example.exists
}

output "id" {
  value = envbuilder_cached_image.example.id
}

output "image" {
  value = envbuilder_cached_image.example.image
}
