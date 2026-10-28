package provider

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// cloudSpec describes one cloud account resource. All six share
// cloudAccountResource; only this table differs. Field names are the
// credential keys the API stores, the same ones the webapp's single-account
// forms send (src/pages/integrations/cloud/form-v2/shared/singleAccount/providerFields.ts).
type cloudSpec struct {
	typeSuffix string // kubesense_<typeSuffix>
	provider   string // the API's provider id
	noun       string // for descriptions
	// accountAttr names the account identifier. It is sent as account_id and,
	// when named differently, under its own name in the credentials too, as
	// the webapp does.
	accountAttr string
	accountDesc string
	fields      []cloudField
}

type cloudField struct {
	name        string
	description string
	// secret fields are masked by the API on read, so a change made outside
	// Terraform is not detected for them.
	secret bool
	// identity fields decide which domain this is (the API derives the id from
	// them), so changing one means a new domain.
	identity   bool
	validators []validator.String
}

var cloudSpecs = []cloudSpec{
	{
		typeSuffix:  "aws_account",
		provider:    "aws",
		noun:        "AWS account",
		accountAttr: "account_id",
		accountDesc: "12-digit AWS account id.",
		fields: []cloudField{
			{name: "mode", description: "role_access (an IAM role KubeSense assumes, recommended), credentials (an access key) or service_account (the collector's own credentials).",
				validators: []validator.String{stringvalidator.OneOf("role_access", "credentials", "service_account")}},
			{name: "role_arn", description: "ARN of the read-only IAM role, for mode = role_access.",
				validators: []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^arn:`), "must be a full IAM role ARN")}},
			{name: "secret_id", description: "Access key id, for mode = credentials."},
			{name: "secret_key", description: "Secret access key, for mode = credentials.", secret: true},
		},
	},
	{
		typeSuffix:  "gcp_project",
		provider:    "gcp",
		noun:        "Google Cloud project",
		accountAttr: "project_id",
		accountDesc: "GCP project id.",
		fields: []cloudField{
			{name: "region", description: "Default region, e.g. us-central1."},
			{name: "service_account_key_json", description: "Service account key file contents, e.g. file(\"key.json\").", secret: true},
		},
	},
	{
		typeSuffix:  "azure_subscription",
		provider:    "azure",
		noun:        "Azure subscription",
		accountAttr: "subscription_id",
		accountDesc: "Subscription GUID.",
		fields: []cloudField{
			{name: "tenant_id", description: "Directory (tenant) id."},
			{name: "client_id", description: "Application (client) id of an app with Reader on the subscription."},
			{name: "client_secret", description: "Client secret value.", secret: true},
			{name: "region", description: "Default region, e.g. eastus."},
		},
	},
	{
		typeSuffix:  "mongodbatlas_org",
		provider:    "mongodbatlas",
		noun:        "MongoDB Atlas organization",
		accountAttr: "org_id",
		accountDesc: "24-character Atlas organization id.",
		fields: []cloudField{
			{name: "client_id", description: "Project public key (project-level access)."},
			{name: "client_secret", description: "Project private key (project-level access).", secret: true},
			{name: "public_key", description: "Organization public key (organization-level access)."},
			{name: "private_key", description: "Organization private key (organization-level access).", secret: true},
		},
	},
	{
		typeSuffix:  "confluent_org",
		provider:    "confluent",
		noun:        "Confluent Cloud organization",
		accountAttr: "org_id",
		accountDesc: "Confluent Cloud organization id.",
		fields: []cloudField{
			{name: "api_key", description: "Cloud API key.", secret: true},
			{name: "api_secret", description: "Cloud API secret.", secret: true},
		},
	},
	{
		typeSuffix:  "kong_control_plane",
		provider:    "kong",
		noun:        "Kong gateway",
		accountAttr: "org_id",
		accountDesc: "Organization id.",
		fields: []cloudField{
			{name: "mode", description: "konnect (Kong Konnect) or self_hosted.",
				validators: []validator.String{stringvalidator.OneOf("konnect", "self_hosted")}},
			{name: "control_plane_id", description: "Konnect control plane UUID, for mode = konnect. It identifies the integration, so changing it creates a new one.", identity: true},
			{name: "bearer_token", description: "Konnect personal access token, for mode = konnect.", secret: true},
			{name: "base_url", description: "Admin API base URL."},
			{name: "admin_api_key", description: "Admin API key, for mode = self_hosted.", secret: true},
		},
	},
}

// cloudResources returns a constructor per cloud spec.
func cloudResources() []func() resource.Resource {
	out := make([]func() resource.Resource, 0, len(cloudSpecs))
	for _, spec := range cloudSpecs {
		out = append(out, func() resource.Resource { return &cloudAccountResource{spec: spec} })
	}
	return out
}
