// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"

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
	BuilderImage types.String `tfsdk:"builder_image"`
	CacheRepo    types.String `tfsdk:"cache_repo"`
	CacheTTLDays types.Number `tfsdk:"cache_ttl_days"`
	Env          types.List   `tfsdk:"env"`
	Exists       types.Bool   `tfsdk:"exists"`
	ExtraEnv     types.Map    `tfsdk:"extra_env"`
	GitPassword  types.String `tfsdk:"git_password"`
	GitURL       types.String `tfsdk:"git_url"`
	GitUsername  types.String `tfsdk:"git_username"`
	ID           types.String `tfsdk:"id"`
	Image        types.String `tfsdk:"image"`
}

func (d *CachedImageDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cached_image"
}

func (d *CachedImageDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "The cached image data source can be used to retrieve a cached image produced by envbuilder.",

		Attributes: map[string]schema.Attribute{
			"builder_image": schema.StringAttribute{
				MarkdownDescription: "The builder image URL to use if the cache does not exist.",
				Required:            true,
			},
			"git_url": schema.StringAttribute{
				MarkdownDescription: "The URL of a Git repository containing a Devcontainer or Docker image to clone.",
				Required:            true,
			},
			"git_username": schema.StringAttribute{
				MarkdownDescription: "The username to use for Git authentication. This is optional.",
				Optional:            true,
			},
			"git_password": schema.StringAttribute{
				MarkdownDescription: "The password to use for Git authentication. This is optional.",
				Sensitive:           true,
				Optional:            true,
			},
			"cache_repo": schema.StringAttribute{
				MarkdownDescription: "The name of the container registry to fetch the cache image from.",
				Required:            true,
			},
			"cache_ttl_days": schema.NumberAttribute{
				MarkdownDescription: "The number of days to use cached layers before expiring them. Defaults to 7 days.",
				Optional:            true,
			},
			// TODO(mafredri): Map vs List? Support both?
			"extra_env": schema.MapAttribute{
				MarkdownDescription: "Extra environment variables to set for the container. This may include evbuilder options.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Cached image identifier",
				Computed:            true,
			},
			"exists": schema.BoolAttribute{
				MarkdownDescription: "Whether the cached image was exists or not for the given config.",
				Computed:            true,
			},
			"image": schema.StringAttribute{
				MarkdownDescription: "Outputs the cached image URL if it exists, otherwise the builder image URL is output instead.",
				Computed:            true,
			},
			// TODO(mafredri): Map vs List? Support both?
			"env": schema.ListAttribute{
				MarkdownDescription: "Computed envbuilder configuration to be set for the container.",
				ElementType:         types.StringType,
				Computed:            true,
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

	// TODO(mafredri): Implement the actual data source read logic.
	data.ID = types.StringValue("cached-image-id")
	data.Exists = types.BoolValue(false)
	data.Image = data.BuilderImage

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
