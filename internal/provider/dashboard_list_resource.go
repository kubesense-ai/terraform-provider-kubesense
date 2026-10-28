package provider

import (
	"context"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/kubesense-ai/terraform-provider-kubesense/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &dashboardListResource{}
	_ resource.ResourceWithImportState = &dashboardListResource{}
)

// NewDashboardListResource returns the kubesense_dashboard_list resource.
func NewDashboardListResource() resource.Resource { return &dashboardListResource{} }

type dashboardListResource struct{ client *client.Client }

type dashboardListModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	DashboardIDs types.Set    `tfsdk:"dashboard_ids"`
}

func (r *dashboardListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dashboard_list"
}

func (r *dashboardListResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A dashboard list: a named group of dashboards, shown as a folder in the UI. " +
			"A dashboard can be in several lists.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "List id.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Description: "List name. Unique across lists.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "List description.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
			},
			"dashboard_ids": schema.SetAttribute{
				Description: "Ids of the dashboards in the list. When set, Terraform owns membership: " +
					"dashboards added in the UI are removed on the next apply. When omitted, membership " +
					"is left to the UI.",
				ElementType:   types.StringType,
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *dashboardListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *dashboardListResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dashboardListModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := r.client.CreateDashboardList(ctx, plan.Name.ValueString(), plan.Description.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Creating dashboard list failed", err.Error())
		return
	}
	plan.ID = types.StringValue(id)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), plan.ID)...)

	if wanted, managed := setStrings(ctx, plan.DashboardIDs, &resp.Diagnostics); managed {
		if err := r.client.AddDashboardsToList(ctx, id, wanted); err != nil {
			resp.Diagnostics.AddError("Adding dashboards to list failed", err.Error())
			return
		}
	}
	r.readMembers(ctx, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *dashboardListResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dashboardListModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	l, err := r.client.GetDashboardList(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading dashboard list failed", err.Error())
		return
	}
	state.Name = types.StringValue(l.Name)
	state.Description = types.StringValue(l.Description)
	r.readMembers(ctx, &state, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *dashboardListResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state dashboardListModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()

	if !plan.Name.Equal(state.Name) || !plan.Description.Equal(state.Description) {
		if err := r.client.UpdateDashboardList(ctx, id, plan.Name.ValueString(), plan.Description.ValueString()); err != nil {
			resp.Diagnostics.AddError("Updating dashboard list failed", err.Error())
			return
		}
	}

	wanted, managed := setStrings(ctx, plan.DashboardIDs, &resp.Diagnostics)
	if managed {
		current, _ := setStrings(ctx, state.DashboardIDs, &resp.Diagnostics)
		add, remove := diffIDs(current, wanted)
		if err := r.client.AddDashboardsToList(ctx, id, add); err != nil {
			resp.Diagnostics.AddError("Adding dashboards to list failed", err.Error())
			return
		}
		if err := r.client.RemoveDashboardsFromList(ctx, id, remove); err != nil {
			resp.Diagnostics.AddError("Removing dashboards from list failed", err.Error())
			return
		}
	}

	plan.ID = state.ID
	r.readMembers(ctx, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *dashboardListResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dashboardListModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteDashboardList(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting dashboard list failed", err.Error())
	}
}

func (r *dashboardListResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *dashboardListResource) readMembers(ctx context.Context, m *dashboardListModel, diags *diag.Diagnostics) {
	ids, err := r.client.DashboardListMembers(ctx, m.ID.ValueString())
	if err != nil {
		diags.AddError("Reading dashboard list members failed", err.Error())
		return
	}
	set, d := types.SetValueFrom(ctx, types.StringType, ids)
	diags.Append(d...)
	m.DashboardIDs = set
}

// setStrings returns a set's elements, and whether the set is known: null or
// unknown means the configuration does not manage membership.
func setStrings(ctx context.Context, s types.Set, diags *diag.Diagnostics) ([]string, bool) {
	if s.IsNull() || s.IsUnknown() {
		return nil, false
	}
	var out []string
	diags.Append(s.ElementsAs(ctx, &out, false)...)
	return out, true
}

// diffIDs returns what to add to current, and what to remove from it, to reach
// wanted.
func diffIDs(current, wanted []string) (add, remove []string) {
	for _, id := range wanted {
		if !slices.Contains(current, id) {
			add = append(add, id)
		}
	}
	for _, id := range current {
		if !slices.Contains(wanted, id) {
			remove = append(remove, id)
		}
	}
	return add, remove
}
