package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/kubesense-ai/terraform-provider-kubesense/internal/client"
)

const noRoute = `{"error":true,"message":"Endpoint doesn't exists","data":null}`

// compatServer answers /schema/versions and /dashboard/validate the way a
// server of some age would. An empty validate answer means the server is too
// old to have the endpoint.
func compatServer(t *testing.T, versions, validate string) *client.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body string
		switch r.URL.Path {
		case "/api/schema/versions":
			body = versions
		case "/api/dashboard/validate":
			body = validate
		}
		if body == "" {
			w.WriteHeader(http.StatusNotFound)
			body = noRoute
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return client.New(srv.URL, "key", "terraform-provider-kubesense/test")
}

func runCheck(t *testing.T, c *client.Client, preset string) []string {
	t.Helper()
	var diags diag.Diagnostics
	var reported []string
	checkDashboard(context.Background(), c, []byte(preset), func(summary, detail string) {
		reported = append(reported, detail)
	}, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	return reported
}

func TestCheckDashboardUsesTheServersContract(t *testing.T) {
	// The server has widened its contract since this provider was built: it
	// accepts a preset the embedded contract refuses. Its verdict wins, so a
	// pinned provider does not block a feature the server already has.
	accepts := compatServer(t, "", `{"error":false,"data":{"valid":true}}`)
	if got := runCheck(t, accepts, badVariablePreset); len(got) != 0 {
		t.Errorf("server accepted the preset, provider still reported %v", got)
	}

	// And it refuses on the server's word too.
	refuses := compatServer(t, "", `{"error":false,"data":{"valid":false,"findings":[{"path":"/panels/0","message":"unknown panel type","rule":"shape"}]}}`)
	got := runCheck(t, refuses, minimalPreset)
	if len(got) != 1 || got[0] != "/panels/0: unknown panel type" {
		t.Errorf("got %v, want the server's one finding", got)
	}
}

func TestCheckDashboardFallsBackToTheEmbeddedContract(t *testing.T) {
	// A server too old for the dry-run endpoint: the provider's own contract
	// is the best there is, and is what every provider before this one used.
	old := compatServer(t, "", "")
	if got := runCheck(t, old, badVariablePreset); len(got) == 0 {
		t.Error("an invalid preset passed with no server to ask")
	}
	if got := runCheck(t, old, minimalPreset); len(got) != 0 {
		t.Errorf("a valid preset was refused: %v", got)
	}
}

func TestForServerLeavesACurrentServerAlone(t *testing.T) {
	for name, versions := range map[string]string{
		"no versions endpoint": "",
		"same version":         `{"error":false,"data":{"schema_versions":{"dashboard-preset":1}}}`,
		"newer server":         `{"error":false,"data":{"schema_versions":{"dashboard-preset":4}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			c := compatServer(t, versions, "")
			got, err := forServer(context.Background(), c, []byte(minimalPreset))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != minimalPreset {
				t.Errorf("preset changed:\n got %s\nwant %s", got, minimalPreset)
			}
		})
	}
}

func TestReadOfAResourceOnAServerWithoutTheEndpointFails(t *testing.T) {
	// The regression guard for skew in the other direction: a provider newer
	// than the server reads a resource type the server does not have. It must
	// fail loudly, not drop the resource from state as if it had been deleted.
	c := compatServer(t, "", "")
	_, err := c.GetDashboard(context.Background(), "d1")
	if client.IsNotFound(err) {
		t.Fatal("missing endpoint read as a deleted object: Read would drop it from state")
	}
	if !strings.Contains(err.Error(), "older than this provider") {
		t.Errorf("error does not explain itself: %v", err)
	}
}
