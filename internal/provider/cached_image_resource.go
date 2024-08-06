// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

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

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &CachedImageResource{}

func NewCachedImageResource() resource.Resource {
	return &CachedImageResource{}
}

// CachedImageResource defines the data source implementation.
type CachedImageResource struct {
	client *http.Client
}

// CachedImageResourceModel describes the data source data model.
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
	SSLCertBase64        types.String `tfsdk:"ssl_cert_base64"`
	Verbose              types.Bool   `tfsdk:"verbose"`
	WorkspaceFolder      types.String `tfsdk:"workspace_folder"`
	// Computed "outputs".
	Env    types.List   `tfsdk:"env"`
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
		MarkdownDescription: "The cached image data source can be used to retrieve a cached image produced by envbuilder. Reading from this data source will clone the specified Git repository, read a Devcontainer specification or Dockerfile, and check for its presence in the provided cache repo.",

		Attributes: map[string]schema.Attribute{
			// Required "inputs".
			"builder_image": schema.StringAttribute{
				MarkdownDescription: "The envbuilder image to use if the cached version is not found.",
				Required:            true,
			},
			"cache_repo": schema.StringAttribute{
				MarkdownDescription: "(Envbuilder option) The name of the container registry to fetch the cache image from.",
				Required:            true,
			},
			"git_url": schema.StringAttribute{
				MarkdownDescription: "(Envbuilder option) The URL of a Git repository containing a Devcontainer or Docker image to clone.",
				Required:            true,
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
			},
			"devcontainer_json_path": schema.StringAttribute{
				MarkdownDescription: "(Envbuilder option) The path to a devcontainer.json file that is either an absolute path or a path relative to DevcontainerDir. This can be used in cases where one wants to substitute an edited devcontainer.json file for the one that exists in the repo.",
				Optional:            true,
			},
			"dockerfile_path": schema.StringAttribute{
				MarkdownDescription: "(Envbuilder option) The relative path to the Dockerfile that will be used to build the workspace. This is an alternative to using a devcontainer that some might find simpler.",
				Optional:            true,
			},
			"docker_config_base64": schema.StringAttribute{
				MarkdownDescription: "(Envbuilder option) The base64 encoded Docker config file that will be used to pull images from private container registries.",
				Optional:            true,
			},
			"exit_on_build_failure": schema.BoolAttribute{
				MarkdownDescription: "(Envbuilder option) Terminates upon a build failure. This is handy when preferring the FALLBACK_IMAGE in cases where no devcontainer.json or image is provided. However, it ensures that the container stops if the build process encounters an error.",
				Optional:            true,
			},
			// TODO(mafredri): Map vs List? Support both?
			"extra_env": schema.MapAttribute{
				MarkdownDescription: "Extra environment variables to set for the container. This may include envbuilder options.",
				ElementType:         types.StringType,
				Optional:            true,
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
			"id": schema.StringAttribute{
				MarkdownDescription: "Cached image identifier. This will generally be the image's SHA256 digest.",
				Computed:            true,
			},
			"image": schema.StringAttribute{
				MarkdownDescription: "Outputs the cached image repo@digest if it exists, and builder image otherwise.",
				Computed:            true,
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
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *http.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

func (r *CachedImageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CachedImageResourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

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
		resp.Diagnostics.AddWarning("Cached Image Not Found", fmt.Sprintf("Unable to check for cached image: %s", err.Error()))
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

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CachedImageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
}

func (r *CachedImageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
}

// runCacheProbe performs a 'fake build' of the requested image and ensures that
// all of the resulting layers of the image are present in the configured cache
// repo. Otherwise, returns an error.
func (r *CachedImageResource) runCacheProbe(ctx context.Context, data CachedImageResourceModel) (v1.Image, error) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "envbuilder-provider-cached-image-data-source")
	if err != nil {
		return nil, fmt.Errorf("Unable to create temp directory: %s", err.Error())
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
		return nil, fmt.Errorf("Failed to fetch the envbuilder binary from the builder image: %s", err.Error())
	}

	workspaceFolder := data.WorkspaceFolder.ValueString()
	if workspaceFolder == "" {
		workspaceFolder = filepath.Join(tmpDir, "workspace")
		tflog.Debug(ctx, "workspace_folder not specified, using temp dir", map[string]any{"workspace_folder": workspaceFolder})
	}

	// TODO: check if this is a "plan" or "apply", and only run envbuilder on "apply".
	// This may require changing this to be a resource instead of a data source.
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
		SSLCertBase64:        data.SSLCertBase64.ValueString(),

		// Other options
		BaseImageCacheDir:  data.BaseImageCacheDir.ValueString(),
		BinaryPath:         envbuilderPath,                        // needed to reproduce the final layer.
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
		PushImage:           false, // This is only relevant when building.
		SetupScript:         "",
		SkipRebuild:         false,
	}

	return envbuilder.RunCacheProbe(ctx, opts)
}

// extractEnvbuilderFromImage reads the image located at imgRef and extracts
// MagicBinaryLocation to destPath.
func extractEnvbuilderFromImage(ctx context.Context, imgRef, destPath string) error {
	needle := filepath.Clean(constants.MagicBinaryLocation)[1:] // skip leading '/'
	ref, err := name.ParseReference(imgRef)
	if err != nil {
		return fmt.Errorf("parse reference: %w", err)
	}

	img, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
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