package provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/kubesense-ai/terraform-provider-kubesense/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &dashboardJSONResource{}
	_ resource.ResourceWithImportState = &dashboardJSONResource{}
	_ resource.ResourceWithModifyPlan  = &dashboardJSONResource{}
)

// NewDashboardJSONResource returns the kubesense_dashboard_json resource: a
// dashboard managed as one JSON string, like datadog_dashboard_json. The
// escape hatch beside the typed kubesense_dashboard.
func NewDashboardJSONResource() resource.Resource { return &dashboardJSONResource{} }

type dashboardJSONResource struct{ client *client.Client }

type dashboardModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Description   types.String `tfsdk:"description"`
	DashboardJSON presetValue  `tfsdk:"dashboard_json"`
	CreatedAt     types.String `tfsdk:"created_at"`
	UpdatedAt     types.String `tfsdk:"updated_at"`
}

func (r *dashboardJSONResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dashboard_json"
}

func (r *dashboardJSONResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A KubeSense dashboard managed as its preset JSON. Use kubesense_dashboard to write panels as blocks instead.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Dashboard id.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Description: "Dashboard name.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "Dashboard description.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"dashboard_json": schema.StringAttribute{
				Description: "The dashboard preset as JSON: panels, layout, variables and tabs. " +
					"Use the file from the dashboard's Export to Terraform action, or " +
					"jsondecode(file(\"export.json\")).preset for a file from Export. " +
					"Checked against the server's dashboard schema at plan time.",
				Required:   true,
				CustomType: presetType{},
				Validators: []validator.String{presetValidator{}},
			},
			"created_at": schema.StringAttribute{
				Description:   "When the dashboard was created (RFC 3339).",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"updated_at": schema.StringAttribute{
				Description: "When the dashboard was last changed (RFC 3339).",
				Computed:    true,
			},
		},
	}
}

func (r *dashboardJSONResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

// ModifyPlan checks a created or changed dashboard_json against the server's
// contract. See checkDashboard.
func (r *dashboardJSONResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil || req.Plan.Raw.IsNull() || req.Plan.Raw.Equal(req.State.Raw) {
		return // destroy, no change, or the provider is not configured yet
	}
	var plan dashboardModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || plan.DashboardJSON.IsUnknown() || plan.DashboardJSON.IsNull() {
		return
	}
	preset, err := normalisePreset(plan.DashboardJSON.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("dashboard_json"), "Invalid dashboard JSON", err.Error())
		return
	}
	checkDashboard(ctx, r.client, preset, func(summary, detail string) {
		resp.Diagnostics.AddAttributeError(path.Root("dashboard_json"), summary, detail)
	}, &resp.Diagnostics)
}

func (r *dashboardJSONResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dashboardModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	preset, err := normalisePreset(plan.DashboardJSON.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("dashboard_json"), "Invalid dashboard JSON", err.Error())
		return
	}
	if preset, err = forServer(ctx, r.client, preset); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("dashboard_json"), "Converting the dashboard for this server failed", err.Error())
		return
	}
	id, err := r.client.CreateDashboard(ctx, client.DashboardInput{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
		Preset:      string(preset),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating dashboard failed", err.Error())
		return
	}
	// Recorded before the read below, so a failed read leaves a tainted
	// resource Terraform can clean up rather than an orphan it cannot see.
	plan.ID = types.StringValue(id)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), plan.ID)...)

	d, err := r.client.GetDashboard(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Reading created dashboard failed", err.Error())
		return
	}
	setTimestamps(&plan, d)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *dashboardJSONResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dashboardModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d, err := r.client.GetDashboard(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading dashboard failed", err.Error())
		return
	}
	if d.Type == "system" {
		resp.Diagnostics.AddError("System dashboards cannot be managed",
			"System dashboards are read-only. Use the kubesense_dashboard data source to read one, "+
				"and create your own dashboard from its dashboard_json to customise it.")
		return
	}

	state.Name = types.StringValue(d.Name)
	state.Description = types.StringValue(d.Description)
	state.DashboardJSON = newPresetValue(d.Preset)
	setTimestamps(&state, d)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *dashboardJSONResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state dashboardModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	preset, err := normalisePreset(plan.DashboardJSON.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("dashboard_json"), "Invalid dashboard JSON", err.Error())
		return
	}
	if preset, err = forServer(ctx, r.client, preset); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("dashboard_json"), "Converting the dashboard for this server failed", err.Error())
		return
	}
	err = r.client.UpdateDashboard(ctx, client.DashboardInput{
		ID:          state.ID.ValueString(),
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
		Preset:      string(preset),
	})
	if client.IsConflict(err) {
		resp.Diagnostics.AddError("Dashboard changed during apply",
			"Someone saved this dashboard while Terraform was updating it. Run terraform plan again to see their change. "+err.Error())
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Updating dashboard failed", err.Error())
		return
	}

	d, err := r.client.GetDashboard(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading updated dashboard failed", err.Error())
		return
	}
	plan.ID = state.ID
	setTimestamps(&plan, d)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *dashboardJSONResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dashboardModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteDashboard(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting dashboard failed", err.Error())
	}
}

func (r *dashboardJSONResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func setTimestamps(m *dashboardModel, d *client.Dashboard) {
	m.CreatedAt = types.StringValue(d.CreatedAt.UTC().Format(time.RFC3339))
	m.UpdatedAt = types.StringValue(d.UpdatedAt.UTC().Format(time.RFC3339))
}
