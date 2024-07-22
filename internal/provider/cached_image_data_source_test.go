// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccExampleDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: testAccCachedImageDataSourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Input
					resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "extra_env.ENVBUILDER_VERBOSE", "true"),
					resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "git_url", "https://github.com/coder/envbuilder-starter-devcontainer"),
					resource.TestCheckNoResourceAttr("data.envbuilder_cached_image.test", "git_username"),
					resource.TestCheckNoResourceAttr("data.envbuilder_cached_image.test", "git_password"),
					resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "cache_repo", "localhost:5000/local/test-cache"),
					resource.TestCheckNoResourceAttr("data.envbuilder_cached_image.test", "cache_ttl_days"),
					// Computed
					resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "id", "cached-image-id"),
					resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "exists", "false"),
					resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "image", "ghcr.io/coder/envbuilder:latest"),
					resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "env.0", "ENVBUILDER_VERBOSE=\"true\""),
				),
			},
		},
	})
}

const testAccCachedImageDataSourceConfig = `
data "envbuilder_cached_image" "test" {
  builder_image = "ghcr.io/coder/envbuilder:latest"
  git_url       = "https://github.com/coder/envbuilder-starter-devcontainer"
  cache_repo    = "localhost:5000/local/test-cache"
  extra_env = {
    "ENVBUILDER_VERBOSE" : "true"
  }
}
`
