// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	kconfig "github.com/GoogleContainerTools/kaniko/pkg/config"
	"github.com/coder/envbuilder"
	eblog "github.com/coder/envbuilder/log"
	eboptions "github.com/coder/envbuilder/options"
	"github.com/go-git/go-billy/v5/osfs"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &CachedImageDataSource{}

func NewCachedImageDataSource() datasource.DataSource {
	return &CachedImageDataSource{}
}

// CachedImageDataSource defines the data source implementation.
type CachedImageDataSource struct {
	client *http.Client
}

// CachedImageDataSourceModel describes the data source data model.
type CachedImageDataSourceModel struct {
	BaseImageCacheDir    types.String `tfsdk:"base_image_cache_dir"`
	BuildContextPath     types.String `tfsdk:"build_context_path"`
	BuilderImage         types.String `tfsdk:"builder_image"`
	CacheRepo            types.String `tfsdk:"cache_repo"`
	CacheTTLDays         types.Int64  `tfsdk:"cache_ttl_days"`
	DevcontainerDir      types.String `tfsdk:"devcontainer_dir"`
	DevcontainerJSONPath types.String `tfsdk:"devcontainer_json_path"`
	DockerfilePath       types.String `tfsdk:"dockerfile_path"`
	DockerConfigBase64   types.String `tfsdk:"docker_config_base64"`
	Env                  types.List   `tfsdk:"env"`
	Exists               types.Bool   `tfsdk:"exists"`
	ExitOnBuildFailure   types.Bool   `tfsdk:"exit_on_build_failure"`
	ExtraEnv             types.Map    `tfsdk:"extra_env"`
	FallbackImage        types.String `tfsdk:"fallback_image"`
	GitCloneDepth        types.Int64  `tfsdk:"git_clone_depth"`
	GitCloneSingleBranch types.Bool   `tfsdk:"git_clone_single_branch"`
	GitHTTPProxyURL      types.String `tfsdk:"git_http_proxy_url"`
	GitPassword          types.String `tfsdk:"git_password"`
	GitSSHPrivateKeyPath types.String `tfsdk:"git_ssh_private_key_path"`
	GitURL               types.String `tfsdk:"git_url"`
	GitUsername          types.String `tfsdk:"git_username"`
	ID                   types.String `tfsdk:"id"`
	IgnorePaths          types.List   `tfsdk:"ignore_paths"`
	Image                types.String `tfsdk:"image"`
	Insecure             types.Bool   `tfsdk:"insecure"`
	SSLCertBase64        types.String `tfsdk:"ssl_cert_base64"`
	Verbose              types.Bool   `tfsdk:"verbose"`
}

func (d *CachedImageDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cached_image"
}

func (d *CachedImageDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "The cached image data source can be used to retrieve a cached image produced by envbuilder.",

		Attributes: map[string]schema.Attribute{
			"base_image_cache_dir": schema.StringAttribute{
				MarkdownDescription: "The path to a directory where the base image can be found. This should be a read-only directory solely mounted for the purpose of caching the base image.",
				Optional:            true,
			},
			"build_context_path": schema.StringAttribute{
				MarkdownDescription: "Can be specified when a DockerfilePath is specified outside the base WorkspaceFolder. This path MUST be relative to the WorkspaceFolder path into which the repo is cloned.",
				Optional:            true,
			},
			"builder_image": schema.StringAttribute{
				MarkdownDescription: "The builder image to use if the cache does not exist.",
				Required:            true,
			},
			"cache_repo": schema.StringAttribute{
				MarkdownDescription: "The name of the container registry to fetch the cache image from.",
				Required:            true,
			},
			"cache_ttl_days": schema.Int64Attribute{
				MarkdownDescription: "The number of days to use cached layers before expiring them. Defaults to 7 days.",
				Optional:            true,
			},
			"devcontainer_dir": schema.StringAttribute{
				MarkdownDescription: "The path to the folder containing the devcontainer.json file that will be used to build the workspace and can either be an absolute path or a path relative to the workspace folder. If not provided, defaults to `.devcontainer`.",
				Optional:            true,
			},
			"devcontainer_json_path": schema.StringAttribute{
				MarkdownDescription: "The path to a devcontainer.json file that is either an absolute path or a path relative to DevcontainerDir. This can be used in cases where one wants to substitute an edited devcontainer.json file for the one that exists in the repo.",
				Optional:            true,
			},
			"dockerfile_path": schema.StringAttribute{
				MarkdownDescription: "The relative path to the Dockerfile that will be used to build the workspace. This is an alternative to using a devcontainer that some might find simpler.",
				Optional:            true,
			},
			"docker_config_base64": schema.StringAttribute{
				MarkdownDescription: "The base64 encoded Docker config file that will be used to pull images from private container registries.",
				Optional:            true,
			},
			// TODO(mafredri): Map vs List? Support both?
			"env": schema.ListAttribute{
				MarkdownDescription: "Computed envbuilder configuration to be set for the container.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"exists": schema.BoolAttribute{
				MarkdownDescription: "Whether the cached image was exists or not for the given config.",
				Computed:            true,
			},
			"exit_on_build_failure": schema.BoolAttribute{
				MarkdownDescription: "Terminates upon a build failure. This is handy when preferring the FALLBACK_IMAGE in cases where no devcontainer.json or image is provided. However, it ensures that the container stops if the build process encounters an error.",
				Optional:            true,
			},
			// TODO(mafredri): Map vs List? Support both?
			"extra_env": schema.MapAttribute{
				MarkdownDescription: "Extra environment variables to set for the container. This may include evbuilder options.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"fallback_image": schema.StringAttribute{
				MarkdownDescription: "Specifies an alternative image to use when neither an image is declared in the devcontainer.json file nor a Dockerfile is present. If there's a build failure (from a faulty Dockerfile) or a misconfiguration, this image will be the substitute. Set ExitOnBuildFailure to true to halt the container if the build faces an issue.",
				Optional:            true,
			},
			"git_clone_depth": schema.Int64Attribute{
				MarkdownDescription: "The depth to use when cloning the Git repository.",
				Optional:            true,
			},
			"git_clone_single_branch": schema.BoolAttribute{
				MarkdownDescription: "Clone only a single branch of the Git repository.",
				Optional:            true,
			},
			"git_http_proxy_url": schema.StringAttribute{
				MarkdownDescription: "The URL for the HTTP proxy. This is optional.",
				Optional:            true,
			},
			"git_password": schema.StringAttribute{
				MarkdownDescription: "The password to use for Git authentication. This is optional.",
				Sensitive:           true,
				Optional:            true,
			},
			"git_ssh_private_key_path": schema.StringAttribute{
				MarkdownDescription: "Path to an SSH private key to be used for Git authentication.",
				Optional:            true,
			},
			"git_username": schema.StringAttribute{
				MarkdownDescription: "The username to use for Git authentication. This is optional.",
				Optional:            true,
			},
			"git_url": schema.StringAttribute{
				MarkdownDescription: "The URL of a Git repository containing a Devcontainer or Docker image to clone.",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Cached image identifier. This will generally be the image's SHA256 digest.",
				Computed:            true,
			},
			"ignore_paths": schema.ListAttribute{
				MarkdownDescription: "The comma separated list of paths to ignore when building the workspace.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"image": schema.StringAttribute{
				MarkdownDescription: "Outputs the cached image URL if it exists, otherwise the builder image URL is output instead.",
				Computed:            true,
			},
			"insecure": schema.BoolAttribute{
				MarkdownDescription: "Bypass TLS verification when cloning and pulling from container registries.",
				Optional:            true,
			},
			"ssl_cert_base64": schema.StringAttribute{
				MarkdownDescription: "The content of an SSL cert file. This is useful for self-signed certificates.",
				Optional:            true,
			},
			"verbose": schema.BoolAttribute{
				MarkdownDescription: "Enable verbose output.",
				Optional:            true,
			},
		},
	}
}

func (d *CachedImageDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*http.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *http.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.client = client
}

func (d *CachedImageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data CachedImageDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	// httpResp, err := d.client.Do(httpReq)
	// if err != nil {
	//     resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read cached image, got error: %s", err))
	//     return
	// }

	tmpDir, err := os.MkdirTemp(os.TempDir(), "cached-image-data-source")
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create temp directory: %s", err.Error()))
		return
	}
	oldKanikoDir := kconfig.KanikoDir
	tmpKanikoDir := filepath.Join(tmpDir, ".envbuilder")
	// Normally you would set the KANIKO_DIR environment variable, but we are importing kaniko directly.
	kconfig.KanikoDir = tmpKanikoDir
	tflog.Info(ctx, "set kaniko dir to "+tmpKanikoDir)
	defer func() {
		kconfig.KanikoDir = oldKanikoDir
		tflog.Info(ctx, "restored kaniko dir to "+oldKanikoDir)
	}()
	if err := os.MkdirAll(tmpKanikoDir, 0o755); err != nil {
		tflog.Error(ctx, "failed to create kaniko dir: "+err.Error())
	}

	opts := eboptions.Options{
		// These options are always required
		CacheRepo:       data.CacheRepo.ValueString(),
		Filesystem:      osfs.New("/"),
		ForceSafe:       false, // This should never be set to true, as this may be running outside of a container!
		GetCachedImage:  true,  // always!
		Logger:          tfLogFunc(ctx),
		Verbose:         data.Verbose.ValueBool(),
		WorkspaceFolder: tmpDir,

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
		SSLCertBase64:        data.SSLCertBase64.ValueString(),

		// Other options
		BaseImageCacheDir:  data.BaseImageCacheDir.ValueString(),
		ExitOnBuildFailure: data.ExitOnBuildFailure.ValueBool(),   // may wish to do this instead of fallback image?
		Insecure:           data.Insecure.ValueBool(),             // might have internal CAs?
		IgnorePaths:        tfListToStringSlice(data.IgnorePaths), // may need to be specified?
		// The below options are not relevant and are set to their zero value explicitly.
		CoderAgentSubsystem: nil,
		CoderAgentToken:     "",
		CoderAgentURL:       "",
		ExportEnvFile:       "",
		InitArgs:            "",
		InitCommand:         "",
		InitScript:          "",
		LayerCacheDir:       "",
		PostStartScriptPath: "",
		PushImage:           false,
		SetupScript:         "",
		SkipRebuild:         false,
	}

	image, err := envbuilder.RunCacheProbe(ctx, opts)
	data.Exists = types.BoolValue(err == nil)
	if err != nil {
		resp.Diagnostics.AddWarning("Cached image not found", err.Error())
	} else {
		digest, err := image.Digest()
		if err != nil {
			resp.Diagnostics.AddError("Failed to get cached image digest", err.Error())
			return
		}
		tflog.Info(ctx, fmt.Sprintf("found image: %s@%s", opts.CacheRepo, digest))
		data.ID = types.StringValue(digest.String())
		data.Image = types.StringValue(fmt.Sprintf("%s@%s", data.CacheRepo, digest.String()))
	}

	// Compute the env attribute from the config map.
	// TODO(mafredri): Convert any other relevant attributes given via schema.
	for key, elem := range data.ExtraEnv.Elements() {
		data.Env = appendKnownEnvToList(data.Env, key, elem)
	}

	data.Env = appendKnownEnvToList(data.Env, "ENVBUILDER_CACHE_REPO", data.CacheRepo)
	data.Env = appendKnownEnvToList(data.Env, "ENVBUILDER_CACHE_TTL_DAYS", data.CacheTTLDays)
	data.Env = appendKnownEnvToList(data.Env, "ENVBUILDER_GIT_URL", data.GitURL)
	data.Env = appendKnownEnvToList(data.Env, "ENVBUILDER_GIT_USERNAME", data.GitUsername)
	data.Env = appendKnownEnvToList(data.Env, "ENVBUILDER_GIT_PASSWORD", data.GitPassword)

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "read a data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
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

// NOTE: the String() method of Terraform values will evalue to `<null>` if unknown.
// Check IsUnknown() first before calling String().
type stringable interface {
	IsUnknown() bool
	String() string
}

func appendKnownEnvToList(list types.List, key string, value stringable) types.List {
	if value.IsUnknown() {
		return list
	}
	elem := types.StringValue(fmt.Sprintf("%s=%s", key, value.String()))
	list, _ = types.ListValue(types.StringType, append(list.Elements(), elem))
	return list
}

func tfListToStringSlice(l types.List) []string {
	var ss []string
	for _, el := range l.Elements() {
		if sv, ok := el.(stringable); !ok {
			panic(fmt.Sprintf("developer error: element %+v must be stringable", el))
		} else if sv.IsUnknown() {
			ss = append(ss, "")
		} else {
			ss = append(ss, sv.String())
		}
	}
	return ss
}
