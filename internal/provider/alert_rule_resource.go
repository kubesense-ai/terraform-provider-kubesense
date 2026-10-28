package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/kubesense-ai/terraform-provider-kubesense/internal/client"
	"github.com/kubesense-ai/terraform-provider-kubesense/internal/kschema/terraform"
)

var (
	_ resource.ResourceWithConfigure   = &alertRuleResource{}
	_ resource.ResourceWithImportState = &alertRuleResource{}
)

// NewAlertRuleResource returns the kubesense_alert_rule resource: an alert
// rule's definition as typed blocks, the way datadog_monitor writes a monitor.
func NewAlertRuleResource() resource.Resource { return &alertRuleResource{} }

type alertRuleResource struct{ client *client.Client }

func (r *alertRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_rule"
}

func (r *alertRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	shape, err := terraform.AlertRuleShape()
	if err != nil {
		resp.Diagnostics.AddError("Building the alert rule schema failed", err.Error())
		return
	}
	attrs, blocks := shapeSchema(shape)
	if _, clash := attrs["id"]; clash {
		resp.Diagnostics.AddError("Alert rule schema clash", "the rule definition has a member named id")
		return
	}
	attrs["id"] = schema.StringAttribute{
		Description:   "Alert rule id.",
		Computed:      true,
		PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
	resp.Schema = schema.Schema{
		Description: "A KubeSense alert rule, written as blocks. Export to Terraform on an alert rule " +
			"writes this for an existing rule. Notification channels are named, not numbered. " +
			"Members the schema cannot type are set with jsonencode() in *_json attributes.",
		Attributes: attrs,
		Blocks:     blocks,
	}
}

func (r *alertRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *alertRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	def, err := alertDefinitionFromValue(req.Plan.Raw)
	if err != nil {
		resp.Diagnostics.AddError("Invalid alert rule", err.Error())
		return
	}
	id, err := r.client.CreateAlertRule(ctx, def)
	if err != nil {
		resp.Diagnostics.AddError("Creating alert rule failed", err.Error())
		return
	}
	stateWithID(req.Plan.Raw, id, &resp.State, &resp.Diagnostics)
}

func (r *alertRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var id string
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	if resp.Diagnostics.HasError() {
		return
	}
	def, err := r.client.GetAlertRule(ctx, id)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading alert rule failed", err.Error())
		return
	}
	shape, err := terraform.AlertRuleShape()
	if err != nil {
		resp.Diagnostics.AddError("Building the alert rule schema failed", err.Error())
		return
	}
	server := terraform.ToTerraform(shape, def)

	// The server fills defaults the configuration may leave out (enabled,
	// sample_limit, a query's visible). Reporting those would show a change on
	// every plan, so only members the configuration manages are read back;
	// a change to one of those still shows as drift. An import (no name in
	// state yet) adopts everything.
	prior, err := valueToTree(shape, req.State.Raw)
	if err != nil {
		resp.Diagnostics.AddError("Reading state failed", err.Error())
		return
	}
	if _, managed := prior["name"]; managed {
		server = keepConfigured(shape, prior, server)
	}

	v, err := treeToValue(resp.State.Schema.Type().TerraformType(ctx), shape, server,
		map[string]tftypes.Value{"id": tftypes.NewValue(tftypes.String, id)})
	if err != nil {
		resp.Diagnostics.AddError("Converting the alert rule failed", err.Error())
		return
	}
	resp.State.Raw = v
}

func (r *alertRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var id string
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	if resp.Diagnostics.HasError() {
		return
	}
	def, err := alertDefinitionFromValue(req.Plan.Raw)
	if err != nil {
		resp.Diagnostics.AddError("Invalid alert rule", err.Error())
		return
	}
	if err := r.client.UpdateAlertRule(ctx, id, def); err != nil {
		resp.Diagnostics.AddError("Updating alert rule failed", err.Error())
		return
	}
	stateWithID(req.Plan.Raw, id, &resp.State, &resp.Diagnostics)
}

func (r *alertRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var id string
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteAlertRule(ctx, id); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting alert rule failed", err.Error())
	}
}

func (r *alertRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func alertDefinitionFromValue(v tftypes.Value) (map[string]any, error) {
	shape, err := terraform.AlertRuleShape()
	if err != nil {
		return nil, err
	}
	tree, err := valueToTree(shape, v)
	if err != nil {
		return nil, err
	}
	return terraform.FromTerraform(shape, tree)
}

// stateWithID records the plan as the new state with the rule's id: what was
// sent is what the configuration says, and the next read reports anything
// the server changed.
func stateWithID(plan tftypes.Value, id string, state *tfsdk.State, diags *diag.Diagnostics) {
	var obj map[string]tftypes.Value
	if err := plan.As(&obj); err != nil {
		diags.AddError("Reading the plan failed", err.Error())
		return
	}
	obj["id"] = tftypes.NewValue(tftypes.String, id)
	state.Raw = tftypes.NewValue(plan.Type(), obj)
}

// keepConfigured narrows a value read from the server to the members the
// prior state sets. Repeated blocks are matched by position; entries beyond
// the prior ones are kept whole, so a block added outside Terraform shows up.
func keepConfigured(n *terraform.Node, prior, server map[string]any) map[string]any {
	out := map[string]any{}
	for _, f := range n.Fields {
		sv, ok := server[f.Name]
		if !ok {
			continue
		}
		pv, had := prior[f.Name]
		if !had || pv == nil {
			continue
		}
		switch f.Kind {
		case terraform.KindBlock:
			pm, _ := pv.(map[string]any)
			sm, _ := sv.(map[string]any)
			out[f.Name] = keepConfigured(f, pm, sm)
		case terraform.KindBlocks:
			pl, _ := pv.([]any)
			sl, _ := sv.([]any)
			items := make([]any, 0, len(sl))
			for i, item := range sl {
				sm, _ := item.(map[string]any)
				if i < len(pl) {
					pm, _ := pl[i].(map[string]any)
					items = append(items, keepConfigured(f, pm, sm))
				} else {
					items = append(items, sm)
				}
			}
			out[f.Name] = items
		default:
			out[f.Name] = sv
		}
	}
	return out
}
