// The below example illustrates the behavior of the envbuilder_cached_image
// resource.

terraform {
  required_providers {
    envbuilder = {
      source = "coder/envbuilder"
    }
    docker = {
      source = "kreuzwerker/docker"
    }
  }
}

// This variable designates the devcontainer repo to build.
// The default is recommended for testing, as it builds relatively quickly!
variable "repo_url" {
  type    = string
  default = "https://github.com/coder/envbuilder-starter-devcontainer"
}

// This variable designates the builder image to use to build the devcontainer.
variable "builder_image" {
  type    = string
  default = "ghcr.io/coder/envbuilder:latest"
}

// If you have an existing repository you want to use as a cache, you can set this here.
// Otherwise, we will stand up a temporary local registry.
variable "cache_repo" {
  type    = string
  default = ""
}

locals {
  // If no registry is specified, use a default.
  cache_repo = var.cache_repo == "" ? "localhost:5000/test" : var.cache_repo
}

// Start a local registry if no cache repo is specified.
resource "docker_container" "registry" {
  count = var.cache_repo == "" ? 1 : 0
  name  = "envbuilder-cached-image-registry"
  image = "registry:2"
  ports {
    internal = 5000
    external = 5000
  }
  network_mode = "host"
  lifecycle {
    // We want to persist this across invocations
    ignore_changes = all
  }
}

// This resource performs the heavy lifting of determining
// if we need to build the devcontainer from scratch, or if
// there is a previously built image we can re-use.
// It fetches the Git repo located at var.git_repo, and
// performs a 'dry-run' build of the Devcontainer/Dockerfile.
// If all of the layers produced by the dry run are present
// in the remote cache repo, that image can then be used
// instead. Otherwise, the cache is stale and the image needs
// to be rebuilt.
resource "envbuilder_cached_image" "example" {
  builder_image = var.builder_image
  git_url       = var.repo_url
  cache_repo    = local.cache_repo
  extra_env = {
    "ENVBUILDER_VERBOSE" : "true"
    "ENVBUILDER_INSECURE" : "true" # due to local registry
    "ENVBUILDER_INIT_SCRIPT" : "sleep infinity"
    "ENVBUILDER_PUSH_IMAGE" : "true"
  }
  depends_on = [docker_container.registry]
}

// Run the cached image. Depending on the contents of
// the cache repo, this will either be var.builder_image
// or a previously built image pusehd to var.cache_repo.
// Running `terraform apply` once (assuming empty cache) 
// will result in the builder image running, and the built
// image being pushed to the cache repo.
// Running `terraform apply` again will result in the
// previously built image running.
resource "docker_container" "example" {
  name         = "envbuilder-cached-image-example"
  image        = envbuilder_cached_image.example.image
  env          = envbuilder_cached_image.example.env
  network_mode = "host" # required to hit local registry
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

