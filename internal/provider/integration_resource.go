package provider

import (
	"context"
	"maps"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/kubesense-ai/terraform-provider-kubesense/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &integrationResource{}
	_ resource.ResourceWithImportState = &integrationResource{}
)

// integrationProviders are the platform integrations a key can connect. GitHub
// is left out on purpose: it connects only through the browser app flow.
var integrationProviders = []string{"jira", "confluence", "atlassian", "custom"}

// preservedConfigKeys are config keys another route writes (the MCP tab). The
// server keeps them whatever a write sends, so they are never adopted on
// import: the next apply would otherwise try to manage them.
var preservedConfigKeys = []string{"mcp"}

const appliedConfigKey = "applied_config"

// NewIntegrationResource returns the kubesense_integration resource.
func NewIntegrationResource() resource.Resource { return &integrationResource{} }

type integrationResource struct{ client *client.Client }

type integrationModel struct {
	ID                  types.String `tfsdk:"id"`
	IntegrationProvider types.String `tfsdk:"integration_provider"`
	Name                types.String `tfsdk:"name"`
	APIKey              types.String `tfsdk:"api_key"`
	AuthScheme          types.String `tfsdk:"auth_scheme"`
	AuthHeader          types.String `tfsdk:"auth_header"`
	Config              types.Map    `tfsdk:"config"`
	Enabled             types.Bool   `tfsdk:"enabled"`
	IntegrationType     types.String `tfsdk:"integration_type"`
	APIKeyHint          types.String `tfsdk:"api_key_hint"`
	Status              types.String `tfsdk:"status"`
}

func (r *integrationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_integration"
}

func (r *integrationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "A platform integration: Jira, Confluence, Atlassian, or a custom MCP server. " +
			"KubeSense tests the connection on every create and on every change to credentials or " +
			"config, and refuses the change when the test fails.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Integration id.",
				Computed:      true,
				PlanModifiers: keep,
			},
			"integration_provider": schema.StringAttribute{
				Description: "One of jira, confluence, atlassian or custom (an MCP server). " +
					"Changing it replaces the integration. GitHub connects only through the UI.",
				Required:      true,
				Validators:    []validator.String{stringvalidator.OneOf(integrationProviders...)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Description: "Name, unique per provider.",
				Required:    true,
			},
			"api_key": schema.StringAttribute{
				Description: "API token or password. Write-only on the server: it is never read back, " +
					"so a change made outside Terraform is not detected, and it is not set on import.",
				Optional:  true,
				Sensitive: true,
			},
			"auth_scheme": schema.StringAttribute{
				Description:   "bearer, basic or header. Defaults to what the deployment uses.",
				Optional:      true,
				Computed:      true,
				Validators:    []validator.String{stringvalidator.OneOf("bearer", "basic", "header")},
				PlanModifiers: keep,
			},
			"auth_header": schema.StringAttribute{
				Description: "Header carrying the key when auth_scheme is header (custom MCP servers).",
				Optional:    true,
			},
			"config": schema.MapAttribute{
				Description: "Connection settings, including deployment (cloud, cloud_service_account, " +
					"datacenter, or remote for custom). Jira and Confluence take base_url and, for basic " +
					"auth, username; custom takes url and extra_headers = jsonencode({...}). " +
					"The whole map is sent on every change.",
				ElementType: types.StringType,
				Required:    true,
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the integration is in use.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"integration_type": schema.StringAttribute{
				Description:   "native or mcp.",
				Computed:      true,
				PlanModifiers: keep,
			},
			"api_key_hint": schema.StringAttribute{
				Description: "The last characters of the stored key.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "pending, healthy or error, from the last connection test.",
				Computed:    true,
			},
		},
	}
}

func (r *integrationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *integrationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan integrationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	cfg := mapStrings(ctx, plan.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	it, err := r.client.CreateIntegration(ctx, client.IntegrationCreate{
		Provider:   plan.IntegrationProvider.ValueString(),
		Name:       plan.Name.ValueString(),
		APIKey:     plan.APIKey.ValueString(),
		AuthScheme: plan.AuthScheme.ValueString(),
		AuthHeader: plan.AuthHeader.ValueString(),
		Config:     anyMap(cfg),
		Enabled:    plan.Enabled.ValueBoolPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating integration failed", err.Error())
		return
	}

	setIntegrationComputed(&plan, it)
	saveApplied(ctx, resp.Private, appliedConfigKey, client.ConfigStrings(it.Config), &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *integrationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state integrationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	it, err := r.client.GetIntegration(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading integration failed", err.Error())
		return
	}

	server := client.ConfigStrings(it.Config)
	var cfg map[string]string
	if state.Config.IsNull() {
		// Import: adopt what the server has, bar the keys it manages itself.
		cfg = maps.Clone(server)
		for _, k := range preservedConfigKeys {
			delete(cfg, k)
		}
	} else {
		prior := mapStrings(ctx, state.Config, &resp.Diagnostics)
		applied := loadApplied(ctx, req.Private, appliedConfigKey, &resp.Diagnostics)
		cfg = reconcile(prior, server, applied)
	}
	config, d := types.MapValueFrom(ctx, types.StringType, cfg)
	resp.Diagnostics.Append(d...)

	state.IntegrationProvider = types.StringValue(it.Provider)
	state.Name = types.StringValue(it.Name)
	state.Enabled = types.BoolValue(it.Enabled)
	state.AuthScheme = types.StringValue(it.AuthScheme)
	if it.AuthHeader != nil && *it.AuthHeader != "" {
		state.AuthHeader = types.StringValue(*it.AuthHeader)
	} else {
		state.AuthHeader = types.StringNull()
	}
	state.Config = config
	setIntegrationComputed(&state, it)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *integrationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state integrationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := integrationPatch(ctx, plan, state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	it, err := r.client.UpdateIntegration(ctx, state.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Updating integration failed", err.Error())
		return
	}

	plan.ID = state.ID
	setIntegrationComputed(&plan, it)
	saveApplied(ctx, resp.Private, appliedConfigKey, client.ConfigStrings(it.Config), &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *integrationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state integrationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteIntegration(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting integration failed", err.Error())
	}
}

func (r *integrationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// integrationPatch sends only what changed: a name-only or enabled-only change
// is then not probed again. Config goes whole, because the server replaces the
// map. The key goes only when it changed; removing it from the configuration
// keeps the stored one.
func integrationPatch(ctx context.Context, plan, state integrationModel, diags *diag.Diagnostics) client.IntegrationUpdate {
	var in client.IntegrationUpdate
	if !plan.Name.Equal(state.Name) {
		in.Name = plan.Name.ValueStringPointer()
	}
	if !plan.APIKey.IsNull() && !plan.APIKey.Equal(state.APIKey) {
		in.APIKey = plan.APIKey.ValueStringPointer()
	}
	if !plan.AuthScheme.IsUnknown() && !plan.AuthScheme.Equal(state.AuthScheme) {
		in.AuthScheme = plan.AuthScheme.ValueStringPointer()
	}
	if !plan.AuthHeader.Equal(state.AuthHeader) {
		header := plan.AuthHeader.ValueString()
		in.AuthHeader = &header
	}
	if !plan.Config.Equal(state.Config) {
		in.Config = anyMap(mapStrings(ctx, plan.Config, diags))
	}
	if !plan.Enabled.Equal(state.Enabled) {
		in.Enabled = plan.Enabled.ValueBoolPointer()
	}
	return in
}

func setIntegrationComputed(m *integrationModel, it *client.Integration) {
	m.ID = types.StringValue(it.ID)
	m.AuthScheme = types.StringValue(it.AuthScheme)
	m.IntegrationType = types.StringValue(it.IntegrationType)
	m.Status = types.StringValue(it.Status)
	m.APIKeyHint = types.StringPointerValue(it.APIKeyHint)
}

func mapStrings(ctx context.Context, m types.Map, diags *diag.Diagnostics) map[string]string {
	out := map[string]string{}
	if m.IsNull() || m.IsUnknown() {
		return out
	}
	diags.Append(m.ElementsAs(ctx, &out, false)...)
	return out
}

func anyMap(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
