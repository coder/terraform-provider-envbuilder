package provider

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	kconfig "github.com/GoogleContainerTools/kaniko/pkg/config"
	"github.com/coder/envbuilder"
	"github.com/coder/envbuilder/constants"
	eblog "github.com/coder/envbuilder/log"
	eboptions "github.com/coder/envbuilder/options"
	"github.com/coder/serpent"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"
	"github.com/spf13/pflag"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &CachedImageResource{}

func NewCachedImageResource() resource.Resource {
	return &CachedImageResource{}
}

// CachedImageResource defines the resource implementation.
type CachedImageResource struct {
	client *http.Client
}

// CachedImageResourceModel describes an envbuilder cached image resource.
type CachedImageResourceModel struct {
	// Required "inputs".
	BuilderImage types.String `tfsdk:"builder_image"`
	CacheRepo    types.String `tfsdk:"cache_repo"`
	GitURL       types.String `tfsdk:"git_url"`
	// Optional "inputs".
	BaseImageCacheDir    types.String `tfsdk:"base_image_cache_dir"`
	BuildContextPath     types.String `tfsdk:"build_context_path"`
	CacheTTLDays         types.Int64  `tfsdk:"cache_ttl_days"`
	DevcontainerDir      types.String `tfsdk:"devcontainer_dir"`
	DevcontainerJSONPath types.String `tfsdk:"devcontainer_json_path"`
	DockerfilePath       types.String `tfsdk:"dockerfile_path"`
	DockerConfigBase64   types.String `tfsdk:"docker_config_base64"`
	ExitOnBuildFailure   types.Bool   `tfsdk:"exit_on_build_failure"`
	ExtraEnv             types.Map    `tfsdk:"extra_env"`
	FallbackImage        types.String `tfsdk:"fallback_image"`
	GitCloneDepth        types.Int64  `tfsdk:"git_clone_depth"`
	GitCloneSingleBranch types.Bool   `tfsdk:"git_clone_single_branch"`
	GitHTTPProxyURL      types.String `tfsdk:"git_http_proxy_url"`
	GitPassword          types.String `tfsdk:"git_password"`
	GitSSHPrivateKeyPath types.String `tfsdk:"git_ssh_private_key_path"`
	GitUsername          types.String `tfsdk:"git_username"`
	IgnorePaths          types.List   `tfsdk:"ignore_paths"`
	Insecure             types.Bool   `tfsdk:"insecure"`
	RemoteRepoBuildMode  types.Bool   `tfsdk:"remote_repo_build_mode"`
	SSLCertBase64        types.String `tfsdk:"ssl_cert_base64"`
	Verbose              types.Bool   `tfsdk:"verbose"`
	WorkspaceFolder      types.String `tfsdk:"workspace_folder"`
	// Computed "outputs".
	Env    types.List   `tfsdk:"env"`
	EnvMap types.Map    `tfsdk:"env_map"`
	Exists types.Bool   `tfsdk:"exists"`
	ID     types.String `tfsdk:"id"`
	Image  types.String `tfsdk:"image"`
}

func (r *CachedImageResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cached_image"
}

func (r *CachedImageResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "The cached image resource can be used to retrieve a cached image produced by envbuilder. Creating this resource will clone the specified Git repository, read a Devcontainer specification or Dockerfile, and check for its presence in the provided cache repo. If any of the layers of the cached image are missing in the provided cache repo, the image will be considered as missing. A cached image in this state will be recreated until found.",

		Attributes: map[string]schema.Attribute{
			// Required "inputs".
			"builder_image": schema.StringAttribute{
				MarkdownDescription: "The envbuilder image to use if the cached version is not found.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cache_repo": schema.StringAttribute{
				MarkdownDescription: "(Envbuilder option) The name of the container registry to fetch the cache image from.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"git_url": schema.StringAttribute{
				MarkdownDescription: "(Envbuilder option) The URL of a Git repository containing a Devcontainer or Docker image to clone.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			// Optional "inputs".
			"base_image_cache_dir": schema.StringAttribute{
				MarkdownDescription: "(Envbuilder option) The path to a directory where the base image can be found. This should be a read-only directory solely mounted for the purpose of caching the base image.",
				Optional:            true,
			},
			"build_context_path": schema.StringAttribute{
				MarkdownDescription: "(Envbuilder option) Can be specified when a DockerfilePath is specified outside the base WorkspaceFolder. This path MUST be relative to the WorkspaceFolder path into which the repo is cloned.",
				Optional:            true,
			},
			"cache_ttl_days": schema.Int64Attribute{
				MarkdownDescription: "(Envbuilder option) The number of days to use cached layers before expiring them. Defaults to 7 days.",
				Optional:            true,
			},
			"devcontainer_dir": schema.StringAttribute{
				MarkdownDescription: "(Envbuilder option) The path to the folder containing the devcontainer.json file that will be used to build the workspace and can either be an absolute path or a path relative to the workspace folder. If not provided, defaults to `.devcontainer`.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"devcontainer_json_path": schema.StringAttribute{
				MarkdownDescription: "(Envbuilder option) The path to a devcontainer.json file that is either an absolute path or a path relative to DevcontainerDir. This can be used in cases where one wants to substitute an edited devcontainer.json file for the one that exists in the repo.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"dockerfile_path": schema.StringAttribute{
				MarkdownDescription: "(Envbuilder option) The relative path to the Dockerfile that will be used to build the workspace. This is an alternative to using a devcontainer that some might find simpler.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"docker_config_base64": schema.StringAttribute{
				MarkdownDescription: "(Envbuilder option) The base64 encoded Docker config file that will be used to pull images from private container registries.",
				Optional:            true,
			},
			"exit_on_build_failure": schema.BoolAttribute{
				MarkdownDescription: "(Envbuilder option) Terminates upon a build failure. This is handy when preferring the FALLBACK_IMAGE in cases where no devcontainer.json or image is provided. However, it ensures that the container stops if the build process encounters an error.",
				Optional:            true,
			},
			"extra_env": schema.MapAttribute{
				MarkdownDescription: "Extra environment variables to set for the container. This may include envbuilder options.",
				ElementType:         types.StringType,
				Optional:            true,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},
			"fallback_image": schema.StringAttribute{
				MarkdownDescription: "(Envbuilder option) Specifies an alternative image to use when neither an image is declared in the devcontainer.json file nor a Dockerfile is present. If there's a build failure (from a faulty Dockerfile) or a misconfiguration, this image will be the substitute. Set ExitOnBuildFailure to true to halt the container if the build faces an issue.",
				Optional:            true,
			},
			"git_clone_depth": schema.Int64Attribute{
				MarkdownDescription: "(Envbuilder option) The depth to use when cloning the Git repository.",
				Optional:            true,
			},
			"git_clone_single_branch": schema.BoolAttribute{
				MarkdownDescription: "(Envbuilder option) Clone only a single branch of the Git repository.",
				Optional:            true,
			},
			"git_http_proxy_url": schema.StringAttribute{
				MarkdownDescription: "(Envbuilder option) The URL for the HTTP proxy. This is optional.",
				Optional:            true,
			},
			"git_password": schema.StringAttribute{
				MarkdownDescription: "(Envbuilder option) The password to use for Git authentication. This is optional.",
				Sensitive:           true,
				Optional:            true,
			},
			"git_ssh_private_key_path": schema.StringAttribute{
				MarkdownDescription: "(Envbuilder option) Path to an SSH private key to be used for Git authentication.",
				Optional:            true,
			},
			"git_username": schema.StringAttribute{
				MarkdownDescription: "(Envbuilder option) The username to use for Git authentication. This is optional.",
				Optional:            true,
			},

			"ignore_paths": schema.ListAttribute{
				MarkdownDescription: "(Envbuilder option) The comma separated list of paths to ignore when building the workspace.",
				ElementType:         types.StringType,
				Optional:            true,
			},

			"insecure": schema.BoolAttribute{
				MarkdownDescription: "(Envbuilder option) Bypass TLS verification when cloning and pulling from container registries.",
				Optional:            true,
			},
			"remote_repo_build_mode": schema.BoolAttribute{
				MarkdownDescription: "(Envbuilder option) RemoteRepoBuildMode uses the remote repository as the source of truth when building the image. Enabling this option ignores user changes to local files and they will not be reflected in the image. This can be used to improve cache utilization when multiple users are working on the same repository. (NOTE: The Terraform provider will **always** use remote repo build mode for probing the cache repo.)",
				Optional:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"ssl_cert_base64": schema.StringAttribute{
				MarkdownDescription: "(Envbuilder option) The content of an SSL cert file. This is useful for self-signed certificates.",
				Optional:            true,
			},
			"verbose": schema.BoolAttribute{
				MarkdownDescription: "(Envbuilder option) Enable verbose output.",
				Optional:            true,
			},
			"workspace_folder": schema.StringAttribute{
				MarkdownDescription: "(Envbuilder option) path to the workspace folder that will be built. This is optional.",
				Optional:            true,
			},

			// Computed "outputs".
			"env": schema.ListAttribute{
				MarkdownDescription: "Computed envbuilder configuration to be set for the container in the form of a list of strings of `key=value`. May contain secrets.",
				ElementType:         types.StringType,
				Computed:            true,
				Sensitive:           true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"env_map": schema.MapAttribute{
				MarkdownDescription: "Computed envbuilder configuration to be set for the container in the form of a key-value map. May contain secrets.",
				ElementType:         types.StringType,
				Computed:            true,
				Sensitive:           true,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},
			"exists": schema.BoolAttribute{
				MarkdownDescription: "Whether the cached image was exists or not for the given config.",
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Cached image identifier. This will generally be the image's SHA256 digest.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"image": schema.StringAttribute{
				MarkdownDescription: "Outputs the cached image repo@digest if it exists, and builder image otherwise.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *CachedImageResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*http.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *http.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

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
		opts.IgnorePaths = tfListToStringSlice(data.IgnorePaths)
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
		extraEnv[k] = tfValueToString(v)
	}
	diags = append(diags, overrideOptionsFromExtraEnv(&opts, extraEnv, overrides)...)

	return opts, diags
}

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

// setComputedEnv sets data.Env and data.EnvMap based on the values of the
// other fields in the model.
func (data *CachedImageResourceModel) setComputedEnv(ctx context.Context, env map[string]string) diag.Diagnostics {
	var diag, ds diag.Diagnostics
	data.EnvMap, ds = basetypes.NewMapValueFrom(ctx, types.StringType, env)
	diag = append(diag, ds...)
	data.Env, ds = basetypes.NewListValueFrom(ctx, types.StringType, sortedKeyValues(env))
	diag = append(diag, ds...)
	return diag
}

func (r *CachedImageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CachedImageResourceModel

	// Read prior state into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get the options from the data model.
	opts, diags := optionsFromDataModel(data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Set the expected environment variables.
	computedEnv := computeEnvFromOptions(opts, tfMapToStringMap(data.ExtraEnv))
	resp.Diagnostics.Append(data.setComputedEnv(ctx, computedEnv)...)

	// If the previous state is that Image == BuilderImage, then we previously did
	// not find the image. We will need to run another cache probe.
	if data.Image.Equal(data.BuilderImage) {
		resp.Diagnostics.AddWarning(
			"Re-running cache probe due to previous miss.",
			fmt.Sprintf(`The previous state specifies image == builder_image %q, which indicates a previous cache miss.`,
				data.Image.ValueString(),
			))
		resp.State.RemoveResource(ctx)
		return
	}

	// Check the remote registry for the image we previously found.
	img, err := getRemoteImage(data.Image.ValueString())
	if err != nil {
		if !strings.Contains(err.Error(), "MANIFEST_UNKNOWN") {
			// Explicitly not making this an error diag.
			resp.Diagnostics.AddWarning("Unable to check remote image.",
				fmt.Sprintf("The repository %q returned the following error while checking for a cached image %q: %q",
					data.CacheRepo.ValueString(),
					data.Image.ValueString(),
					err.Error(),
				))
			return
		}
		// Image does not exist any longer! Remove the resource so we can re-create
		// it next time.
		resp.Diagnostics.AddWarning("Previously built image not found, recreating.",
			fmt.Sprintf("The repository %q does not contain the cached image %q. It will be rebuilt in the next apply.",
				data.CacheRepo.ValueString(),
				data.Image.ValueString(),
			))
		resp.State.RemoveResource(ctx)
		return
	}

	// Found image! Get the digest.
	digest, err := img.Digest()
	if err != nil {
		resp.Diagnostics.AddError("Error fetching image digest", err.Error())
		return
	}

	data.ID = types.StringValue(digest.String())
	data.Image = types.StringValue(fmt.Sprintf("%s@%s", data.CacheRepo.ValueString(), digest))
	data.Exists = types.BoolValue(true)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CachedImageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CachedImageResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Get the options from the data model.
	opts, diags := optionsFromDataModel(data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set the expected environment variables.
	computedEnv := computeEnvFromOptions(opts, tfMapToStringMap(data.ExtraEnv))
	resp.Diagnostics.Append(data.setComputedEnv(ctx, computedEnv)...)

	cachedImg, err := runCacheProbe(ctx, data.BuilderImage.ValueString(), opts)
	data.ID = types.StringValue(uuid.Nil.String())
	data.Exists = types.BoolValue(err == nil)
	if err != nil {
		// FIXME: there are legit errors that can crop up here.
		// We should add a sentinel error in Kaniko for uncached layers, and check
		// it here.
		resp.Diagnostics.AddWarning("Cached image not found.", fmt.Sprintf(
			"Failed to find cached image in repository %q. It will be rebuilt in the next apply. Error: %s",
			data.CacheRepo.ValueString(),
			err.Error(),
		))
		data.Image = data.BuilderImage
	} else if digest, err := cachedImg.Digest(); err != nil {
		// There's something seriously up with this image!
		resp.Diagnostics.AddError("Failed to get cached image digest", err.Error())
		return
	} else {
		tflog.Info(ctx, fmt.Sprintf("found image: %s@%s", data.CacheRepo.ValueString(), digest))
		data.Image = types.StringValue(fmt.Sprintf("%s@%s", data.CacheRepo.ValueString(), digest))
		data.ID = types.StringValue(digest.String())
	}

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CachedImageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Updates are a no-op.
	var data CachedImageResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CachedImageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Deletes are a no-op.
	var data CachedImageResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// runCacheProbe performs a 'fake build' of the requested image and ensures that
// all of the resulting layers of the image are present in the configured cache
// repo. Otherwise, returns an error.
func runCacheProbe(ctx context.Context, builderImage string, opts eboptions.Options) (v1.Image, error) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "envbuilder-provider-cached-image-data-source")
	if err != nil {
		return nil, fmt.Errorf("unable to create temp directory: %s", err.Error())
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			tflog.Error(ctx, "failed to clean up tmpDir", map[string]any{"tmpDir": tmpDir, "err": err})
		}
	}()

	oldKanikoDir := kconfig.KanikoDir
	tmpKanikoDir := filepath.Join(tmpDir, constants.MagicDir)
	// Normally you would set the KANIKO_DIR environment variable, but we are importing kaniko directly.
	kconfig.KanikoDir = tmpKanikoDir
	tflog.Info(ctx, "set kaniko dir to "+tmpKanikoDir)
	defer func() {
		kconfig.KanikoDir = oldKanikoDir
		tflog.Info(ctx, "restored kaniko dir to "+oldKanikoDir)
	}()

	if err := os.MkdirAll(tmpKanikoDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create kaniko dir: %w", err)
	}

	// In order to correctly reproduce the final layer of the cached image, we
	// need the envbuilder binary used to originally build the image!
	envbuilderPath := filepath.Join(tmpDir, "envbuilder")
	if err := extractEnvbuilderFromImage(ctx, builderImage, envbuilderPath); err != nil {
		tflog.Error(ctx, "failed to fetch envbuilder binary from builder image", map[string]any{"err": err})
		return nil, fmt.Errorf("failed to fetch the envbuilder binary from the builder image: %s", err.Error())
	}
	opts.BinaryPath = envbuilderPath

	// We need a filesystem to work with.
	opts.Filesystem = osfs.New("/")
	// This should never be set to true, as this may be running outside of a container!
	opts.ForceSafe = false
	// We always want to get the cached image.
	opts.GetCachedImage = true
	// Log to the Terraform logger.
	opts.Logger = tfLogFunc(ctx)

	// We don't require users to set a workspace folder, but maybe there's a
	// reason someone may need to.
	if opts.WorkspaceFolder == "" {
		opts.WorkspaceFolder = filepath.Join(tmpDir, "workspace")
		if err := os.MkdirAll(opts.WorkspaceFolder, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create workspace folder: %w", err)
		}
		tflog.Debug(ctx, "workspace_folder not specified, using temp dir", map[string]any{"workspace_folder": opts.WorkspaceFolder})
	}

	// We need a place to clone the repo.
	repoDir := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create repo dir: %w", err)
	}
	opts.RemoteRepoDir = repoDir

	// The below options are not relevant and are set to their zero value
	// explicitly.
	// They must be set by extra_env to be used in the final builder image.
	opts.CoderAgentSubsystem = nil
	opts.CoderAgentToken = ""
	opts.CoderAgentURL = ""
	opts.ExportEnvFile = ""
	opts.InitArgs = ""
	opts.InitCommand = ""
	opts.InitScript = ""
	opts.LayerCacheDir = ""
	opts.PostStartScriptPath = ""
	opts.PushImage = false
	opts.SetupScript = ""
	opts.SkipRebuild = false

	return envbuilder.RunCacheProbe(ctx, opts)
}

// getRemoteImage fetches the image manifest of the image.
func getRemoteImage(imgRef string) (v1.Image, error) {
	ref, err := name.ParseReference(imgRef)
	if err != nil {
		return nil, fmt.Errorf("parse reference: %w", err)
	}

	img, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, fmt.Errorf("check remote image: %w", err)
	}

	return img, nil
}

// extractEnvbuilderFromImage reads the image located at imgRef and extracts
// MagicBinaryLocation to destPath.
func extractEnvbuilderFromImage(ctx context.Context, imgRef, destPath string) error {
	needle := filepath.Clean(constants.MagicBinaryLocation)[1:] // skip leading '/'
	img, err := getRemoteImage(imgRef)
	if err != nil {
		return fmt.Errorf("check remote image: %w", err)
	}

	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("get image layers: %w", err)
	}

	// Check the layers in reverse order. The last layers are more likely to
	// include the binary.
	for i := len(layers) - 1; i >= 0; i-- {
		ul, err := layers[i].Uncompressed()
		if err != nil {
			return fmt.Errorf("get uncompressed layer: %w", err)
		}

		tr := tar.NewReader(ul)
		for {
			th, err := tr.Next()
			if err == io.EOF {
				break
			}

			if err != nil {
				return fmt.Errorf("read tar header: %w", err)
			}

			name := filepath.Clean(th.Name)
			if th.Typeflag != tar.TypeReg {
				tflog.Debug(ctx, "skip non-regular file", map[string]any{"name": name, "layer_idx": i + 1})
				continue
			}

			if name != needle {
				tflog.Debug(ctx, "skip file", map[string]any{"name": name, "layer_idx": i + 1})
				continue
			}

			tflog.Debug(ctx, "found file", map[string]any{"name": name, "layer_idx": i + 1})
			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				return fmt.Errorf("create parent directories: %w", err)
			}
			destF, err := os.Create(destPath)
			if err != nil {
				return fmt.Errorf("create dest file for writing: %w", err)
			}
			defer destF.Close()
			_, err = io.Copy(destF, tr)
			if err != nil {
				return fmt.Errorf("copy dest file from image: %w", err)
			}
			if err := destF.Close(); err != nil {
				return fmt.Errorf("close dest file: %w", err)
			}

			if err := os.Chmod(destPath, 0o755); err != nil {
				return fmt.Errorf("chmod file: %w", err)
			}
			return nil
		}
	}

	return fmt.Errorf("extract envbuilder binary from image %q: %w", imgRef, os.ErrNotExist)
}

// tfValueToString converts an attr.Value to its string representation
// based on its Terraform type. This is needed because the String()
// method on an attr.Value creates a 'human-readable' version of the type, which
// leads to quotes, escaped characters, and other assorted sadness.
func tfValueToString(val attr.Value) string {
	if val.IsUnknown() || val.IsNull() {
		return ""
	}
	if vs, ok := val.(interface{ ValueString() string }); ok {
		return vs.ValueString()
	}
	if vb, ok := val.(interface{ ValueBool() bool }); ok {
		return fmt.Sprintf("%t", vb.ValueBool())
	}
	if vi, ok := val.(interface{ ValueInt64() int64 }); ok {
		return fmt.Sprintf("%d", vi.ValueInt64())
	}
	panic(fmt.Errorf("tfValueToString: value %T is not a supported type", val))
}

// tfListToStringSlice converts a types.List to a []string by calling
// tfValueToString on each element.
func tfListToStringSlice(l types.List) []string {
	var ss []string
	for _, el := range l.Elements() {
		ss = append(ss, tfValueToString(el))
	}
	return ss
}

// tfMapToStringMap converts a types.Map to a map[string]string by calling
// tfValueToString on each element.
func tfMapToStringMap(m types.Map) map[string]string {
	res := make(map[string]string)
	for k, v := range m.Elements() {
		res[k] = tfValueToString(v)
	}
	return res
}

// tfLogFunc is an adapter to envbuilder/log.Func.
func tfLogFunc(ctx context.Context) eblog.Func {
	return func(level eblog.Level, format string, args ...any) {
		var logFn func(context.Context, string, ...map[string]interface{})
		switch level {
		case eblog.LevelTrace:
			logFn = tflog.Trace
		case eblog.LevelDebug:
			logFn = tflog.Debug
		case eblog.LevelWarn:
			logFn = tflog.Warn
		case eblog.LevelError:
			logFn = tflog.Error
		default:
			logFn = tflog.Info
		}
		logFn(ctx, fmt.Sprintf(format, args...))
	}
}

// sortedKeyValues returns the keys and values of the map in the form "key=value"
// sorted by key in lexicographical order.
func sortedKeyValues(m map[string]string) []string {
	pairs := make([]string, 0, len(m))
	var sb strings.Builder
	for k := range m {
		_, _ = sb.WriteString(k)
		_, _ = sb.WriteRune('=')
		_, _ = sb.WriteString(m[k])
		pairs = append(pairs, sb.String())
		sb.Reset()
	}
	sort.Strings(pairs)
	return pairs
}
