package provider

import (
	"fmt"
	"strings"

	eboptions "github.com/coder/envbuilder/options"
	"github.com/coder/serpent"
	"github.com/coder/terraform-provider-envbuilder/internal/tfutil"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/spf13/pflag"
)

// optionsFromDataModel converts a CachedImageResourceModel into a corresponding set of
// Envbuilder options. It returns the options and any diagnostics encountered.
func optionsFromDataModel(data CachedImageResourceModel) (eboptions.Options, diag.Diagnostics) {
	var diags diag.Diagnostics
	var opts eboptions.Options

	// Required options. Cannot be overridden by extra_env.
	opts.CacheRepo = data.CacheRepo.ValueString()
	opts.GitURL = data.GitURL.ValueString()

	// Other options can be overridden by extra_env, with a warning.
	// Keep track of which options are overridden.
	overrides := make(map[string]struct{})

	if !data.BaseImageCacheDir.IsNull() {
		overrides["ENVBUILDER_BASE_IMAGE_CACHE_DIR"] = struct{}{}
		opts.BaseImageCacheDir = data.BaseImageCacheDir.ValueString()
	}

	if !data.BuildContextPath.IsNull() {
		overrides["ENVBUILDER_BUILD_CONTEXT_PATH"] = struct{}{}
		opts.BuildContextPath = data.BuildContextPath.ValueString()
	}

	if !data.CacheTTLDays.IsNull() {
		overrides["ENVBUILDER_CACHE_TTL_DAYS"] = struct{}{}
		opts.CacheTTLDays = data.CacheTTLDays.ValueInt64()
	}

	if !data.DevcontainerDir.IsNull() {
		overrides["ENVBUILDER_DEVCONTAINER_DIR"] = struct{}{}
		opts.DevcontainerDir = data.DevcontainerDir.ValueString()
	}

	if !data.DevcontainerJSONPath.IsNull() {
		overrides["ENVBUILDER_DEVCONTAINER_JSON_PATH"] = struct{}{}
		opts.DevcontainerJSONPath = data.DevcontainerJSONPath.ValueString()
	}

	if !data.DockerfilePath.IsNull() {
		overrides["ENVBUILDER_DOCKERFILE_PATH"] = struct{}{}
		opts.DockerfilePath = data.DockerfilePath.ValueString()
	}

	if !data.DockerConfigBase64.IsNull() {
		overrides["ENVBUILDER_DOCKER_CONFIG_BASE64"] = struct{}{}
		opts.DockerConfigBase64 = data.DockerConfigBase64.ValueString()
	}

	if !data.ExitOnBuildFailure.IsNull() {
		overrides["ENVBUILDER_EXIT_ON_BUILD_FAILURE"] = struct{}{}
		opts.ExitOnBuildFailure = data.ExitOnBuildFailure.ValueBool()
	}

	if !data.FallbackImage.IsNull() {
		overrides["ENVBUILDER_FALLBACK_IMAGE"] = struct{}{}
		opts.FallbackImage = data.FallbackImage.ValueString()
	}

	if !data.GitCloneDepth.IsNull() {
		overrides["ENVBUILDER_GIT_CLONE_DEPTH"] = struct{}{}
		opts.GitCloneDepth = data.GitCloneDepth.ValueInt64()
	}

	if !data.GitCloneSingleBranch.IsNull() {
		overrides["ENVBUILDER_GIT_CLONE_SINGLE_BRANCH"] = struct{}{}
		opts.GitCloneSingleBranch = data.GitCloneSingleBranch.ValueBool()
	}

	if !data.GitHTTPProxyURL.IsNull() {
		overrides["ENVBUILDER_GIT_HTTP_PROXY_URL"] = struct{}{}
		opts.GitHTTPProxyURL = data.GitHTTPProxyURL.ValueString()
	}

	if !data.GitSSHPrivateKeyPath.IsNull() {
		overrides["ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH"] = struct{}{}
		opts.GitSSHPrivateKeyPath = data.GitSSHPrivateKeyPath.ValueString()
	}

	if !data.GitUsername.IsNull() {
		overrides["ENVBUILDER_GIT_USERNAME"] = struct{}{}
		opts.GitUsername = data.GitUsername.ValueString()
	}

	if !data.GitPassword.IsNull() {
		overrides["ENVBUILDER_GIT_PASSWORD"] = struct{}{}
		opts.GitPassword = data.GitPassword.ValueString()
	}

	if !data.IgnorePaths.IsNull() {
		overrides["ENVBUILDER_IGNORE_PATHS"] = struct{}{}
		opts.IgnorePaths = tfutil.TFListToStringSlice(data.IgnorePaths)
	}

	if !data.Insecure.IsNull() {
		overrides["ENVBUILDER_INSECURE"] = struct{}{}
		opts.Insecure = data.Insecure.ValueBool()
	}

	if data.RemoteRepoBuildMode.IsNull() {
		opts.RemoteRepoBuildMode = true
	} else {
		overrides["ENVBUILDER_REMOTE_REPO_BUILD_MODE"] = struct{}{}
		opts.RemoteRepoBuildMode = data.RemoteRepoBuildMode.ValueBool()
	}

	if !data.SSLCertBase64.IsNull() {
		overrides["ENVBUILDER_SSL_CERT_BASE64"] = struct{}{}
		opts.SSLCertBase64 = data.SSLCertBase64.ValueString()
	}

	if !data.Verbose.IsNull() {
		overrides["ENVBUILDER_VERBOSE"] = struct{}{}
		opts.Verbose = data.Verbose.ValueBool()
	}

	if !data.WorkspaceFolder.IsNull() {
		overrides["ENVBUILDER_WORKSPACE_FOLDER"] = struct{}{}
		opts.WorkspaceFolder = data.WorkspaceFolder.ValueString()
	}

	// convert extraEnv to a map for ease of use.
	extraEnv := make(map[string]string)
	for k, v := range data.ExtraEnv.Elements() {
		extraEnv[k] = tfutil.TFValueToString(v)
	}
	diags = append(diags, overrideOptionsFromExtraEnv(&opts, extraEnv, overrides)...)

	return opts, diags
}

// overrideOptionsFromExtraEnv overrides the options in opts with values from extraEnv.
// It returns any diagnostics encountered.
// It will not override certain options, such as ENVBUILDER_CACHE_REPO and ENVBUILDER_GIT_URL.
func overrideOptionsFromExtraEnv(opts *eboptions.Options, extraEnv map[string]string, overrides map[string]struct{}) diag.Diagnostics {
	var diags diag.Diagnostics
	// Make a map of the options for easy lookup.
	optsMap := make(map[string]pflag.Value)
	for _, opt := range opts.CLI() {
		optsMap[opt.Env] = opt.Value
	}
	for key, val := range extraEnv {
		switch key {

		// These options may not be overridden.
		case "ENVBUILDER_CACHE_REPO", "ENVBUILDER_GIT_URL":
			diags.AddAttributeWarning(path.Root("extra_env"),
				"Cannot override required environment variable",
				fmt.Sprintf("The key %q in extra_env cannot be overridden.", key),
			)
			continue

		default:
			// Check if the option was set on the provider data model and generate a warning if so.
			if _, overridden := overrides[key]; overridden {
				diags.AddAttributeWarning(path.Root("extra_env"),
					"Overriding provider environment variable",
					fmt.Sprintf("The key %q in extra_env overrides an option set on the provider.", key),
				)
			}

			// XXX: workaround for serpent behaviour where calling Set() on a
			// string slice will append instead of replace: set to empty first.
			if key == "ENVBUILDER_IGNORE_PATHS" {
				_ = optsMap[key].Set("")
			}

			opt, found := optsMap[key]
			if !found {
				// ignore unknown keys
				continue
			}

			if err := opt.Set(val); err != nil {
				diags.AddAttributeError(path.Root("extra_env"),
					"Invalid value for environment variable",
					fmt.Sprintf("The key %q in extra_env has an invalid value: %s", key, err),
				)
			}
		}
	}
	return diags
}

// computeEnvFromOptions computes the environment variables to set based on the
// options in opts and the extra environment variables in extraEnv.
// It returns the computed environment variables as a map.
// It will not set certain options, such as ENVBUILDER_CACHE_REPO and ENVBUILDER_GIT_URL.
// It will also not handle legacy Envbuilder options (i.e. those not prefixed with ENVBUILDER_).
func computeEnvFromOptions(opts eboptions.Options, extraEnv map[string]string) map[string]string {
	allEnvKeys := make(map[string]struct{})
	for _, opt := range opts.CLI() {
		if opt.Env == "" {
			continue
		}
		allEnvKeys[opt.Env] = struct{}{}
	}

	// Only set the environment variables from opts that are not legacy options.
	// Legacy options are those that are not prefixed with ENVBUILDER_.
	// While we can detect when a legacy option is set, overriding it becomes
	// problematic. Erring on the side of caution, we will not override legacy options.
	isEnvbuilderOption := func(key string) bool {
		switch key {
		case "CODER_AGENT_URL", "CODER_AGENT_TOKEN", "CODER_AGENT_SUBSYSTEM":
			return true // kinda
		default:
			return strings.HasPrefix(key, "ENVBUILDER_")
		}
	}

	computed := make(map[string]string)
	for _, opt := range opts.CLI() {
		if opt.Env == "" {
			continue
		}
		// TODO: remove this check once support for legacy options is removed.
		if !isEnvbuilderOption(opt.Env) {
			continue
		}
		var val string
		if sa, ok := opt.Value.(*serpent.StringArray); ok {
			val = strings.Join(sa.GetSlice(), ",")
		} else {
			val = opt.Value.String()
		}

		switch val {
		case "", "false", "0":
			// Skip zero values.
			continue
		}
		computed[opt.Env] = val
	}

	// Merge in extraEnv, which may override values from opts.
	// Skip any keys that are envbuilder options.
	for key, val := range extraEnv {
		if isEnvbuilderOption(key) {
			continue
		}
		computed[key] = val
	}
	return computed
}
