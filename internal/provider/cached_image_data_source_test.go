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

// TODO: change this to only test for a non-existent image.
// Move the heavy lifting to integration.
func TestAccCachedImageDataSource(t *testing.T) {
	t.Run("Found", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		t.Cleanup(cancel)
		files := map[string]string{
			".devcontainer/devcontainer.json": `{"build": { "dockerfile": "Dockerfile" }}`,
			".devcontainer/Dockerfile": `FROM localhost:5000/test-ubuntu:latest
	RUN apt-get update && apt-get install -y cowsay`,
		}
		deps := setup(t, files)
		seedCache(ctx, t, deps)
		tfCfg := fmt.Sprintf(`data "envbuilder_cached_image" "test" {
	builder_image = %q
	workspace_folder = %q
	git_url = %q
	extra_env = {
	"FOO" : "bar"
	}
	cache_repo = %q
	verbose = true
}`, deps.BuilderImage, deps.RepoDir, deps.RepoDir, deps.CacheRepo)
		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{
				{
					Config: tfCfg,
					Check: resource.ComposeAggregateTestCheckFunc(
						// Inputs should still be present.
						resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "cache_repo", deps.CacheRepo),
						resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "extra_env.FOO", "bar"),
						resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "git_url", deps.RepoDir),
						// Should be empty
						resource.TestCheckNoResourceAttr("data.envbuilder_cached_image.test", "git_username"),
						resource.TestCheckNoResourceAttr("data.envbuilder_cached_image.test", "git_password"),
						resource.TestCheckNoResourceAttr("data.envbuilder_cached_image.test", "cache_ttl_days"),
						// Computed
						resource.TestCheckResourceAttrWith("data.envbuilder_cached_image.test", "id", func(value string) error {
							// value is enclosed in quotes
							value = strings.Trim(value, `"`)
							if !strings.HasPrefix(value, "sha256:") {
								return fmt.Errorf("expected image %q to have prefix %q", value, deps.CacheRepo)
							}
							return nil
						}),
						resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "exists", "true"),
						resource.TestCheckResourceAttrSet("data.envbuilder_cached_image.test", "image"),
						resource.TestCheckResourceAttrWith("data.envbuilder_cached_image.test", "image", func(value string) error {
							// value is enclosed in quotes
							value = strings.Trim(value, `"`)
							if !strings.HasPrefix(value, deps.CacheRepo) {
								return fmt.Errorf("expected image %q to have prefix %q", value, deps.CacheRepo)
							}
							return nil
						}),
						resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "env.0", "FOO=\"bar\""),
					),
				},
			},
		})
	})

	t.Run("NotFound", func(t *testing.T) {
		files := map[string]string{
			".devcontainer/devcontainer.json": `{"build": { "dockerfile": "Dockerfile" }}`,
			".devcontainer/Dockerfile": `FROM localhost:5000/test-ubuntu:latest
	RUN apt-get update && apt-get install -y cowsay`,
		}
		deps := setup(t, files)
		// We do not seed the cache.
		tfCfg := fmt.Sprintf(`data "envbuilder_cached_image" "test" {
	builder_image = %q
	workspace_folder = %q
	git_url = %q
	extra_env = {
	"FOO" : "bar"
	}
	cache_repo = %q
	verbose = true
}`, deps.BuilderImage, deps.RepoDir, deps.RepoDir, deps.CacheRepo)
		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{
				{
					Config: tfCfg,
					Check: resource.ComposeAggregateTestCheckFunc(
						// Inputs should still be present.
						resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "cache_repo", deps.CacheRepo),
						resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "extra_env.FOO", "bar"),
						resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "git_url", deps.RepoDir),
						resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "exists", "false"),
						resource.TestCheckResourceAttr("data.envbuilder_cached_image.test", "image", deps.BuilderImage),
						// Should be empty
						resource.TestCheckNoResourceAttr("data.envbuilder_cached_image.test", "git_username"),
						resource.TestCheckNoResourceAttr("data.envbuilder_cached_image.test", "git_password"),
						resource.TestCheckNoResourceAttr("data.envbuilder_cached_image.test", "cache_ttl_days"),
						// Computed values should be empty.
						resource.TestCheckNoResourceAttr("data.envbuilder_cached_image.test", "id"),
						resource.TestCheckResourceAttrSet("data.envbuilder_cached_image.test", "env.0"),
					),
				},
			},
		})
	})
}
