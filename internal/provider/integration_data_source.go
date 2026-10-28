package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/kubesense-ai/terraform-provider-kubesense/internal/client"
)

var (
	_ datasource.DataSourceWithConfigure        = &integrationDataSource{}
	_ datasource.DataSourceWithConfigValidators = &integrationDataSource{}
)

// NewIntegrationDataSource returns the kubesense_integration data source.
func NewIntegrationDataSource() datasource.DataSource { return &integrationDataSource{} }

type integrationDataSource struct{ client *client.Client }

type integrationDataModel struct {
	ID                  types.String `tfsdk:"id"`
	IntegrationProvider types.String `tfsdk:"integration_provider"`
	Name                types.String `tfsdk:"name"`
	IntegrationType     types.String `tfsdk:"integration_type"`
	Config              types.Map    `tfsdk:"config"`
	Enabled             types.Bool   `tfsdk:"enabled"`
	Status              types.String `tfsdk:"status"`
}

func (d *integrationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_integration"
}

func (d *integrationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a platform integration by id, or by provider and name. Works for GitHub, " +
			"which can only be connected in the UI.",
		Attributes: map[string]schema.Attribute{
			"id":                   schema.StringAttribute{Description: "Integration id.", Optional: true, Computed: true},
			"integration_provider": schema.StringAttribute{Description: "Provider, e.g. jira or github.", Optional: true, Computed: true},
			"name":                 schema.StringAttribute{Description: "Name.", Optional: true, Computed: true},
			"integration_type":     schema.StringAttribute{Description: "native or mcp.", Computed: true},
			"config":               schema.MapAttribute{Description: "Connection settings, secrets excluded.", ElementType: types.StringType, Computed: true},
			"enabled":              schema.BoolAttribute{Description: "Whether the integration is in use.", Computed: true},
			"status":               schema.StringAttribute{Description: "pending, healthy or error.", Computed: true},
		},
	}
}

func (d *integrationDataSource) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
		datasourcevalidator.RequiredTogether(path.MatchRoot("integration_provider"), path.MatchRoot("name")),
	}
}

func (d *integrationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (d *integrationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg integrationDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var it *client.Integration
	var err error
	if id := cfg.ID.ValueString(); id != "" {
		it, err = d.client.GetIntegration(ctx, id)
	} else {
		it, err = d.byName(ctx, cfg.IntegrationProvider.ValueString(), cfg.Name.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading integration failed", err.Error())
		return
	}

	config, diags := types.MapValueFrom(ctx, types.StringType, client.ConfigStrings(it.Config))
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, integrationDataModel{
		ID:                  types.StringValue(it.ID),
		IntegrationProvider: types.StringValue(it.Provider),
		Name:                types.StringValue(it.Name),
		IntegrationType:     types.StringValue(it.IntegrationType),
		Config:              config,
		Enabled:             types.BoolValue(it.Enabled),
		Status:              types.StringValue(it.Status),
	})...)
}

func (d *integrationDataSource) byName(ctx context.Context, provider, name string) (*client.Integration, error) {
	all, err := d.client.ListIntegrations(ctx, provider)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].Name == name {
			return &all[i], nil
		}
	}
	return nil, fmt.Errorf("no %s integration named %q", provider, name)
}
