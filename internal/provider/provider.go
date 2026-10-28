// Package provider implements the kubesense Terraform provider.
package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/kubesense-ai/terraform-provider-kubesense/internal/client"
	kschema "github.com/kubesense-ai/terraform-provider-kubesense/internal/kschema"
)

// UserAgentPrefix starts every request's User-Agent. kubeapi keys off it to
// hold Terraform's dashboard writes to the contract strictly.
const UserAgentPrefix = "terraform-provider-kubesense/"

type kubesenseProvider struct{ version string }

type providerModel struct {
	APIURL types.String `tfsdk:"api_url"`
	APIKey types.String `tfsdk:"api_key"`
}

// New returns the provider factory for the given release version.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &kubesenseProvider{version: version} }
}

func (p *kubesenseProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "kubesense"
	resp.Version = p.version
}

func (p *kubesenseProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage KubeSense dashboards and integrations.",
		Attributes: map[string]schema.Attribute{
			"api_url": schema.StringAttribute{
				Description: "Base URL of the KubeSense deployment, e.g. https://kubesense.example.com. " +
					"Defaults to the KUBESENSE_API_URL environment variable.",
				Optional: true,
			},
			"api_key": schema.StringAttribute{
				Description: "Service-account API key. Defaults to the KUBESENSE_API_KEY environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
		},
	}
}

func (p *kubesenseProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiURL := setting(cfg.APIURL, "KUBESENSE_API_URL", "api_url", &resp.Diagnostics)
	apiKey := setting(cfg.APIKey, "KUBESENSE_API_KEY", "api_key", &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	userAgent := fmt.Sprintf("%s%s terraform/%s", UserAgentPrefix, p.version, req.TerraformVersion)
	// The versions this provider was built against, so a newer server answers
	// in them rather than in its own. See compat.go.
	c := client.New(apiURL, apiKey, userAgent).WithSchemaVersions(kschema.CurrentVersions().Header())
	resp.DataSourceData = c
	resp.ResourceData = c
}

// setting resolves one provider argument: the configuration first, then the
// environment variable. A value still unknown at configure time (computed from
// another resource) cannot be used to build the client.
func setting(v types.String, env, attr string, diags *diag.Diagnostics) string {
	if v.IsUnknown() {
		diags.AddAttributeError(path.Root(attr), "Unknown "+attr,
			"The provider cannot be configured with a value that is only known after apply.")
		return ""
	}
	if s := v.ValueString(); s != "" {
		return s
	}
	if s := os.Getenv(env); s != "" {
		return s
	}
	diags.AddAttributeError(path.Root(attr), "Missing "+attr,
		fmt.Sprintf("Set %s in the provider block or the %s environment variable.", attr, env))
	return ""
}

func (p *kubesenseProvider) Resources(context.Context) []func() resource.Resource {
	return append([]func() resource.Resource{
		NewDashboardResource,
		NewDashboardJSONResource,
		NewDashboardListResource,
		NewAlertRuleResource,
		NewIntegrationResource,
	}, cloudResources()...)
}

func (p *kubesenseProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewDashboardDataSource,
		NewIntegrationDataSource,
	}
}

// clientFrom unwraps the client the provider handed to a resource or data
// source. Data is nil while Terraform validates configuration before the
// provider is configured, which is not an error.
func clientFrom(data any, diags *diag.Diagnostics) *client.Client {
	if data == nil {
		return nil
	}
	c, ok := data.(*client.Client)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("expected *client.Client, got %T", data))
	}
	return c
}
