package provider

import (
	"fmt"
	"slices"
	"strings"

	eboptions "github.com/coder/envbuilder/options"
	"github.com/coder/serpent"
	"github.com/coder/terraform-provider-envbuilder/internal/tfutil"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/spf13/pflag"
)

const (
	envbuilderOptionPrefix = "ENVBUILDER_"
)

// nonOverrideOptions are options that cannot be overridden by extra_env.
var nonOverrideOptions = map[string]bool{
	"ENVBUILDER_CACHE_REPO": true,
	"ENVBUILDER_GIT_URL":    true,
}

// optionsFromDataModel converts a CachedImageResourceModel into a corresponding set of
// Envbuilder options. It returns the options and any diagnostics encountered.
func optionsFromDataModel(data CachedImageResourceModel) (eboptions.Options, diag.Diagnostics) {
	var diags diag.Diagnostics
	var opts eboptions.Options

	// Required options. Cannot be overridden by extra_env.
	opts.CacheRepo = data.CacheRepo.ValueString()
	opts.GitURL = data.GitURL.ValueString()

	// Other options can be overridden by extra_env, with a warning.
	// Keep track of which options are set from the data model so we
	// can check if they are being overridden.
	providerOpts := make(map[string]bool)

	if !data.BaseImageCacheDir.IsNull() {
		providerOpts["ENVBUILDER_BASE_IMAGE_CACHE_DIR"] = true
		opts.BaseImageCacheDir = data.BaseImageCacheDir.ValueString()
	}

	if !data.BuildContextPath.IsNull() {
		providerOpts["ENVBUILDER_BUILD_CONTEXT_PATH"] = true
		opts.BuildContextPath = data.BuildContextPath.ValueString()
	}

	if !data.BuildSecrets.IsNull() {
		providerOpts["ENVBUILDER_BUILD_SECRETS"] = true

		// Depending on use case, users might want to provide build secrets as a map or a list of strings.
		// The string list option is supported by extra_env, so we support the map option here. Envbuilder
		// expects a list of strings, so we convert the map to a list of strings here.
		buildSecretMap := tfutil.TFMapToStringMap(data.BuildSecrets)
		buildSecretSlice := make([]string, 0, len(buildSecretMap))
		for k, v := range buildSecretMap {
			buildSecretSlice = append(buildSecretSlice, fmt.Sprintf("%s=%s", k, v))
		}
		slices.Sort(buildSecretSlice)

		opts.BuildSecrets = buildSecretSlice
	}

	if !data.CacheTTLDays.IsNull() {
		providerOpts["ENVBUILDER_CACHE_TTL_DAYS"] = true
		opts.CacheTTLDays = data.CacheTTLDays.ValueInt64()
	}

	if !data.DevcontainerDir.IsNull() {
		providerOpts["ENVBUILDER_DEVCONTAINER_DIR"] = true
		opts.DevcontainerDir = data.DevcontainerDir.ValueString()
	}

	if !data.DevcontainerJSONPath.IsNull() {
		providerOpts["ENVBUILDER_DEVCONTAINER_JSON_PATH"] = true
		opts.DevcontainerJSONPath = data.DevcontainerJSONPath.ValueString()
	}

	if !data.DockerfilePath.IsNull() {
		providerOpts["ENVBUILDER_DOCKERFILE_PATH"] = true
		opts.DockerfilePath = data.DockerfilePath.ValueString()
	}

	if !data.DockerConfigBase64.IsNull() {
		providerOpts["ENVBUILDER_DOCKER_CONFIG_BASE64"] = true
		opts.DockerConfigBase64 = data.DockerConfigBase64.ValueString()
	}

	if !data.ExitOnBuildFailure.IsNull() {
		providerOpts["ENVBUILDER_EXIT_ON_BUILD_FAILURE"] = true
		opts.ExitOnBuildFailure = data.ExitOnBuildFailure.ValueBool()
	}

	if !data.FallbackImage.IsNull() {
		providerOpts["ENVBUILDER_FALLBACK_IMAGE"] = true
		opts.FallbackImage = data.FallbackImage.ValueString()
	}

	if !data.GitCloneDepth.IsNull() {
		providerOpts["ENVBUILDER_GIT_CLONE_DEPTH"] = true
		opts.GitCloneDepth = data.GitCloneDepth.ValueInt64()
	}

	if !data.GitCloneSingleBranch.IsNull() {
		providerOpts["ENVBUILDER_GIT_CLONE_SINGLE_BRANCH"] = true
		opts.GitCloneSingleBranch = data.GitCloneSingleBranch.ValueBool()
	}

	if !data.GitHTTPProxyURL.IsNull() {
		providerOpts["ENVBUILDER_GIT_HTTP_PROXY_URL"] = true
		opts.GitHTTPProxyURL = data.GitHTTPProxyURL.ValueString()
	}

	if !data.GitSSHPrivateKeyPath.IsNull() {
		providerOpts["ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH"] = true
		opts.GitSSHPrivateKeyPath = data.GitSSHPrivateKeyPath.ValueString()
	}

	if !data.GitSSHPrivateKeyBase64.IsNull() {
		providerOpts["ENVBUILDER_GIT_SSH_PRIVATE_KEY_BASE64"] = true
		opts.GitSSHPrivateKeyBase64 = data.GitSSHPrivateKeyBase64.ValueString()
	}

	if !data.GitUsername.IsNull() {
		providerOpts["ENVBUILDER_GIT_USERNAME"] = true
		opts.GitUsername = data.GitUsername.ValueString()
	}

	if !data.GitPassword.IsNull() {
		providerOpts["ENVBUILDER_GIT_PASSWORD"] = true
		opts.GitPassword = data.GitPassword.ValueString()
	}

	if !data.IgnorePaths.IsNull() {
		providerOpts["ENVBUILDER_IGNORE_PATHS"] = true
		opts.IgnorePaths = tfutil.TFListToStringSlice(data.IgnorePaths)
	}

	if !data.Insecure.IsNull() {
		providerOpts["ENVBUILDER_INSECURE"] = true
		opts.Insecure = data.Insecure.ValueBool()
	}

	if data.RemoteRepoBuildMode.IsNull() {
		opts.RemoteRepoBuildMode = true
	} else {
		providerOpts["ENVBUILDER_REMOTE_REPO_BUILD_MODE"] = true
		opts.RemoteRepoBuildMode = data.RemoteRepoBuildMode.ValueBool()
	}

	if !data.SSLCertBase64.IsNull() {
		providerOpts["ENVBUILDER_SSL_CERT_BASE64"] = true
		opts.SSLCertBase64 = data.SSLCertBase64.ValueString()
	}

	if !data.Verbose.IsNull() {
		providerOpts["ENVBUILDER_VERBOSE"] = true
		opts.Verbose = data.Verbose.ValueBool()
	}

	if !data.WorkspaceFolder.IsNull() {
		providerOpts["ENVBUILDER_WORKSPACE_FOLDER"] = true
		opts.WorkspaceFolder = data.WorkspaceFolder.ValueString()
	}

	// convert extraEnv to a map for ease of use.
	extraEnv := make(map[string]string)
	for k, v := range data.ExtraEnv.Elements() {
		extraEnv[k] = tfutil.TFValueToString(v)
	}
	diags = append(diags, overrideOptionsFromExtraEnv(&opts, extraEnv, providerOpts)...)

	if opts.GitSSHPrivateKeyPath != "" && opts.GitSSHPrivateKeyBase64 != "" {
		diags.AddError("Cannot set more than one git ssh private key option",
			"Both ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH and ENVBUILDER_GIT_SSH_PRIVATE_KEY_BASE64 have been set.")
	}

	return opts, diags
}

// overrideOptionsFromExtraEnv overrides the options in opts with values from extraEnv.
// It returns any diagnostics encountered.
// It will not override certain options, such as ENVBUILDER_CACHE_REPO and ENVBUILDER_GIT_URL.
func overrideOptionsFromExtraEnv(opts *eboptions.Options, extraEnv map[string]string, providerOpts map[string]bool) diag.Diagnostics {
	var diags diag.Diagnostics
	// Make a map of the options for easy lookup.
	optsMap := make(map[string]pflag.Value)
	for _, opt := range opts.CLI() {
		optsMap[opt.Env] = opt.Value
	}
	for key, val := range extraEnv {
		opt, found := optsMap[key]
		if !found {
			// ignore unknown keys
			continue
		}

		if nonOverrideOptions[key] {
			diags.AddAttributeWarning(path.Root("extra_env"),
				"Cannot override required environment variable",
				fmt.Sprintf("The key %q in extra_env cannot be overridden.", key),
			)
			continue
		}

		// Check if the option was set on the provider data model and generate a warning if so.
		if providerOpts[key] {
			diags.AddAttributeWarning(path.Root("extra_env"),
				"Overriding provider environment variable",
				fmt.Sprintf("The key %q in extra_env overrides an option set on the provider.", key),
			)
		}

		// XXX: workaround for serpent behaviour where calling Set() on a
		// string slice will append instead of replace: set to empty first.
		if _, ok := optsMap[key].(*serpent.StringArray); ok {
			_ = optsMap[key].Set("")
		}

		if err := opt.Set(val); err != nil {
			diags.AddAttributeError(path.Root("extra_env"),
				"Invalid value for environment variable",
				fmt.Sprintf("The key %q in extra_env has an invalid value: %s", key, err),
			)
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
	for _, opt := range opts.CLI() {
		if opt.Env == "" {
			continue
		}
	}

	computed := make(map[string]string)
	for _, opt := range opts.CLI() {
		if opt.Env == "" {
			continue
		}
		// TODO: remove this check once support for legacy options is removed.
		// Only set the environment variables from opts that are not legacy options.
		// Legacy options are those that are not prefixed with ENVBUILDER_.
		// While we can detect when a legacy option is set, overriding it becomes
		// problematic. Erring on the side of caution, we will not override legacy options.
		if !strings.HasPrefix(opt.Env, envbuilderOptionPrefix) {
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
		if strings.HasPrefix(key, envbuilderOptionPrefix) {
			continue
		}
		computed[key] = val
	}
	return computed
}
