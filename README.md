# terraform-provider-envbuilder

The `terraform-provider-envbuilder` is a Terraform provider that acts as a helper for setting up [`envbuilder`](https://envbuilder.sh) environments.

It is used to determine if a pre-built image of a dev container built from a given Git repository is present in a given Docker registry.
If it is found that building a particular dev container would produce the same image that is already present in the remote registry, then that image can be used to start the container instead, skipping over the build phase.

> **Note:** currently, this provider can only be run on Linux platforms. We are [investigating support](https://github.com/coder/terraform-provider-envbuilder/issues/26) for other platforms. 

## Usage

Take a look at the [`envbuilder_cached_image_resource.tf`](./examples/resources/envbuilder_cached_image/envbuilder_cached_image_resource.tf) example for a detailed usage example.

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

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `go generate`.

In order to run the full suite of Acceptance tests, run `make testacc`.

*Note:* Acceptance tests create real resources, and often cost money to run.

```shell
make testacc
```
