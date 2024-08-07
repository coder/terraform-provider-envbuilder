// The below example illustrates the behavior of the envbuilder_cached_image
// resource.
// 1) Run a local registry:
// 
//    ```shell
//    docker run -d -p 5000:5000 --name test-registry registry:2
//    ```
//
// 2) Running a `terraform plan` should result in the following outputs:
// 
//    ```
//    + builder_image = "ghcr.io/coder/envbuilder-preview:latest"
//    + exists        = (known after apply)
//    + id            = (known after apply)
//    + image         = (known after apply)
//    ```
//
// 3) Running `terraform apply` should result in outputs similar to the below:
// 
//    ```
//       builder_image = "ghcr.io/coder/envbuilder-preview:latest"
//       exists = false
//       id = "00000000-0000-0000-0000-000000000000"
//       image = "ghcr.io/coder/envbuilder-preview:latest"
//    ```
//
// 4) Populate the cache by running Envbuilder and pushing the built image to
//    the local registry:
//
//    ```shell
//       docker run -it --rm \
//         -e ENVBUILDER_CACHE_REPO=localhost:5000/test \
//         -e ENVBUILDER_GIT_URL=https://github.com/coder/envbuilder-starter-devcontainer \
//         -e ENVBUILDER_PUSH_IMAGE=true \
//         -e ENVBUILDER_INIT_SCRIPT=exit \
//         --net=host \
//         ghcr.io/coder/envbuilder-preview:latest
//    ```
//
// 5) Run `terraform plan` once more. Now, the cached image will be detected:
//
//    ```
//       Note: Objects have changed outside of Terraform
//
//       Terraform detected the following changes made outside of Terraform since the last "terraform apply" which may have affected this plan:
//        envbuilder_cached_image.example has been deleted
//         - resource "envbuilder_cached_image" "example" {
//         - exists        = false -> null
//         - id            = "00000000-0000-0000-0000-000000000000" -> null
//         - image         = "ghcr.io/coder/envbuilder-preview:latest" -> null
//           # (5 unchanged attributes hidden)
//    ```
//
//    6) Run `terraform apply` and the newly pushed image will be saved in the Terraform state:
//    ```shell
//       builder_image = "ghcr.io/coder/envbuilder-preview:latest"
//       exists = true
//       id = "sha256:xxx..."
//       image = "localhost:5000/test@sha256:xxx..."
//    ```

terraform {
  required_providers {
    envbuilder = {
      source = "coder/envbuilder"
    }
  }
}

resource "envbuilder_cached_image" "example" {
  builder_image = "ghcr.io/coder/envbuilder-preview:latest"
  git_url       = "https://github.com/coder/envbuilder-starter-devcontainer"
  cache_repo    = "localhost:5000/test"
  extra_env = {
    "ENVBUILDER_VERBOSE" : "true"
  }
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
