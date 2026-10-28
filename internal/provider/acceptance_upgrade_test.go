package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccProviderUpgradeLeavesThePlanEmpty applies a configuration with the
// previous published release, then plans it with this build. The plan must be
// empty: upgrading the provider must never change a dashboard nobody touched.
//
// The previous release is downloaded from the registry, so this runs only
// when KUBESENSE_PREVIOUS_PROVIDER_VERSION names one (the release job sets it
// to the latest published version). It skips until the first release exists.
func TestAccProviderUpgradeLeavesThePlanEmpty(t *testing.T) {
	previous := os.Getenv("KUBESENSE_PREVIOUS_PROVIDER_VERSION")
	if previous == "" {
		t.Skip("KUBESENSE_PREVIOUS_PROVIDER_VERSION is not set; there is no published release to upgrade from")
	}
	api := newFakeAPI(t)
	api.current = true
	config := dashboardJSONConfig(api.URL, minimalPreset)

	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"kubesense": {Source: "kubesense-ai/kubesense", VersionConstraint: "= " + previous},
				},
				Config: config,
			},
			{
				ProtoV6ProviderFactories: testProviders,
				Config:                   config,
				PlanOnly:                 true,
			},
		},
	})
}
