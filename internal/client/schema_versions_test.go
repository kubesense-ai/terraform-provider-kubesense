package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// noRoute is kubeapi's answer for a path it does not serve (app.NoRoute).
const noRoute = `{"error":true,"message":"Endpoint doesn't exists","data":null}`

func TestEveryRequestCarriesTheSchemaVersions(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(SchemaVersionsHeader); got != "alert-rule=1, dashboard-preset=1" {
			t.Errorf("%s = %q", SchemaVersionsHeader, got)
		}
		_, _ = io.WriteString(w, `{"error":false,"data":{"id":"d1"}}`)
	})
	c.WithSchemaVersions("alert-rule=1, dashboard-preset=1")
	if _, err := c.GetDashboard(context.Background(), "d1"); err != nil {
		t.Fatal(err)
	}
}

func TestAMissingEndpointIsNotAMissingObject(t *testing.T) {
	// The bug this guards: Read treats NotFound as "deleted outside Terraform"
	// and drops the resource from state. A server too old for the endpoint
	// must not look like that, or every resource of the type vanishes.
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, noRoute)
	})
	_, err := c.GetDashboard(context.Background(), "d1")
	if IsNotFound(err) {
		t.Fatal("a missing endpoint was reported as a missing object")
	}
	if !IsEndpointMissing(err) {
		t.Fatalf("IsEndpointMissing = false for %v", err)
	}
	if msg := err.Error(); !strings.Contains(msg, "GET /dashboard/d1") || !strings.Contains(msg, "Upgrade KubeSense") {
		t.Errorf("message does not say what to do: %s", msg)
	}
}

func TestAMissingObjectIsStillNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":true,"message":"dashboard not found","data":null}`)
	})
	_, err := c.GetDashboard(context.Background(), "d1")
	if !IsNotFound(err) || IsEndpointMissing(err) {
		t.Fatalf("IsNotFound = %v, IsEndpointMissing = %v for %v", IsNotFound(err), IsEndpointMissing(err), err)
	}
}

func TestServerSchemaVersionsIsAskedOnce(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/schema/versions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"error":false,"data":{"schema_versions":{"dashboard-preset":3,"alert-rule":2},"oldest":1}}`)
	})
	for range 3 {
		got, err := c.ServerSchemaVersions(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if got["dashboard-preset"] != 3 || got["alert-rule"] != 2 {
			t.Fatalf("got %v", got)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("asked %d times, want once", n)
	}
}

func TestAServerWithoutTheVersionsEndpointIsV1(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, noRoute)
	})
	got, err := c.ServerSchemaVersions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got["dashboard-preset"] != 1 || got["alert-rule"] != 1 {
		t.Fatalf("got %v, want v1 throughout", got)
	}
}

func TestValidateDashboardReturnsTheServersVerdict(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/dashboard/validate" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"panels":[]}` {
			t.Errorf("body = %s, want the preset itself", body)
		}
		_, _ = io.WriteString(w, `{"error":false,"data":{"valid":false,"findings":[{"path":"/panels/0/panelType","message":"must be one of …","rule":"shape"}]}}`)
	})
	valid, findings, err := c.ValidateDashboard(context.Background(), []byte(`{"panels":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if valid || len(findings) != 1 || findings[0].String() != "/panels/0/panelType: must be one of …" {
		t.Fatalf("valid = %v, findings = %v", valid, findings)
	}
}
