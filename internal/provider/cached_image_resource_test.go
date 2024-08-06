// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCachedImageDataSource(t *testing.T) {
	t.Run("Found", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		files := map[string]string{
			".devcontainer/devcontainer.json": `{"build": { "dockerfile": "Dockerfile" }}`,
			".devcontainer/Dockerfile": `FROM localhost:5000/test-ubuntu:latest
	RUN date > /date.txt`,
		}

		deps := setup(ctx, t, files)
		seedCache(ctx, t, deps)
		deps.ExtraEnv["FOO"] = "bar"
		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{
				{
					Config: deps.Config(t),
					Check: resource.ComposeAggregateTestCheckFunc(
						// Inputs should still be present.
						resource.TestCheckResourceAttr("envbuilder_cached_image.test", "cache_repo", deps.CacheRepo),
						resource.TestCheckResourceAttr("envbuilder_cached_image.test", "extra_env.FOO", "bar"),
						resource.TestCheckResourceAttr("envbuilder_cached_image.test", "git_url", deps.Repo.URL),
						// Should be empty
						resource.TestCheckNoResourceAttr("envbuilder_cached_image.test", "git_username"),
						resource.TestCheckNoResourceAttr("envbuilder_cached_image.test", "git_password"),
						resource.TestCheckNoResourceAttr("envbuilder_cached_image.test", "cache_ttl_days"),
						// Computed
						resource.TestCheckResourceAttrWith("envbuilder_cached_image.test", "id", func(value string) error {
							// value is enclosed in quotes
							value = strings.Trim(value, `"`)
							if !strings.HasPrefix(value, "sha256:") {
								return fmt.Errorf("expected image %q to have prefix %q", value, deps.CacheRepo)
							}
							return nil
						}),
						resource.TestCheckResourceAttr("envbuilder_cached_image.test", "exists", "true"),
						resource.TestCheckResourceAttrSet("envbuilder_cached_image.test", "image"),
						resource.TestCheckResourceAttrWith("envbuilder_cached_image.test", "image", func(value string) error {
							// value is enclosed in quotes
							value = strings.Trim(value, `"`)
							if !strings.HasPrefix(value, deps.CacheRepo) {
								return fmt.Errorf("expected image %q to have prefix %q", value, deps.CacheRepo)
							}
							return nil
						}),
						resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.0", "FOO=\"bar\""),
					),
				},
			},
		})
	})

	t.Run("NotFound", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		files := map[string]string{
			".devcontainer/devcontainer.json": `{"build": { "dockerfile": "Dockerfile" }}`,
			".devcontainer/Dockerfile": `FROM localhost:5000/test-ubuntu:latest
	RUN date > /date.txt`,
		}
		deps := setup(ctx, t, files)
		deps.ExtraEnv["FOO"] = "bar"
		// We do not seed the cache.
		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{
				{
					Config: deps.Config(t),
					Check: resource.ComposeAggregateTestCheckFunc(
						// Computed values MUST be present.
						resource.TestCheckResourceAttr("envbuilder_cached_image.test", "id", uuid.Nil.String()),
						resource.TestCheckResourceAttr("envbuilder_cached_image.test", "exists", "false"),
						resource.TestCheckResourceAttrSet("envbuilder_cached_image.test", "env.0"),
						// Cached image should be set to the builder image.
						resource.TestCheckResourceAttr("envbuilder_cached_image.test", "image", deps.BuilderImage),
						// Inputs should still be present.
						resource.TestCheckResourceAttr("envbuilder_cached_image.test", "cache_repo", deps.CacheRepo),
						resource.TestCheckResourceAttr("envbuilder_cached_image.test", "extra_env.FOO", "bar"),
						resource.TestCheckResourceAttr("envbuilder_cached_image.test", "git_url", deps.Repo.URL),
						// Should be empty
						resource.TestCheckNoResourceAttr("envbuilder_cached_image.test", "git_username"),
						resource.TestCheckNoResourceAttr("envbuilder_cached_image.test", "git_password"),
						resource.TestCheckNoResourceAttr("envbuilder_cached_image.test", "cache_ttl_days"),
					),
				},
			},
		})
	})
}
