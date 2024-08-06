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

resource "envbuilder_cached_image" "example" {
  builder_image = "ghcr.io/coder/envbuilder:latest"
  git_url       = "https://github.com/coder/envbuilder-starter-devcontainer"
  cache_repo    = "localhost:5000/local/test-cache"
  extra_env = {
    "ENVBUILDER_VERBOSE" : "true"
  }
}

resource "docker_container" "container" {
  image = envbuilder_cached_image.example.image
  env   = envbuilder_cached_image.example.env
  name  = "myenv"
}
