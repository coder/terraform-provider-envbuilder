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
				"CODER_AGENT_TOKEN":     "some-token",
				"CODER_AGENT_URL":       "https://coder.example.com",
				"ENVBUILDER_GIT_URL":    "https://not.the.real.git/url",
				"ENVBUILDER_CACHE_REPO": "not-the-real-cache-repo",
				"FOO":                   testEnvValue,
			},
			assertEnv: func(t *testing.T, deps testDependencies) resource.TestCheckFunc {
				return resource.ComposeAggregateTestCheckFunc(
					assertEnv(t,
						"CODER_AGENT_TOKEN", "some-token",
						"CODER_AGENT_URL", "https://coder.example.com",
						"ENVBUILDER_CACHE_REPO", deps.CacheRepo,
						"ENVBUILDER_DOCKER_CONFIG_BASE64", deps.DockerConfigBase64,
						"ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH", deps.Repo.Key,
						"ENVBUILDER_GIT_URL", deps.Repo.URL,
						"ENVBUILDER_REMOTE_REPO_BUILD_MODE", "true",
						"ENVBUILDER_VERBOSE", "true",
						"FOO", "bar\nbaz",
					),
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
				"CODER_AGENT_TOKEN":     "some-token",
				"CODER_AGENT_URL":       "https://coder.example.com",
				"FOO":                   testEnvValue,
				"ENVBUILDER_GIT_URL":    "https://not.the.real.git/url",
				"ENVBUILDER_CACHE_REPO": "not-the-real-cache-repo",
			},
			assertEnv: func(t *testing.T, deps testDependencies) resource.TestCheckFunc {
				return resource.ComposeAggregateTestCheckFunc(
					assertEnv(t,
						"CODER_AGENT_TOKEN", "some-token",
						"CODER_AGENT_URL", "https://coder.example.com",
						"ENVBUILDER_CACHE_REPO", deps.CacheRepo,
						"ENVBUILDER_DOCKER_CONFIG_BASE64", deps.DockerConfigBase64,
						"ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH", deps.Repo.Key,
						"ENVBUILDER_GIT_URL", deps.Repo.URL,
						"ENVBUILDER_REMOTE_REPO_BUILD_MODE", "true",
						"ENVBUILDER_VERBOSE", "true",
						"FOO", "bar\nbaz",
					),
				)
			},
		},
		{
			// This test case ensures that overriding the devcontainer directory works.
			name: "different_dir",
			files: map[string]string{
				"path/to/.devcontainer/devcontainer.json": `{"build": { "dockerfile": "Dockerfile" }}`,
				"path/to/.devcontainer/Dockerfile": `FROM localhost:5000/test-ubuntu:latest
		RUN date > /date.txt`,
			},
			extraEnv: map[string]string{
				"CODER_AGENT_TOKEN":                 "some-token",
				"CODER_AGENT_URL":                   "https://coder.example.com",
				"FOO":                               testEnvValue,
				"ENVBUILDER_GIT_URL":                "https://not.the.real.git/url",
				"ENVBUILDER_CACHE_REPO":             "not-the-real-cache-repo",
				"ENVBUILDER_DEVCONTAINER_DIR":       "path/to/.devcontainer",
				"ENVBUILDER_DEVCONTAINER_JSON_PATH": "path/to/.devcontainer/devcontainer.json",
				"ENVBUILDER_DOCKERFILE_PATH":        "path/to/.devcontainer/Dockerfile",
			},
			assertEnv: func(t *testing.T, deps testDependencies) resource.TestCheckFunc {
				return resource.ComposeAggregateTestCheckFunc(
					assertEnv(t,
						"CODER_AGENT_TOKEN", "some-token",
						"CODER_AGENT_URL", "https://coder.example.com",
						"ENVBUILDER_CACHE_REPO", deps.CacheRepo,
						"ENVBUILDER_DEVCONTAINER_DIR", "path/to/.devcontainer",
						"ENVBUILDER_DEVCONTAINER_JSON_PATH", "path/to/.devcontainer/devcontainer.json",
						"ENVBUILDER_DOCKERFILE_PATH", "path/to/.devcontainer/Dockerfile",
						"ENVBUILDER_DOCKER_CONFIG_BASE64", deps.DockerConfigBase64,
						"ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH", deps.Repo.Key,
						"ENVBUILDER_GIT_URL", deps.Repo.URL,
						"ENVBUILDER_REMOTE_REPO_BUILD_MODE", "true",
						"ENVBUILDER_VERBOSE", "true",
						"FOO", "bar\nbaz",
					),
				)
			},
		},
		{
			// This tests that a multi-stage build works correctly.
			name: "multistage_run_copy",
			files: map[string]string{
				"Dockerfile": `
		FROM localhost:5000/test-ubuntu:latest AS a
		RUN date > /date.txt
		FROM localhost:5000/test-ubuntu:latest
		COPY --from=a /date.txt /date.txt`,
			},
			extraEnv: map[string]string{
				"CODER_AGENT_TOKEN":          "some-token",
				"CODER_AGENT_URL":            "https://coder.example.com",
				"FOO":                        testEnvValue,
				"ENVBUILDER_GIT_URL":         "https://not.the.real.git/url",
				"ENVBUILDER_CACHE_REPO":      "not-the-real-cache-repo",
				"ENVBUILDER_DOCKERFILE_PATH": "Dockerfile",
			},
			assertEnv: func(t *testing.T, deps testDependencies) resource.TestCheckFunc {
				return resource.ComposeAggregateTestCheckFunc(
					assertEnv(t,
						"CODER_AGENT_TOKEN", "some-token",
						"CODER_AGENT_URL", "https://coder.example.com",
						"ENVBUILDER_CACHE_REPO", deps.CacheRepo,
						"ENVBUILDER_DOCKERFILE_PATH", "Dockerfile",
						"ENVBUILDER_DOCKER_CONFIG_BASE64", deps.DockerConfigBase64,
						"ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH", deps.Repo.Key,
						"ENVBUILDER_GIT_URL", deps.Repo.URL,
						"ENVBUILDER_REMOTE_REPO_BUILD_MODE", "true",
						"ENVBUILDER_VERBOSE", "true",
						"FOO", "bar\nbaz",
					),
				)
			},
		},
		{
			// This tests correct handling of the difference in permissions between
			// the provider and the image when running a COPY instruction.
			// Added to verify fix for coder/terraform-provider-envbuilder#43
			name: "copy_perms",
			files: map[string]string{
				"Dockerfile": `
		FROM localhost:5000/test-ubuntu:latest AS a
		COPY date.txt /date.txt
		FROM localhost:5000/test-ubuntu:latest
		COPY --from=a /date.txt /date.txt`,
				"date.txt": fmt.Sprintf("%d", time.Now().Unix()),
			},
			extraEnv: map[string]string{
				"CODER_AGENT_TOKEN":          "some-token",
				"CODER_AGENT_URL":            "https://coder.example.com",
				"FOO":                        testEnvValue,
				"ENVBUILDER_GIT_URL":         "https://not.the.real.git/url",
				"ENVBUILDER_CACHE_REPO":      "not-the-real-cache-repo",
				"ENVBUILDER_DOCKERFILE_PATH": "Dockerfile",
			},
			assertEnv: func(t *testing.T, deps testDependencies) resource.TestCheckFunc {
				return resource.ComposeAggregateTestCheckFunc(
					assertEnv(t,
						"CODER_AGENT_TOKEN", "some-token",
						"CODER_AGENT_URL", "https://coder.example.com",
						"ENVBUILDER_CACHE_REPO", deps.CacheRepo,
						"ENVBUILDER_DOCKERFILE_PATH", "Dockerfile",
						"ENVBUILDER_DOCKER_CONFIG_BASE64", deps.DockerConfigBase64,
						"ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH", deps.Repo.Key,
						"ENVBUILDER_GIT_URL", deps.Repo.URL,
						"ENVBUILDER_REMOTE_REPO_BUILD_MODE", "true",
						"ENVBUILDER_VERBOSE", "true",
						"FOO", "bar\nbaz",
					),
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

// assertEnv is a test helper that checks the environment variables, in order,
// on both the env and env_map attributes of the cached image resource.
func assertEnv(t *testing.T, kvs ...string) resource.TestCheckFunc {
	t.Helper()
	if len(kvs)%2 != 0 {
		t.Fatalf("assertEnv: expected an even number of key-value pairs, got %d", len(kvs))
	}

	funcs := make([]resource.TestCheckFunc, 0)
	for i := 0; i < len(kvs); i += 2 {
		resKey := fmt.Sprintf("env.%d", len(funcs))
		resVal := fmt.Sprintf("%s=%s", kvs[i], kvs[i+1])
		fn := resource.TestCheckResourceAttr("envbuilder_cached_image.test", resKey, resVal)
		funcs = append(funcs, fn)
	}

	lastKey := fmt.Sprintf("env.%d", len(funcs))
	lastFn := resource.TestCheckNoResourceAttr("envbuilder_cached_image.test", lastKey)
	funcs = append(funcs, lastFn)

	for i := 0; i < len(kvs); i += 2 {
		resKey := fmt.Sprintf("env_map.%s", kvs[i])
		fn := resource.TestCheckResourceAttr("envbuilder_cached_image.test", resKey, kvs[i+1])
		funcs = append(funcs, fn)
	}

	return resource.ComposeAggregateTestCheckFunc(funcs...)
}
