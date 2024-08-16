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
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"

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

// setComputedEnv sets data.Env and data.EnvMap based on the values of the
// other fields in the model.
func (data *CachedImageResourceModel) setComputedEnv(ctx context.Context) diag.Diagnostics {
	env := make(map[string]string)

	env["ENVBUILDER_CACHE_REPO"] = tfValueToString(data.CacheRepo)
	env["ENVBUILDER_GIT_URL"] = tfValueToString(data.GitURL)

	if !data.BaseImageCacheDir.IsNull() {
		env["ENVBUILDER_BASE_IMAGE_CACHE_DIR"] = tfValueToString(data.BaseImageCacheDir)
	}

	if !data.BuildContextPath.IsNull() {
		env["ENVBUILDER_BUILD_CONTEXT_PATH"] = tfValueToString(data.BuildContextPath)
	}

	if !data.CacheTTLDays.IsNull() {
		env["ENVBUILDER_CACHE_TTL_DAYS"] = tfValueToString(data.CacheTTLDays)
	}

	if !data.DevcontainerDir.IsNull() {
		env["ENVBUILDER_DEVCONTAINER_DIR"] = tfValueToString(data.DevcontainerDir)
	}

	if !data.DevcontainerJSONPath.IsNull() {
		env["ENVBUILDER_DEVCONTAINER_JSON_PATH"] = tfValueToString(data.DevcontainerJSONPath)
	}

	if !data.DockerfilePath.IsNull() {
		env["ENVBUILDER_DOCKERFILE_PATH"] = tfValueToString(data.DockerfilePath)
	}

	if !data.DockerConfigBase64.IsNull() {
		env["ENVBUILDER_DOCKER_CONFIG_BASE64"] = tfValueToString(data.DockerConfigBase64)
	}

	if !data.ExitOnBuildFailure.IsNull() {
		env["ENVBUILDER_EXIT_ON_BUILD_FAILURE"] = tfValueToString(data.ExitOnBuildFailure)
	}

	if !data.FallbackImage.IsNull() {
		env["ENVBUILDER_FALLBACK_IMAGE"] = tfValueToString(data.FallbackImage)
	}

	if !data.GitCloneDepth.IsNull() {
		env["ENVBUILDER_GIT_CLONE_DEPTH"] = tfValueToString(data.GitCloneDepth)
	}

	if !data.GitCloneSingleBranch.IsNull() {
		env["ENVBUILDER_GIT_CLONE_SINGLE_BRANCH"] = tfValueToString(data.GitCloneSingleBranch)
	}

	if !data.GitHTTPProxyURL.IsNull() {
		env["ENVBUILDER_GIT_HTTP_PROXY_URL"] = tfValueToString(data.GitHTTPProxyURL)
	}

	if !data.GitSSHPrivateKeyPath.IsNull() {
		env["ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH"] = tfValueToString(data.GitSSHPrivateKeyPath)
	}

	if !data.GitUsername.IsNull() {
		env["ENVBUILDER_GIT_USERNAME"] = tfValueToString(data.GitUsername)
	}

	if !data.GitPassword.IsNull() {
		env["ENVBUILDER_GIT_PASSWORD"] = tfValueToString(data.GitPassword)
	}

	if !data.IgnorePaths.IsNull() {
		env["ENVBUILDER_IGNORE_PATHS"] = strings.Join(tfListToStringSlice(data.IgnorePaths), ",")
	}

	if !data.Insecure.IsNull() {
		env["ENVBUILDER_INSECURE"] = tfValueToString(data.Insecure)
	}

	// Default to remote build mode.
	if data.RemoteRepoBuildMode.IsNull() {
		env["ENVBUILDER_REMOTE_REPO_BUILD_MODE"] = "true"
	} else {
		env["ENVBUILDER_REMOTE_REPO_BUILD_MODE"] = tfValueToString(data.RemoteRepoBuildMode)
	}

	if !data.SSLCertBase64.IsNull() {
		env["ENVBUILDER_SSL_CERT_BASE64"] = tfValueToString(data.SSLCertBase64)
	}

	if !data.Verbose.IsNull() {
		env["ENVBUILDER_VERBOSE"] = tfValueToString(data.Verbose)
	}

	if !data.WorkspaceFolder.IsNull() {
		env["ENVBUILDER_WORKSPACE_FOLDER"] = tfValueToString(data.WorkspaceFolder)
	}

	// Do ExtraEnv last so that it can override any other values.
	var diag, ds diag.Diagnostics
	if !data.ExtraEnv.IsNull() {
		for key, elem := range data.ExtraEnv.Elements() {
			if _, ok := env[key]; ok {
				// This is a warning because it's possible that the user wants to override
				// a value set by the provider.
				diag.AddAttributeWarning(path.Root("extra_env"),
					"Overriding provider environment variable",
					fmt.Sprintf("The key %q in extra_env overrides an environment variable set by the provider.", key),
				)
			}
			env[key] = tfValueToString(elem)
		}
	}

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

	// Set the expected environment variables.
	resp.Diagnostics.Append(data.setComputedEnv(ctx)...)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CachedImageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CachedImageResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	cachedImg, err := r.runCacheProbe(ctx, data)
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

	// Set the expected environment variables.
	resp.Diagnostics.Append(data.setComputedEnv(ctx)...)

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
func (r *CachedImageResource) runCacheProbe(ctx context.Context, data CachedImageResourceModel) (v1.Image, error) {
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
	if err := extractEnvbuilderFromImage(ctx, data.BuilderImage.ValueString(), envbuilderPath); err != nil {
		tflog.Error(ctx, "failed to fetch envbuilder binary from builder image", map[string]any{"err": err})
		return nil, fmt.Errorf("failed to fetch the envbuilder binary from the builder image: %s", err.Error())
	}

	workspaceFolder := data.WorkspaceFolder.ValueString()
	if workspaceFolder == "" {
		workspaceFolder = filepath.Join(tmpDir, "workspace")
		tflog.Debug(ctx, "workspace_folder not specified, using temp dir", map[string]any{"workspace_folder": workspaceFolder})
	}

	opts := eboptions.Options{
		// These options are always required
		CacheRepo:       data.CacheRepo.ValueString(),
		Filesystem:      osfs.New("/"),
		ForceSafe:       false, // This should never be set to true, as this may be running outside of a container!
		GetCachedImage:  true,  // always!
		Logger:          tfLogFunc(ctx),
		Verbose:         data.Verbose.ValueBool(),
		WorkspaceFolder: workspaceFolder,

		// Options related to compiling the devcontainer
		BuildContextPath:     data.BuildContextPath.ValueString(),
		DevcontainerDir:      data.DevcontainerDir.ValueString(),
		DevcontainerJSONPath: data.DevcontainerJSONPath.ValueString(),
		DockerfilePath:       data.DockerfilePath.ValueString(),
		DockerConfigBase64:   data.DockerConfigBase64.ValueString(),
		FallbackImage:        data.FallbackImage.ValueString(),

		// These options are required for cloning the Git repo
		CacheTTLDays:         data.CacheTTLDays.ValueInt64(),
		GitURL:               data.GitURL.ValueString(),
		GitCloneDepth:        data.GitCloneDepth.ValueInt64(),
		GitCloneSingleBranch: data.GitCloneSingleBranch.ValueBool(),
		GitUsername:          data.GitUsername.ValueString(),
		GitPassword:          data.GitPassword.ValueString(),
		GitSSHPrivateKeyPath: data.GitSSHPrivateKeyPath.ValueString(),
		GitHTTPProxyURL:      data.GitHTTPProxyURL.ValueString(),
		RemoteRepoBuildMode:  data.RemoteRepoBuildMode.ValueBool(),
		RemoteRepoDir:        filepath.Join(tmpDir, "repo"),
		SSLCertBase64:        data.SSLCertBase64.ValueString(),

		// Other options
		BaseImageCacheDir:  data.BaseImageCacheDir.ValueString(),
		BinaryPath:         envbuilderPath,                        // needed to reproduce the final layer.
		ExitOnBuildFailure: data.ExitOnBuildFailure.ValueBool(),   // may wish to do this instead of fallback image?
		Insecure:           data.Insecure.ValueBool(),             // might have internal CAs?
		IgnorePaths:        tfListToStringSlice(data.IgnorePaths), // may need to be specified?
		// The below options are not relevant and are set to their zero value explicitly.
		// They must be set by extra_env.
		CoderAgentSubsystem: nil,
		CoderAgentToken:     "",
		CoderAgentURL:       "",
		ExportEnvFile:       "",
		InitArgs:            "",
		InitCommand:         "",
		InitScript:          "",
		LayerCacheDir:       "",
		PostStartScriptPath: "",
		PushImage:           false, // This is only relevant when building.
		SetupScript:         "",
		SkipRebuild:         false,
	}

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
