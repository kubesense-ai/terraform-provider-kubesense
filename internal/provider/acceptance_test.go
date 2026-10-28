package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/kubesense-ai/terraform-provider-kubesense/internal/client"
)

// These run real plan/apply/import cycles through the terraform binary, so
// they need TF_ACC=1 and terraform on PATH. They target fakeAPI below, an
// in-memory stand-in for kubeapi, so they need no running server.

var testProviders = map[string]func() (tfprotov6.ProviderServer, error){
	"kubesense": providerserver.NewProtocol6WithError(New("test")()),
}

func TestAccDashboardAndList(t *testing.T) {
	api := newFakeAPI(t)

	config := func(name string) string {
		return fmt.Sprintf(`
provider "kubesense" {
  api_url = %q
  api_key = "test-key"
}

resource "kubesense_dashboard_json" "checkout" {
  name           = %q
  dashboard_json = <<EOT
%s
EOT
}

resource "kubesense_dashboard_list" "payments" {
  name          = "Payments"
  dashboard_ids = [kubesense_dashboard_json.checkout.id]
}
`, api.URL, name, minimalPreset)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{
				Config: config("Checkout"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("kubesense_dashboard_json.checkout", "id"),
					resource.TestCheckResourceAttr("kubesense_dashboard_json.checkout", "name", "Checkout"),
					resource.TestCheckResourceAttr("kubesense_dashboard_list.payments", "dashboard_ids.#", "1"),
				),
			},
			{
				// The fake stores presets re-indented, as a database might.
				// Semantic equality has to keep this plan empty.
				Config:   config("Checkout"),
				PlanOnly: true,
			},
			{
				Config: config("Checkout service"),
				Check:  resource.TestCheckResourceAttr("kubesense_dashboard_json.checkout", "name", "Checkout service"),
			},
			{
				ResourceName:      "kubesense_dashboard_json.checkout",
				ImportState:       true,
				ImportStateVerify: true,
				// Imported state holds the server's string; config holds the
				// user's. They are semantically equal, not byte-equal.
				ImportStateVerifyIgnore: []string{"dashboard_json"},
			},
			{
				ResourceName:      "kubesense_dashboard_list.payments",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// fakeAPI is an in-memory stand-in for the dashboard and dashboard-list
// endpoints, faithful to the envelope, the id-in-body routes and soft delete.
type fakeAPI struct {
	*httptest.Server
	mu           sync.Mutex
	next         int
	dashboards   map[string]map[string]any
	lists        map[string]map[string]any
	members      map[string]map[string]bool // list id -> dashboard ids
	integrations map[string]map[string]any
	domains      map[string]*fakeDomain
	alertRules   map[string]map[string]any

	// Server age, for the version-skew tests (acceptance_skew_test.go). By
	// default the fake is an OLD server: no /schema/versions, no
	// /dashboard/validate. current adds both; validate overrides the dry
	// run's verdict; missing makes every path under a prefix answer as the
	// real API does for a route it does not have.
	current  bool
	validate func(preset []byte) (bool, []map[string]string)
	missing  []string

	// versionsHeader is the last client.SchemaVersionsHeader the fake received.
	versionsHeader string
}

func newFakeAPI(t *testing.T) *fakeAPI {
	f := &fakeAPI{
		dashboards: map[string]map[string]any{}, lists: map[string]map[string]any{}, members: map[string]map[string]bool{},
		integrations: map[string]map[string]any{}, domains: map[string]*fakeDomain{},
		alertRules: map[string]map[string]any{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/dashboard", f.createDashboard)
	mux.HandleFunc("PUT /api/dashboard", f.updateDashboard)
	mux.HandleFunc("DELETE /api/dashboard", f.deleteDashboard)
	mux.HandleFunc("GET /api/dashboard", f.listDashboards)
	mux.HandleFunc("GET /api/dashboard/{id}", f.getDashboard)
	mux.HandleFunc("POST /api/dashboard-lists", f.createList)
	mux.HandleFunc("GET /api/dashboard-lists/{id}", f.getList)
	mux.HandleFunc("PUT /api/dashboard-lists/{id}", f.updateList)
	mux.HandleFunc("DELETE /api/dashboard-lists/{id}", f.deleteList)
	mux.HandleFunc("POST /api/dashboard-lists/{id}/dashboards", f.membership(true))
	mux.HandleFunc("DELETE /api/dashboard-lists/{id}/dashboards", f.membership(false))
	f.registerIntegrations(mux)
	f.registerAlertRules(mux)
	f.registerCloudDomains(mux)
	f.registerSchemaVersions(mux)
	// Anything else is answered as kubeapi's app.NoRoute answers it, which is
	// what the provider keys off to tell a missing endpoint from a missing object.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 404, nil, "Endpoint doesn't exists")
	})
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			reply(w, 401, nil, "unauthorized")
			return
		}
		if !strings.HasPrefix(r.Header.Get("User-Agent"), UserAgentPrefix) {
			t.Errorf("User-Agent %q lacks %q", r.Header.Get("User-Agent"), UserAgentPrefix)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.versionsHeader = r.Header.Get(client.SchemaVersionsHeader)
		for _, prefix := range f.missing {
			if strings.HasPrefix(r.URL.Path, prefix) {
				reply(w, 404, nil, "Endpoint doesn't exists")
				return
			}
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(f.Close)
	return f
}

func reply(w http.ResponseWriter, status int, data any, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": status >= 400, "message": msg, "data": data})
}

func (f *fakeAPI) id(prefix string) string {
	f.next++
	return fmt.Sprintf("%s-%04d", prefix, f.next)
}

func (f *fakeAPI) createDashboard(w http.ResponseWriter, r *http.Request) {
	var in map[string]any
	_ = json.NewDecoder(r.Body).Decode(&in)
	id := f.id("dash")
	now := time.Now().UTC()
	in["id"], in["type"], in["created_at"], in["updated_at"] = id, "user", now, now
	in["preset"] = reindent(in["preset"].(string))
	f.dashboards[id] = in
	reply(w, 200, id, "dashboard created successfully")
}

func (f *fakeAPI) updateDashboard(w http.ResponseWriter, r *http.Request) {
	var in map[string]any
	_ = json.NewDecoder(r.Body).Decode(&in)
	d, ok := f.dashboards[in["id"].(string)]
	if !ok {
		reply(w, 404, nil, "dashboard not found")
		return
	}
	for _, k := range []string{"name", "description", "status"} {
		d[k] = in[k]
	}
	d["preset"] = reindent(in["preset"].(string))
	d["updated_at"] = time.Now().UTC()
	reply(w, 200, nil, "dashboard updated successfully")
}

func (f *fakeAPI) deleteDashboard(w http.ResponseWriter, r *http.Request) {
	var in map[string]string
	_ = json.NewDecoder(r.Body).Decode(&in)
	delete(f.dashboards, in["id"])
	for _, m := range f.members {
		delete(m, in["id"])
	}
	reply(w, 200, nil, "dashboard removed successfully")
}

func (f *fakeAPI) getDashboard(w http.ResponseWriter, r *http.Request) {
	d, ok := f.dashboards[r.PathValue("id")]
	if !ok {
		reply(w, 404, nil, "dashboard not found")
		return
	}
	reply(w, 200, d, "ok")
}

func (f *fakeAPI) listDashboards(w http.ResponseWriter, r *http.Request) {
	// As kubeapi does: the list refuses a request with no status filter.
	if r.URL.Query().Get("status") == "" {
		reply(w, 400, nil, "status filter is required")
		return
	}
	listID := r.URL.Query().Get("list_id")
	out := []map[string]any{}
	for id, d := range f.dashboards {
		if listID != "" && !f.members[listID][id] {
			continue
		}
		summary := map[string]any{}
		for k, v := range d {
			if k != "preset" {
				summary[k] = v
			}
		}
		out = append(out, summary)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"error": false, "data": out, "count": len(out)})
}

func (f *fakeAPI) createList(w http.ResponseWriter, r *http.Request) {
	var in map[string]any
	_ = json.NewDecoder(r.Body).Decode(&in)
	id := f.id("list")
	in["id"] = id
	f.lists[id] = in
	f.members[id] = map[string]bool{}
	reply(w, 200, id, "dashboard list created successfully")
}

func (f *fakeAPI) getList(w http.ResponseWriter, r *http.Request) {
	l, ok := f.lists[r.PathValue("id")]
	if !ok {
		reply(w, 404, nil, "dashboard list not found")
		return
	}
	reply(w, 200, l, "ok")
}

func (f *fakeAPI) updateList(w http.ResponseWriter, r *http.Request) {
	l, ok := f.lists[r.PathValue("id")]
	if !ok {
		reply(w, 404, nil, "dashboard list not found")
		return
	}
	var in map[string]any
	_ = json.NewDecoder(r.Body).Decode(&in)
	l["name"], l["description"] = in["name"], in["description"]
	reply(w, 200, nil, "ok")
}

func (f *fakeAPI) deleteList(w http.ResponseWriter, r *http.Request) {
	delete(f.lists, r.PathValue("id"))
	delete(f.members, r.PathValue("id"))
	reply(w, 200, nil, "ok")
}

func (f *fakeAPI) membership(add bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m, ok := f.members[r.PathValue("id")]
		if !ok {
			reply(w, 404, nil, "dashboard list not found")
			return
		}
		var in struct {
			DashboardIDs []string `json:"dashboard_ids"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		for _, id := range in.DashboardIDs {
			if add {
				m[id] = true
			} else {
				delete(m, id)
			}
		}
		reply(w, 200, nil, "ok")
	}
}

func reindent(s string) string {
	var buf bytes.Buffer
	if json.Indent(&buf, []byte(s), "", "    ") != nil {
		return s
	}
	return buf.String()
}
