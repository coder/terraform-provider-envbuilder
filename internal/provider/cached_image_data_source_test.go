// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCachedImageDataSource(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	files := map[string]string{
		"devcontainer.json": `{"build": { "dockerfile": "Dockerfile" }}`,
		"Dockerfile": `FROM ubuntu:latest
	RUN apt-get update && apt-get install -y cowsay`,
	}
	deps := setup(ctx, t, files)
	tfCfg := fmt.Sprintf(`data "envbuilder_cached_image" "test" {
	builder_image = %q
	devcontainer_dir = %q
	git_url = %q
	extra_env = {
	"FOO" : "bar"
	}
	cache_repo = %q
}`, deps.BuilderImage, deps.RepoDir, deps.RepoDir, deps.CacheRepo)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: tfCfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Input
					resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "cache_repo", deps.CacheRepo),
					resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "extra_env.FOO", "bar"),
					resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "git_url", deps.RepoDir),
					// Should be empty
					resource.TestCheckNoResourceAttr("data.envbuilder_cached_image.test", "git_username"),
					resource.TestCheckNoResourceAttr("data.envbuilder_cached_image.test", "git_password"),
					resource.TestCheckNoResourceAttr("data.envbuilder_cached_image.test", "cache_ttl_days"),
					// Computed
					resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "id", "cached-image-id"),
					resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "exists", "true"),
					resource.TestCheckResourceAttrSet("data.envbuilder_cached_image.test", "image"),
					resource.TestCheckResourceAttrWith("data.envbuilder_cached_image.test", "image", func(value string) error {
						if !strings.HasPrefix(value, deps.CacheRepo) {
							return fmt.Errorf("expected prefix %q", deps.CacheRepo)
						}
						return nil
					}),
					resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "env.0", "FOO=\"bar\""),
				),
			},
		},
	})
}
