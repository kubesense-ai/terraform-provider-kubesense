package provider

import (
	"context"
	"fmt"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/kubesense-ai/terraform-provider-kubesense/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &cloudAccountResource{}
	_ resource.ResourceWithImportState = &cloudAccountResource{}
)

const appliedCredentialsKey = "applied_credentials"

// cloudAccountResource is every cloud account resource, parameterised by its
// spec. Credential attributes are named by the spec, so the model is read and
// written attribute by attribute rather than through a struct.
type cloudAccountResource struct {
	spec   cloudSpec
	client *client.Client
}

// cloudFlagModel is one entry of resource_types.
type cloudFlagModel struct {
	Collect       types.Bool   `tfsdk:"collect"`
	MetricEnabled types.Bool   `tfsdk:"metric_enabled"`
	MetricTier    types.String `tfsdk:"metric_tier"`
	MetricExtras  types.Set    `tfsdk:"metric_extras"`
}

var cloudFlagType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"collect":        types.BoolType,
	"metric_enabled": types.BoolType,
	"metric_tier":    types.StringType,
	"metric_extras":  types.SetType{ElemType: types.StringType},
}}

func (r *cloudAccountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.spec.typeSuffix
}

func (r *cloudAccountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}

	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Description:   "Integration id (the API's kuid).",
			Computed:      true,
			PlanModifiers: keep,
		},
		r.spec.accountAttr: schema.StringAttribute{
			Description:   r.spec.accountDesc + " Changing it creates a new integration.",
			Required:      true,
			PlanModifiers: replace,
		},
		"resource_types": schema.MapNestedAttribute{
			Description: "Resource types to collect, keyed by resource type (see Settings → Integrations " +
				"for the names). When set, Terraform owns collection: types enabled in the UI but not " +
				"listed here are switched off on the next apply. When omitted, collection is left to the UI.",
			Optional: true,
			NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"collect":        schema.BoolAttribute{Description: "Collect inventory.", Required: true},
				"metric_enabled": schema.BoolAttribute{Description: "Collect metrics.", Required: true},
				"metric_tier": schema.StringAttribute{
					Description: "core (the default) or expanded.",
					Optional:    true,
					Validators:  []validator.String{stringvalidator.OneOf("core", "expanded")},
				},
				"metric_extras": schema.SetAttribute{
					Description: "Extra metric names beyond the tier.",
					ElementType: types.StringType,
					Optional:    true,
				},
			}},
		},
		"active": schema.BoolAttribute{
			Description: "Whether collection runs. false pauses it and keeps credentials and settings.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(true),
		},
		"display_name": schema.StringAttribute{
			Description: "Name shown in the UI: the account name the credentials resolved to.",
			Computed:    true,
		},
		"status": schema.StringAttribute{
			Description: "pending, healthy, error, paused or deleting.",
			Computed:    true,
		},
	}
	for _, f := range r.spec.fields {
		a := schema.StringAttribute{
			Description: f.description,
			Optional:    true,
			Sensitive:   f.secret,
			Validators:  f.validators,
		}
		if f.secret {
			a.Description += " Write-only on the server, so a change made outside Terraform is not detected, and it is not set on import."
		}
		if f.identity {
			a.PlanModifiers = replace
		}
		attrs[f.name] = a
	}

	resp.Schema = schema.Schema{
		Description: fmt.Sprintf("A %s integration. KubeSense checks the credentials when the integration "+
			"is created and refuses ones that fail or belong to another account.", r.spec.noun),
		Attributes: attrs,
	}
}

func (r *cloudAccountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *cloudAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	account := getString(ctx, req.Plan, r.spec.accountAttr, &resp.Diagnostics)
	fields := r.getFields(ctx, req.Plan, &resp.Diagnostics)
	flags := getFlags(ctx, req.Plan, &resp.Diagnostics)
	active := getBool(ctx, req.Plan, "active", &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	creds := map[string]string{"account_id": account.ValueString()}
	if r.spec.accountAttr != "account_id" {
		creds[r.spec.accountAttr] = account.ValueString()
	}
	for name, v := range fields {
		if !v.IsNull() && v.ValueString() != "" {
			creds[name] = v.ValueString()
		}
	}

	kuid, err := r.client.CreateCloudDomain(ctx, client.CloudDomainCreate{
		Provider:    r.spec.provider,
		AccountID:   account.ValueString(),
		Credentials: creds,
		Flags:       flagsToAPI(ctx, flags, nil, &resp.Diagnostics),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating "+r.spec.noun+" integration failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), kuid)...)

	if !active.ValueBool() {
		if err := r.client.SetCloudDomainActive(ctx, kuid, false); err != nil {
			resp.Diagnostics.AddError("Pausing "+r.spec.noun+" integration failed", err.Error())
			return
		}
	}

	r.writeState(ctx, &resp.State, account, fields, flags, active, &resp.Diagnostics)
	r.refreshComputed(ctx, kuid, &resp.State, resp.Private, &resp.Diagnostics)
}

func (r *cloudAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	kuid := getString(ctx, req.State, "id", &resp.Diagnostics).ValueString()
	prior := r.getFields(ctx, req.State, &resp.Diagnostics)
	priorFlags := getFlags(ctx, req.State, &resp.Diagnostics)
	importing := getString(ctx, req.State, r.spec.accountAttr, &resp.Diagnostics).IsNull()
	if resp.Diagnostics.HasError() {
		return
	}

	d, err := r.client.GetCloudDomain(ctx, kuid)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading "+r.spec.noun+" integration failed", err.Error())
		return
	}
	if d.Provider != r.spec.provider {
		resp.Diagnostics.AddError("Wrong integration type",
			fmt.Sprintf("%s is a %s integration, not %s.", kuid, d.Provider, r.spec.provider))
		return
	}
	server, err := r.client.GetCloudCredentials(ctx, kuid)
	if err != nil {
		resp.Diagnostics.AddError("Reading credentials failed", err.Error())
		return
	}
	serverFlags, err := r.client.GetCloudFlags(ctx, kuid)
	if err != nil {
		resp.Diagnostics.AddError("Reading resource types failed", err.Error())
		return
	}

	applied := loadApplied(ctx, req.Private, appliedCredentialsKey, &resp.Diagnostics)
	fields := make(map[string]types.String, len(r.spec.fields))
	for _, f := range r.spec.fields {
		fields[f.name] = readCredential(prior[f.name], server, applied, f.name, importing)
	}
	flags := readFlags(ctx, priorFlags, serverFlags, importing, &resp.Diagnostics)

	// The account id is read from the credentials when it is there: Kong's
	// domain stores the control plane id as its account_id, not the org id the
	// configuration names.
	account := d.AccountID
	if v, ok := server[r.spec.accountAttr]; ok && v != "" {
		account = v
	}
	r.writeState(ctx, &resp.State, types.StringValue(account), fields, flags, types.BoolValue(d.Active), &resp.Diagnostics)
	setComputed(ctx, &resp.State, d, &resp.Diagnostics)
}

func (r *cloudAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	kuid := getString(ctx, req.State, "id", &resp.Diagnostics).ValueString()
	account := getString(ctx, req.Plan, r.spec.accountAttr, &resp.Diagnostics)
	planFields := r.getFields(ctx, req.Plan, &resp.Diagnostics)
	stateFields := r.getFields(ctx, req.State, &resp.Diagnostics)
	planFlags := getFlags(ctx, req.Plan, &resp.Diagnostics)
	stateFlags := getFlags(ctx, req.State, &resp.Diagnostics)
	active := getBool(ctx, req.Plan, "active", &resp.Diagnostics)
	wasActive := getBool(ctx, req.State, "active", &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Credentials merge on the server, so only changed keys are sent. A key
	// removed from the configuration stays stored.
	changed := map[string]string{}
	for name, v := range planFields {
		if !v.IsNull() && !v.Equal(stateFields[name]) {
			changed[name] = v.ValueString()
		}
	}
	if err := r.client.UpdateCloudCredentials(ctx, kuid, changed); err != nil {
		resp.Diagnostics.AddError("Updating credentials failed", err.Error())
		return
	}

	if planFlags != nil {
		// Terraform owns the full set once it lists any type, so every type the
		// server has on, not only the ones state knew about, is switched off
		// unless listed. State has none when resource_types was unset until now,
		// and taking control must not need a second apply.
		server, err := r.client.GetCloudFlags(ctx, kuid)
		if err != nil {
			resp.Diagnostics.AddError("Reading resource types failed", err.Error())
			return
		}
		had := map[string]cloudFlagModel{}
		for name, m := range stateFlags {
			had[name] = m
		}
		for _, f := range server {
			if f.Collect || f.MetricEnabled {
				had[f.ResourceType] = cloudFlagModel{}
			}
		}
		if err := r.client.UpdateCloudFlags(ctx, kuid, flagsToAPI(ctx, planFlags, had, &resp.Diagnostics)); err != nil {
			resp.Diagnostics.AddError("Updating resource types failed", err.Error())
			return
		}
	}

	if !active.Equal(wasActive) {
		if err := r.client.SetCloudDomainActive(ctx, kuid, active.ValueBool()); err != nil {
			resp.Diagnostics.AddError("Changing collection state failed", err.Error())
			return
		}
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), kuid)...)
	r.writeState(ctx, &resp.State, account, planFields, planFlags, active, &resp.Diagnostics)
	r.refreshComputed(ctx, kuid, &resp.State, resp.Private, &resp.Diagnostics)
}

func (r *cloudAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	kuid := getString(ctx, req.State, "id", &resp.Diagnostics).ValueString()
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteCloudDomain(ctx, kuid); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting "+r.spec.noun+" integration failed", err.Error())
	}
}

func (r *cloudAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *cloudAccountResource) getFields(ctx context.Context, src attrGetter, diags *diag.Diagnostics) map[string]types.String {
	out := make(map[string]types.String, len(r.spec.fields))
	for _, f := range r.spec.fields {
		out[f.name] = getString(ctx, src, f.name, diags)
	}
	return out
}

func (r *cloudAccountResource) writeState(ctx context.Context, st *tfsdk.State, account types.String, fields map[string]types.String,
	flags map[string]cloudFlagModel, active types.Bool, diags *diag.Diagnostics,
) {
	diags.Append(st.SetAttribute(ctx, path.Root(r.spec.accountAttr), account)...)
	for name, v := range fields {
		diags.Append(st.SetAttribute(ctx, path.Root(name), v)...)
	}
	if flags == nil {
		diags.Append(st.SetAttribute(ctx, path.Root("resource_types"), types.MapNull(cloudFlagType))...)
	} else {
		m, d := types.MapValueFrom(ctx, cloudFlagType, flags)
		diags.Append(d...)
		diags.Append(st.SetAttribute(ctx, path.Root("resource_types"), m)...)
	}
	diags.Append(st.SetAttribute(ctx, path.Root("active"), active)...)
}

// refreshComputed reads back what the server decided after a write: the
// resolved display name and status, and the credentials as stored, recorded
// so the next read can tell the server's normalising from a real change.
func (r *cloudAccountResource) refreshComputed(ctx context.Context, kuid string, st *tfsdk.State, private privateSetter, diags *diag.Diagnostics) {
	d, err := r.client.GetCloudDomain(ctx, kuid)
	if err != nil {
		diags.AddError("Reading "+r.spec.noun+" integration failed", err.Error())
		return
	}
	setComputed(ctx, st, d, diags)

	creds, err := r.client.GetCloudCredentials(ctx, kuid)
	if err != nil {
		diags.AddError("Reading credentials failed", err.Error())
		return
	}
	plain := map[string]string{}
	for k, v := range creds {
		if v != client.MaskedSecret {
			plain[k] = v
		}
	}
	saveApplied(ctx, private, appliedCredentialsKey, plain, diags)
}

func setComputed(ctx context.Context, st *tfsdk.State, d *client.CloudDomain, diags *diag.Diagnostics) {
	diags.Append(st.SetAttribute(ctx, path.Root("display_name"), d.DisplayName)...)
	diags.Append(st.SetAttribute(ctx, path.Root("status"), d.Status)...)
}

// readCredential decides one credential attribute after a read. A masked
// secret keeps the configured value: the server cannot say whether it changed.
// A key never configured stays unset, except on import, where every readable
// value is adopted.
func readCredential(prior types.String, server, applied map[string]string, name string, importing bool) types.String {
	theirs, ok := server[name]
	if importing {
		if !ok || theirs == client.MaskedSecret {
			return types.StringNull()
		}
		return types.StringValue(theirs)
	}
	if prior.IsNull() {
		return prior
	}
	switch {
	case !ok:
		return types.StringNull()
	case theirs == client.MaskedSecret, theirs == prior.ValueString(), applied[name] == theirs:
		return prior
	default:
		return types.StringValue(theirs)
	}
}

// readFlags turns the server's resource types into resource_types. It keeps
// every type the configuration lists, and adds any other type that is enabled,
// so enabling one in the UI shows up as drift. Optional fields left unset stay
// unset while the server holds their default.
func readFlags(ctx context.Context, prior map[string]cloudFlagModel, server []client.CloudFlag, importing bool, diags *diag.Diagnostics) map[string]cloudFlagModel {
	if prior == nil && !importing {
		return nil
	}
	out := map[string]cloudFlagModel{}
	for _, f := range server {
		mine, listed := prior[f.ResourceType]
		if !listed && !f.Collect && !f.MetricEnabled {
			continue
		}
		m := cloudFlagModel{
			Collect:       types.BoolValue(f.Collect),
			MetricEnabled: types.BoolValue(f.MetricEnabled),
			MetricTier:    types.StringValue(f.MetricTier),
			MetricExtras:  types.SetNull(types.StringType),
		}
		if listed && mine.MetricTier.IsNull() && f.MetricTier == "core" {
			m.MetricTier = types.StringNull()
		}
		if !listed && f.MetricTier == "core" {
			m.MetricTier = types.StringNull()
		}
		if len(f.MetricExtras) > 0 || (listed && !mine.MetricExtras.IsNull()) {
			set, d := types.SetValueFrom(ctx, types.StringType, f.MetricExtras)
			diags.Append(d...)
			m.MetricExtras = set
		}
		out[f.ResourceType] = m
	}
	if importing && len(out) == 0 {
		return nil
	}
	return out
}

// flagsToAPI is the flags to send: every type in wanted, and every type only
// in had switched off, since Terraform owns the full set once it lists any.
func flagsToAPI(ctx context.Context, wanted, had map[string]cloudFlagModel, diags *diag.Diagnostics) []client.CloudFlag {
	out := make([]client.CloudFlag, 0, len(wanted)+len(had))
	for name, m := range wanted {
		f := client.CloudFlag{
			ResourceType:  name,
			Collect:       m.Collect.ValueBool(),
			MetricEnabled: m.MetricEnabled.ValueBool(),
			MetricTier:    m.MetricTier.ValueString(),
		}
		if f.MetricTier == "" {
			f.MetricTier = "core"
		}
		if !m.MetricExtras.IsNull() {
			diags.Append(m.MetricExtras.ElementsAs(ctx, &f.MetricExtras, false)...)
		}
		out = append(out, f)
	}
	for name := range had {
		if _, ok := wanted[name]; !ok {
			out = append(out, client.CloudFlag{ResourceType: name, MetricTier: "core"})
		}
	}
	slices.SortFunc(out, func(a, b client.CloudFlag) int {
		switch {
		case a.ResourceType < b.ResourceType:
			return -1
		case a.ResourceType > b.ResourceType:
			return 1
		}
		return 0
	})
	return out
}

// attrGetter is what plans, states and configs share for reading one attribute.
type attrGetter interface {
	GetAttribute(ctx context.Context, p path.Path, target any) diag.Diagnostics
}

func getString(ctx context.Context, src attrGetter, name string, diags *diag.Diagnostics) types.String {
	var v types.String
	diags.Append(src.GetAttribute(ctx, path.Root(name), &v)...)
	return v
}

func getBool(ctx context.Context, src attrGetter, name string, diags *diag.Diagnostics) types.Bool {
	var v types.Bool
	diags.Append(src.GetAttribute(ctx, path.Root(name), &v)...)
	return v
}

// getFlags reads resource_types; nil means it is not set.
func getFlags(ctx context.Context, src attrGetter, diags *diag.Diagnostics) map[string]cloudFlagModel {
	var m types.Map
	diags.Append(src.GetAttribute(ctx, path.Root("resource_types"), &m)...)
	if m.IsNull() || m.IsUnknown() {
		return nil
	}
	out := map[string]cloudFlagModel{}
	diags.Append(m.ElementsAs(ctx, &out, false)...)
	return out
}
