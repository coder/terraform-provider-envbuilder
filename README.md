# terraform-provider-envbuilder

The `terraform-provider-envbuilder` is a Terraform provider that acts as a helper for setting up [`envbuilder`](https://envbuilder.sh) environments.

It is used to determine if a pre-built image of a Devcontainer built from a given Git repository is present in a given Docker registry.
If all layers that would result from building a particular devcontainer are present in the remote registry, then that image can simply be used as the starting point instead.

## Usage

Below is a very basic usage example. This checks a local registry running on port 5000 for an image built from the [Envbuilder Starter Devcontainer repo](https://github.com/coder/envbuilder-starter-devcontainer).

```terraform
terraform {
  required_providers {
    envbuilder = {
      source = "coder/envbuilder"
    }
  }
}

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

```

Take a look at [`envbuilder_cached_image_resource.tf`](./examples/resources/envbuilder_cached_image/envbuilder_cached_image_resource.tf) folder for a more detailed example.

For use with [Coder](https://github.com/coder/coder), see the [Dev Containers documentation](https://coder.com/docs/templates/dev-containers) and check out the example templates:
- [Docker](https://github.com/coder/coder/tree/main/examples/templates/devcontainer-docker)
- [Kubernetes](https://github.com/coder/coder/tree/main/examples/templates/devcontainer-kuberntes)
- [AWS VM](https://github.com/coder/coder/tree/main/examples/templates/devcontainer-aws-vm)
- [GCP VM](https://github.com/coder/coder/tree/main/examples/templates/devcontainer-gcp-vm)

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.22

## Building The Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command:

```shell
go install
```

## Using the provider

Fill this in for each provider

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `go generate`.

In order to run the full suite of Acceptance tests, run `make testacc`.

*Note:* Acceptance tests create real resources, and often cost money to run.

```shell
make testacc
```
