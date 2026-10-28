package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/kubesense-ai/terraform-provider-kubesense/internal/client"
)

var (
	_ datasource.DataSourceWithConfigure        = &dashboardDataSource{}
	_ datasource.DataSourceWithConfigValidators = &dashboardDataSource{}
)

// NewDashboardDataSource returns the kubesense_dashboard data source.
func NewDashboardDataSource() datasource.DataSource { return &dashboardDataSource{} }

type dashboardDataSource struct{ client *client.Client }

type dashboardDataModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Description   types.String `tfsdk:"description"`
	DashboardJSON types.String `tfsdk:"dashboard_json"`
	Type          types.String `tfsdk:"type"`
}

func (d *dashboardDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dashboard"
}

func (d *dashboardDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a dashboard by id or exact name, including system dashboards.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Description: "Dashboard id.", Optional: true, Computed: true},
			"name":           schema.StringAttribute{Description: "Exact dashboard name.", Optional: true, Computed: true},
			"description":    schema.StringAttribute{Description: "Dashboard description.", Computed: true},
			"dashboard_json": schema.StringAttribute{Description: "The dashboard preset as JSON.", Computed: true},
			"type":           schema.StringAttribute{Description: `"user" or "system".`, Computed: true},
		},
	}
}

func (d *dashboardDataSource) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *dashboardDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (d *dashboardDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg dashboardDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := cfg.ID.ValueString()
	if id == "" {
		var err error
		if id, err = d.idByName(ctx, cfg.Name.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("name"), "Dashboard lookup failed", err.Error())
			return
		}
	}

	dash, err := d.client.GetDashboard(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Reading dashboard failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, dashboardDataModel{
		ID:            types.StringValue(dash.ID),
		Name:          types.StringValue(dash.Name),
		Description:   types.StringValue(dash.Description),
		DashboardJSON: types.StringValue(dash.Preset),
		Type:          types.StringValue(dash.Type),
	})...)
}

// idByName finds the one dashboard with exactly this name. Search is a
// substring match on the server, so the exact match is picked out here.
func (d *dashboardDataSource) idByName(ctx context.Context, name string) (string, error) {
	found, err := d.client.ListDashboards(ctx, client.DashboardQuery{Search: name})
	if err != nil {
		return "", err
	}
	var ids []string
	for _, dash := range found {
		if dash.Name == name {
			ids = append(ids, dash.ID)
		}
	}
	switch len(ids) {
	case 0:
		return "", fmt.Errorf("no dashboard named %q is visible to this API key", name)
	case 1:
		return ids[0], nil
	default:
		return "", fmt.Errorf("%d dashboards are named %q (%s); look one up by id instead",
			len(ids), name, strings.Join(ids, ", "))
	}
}
