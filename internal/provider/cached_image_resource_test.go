package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// testEnvValue is a multi-line environment variable value that we use in
// tests to ensure that we can handle multi-line values correctly.
var testEnvValue = `bar
baz`

func TestAccCachedImageResource(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	for _, tc := range []struct {
		name      string
		files     map[string]string
		extraEnv  map[string]string
		assertEnv func(t *testing.T, deps testDependencies) resource.TestCheckFunc
	}{
		{
			// This test case is the simplest possible case: a devcontainer.json.
			// However, it also makes sure we are able to generate a Dockerfile
			// from the devcontainer.json.
			name: "devcontainer only",
			files: map[string]string{
				".devcontainer/devcontainer.json": `{"image": "localhost:5000/test-ubuntu:latest"}`,
			},
			extraEnv: map[string]string{
				"FOO":                   testEnvValue,
				"ENVBUILDER_GIT_URL":    "https://not.the.real.git/url",
				"ENVBUILDER_CACHE_REPO": "not-the-real-cache-repo",
			},
			assertEnv: func(t *testing.T, deps testDependencies) resource.TestCheckFunc {
				return resource.ComposeAggregateTestCheckFunc(
					// Check that the environment variables are set correctly.
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.0", fmt.Sprintf("ENVBUILDER_CACHE_REPO=%s", deps.CacheRepo)),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.1", fmt.Sprintf("ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH=%s", deps.Repo.Key)),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.2", fmt.Sprintf("ENVBUILDER_GIT_URL=%s", deps.Repo.URL)),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.3", "ENVBUILDER_REMOTE_REPO_BUILD_MODE=true"),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.4", "ENVBUILDER_VERBOSE=true"),
					// Check that the extra environment variables are set correctly.
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.5", "FOO=bar\nbaz"),
					// We should not have any other environment variables set.
					resource.TestCheckNoResourceAttr("envbuilder_cached_image.test", "env.6"),

					// Check that the same values are set in env_map.
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_CACHE_REPO", deps.CacheRepo),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH", deps.Repo.Key),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_GIT_URL", deps.Repo.URL),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_REMOTE_REPO_BUILD_MODE", "true"),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_VERBOSE", "true"),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.FOO", "bar\nbaz"),
				)
			},
		},
		{
			// This test case includes a Dockerfile in addition to the devcontainer.json.
			// The Dockerfile writes the current date to a file. This is currently not checked but
			// illustrates that a RUN instruction is cached.
			name: "devcontainer and Dockerfile",
			files: map[string]string{
				".devcontainer/devcontainer.json": `{"build": { "dockerfile": "Dockerfile" }}`,
				".devcontainer/Dockerfile": `FROM localhost:5000/test-ubuntu:latest
RUN date > /date.txt`,
			},
			extraEnv: map[string]string{
				"FOO":                   testEnvValue,
				"ENVBUILDER_GIT_URL":    "https://not.the.real.git/url",
				"ENVBUILDER_CACHE_REPO": "not-the-real-cache-repo",
			},
			assertEnv: func(t *testing.T, deps testDependencies) resource.TestCheckFunc {
				return resource.ComposeAggregateTestCheckFunc(
					// Check that the environment variables are set correctly.
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.0", fmt.Sprintf("ENVBUILDER_CACHE_REPO=%s", deps.CacheRepo)),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.1", fmt.Sprintf("ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH=%s", deps.Repo.Key)),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.2", fmt.Sprintf("ENVBUILDER_GIT_URL=%s", deps.Repo.URL)),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.3", "ENVBUILDER_REMOTE_REPO_BUILD_MODE=true"),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.4", "ENVBUILDER_VERBOSE=true"),
					// Check that the extra environment variables are set correctly.
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.5", "FOO=bar\nbaz"),
					// We should not have any other environment variables set.
					resource.TestCheckNoResourceAttr("envbuilder_cached_image.test", "env.6"),

					// Check that the same values are set in env_map.
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_CACHE_REPO", deps.CacheRepo),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH", deps.Repo.Key),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_GIT_URL", deps.Repo.URL),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_REMOTE_REPO_BUILD_MODE", "true"),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_VERBOSE", "true"),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.FOO", "bar\nbaz"),
				)
			},
		},
		{
			// This test case ensures that parameters passed via extra_env are
			// handled correctly.
			name: "extra_env",
			files: map[string]string{
				"path/to/.devcontainer/devcontainer.json": `{"build": { "dockerfile": "Dockerfile" }}`,
				"path/to/.devcontainer/Dockerfile": `FROM localhost:5000/test-ubuntu:latest
		RUN date > /date.txt`,
			},
			extraEnv: map[string]string{
				"FOO":                               testEnvValue,
				"ENVBUILDER_GIT_URL":                "https://not.the.real.git/url",
				"ENVBUILDER_CACHE_REPO":             "not-the-real-cache-repo",
				"ENVBUILDER_DEVCONTAINER_DIR":       "path/to/.devcontainer",
				"ENVBUILDER_DEVCONTAINER_JSON_PATH": "path/to/.devcontainer/devcontainer.json",
				"ENVBUILDER_DOCKERFILE_PATH":        "path/to/.devcontainer/Dockerfile",
			},
			assertEnv: func(t *testing.T, deps testDependencies) resource.TestCheckFunc {
				return resource.ComposeAggregateTestCheckFunc(
					// Check that the environment variables are set correctly.
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.0", fmt.Sprintf("ENVBUILDER_CACHE_REPO=%s", deps.CacheRepo)),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.1", fmt.Sprintf("ENVBUILDER_DEVCONTAINER_DIR=%s", deps.ExtraEnv["ENVBUILDER_DEVCONTAINER_DIR"])),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.2", fmt.Sprintf("ENVBUILDER_DEVCONTAINER_JSON_PATH=%s", deps.ExtraEnv["ENVBUILDER_DEVCONTAINER_JSON_PATH"])),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.3", fmt.Sprintf("ENVBUILDER_DOCKERFILE_PATH=%s", deps.ExtraEnv["ENVBUILDER_DOCKERFILE_PATH"])),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.4", fmt.Sprintf("ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH=%s", deps.Repo.Key)),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.5", fmt.Sprintf("ENVBUILDER_GIT_URL=%s", deps.Repo.URL)),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.6", "ENVBUILDER_REMOTE_REPO_BUILD_MODE=true"),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.7", "ENVBUILDER_VERBOSE=true"),
					// Check that the extra environment variables are set correctly.
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.8", "FOO=bar\nbaz"),
					// We should not have any other environment variables set.
					resource.TestCheckNoResourceAttr("envbuilder_cached_image.test", "env.9"),

					// Check that the same values are set in env_map.
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_CACHE_REPO", deps.CacheRepo),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_DEVCONTAINER_DIR", deps.ExtraEnv["ENVBUILDER_DEVCONTAINER_DIR"]),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_DEVCONTAINER_JSON_PATH", deps.ExtraEnv["ENVBUILDER_DEVCONTAINER_JSON_PATH"]),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_DOCKERFILE_PATH", deps.ExtraEnv["ENVBUILDER_DOCKERFILE_PATH"]),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH", deps.Repo.Key),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_GIT_URL", deps.Repo.URL),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_REMOTE_REPO_BUILD_MODE", "true"),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_VERBOSE", "true"),
					resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.FOO", "bar\nbaz"),
				)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			//nolint: paralleltest
			deps := setup(ctx, t, tc.extraEnv, tc.files)

			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					// 1) Initial state: cache has not been seeded.
					{
						Config:             deps.Config(t),
						PlanOnly:           true,
						ExpectNonEmptyPlan: true,
					},
					// 2) Should detect that no cached image is present and plan to create the resource.
					{
						Config: deps.Config(t),
						Check: resource.ComposeAggregateTestCheckFunc(
							// Computed values MUST be present.
							resource.TestCheckResourceAttr("envbuilder_cached_image.test", "id", uuid.Nil.String()),
							resource.TestCheckResourceAttr("envbuilder_cached_image.test", "exists", "false"),
							// Cached image should be set to the builder image.
							resource.TestCheckResourceAttr("envbuilder_cached_image.test", "image", deps.BuilderImage),
							// Inputs should still be present.
							resource.TestCheckResourceAttr("envbuilder_cached_image.test", "cache_repo", deps.CacheRepo),
							resource.TestCheckResourceAttr("envbuilder_cached_image.test", "extra_env.FOO", "bar\nbaz"),
							resource.TestCheckResourceAttr("envbuilder_cached_image.test", "git_url", deps.Repo.URL),
							// Should be empty
							resource.TestCheckNoResourceAttr("envbuilder_cached_image.test", "git_username"),
							resource.TestCheckNoResourceAttr("envbuilder_cached_image.test", "git_password"),
							resource.TestCheckNoResourceAttr("envbuilder_cached_image.test", "cache_ttl_days"),
							// Environment variables
							tc.assertEnv(t, deps),
						),
						ExpectNonEmptyPlan: true, // TODO: check the plan.
					},
					// 3) Re-running plan should have the same effect.
					{
						Config: deps.Config(t),
						Check: resource.ComposeAggregateTestCheckFunc(
							// Computed values MUST be present.
							resource.TestCheckResourceAttr("envbuilder_cached_image.test", "id", uuid.Nil.String()),
							resource.TestCheckResourceAttr("envbuilder_cached_image.test", "exists", "false"),
							// Cached image should be set to the builder image.
							resource.TestCheckResourceAttr("envbuilder_cached_image.test", "image", deps.BuilderImage),
							// Inputs should still be present.
							resource.TestCheckResourceAttr("envbuilder_cached_image.test", "cache_repo", deps.CacheRepo),
							resource.TestCheckResourceAttr("envbuilder_cached_image.test", "extra_env.FOO", "bar\nbaz"),
							resource.TestCheckResourceAttr("envbuilder_cached_image.test", "git_url", deps.Repo.URL),
							// Should be empty
							resource.TestCheckNoResourceAttr("envbuilder_cached_image.test", "git_username"),
							resource.TestCheckNoResourceAttr("envbuilder_cached_image.test", "git_password"),
							resource.TestCheckNoResourceAttr("envbuilder_cached_image.test", "cache_ttl_days"),
							// Environment variables
							tc.assertEnv(t, deps),
						),
						ExpectNonEmptyPlan: true, // TODO: check the plan.
					},
					// 4) Now, seed the cache and re-run. We should now successfully create the cached image resource.
					{
						PreConfig: func() {
							seedCache(ctx, t, deps)
						},
						Config: deps.Config(t),
						Check: resource.ComposeAggregateTestCheckFunc(
							// Inputs should still be present.
							resource.TestCheckResourceAttr("envbuilder_cached_image.test", "cache_repo", deps.CacheRepo),
							resource.TestCheckResourceAttr("envbuilder_cached_image.test", "extra_env.FOO", "bar\nbaz"),
							resource.TestCheckResourceAttr("envbuilder_cached_image.test", "git_url", deps.Repo.URL),
							// Should be empty
							resource.TestCheckNoResourceAttr("envbuilder_cached_image.test", "git_username"),
							resource.TestCheckNoResourceAttr("envbuilder_cached_image.test", "git_password"),
							resource.TestCheckNoResourceAttr("envbuilder_cached_image.test", "cache_ttl_days"),
							// Computed
							resource.TestCheckResourceAttrWith("envbuilder_cached_image.test", "id", quotedPrefix("sha256:")),
							resource.TestCheckResourceAttr("envbuilder_cached_image.test", "exists", "true"),
							resource.TestCheckResourceAttrSet("envbuilder_cached_image.test", "image"),
							resource.TestCheckResourceAttrWith("envbuilder_cached_image.test", "image", quotedPrefix(deps.CacheRepo)),
							// Environment variables
							tc.assertEnv(t, deps),
						),
					},
					// 5) Should produce an empty plan after apply
					{
						Config:   deps.Config(t),
						PlanOnly: true,
					},
					// 6) Ensure idempotence in this state!
					{
						Config:   deps.Config(t),
						PlanOnly: true,
					},
				},
			})
		})
	}
}

// assertEnv is a test helper that checks the environment variables set on the
// cached image resource based on the provided test dependencies.
func assertEnv(t *testing.T, deps testDependencies) resource.TestCheckFunc {
	t.Helper()
	return resource.ComposeAggregateTestCheckFunc(
		// Check that the environment variables are set correctly.
		resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.0", fmt.Sprintf("ENVBUILDER_CACHE_REPO=%s", deps.CacheRepo)),
		resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.1", fmt.Sprintf("ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH=%s", deps.Repo.Key)),
		resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.2", fmt.Sprintf("ENVBUILDER_GIT_URL=%s", deps.Repo.URL)),
		resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.3", "ENVBUILDER_REMOTE_REPO_BUILD_MODE=true"),
		resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.4", "ENVBUILDER_VERBOSE=true"),
		// Check that the extra environment variables are set correctly.
		resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env.5", "FOO=bar\nbaz"),
		// We should not have any other environment variables set.
		resource.TestCheckNoResourceAttr("envbuilder_cached_image.test", "env.6"),

		// Check that the same values are set in env_map.
		resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_CACHE_REPO", deps.CacheRepo),
		resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH", deps.Repo.Key),
		resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_GIT_URL", deps.Repo.URL),
		resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_REMOTE_REPO_BUILD_MODE", "true"),
		resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.ENVBUILDER_VERBOSE", "true"),
		resource.TestCheckResourceAttr("envbuilder_cached_image.test", "env_map.FOO", "bar\nbaz"),
	)
}
