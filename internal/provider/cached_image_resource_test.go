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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	files := map[string]string{
		".devcontainer/devcontainer.json": `{"build": { "dockerfile": "Dockerfile" }}`,
		".devcontainer/Dockerfile": `FROM localhost:5000/test-ubuntu:latest
	RUN date > /date.txt`,
	}

	deps := setup(ctx, t, files)
	deps.ExtraEnv["FOO"] = "bar"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Initial state: cache has not been seeded.
			{
				Config:             deps.Config(t),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			// Should detect that no cached image is present.
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
			// Now, seed the cache. We should detect the cached image resource.
			{
				PreConfig: func() {
					seedCache(ctx, t, deps)
				},
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
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.1", fmt.Sprintf("ENVBUILDER_CACHE_REPO=%q", deps.CacheRepo)),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.2", fmt.Sprintf("ENVBUILDER_GIT_URL=%q", deps.Repo.URL)),
				),
			},
			// Should produce an empty plan after apply
			{
				Config:   deps.Config(t),
				PlanOnly: true,
			},
		},
	})
}
