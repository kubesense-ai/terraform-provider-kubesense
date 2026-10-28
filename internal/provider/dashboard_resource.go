package provider

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/kubesense-ai/terraform-provider-kubesense/internal/client"
	"github.com/kubesense-ai/terraform-provider-kubesense/internal/kschema/terraform"
)

var (
	_ resource.ResourceWithConfigure      = &dashboardResource{}
	_ resource.ResourceWithImportState    = &dashboardResource{}
	_ resource.ResourceWithValidateConfig = &dashboardResource{}
	_ resource.ResourceWithModifyPlan     = &dashboardResource{}
)

// The resource's own members, beside the preset's.
var dashboardOwnAttributes = []string{"id", "name", "description", "created_at", "updated_at"}

// NewDashboardResource returns the kubesense_dashboard resource: a dashboard
// whose panels, variables and layout are typed blocks, like datadog_dashboard.
func NewDashboardResource() resource.Resource { return &dashboardResource{} }

type dashboardResource struct{ client *client.Client }

func (r *dashboardResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dashboard"
}

func (r *dashboardResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	shape, err := terraform.DashboardShape()
	if err != nil {
		resp.Diagnostics.AddError("Building the dashboard schema failed", err.Error())
		return
	}
	attrs, blocks := shapeSchema(shape)
	for _, own := range dashboardOwnAttributes {
		if _, clash := attrs[own]; clash {
			resp.Diagnostics.AddError("Dashboard schema clash", "the preset has a member named "+own)
			return
		}
	}
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	attrs["id"] = schema.StringAttribute{Description: "Dashboard id.", Computed: true, PlanModifiers: keep}
	attrs["name"] = schema.StringAttribute{Description: "Dashboard name.", Required: true}
	attrs["description"] = schema.StringAttribute{Description: "Dashboard description.",
		Optional: true, Computed: true, Default: stringdefault.StaticString("")}
	attrs["created_at"] = schema.StringAttribute{Description: "When the dashboard was created (RFC 3339).",
		Computed: true, PlanModifiers: keep}
	attrs["updated_at"] = schema.StringAttribute{Description: "When the dashboard was last changed (RFC 3339).", Computed: true}

	resp.Schema = schema.Schema{
		Description: "A KubeSense dashboard, its panels, variables and layout written as blocks. " +
			"Export to Terraform in the KubeSense UI writes this for an existing dashboard. " +
			"Members the schema cannot type are set with jsonencode() in *_json attributes.",
		Attributes: attrs,
		Blocks:     blocks,
	}
}

func (r *dashboardResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

// ValidateConfig holds the configuration to the dashboard contract at plan
// time, as the server will on apply.
func (r *dashboardResource) ValidateConfig(_ context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	if !req.Config.Raw.IsFullyKnown() {
		return
	}
	// Only that the blocks convert to a preset at all. The contract is checked
	// in ModifyPlan, against the server's; see checkDashboard.
	if _, err := presetFromValue(req.Config.Raw); err != nil {
		resp.Diagnostics.AddError("Invalid dashboard", err.Error())
	}
}

// ModifyPlan checks a created or changed dashboard against the server's
// contract. See checkDashboard.
func (r *dashboardResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil || req.Plan.Raw.IsNull() || req.Plan.Raw.Equal(req.State.Raw) {
		return // destroy, no change, or the provider is not configured yet
	}
	if !req.Config.Raw.IsFullyKnown() {
		return
	}
	preset, err := presetFromValue(req.Config.Raw)
	if err != nil {
		resp.Diagnostics.AddError("Invalid dashboard", err.Error())
		return
	}
	checkDashboard(ctx, r.client, preset, func(summary, detail string) {
		resp.Diagnostics.AddError(summary, detail)
	}, &resp.Diagnostics)
}

func (r *dashboardResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	preset, err := presetFromValue(req.Plan.Raw)
	if err != nil {
		resp.Diagnostics.AddError("Invalid dashboard", err.Error())
		return
	}
	if preset, err = forServer(ctx, r.client, preset); err != nil {
		resp.Diagnostics.AddError("Converting the dashboard for this server failed", err.Error())
		return
	}
	name, description := ownStrings(req.Plan.Raw)
	id, err := r.client.CreateDashboard(ctx, client.DashboardInput{Name: name, Description: description, Preset: string(preset)})
	if err != nil {
		resp.Diagnostics.AddError("Creating dashboard failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
	r.stateFromPlan(ctx, id, req.Plan.Raw, &resp.State, &resp.Diagnostics)
}

func (r *dashboardResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var id string
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.GetDashboard(ctx, id)
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
			"System dashboards are read-only. Use the kubesense_dashboard data source to read one.")
		return
	}

	shape, err := terraform.DashboardShape()
	if err != nil {
		resp.Diagnostics.AddError("Building the dashboard schema failed", err.Error())
		return
	}
	preset, err := terraform.CanonicalPreset([]byte(d.Preset))
	if err != nil {
		resp.Diagnostics.AddError("Reading the dashboard preset failed", err.Error())
		return
	}
	stateType := resp.State.Schema.Type().TerraformType(ctx)
	v, err := treeToValue(stateType, shape, terraform.ToTerraform(shape, preset), map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, d.ID),
		"name":        tftypes.NewValue(tftypes.String, d.Name),
		"description": tftypes.NewValue(tftypes.String, d.Description),
		"created_at":  tftypes.NewValue(tftypes.String, d.CreatedAt.UTC().Format(time.RFC3339)),
		"updated_at":  tftypes.NewValue(tftypes.String, d.UpdatedAt.UTC().Format(time.RFC3339)),
	})
	if err != nil {
		resp.Diagnostics.AddError("Converting the dashboard failed", err.Error())
		return
	}
	resp.State.Raw = v
}

func (r *dashboardResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var id string
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	if resp.Diagnostics.HasError() {
		return
	}
	preset, err := presetFromValue(req.Plan.Raw)
	if err != nil {
		resp.Diagnostics.AddError("Invalid dashboard", err.Error())
		return
	}
	preset, err = r.keepPublicPath(ctx, id, preset)
	if err != nil {
		resp.Diagnostics.AddError("Reading dashboard failed", err.Error())
		return
	}
	if preset, err = forServer(ctx, r.client, preset); err != nil {
		resp.Diagnostics.AddError("Converting the dashboard for this server failed", err.Error())
		return
	}
	name, description := ownStrings(req.Plan.Raw)
	err = r.client.UpdateDashboard(ctx, client.DashboardInput{ID: id, Name: name, Description: description, Preset: string(preset)})
	if client.IsConflict(err) {
		resp.Diagnostics.AddError("Dashboard changed during apply",
			"Someone saved this dashboard while Terraform was updating it. Run terraform plan again to see their change. "+err.Error())
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Updating dashboard failed", err.Error())
		return
	}
	r.stateFromPlan(ctx, id, req.Plan.Raw, &resp.State, &resp.Diagnostics)
}

func (r *dashboardResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var id string
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteDashboard(ctx, id); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting dashboard failed", err.Error())
	}
}

func (r *dashboardResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// stateFromPlan records the planned configuration as the new state, with the
// server's id and timestamps. The configuration is what was sent, so storing
// it rather than a conversion of the server's copy keeps the state exactly as
// planned; the next read reports anything the server changed.
func (r *dashboardResource) stateFromPlan(ctx context.Context, id string, plan tftypes.Value, state *tfsdk.State, diags *diag.Diagnostics) {
	d, err := r.client.GetDashboard(ctx, id)
	if err != nil {
		diags.AddError("Reading dashboard failed", err.Error())
		return
	}
	var obj map[string]tftypes.Value
	if err := plan.As(&obj); err != nil {
		diags.AddError("Reading the plan failed", err.Error())
		return
	}
	obj["id"] = tftypes.NewValue(tftypes.String, id)
	obj["created_at"] = tftypes.NewValue(tftypes.String, d.CreatedAt.UTC().Format(time.RFC3339))
	obj["updated_at"] = tftypes.NewValue(tftypes.String, d.UpdatedAt.UTC().Format(time.RFC3339))
	state.Raw = tftypes.NewValue(plan.Type(), obj)
}

// keepPublicPath carries over the public link path, which is kept off the
// Terraform surface: someone publishing the dashboard in the UI must not have
// the link taken away by the next apply.
func (r *dashboardResource) keepPublicPath(ctx context.Context, id string, preset []byte) ([]byte, error) {
	current, err := r.client.GetDashboard(ctx, id)
	if err != nil {
		return nil, err
	}
	var cur map[string]any
	if json.Unmarshal([]byte(current.Preset), &cur) != nil || cur["publicDashboardPath"] == nil {
		return preset, nil
	}
	var next map[string]any
	if err := json.Unmarshal(preset, &next); err != nil {
		return nil, err
	}
	next["publicDashboardPath"] = cur["publicDashboardPath"]
	return json.Marshal(next)
}

// presetFromValue converts the resource's value to the preset to send:
// through the shared conversion, then migrated and given a tabs key as the
// server would store it.
func presetFromValue(v tftypes.Value) ([]byte, error) {
	shape, err := terraform.DashboardShape()
	if err != nil {
		return nil, err
	}
	tree, err := valueToTree(shape, v)
	if err != nil {
		return nil, err
	}
	obj, err := terraform.FromTerraform(shape, tree)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	canonical, err := terraform.CanonicalPreset(raw)
	if err != nil {
		return nil, err
	}
	return json.Marshal(canonical)
}

func ownStrings(v tftypes.Value) (name, description string) {
	var obj map[string]tftypes.Value
	if v.As(&obj) != nil {
		return "", ""
	}
	if n, ok := obj["name"]; ok && n.IsKnown() && !n.IsNull() {
		_ = n.As(&name)
	}
	if d, ok := obj["description"]; ok && d.IsKnown() && !d.IsNull() {
		_ = d.As(&description)
	}
	return name, description
}
