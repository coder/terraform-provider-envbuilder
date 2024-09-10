package provider

import (
	"testing"

	eboptions "github.com/coder/envbuilder/options"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/assert"
)

func Test_optionsFromDataModel(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                  string
		data                  CachedImageResourceModel
		expectOpts            eboptions.Options
		expectNumErrorDiags   int
		expectNumWarningDiags int
	}{
		{
			name: "required only",
			data: CachedImageResourceModel{
				BuilderImage: basetypes.NewStringValue("envbuilder:latest"),
				CacheRepo:    basetypes.NewStringValue("localhost:5000/cache"),
				GitURL:       basetypes.NewStringValue("git@git.local/devcontainer.git"),
			},
			expectOpts: eboptions.Options{
				CacheRepo:           "localhost:5000/cache",
				GitURL:              "git@git.local/devcontainer.git",
				RemoteRepoBuildMode: true,
			},
		},
		{
			name: "all options without extra_env",
			data: CachedImageResourceModel{
				BuilderImage:         basetypes.NewStringValue("envbuilder:latest"),
				CacheRepo:            basetypes.NewStringValue("localhost:5000/cache"),
				GitURL:               basetypes.NewStringValue("git@git.local/devcontainer.git"),
				BaseImageCacheDir:    basetypes.NewStringValue("/tmp/cache"),
				BuildContextPath:     basetypes.NewStringValue("."),
				CacheTTLDays:         basetypes.NewInt64Value(7),
				DevcontainerDir:      basetypes.NewStringValue(".devcontainer"),
				DevcontainerJSONPath: basetypes.NewStringValue(".devcontainer/devcontainer.json"),
				DockerfilePath:       basetypes.NewStringValue("Dockerfile"),
				DockerConfigBase64:   basetypes.NewStringValue("some base64"),
				ExitOnBuildFailure:   basetypes.NewBoolValue(true),
				// ExtraEnv: map[string]basetypes.Value{},
				FallbackImage:        basetypes.NewStringValue("fallback"),
				GitCloneDepth:        basetypes.NewInt64Value(1),
				GitCloneSingleBranch: basetypes.NewBoolValue(true),
				GitHTTPProxyURL:      basetypes.NewStringValue("http://proxy"),
				GitPassword:          basetypes.NewStringValue("password"),
				GitSSHPrivateKeyPath: basetypes.NewStringValue("/tmp/id_rsa"),
				GitUsername:          basetypes.NewStringValue("user"),
				IgnorePaths:          listValue("ignore", "paths"),
				Insecure:             basetypes.NewBoolValue(true),
				RemoteRepoBuildMode:  basetypes.NewBoolValue(false),
				SSLCertBase64:        basetypes.NewStringValue("cert"),
				Verbose:              basetypes.NewBoolValue(true),
				WorkspaceFolder:      basetypes.NewStringValue("workspace"),
			},
			expectOpts: eboptions.Options{
				CacheRepo:            "localhost:5000/cache",
				GitURL:               "git@git.local/devcontainer.git",
				BaseImageCacheDir:    "/tmp/cache",
				BuildContextPath:     ".",
				CacheTTLDays:         7,
				DevcontainerDir:      ".devcontainer",
				DevcontainerJSONPath: ".devcontainer/devcontainer.json",
				DockerfilePath:       "Dockerfile",
				DockerConfigBase64:   "some base64",
				ExitOnBuildFailure:   true,
				FallbackImage:        "fallback",
				GitCloneDepth:        1,
				GitCloneSingleBranch: true,
				GitHTTPProxyURL:      "http://proxy",
				GitPassword:          "password",
				GitSSHPrivateKeyPath: "/tmp/id_rsa",
				GitUsername:          "user",
				IgnorePaths:          []string{"ignore", "paths"},
				Insecure:             true,
				RemoteRepoBuildMode:  false,
				SSLCertBase64:        "cert",
				Verbose:              true,
				WorkspaceFolder:      "workspace",
			},
		},
		{
			name: "extra env override",
			data: CachedImageResourceModel{
				BuilderImage: basetypes.NewStringValue("envbuilder:latest"),
				CacheRepo:    basetypes.NewStringValue("localhost:5000/cache"),
				GitURL:       basetypes.NewStringValue("git@git.local/devcontainer.git"),
				ExtraEnv: extraEnvMap(t,
					"CODER_AGENT_TOKEN", "token",
					"CODER_AGENT_URL", "http://coder",
					"FOO", "bar",
				),
			},
			expectOpts: eboptions.Options{
				CacheRepo:           "localhost:5000/cache",
				GitURL:              "git@git.local/devcontainer.git",
				RemoteRepoBuildMode: true,
				CoderAgentToken:     "token",
				CoderAgentURL:       "http://coder",
			},
		},
		{
			name: "extra_env override warnings",
			data: CachedImageResourceModel{
				BuilderImage:         basetypes.NewStringValue("envbuilder:latest"),
				CacheRepo:            basetypes.NewStringValue("localhost:5000/cache"),
				GitURL:               basetypes.NewStringValue("git@git.local/devcontainer.git"),
				BaseImageCacheDir:    basetypes.NewStringValue("/tmp/cache"),
				BuildContextPath:     basetypes.NewStringValue("."),
				CacheTTLDays:         basetypes.NewInt64Value(7),
				DevcontainerDir:      basetypes.NewStringValue(".devcontainer"),
				DevcontainerJSONPath: basetypes.NewStringValue(".devcontainer/devcontainer.json"),
				DockerfilePath:       basetypes.NewStringValue("Dockerfile"),
				DockerConfigBase64:   basetypes.NewStringValue("some base64"),
				ExitOnBuildFailure:   basetypes.NewBoolValue(true),
				// ExtraEnv: map[string]basetypes.Value{},
				FallbackImage:        basetypes.NewStringValue("fallback"),
				GitCloneDepth:        basetypes.NewInt64Value(1),
				GitCloneSingleBranch: basetypes.NewBoolValue(true),
				GitHTTPProxyURL:      basetypes.NewStringValue("http://proxy"),
				GitPassword:          basetypes.NewStringValue("password"),
				GitSSHPrivateKeyPath: basetypes.NewStringValue("/tmp/id_rsa"),
				GitUsername:          basetypes.NewStringValue("user"),
				IgnorePaths:          listValue("ignore", "paths"),
				Insecure:             basetypes.NewBoolValue(true),
				RemoteRepoBuildMode:  basetypes.NewBoolValue(false),
				SSLCertBase64:        basetypes.NewStringValue("cert"),
				Verbose:              basetypes.NewBoolValue(true),
				WorkspaceFolder:      basetypes.NewStringValue("workspace"),
				ExtraEnv: extraEnvMap(t,
					"ENVBUILDER_CACHE_REPO", "override",
					"ENVBUILDER_GIT_URL", "override",
					"ENVBUILDER_BASE_IMAGE_CACHE_DIR", "override",
					"ENVBUILDER_BUILD_CONTEXT_PATH", "override",
					"ENVBUILDER_CACHE_TTL_DAYS", "8",
					"ENVBUILDER_DEVCONTAINER_DIR", "override",
					"ENVBUILDER_DEVCONTAINER_JSON_PATH", "override",
					"ENVBUILDER_DOCKERFILE_PATH", "override",
					"ENVBUILDER_DOCKER_CONFIG_BASE64", "override",
					"ENVBUILDER_EXIT_ON_BUILD_FAILURE", "false",
					"ENVBUILDER_FALLBACK_IMAGE", "override",
					"ENVBUILDER_GIT_CLONE_DEPTH", "2",
					"ENVBUILDER_GIT_CLONE_SINGLE_BRANCH", "false",
					"ENVBUILDER_GIT_HTTP_PROXY_URL", "override",
					"ENVBUILDER_GIT_PASSWORD", "override",
					"ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH", "override",
					"ENVBUILDER_GIT_USERNAME", "override",
					"ENVBUILDER_IGNORE_PATHS", "override",
					"ENVBUILDER_INSECURE", "false",
					"ENVBUILDER_REMOTE_REPO_BUILD_MODE", "true",
					"ENVBUILDER_SSL_CERT_BASE64", "override",
					"ENVBUILDER_VERBOSE", "false",
					"ENVBUILDER_WORKSPACE_FOLDER", "override",
					"FOO", "bar",
				),
			},
			expectOpts: eboptions.Options{
				// not overridden
				CacheRepo: "localhost:5000/cache",
				GitURL:    "git@git.local/devcontainer.git",
				// overridden
				BaseImageCacheDir:    "override",
				BuildContextPath:     "override",
				CacheTTLDays:         8,
				DevcontainerDir:      "override",
				DevcontainerJSONPath: "override",
				DockerfilePath:       "override",
				DockerConfigBase64:   "override",
				ExitOnBuildFailure:   false,
				FallbackImage:        "override",
				GitCloneDepth:        2,
				GitCloneSingleBranch: false,
				GitHTTPProxyURL:      "override",
				GitPassword:          "override",
				GitSSHPrivateKeyPath: "override",
				GitUsername:          "override",
				IgnorePaths:          []string{"override"},
				Insecure:             false,
				RemoteRepoBuildMode:  true,
				SSLCertBase64:        "override",
				Verbose:              false,
				WorkspaceFolder:      "override",
			},
			expectNumWarningDiags: 23,
		},
		{
			name: "extra_env override errors",
			data: CachedImageResourceModel{
				BuilderImage: basetypes.NewStringValue("envbuilder:latest"),
				CacheRepo:    basetypes.NewStringValue("localhost:5000/cache"),
				GitURL:       basetypes.NewStringValue("git@git.local/devcontainer.git"),
				ExtraEnv: extraEnvMap(t,
					"ENVBUILDER_CACHE_TTL_DAYS", "not a number",
					"ENVBUILDER_VERBOSE", "not a bool",
					"FOO", "bar",
				),
			},
			expectOpts: eboptions.Options{
				// not overridden
				CacheRepo:           "localhost:5000/cache",
				GitURL:              "git@git.local/devcontainer.git",
				RemoteRepoBuildMode: true,
			},
			expectNumErrorDiags: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actual, diags := optionsFromDataModel(tc.data)
			assert.Equal(t, tc.expectNumErrorDiags, diags.ErrorsCount())
			assert.Equal(t, tc.expectNumWarningDiags, diags.WarningsCount())
			assert.EqualValues(t, tc.expectOpts, actual)
		})
	}
}

func Test_computeEnvFromOptions(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		opts      eboptions.Options
		extraEnv  map[string]string
		expectEnv map[string]string
	}{
		{
			name:      "empty",
			opts:      eboptions.Options{},
			expectEnv: map[string]string{},
		},
		{
			name: "all options",
			opts: eboptions.Options{
				BaseImageCacheDir:    "string",
				BinaryPath:           "string",
				BuildContextPath:     "string",
				CacheRepo:            "string",
				CacheTTLDays:         1,
				CoderAgentSubsystem:  []string{"one", "two"},
				CoderAgentToken:      "string",
				CoderAgentURL:        "string",
				DevcontainerDir:      "string",
				DevcontainerJSONPath: "string",
				DockerConfigBase64:   "string",
				DockerfilePath:       "string",
				ExitOnBuildFailure:   true,
				ExportEnvFile:        "string",
				FallbackImage:        "string",
				ForceSafe:            true,
				GetCachedImage:       true,
				GitCloneDepth:        1,
				GitCloneSingleBranch: true,
				GitHTTPProxyURL:      "string",
				GitPassword:          "string",
				GitSSHPrivateKeyPath: "string",
				GitURL:               "string",
				GitUsername:          "string",
				IgnorePaths:          []string{"one", "two"},
				InitArgs:             "string",
				InitCommand:          "string",
				InitScript:           "string",
				Insecure:             true,
				LayerCacheDir:        "string",
				PostStartScriptPath:  "string",
				PushImage:            true,
				RemoteRepoBuildMode:  true,
				SetupScript:          "string",
				SkipRebuild:          true,
				SSLCertBase64:        "string",
				Verbose:              true,
				WorkspaceFolder:      "string",
			},
			extraEnv: map[string]string{
				"ENVBUILDER_SOMETHING": "string", // should be ignored
				"FOO":                  "bar",    // should be included
			},
			expectEnv: map[string]string{
				"ENVBUILDER_BASE_IMAGE_CACHE_DIR":     "string",
				"ENVBUILDER_BINARY_PATH":              "string",
				"ENVBUILDER_BUILD_CONTEXT_PATH":       "string",
				"ENVBUILDER_CACHE_REPO":               "string",
				"ENVBUILDER_CACHE_TTL_DAYS":           "1",
				"ENVBUILDER_DEVCONTAINER_DIR":         "string",
				"ENVBUILDER_DEVCONTAINER_JSON_PATH":   "string",
				"ENVBUILDER_DOCKER_CONFIG_BASE64":     "string",
				"ENVBUILDER_DOCKERFILE_PATH":          "string",
				"ENVBUILDER_EXIT_ON_BUILD_FAILURE":    "true",
				"ENVBUILDER_EXPORT_ENV_FILE":          "string",
				"ENVBUILDER_FALLBACK_IMAGE":           "string",
				"ENVBUILDER_FORCE_SAFE":               "true",
				"ENVBUILDER_GET_CACHED_IMAGE":         "true",
				"ENVBUILDER_GIT_CLONE_DEPTH":          "1",
				"ENVBUILDER_GIT_CLONE_SINGLE_BRANCH":  "true",
				"ENVBUILDER_GIT_HTTP_PROXY_URL":       "string",
				"ENVBUILDER_GIT_PASSWORD":             "string",
				"ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH": "string",
				"ENVBUILDER_GIT_URL":                  "string",
				"ENVBUILDER_GIT_USERNAME":             "string",
				"ENVBUILDER_IGNORE_PATHS":             "one,two",
				"ENVBUILDER_INIT_ARGS":                "string",
				"ENVBUILDER_INIT_COMMAND":             "string",
				"ENVBUILDER_INIT_SCRIPT":              "string",
				"ENVBUILDER_INSECURE":                 "true",
				"ENVBUILDER_LAYER_CACHE_DIR":          "string",
				"ENVBUILDER_POST_START_SCRIPT_PATH":   "string",
				"ENVBUILDER_PUSH_IMAGE":               "true",
				"ENVBUILDER_REMOTE_REPO_BUILD_MODE":   "true",
				"ENVBUILDER_SETUP_SCRIPT":             "string",
				"ENVBUILDER_SKIP_REBUILD":             "true",
				"ENVBUILDER_SSL_CERT_BASE64":          "string",
				"ENVBUILDER_VERBOSE":                  "true",
				"ENVBUILDER_WORKSPACE_FOLDER":         "string",
				"FOO":                                 "bar",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.extraEnv == nil {
				tc.extraEnv = map[string]string{}
			}
			actual := computeEnvFromOptions(tc.opts, tc.extraEnv)
			assert.EqualValues(t, tc.expectEnv, actual)
		})
	}
}

func listValue(vs ...string) basetypes.ListValue {
	vals := make([]attr.Value, len(vs))
	for i, s := range vs {
		vals[i] = basetypes.NewStringValue(s)
	}
	return basetypes.NewListValueMust(basetypes.StringType{}, vals)
}

func extraEnvMap(t *testing.T, kvs ...string) basetypes.MapValue {
	t.Helper()
	if len(kvs)%2 != 0 {
		t.Fatalf("extraEnvMap: expected even number of key-value pairs, got %d", len(kvs))
	}
	vals := make(map[string]attr.Value)
	for i := 0; i < len(kvs); i += 2 {
		vals[kvs[i]] = basetypes.NewStringValue(kvs[i+1])
	}
	return basetypes.NewMapValueMust(basetypes.StringType{}, vals)
}
