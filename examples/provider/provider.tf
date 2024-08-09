terraform {
  required_providers {
    envbuilder = {
      source = "coder/envbuilder"
    }
  }
}

// The provider currently requires no additional configuration.
provider "envbuilder" {}

// Creating this resource will check the registry located at
// localhost:5000 for the presence of a devcontainer built from
// the specified Git repo.
resource "envbuilder_cached_image" "example" {
  builder_image = "ghcr.io/coder/envbuilder:latest"
  git_url       = "https://github.com/coder/envbuilder-starter-devcontainer"
  cache_repo    = "localhost:5000"
}


// If the image is found in the remote repo, this output
// will be equal to the remote cached image. Otherwise
// it will be set to the value of `builder_image`.
output "image" {
  value = envbuilder_cached_image.example.image
}

// Whether the remote cached image was found or not.
output "exists" {
  value = envbuilder_cached_image.example.exists
}

// The SHA256 repo digest of the remote image, if found.
// Otherwise, this will be the null UUID.
output "id" {
  value = envbuilder_cached_image.example.id
}

