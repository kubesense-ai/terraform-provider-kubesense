package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(srv.URL, "key-123", "terraform-provider-kubesense/test")
	c.sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

func TestCreateDashboardSendsAuthAndActiveStatus(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/dashboard" {
			t.Errorf("got %s %s, want POST /api/dashboard", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "key-123" {
			t.Errorf("x-api-key = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "terraform-provider-kubesense/test" {
			t.Errorf("User-Agent = %q", got)
		}
		var body DashboardInput
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Status != "active" || body.Name != "Checkout" {
			t.Errorf("body = %+v", body)
		}
		_, _ = io.WriteString(w, `{"error":false,"message":"ok","data":"7f3c"}`)
	})

	id, err := c.CreateDashboard(context.Background(), DashboardInput{Name: "Checkout", Preset: "{}"})
	if err != nil || id != "7f3c" {
		t.Fatalf("CreateDashboard = %q, %v", id, err)
	}
}

func TestDeleteDashboardPutsIDInBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.Method != http.MethodDelete || body["id"] != "abc" {
			t.Errorf("got %s with body %v", r.Method, body)
		}
		_, _ = io.WriteString(w, `{"error":false,"data":null}`)
	})
	if err := c.DeleteDashboard(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		notFound bool
		conflict bool
		message  string
	}{
		{"not found", 404, `{"error":true,"message":"dashboard not found"}`, true, false, "dashboard not found"},
		{"conflict", 409, `{"error":true,"message":"changed by someone else"}`, false, true, "changed by someone else"},
		{"rbac warn", 403, `{"warn":true,"message":"no write access"}`, false, false, "no write access"},
		{"html proxy page", 502, `<html>bad gateway</html>`, false, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			c.maxRetries = 0
			_, err := c.GetDashboard(context.Background(), "x")
			apiErr, ok := err.(*APIError)
			if !ok {
				t.Fatalf("err = %T %v, want *APIError", err, err)
			}
			if apiErr.Status != tc.status || apiErr.Message != tc.message {
				t.Errorf("got %+v", apiErr)
			}
			if IsNotFound(err) != tc.notFound || IsConflict(err) != tc.conflict {
				t.Errorf("IsNotFound=%v IsConflict=%v", IsNotFound(err), IsConflict(err))
			}
		})
	}
}

func TestEnvelopeErrorOn200IsAnError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"error":true,"message":"nope"}`)
	})
	if _, err := c.GetDashboard(context.Background(), "x"); err == nil {
		t.Fatal("want error for error:true on HTTP 200")
	}
}

func TestRetries(t *testing.T) {
	cases := []struct {
		name      string
		method    string
		status    int
		wantCalls int32
		wantOK    bool
	}{
		{"429 on POST is retried", http.MethodPost, 429, 2, true},
		{"503 on GET is retried", http.MethodGet, 503, 2, true},
		{"503 on POST is not retried", http.MethodPost, 503, 1, false},
		{"400 is never retried", http.MethodGet, 400, 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				if calls.Add(1) == 1 {
					w.WriteHeader(tc.status)
					_, _ = io.WriteString(w, `{"error":true,"message":"first"}`)
					return
				}
				_, _ = io.WriteString(w, `{"error":false,"data":"ok"}`)
			})
			var out string
			_, err := c.do(context.Background(), tc.method, "/x", nil, nil, &out)
			if (err == nil) != tc.wantOK || calls.Load() != tc.wantCalls {
				t.Errorf("err=%v calls=%d, want ok=%v calls=%d", err, calls.Load(), tc.wantOK, tc.wantCalls)
			}
		})
	}
}

func TestListDashboardsFollowsPages(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list_id") != "L1" {
			t.Errorf("list_id = %q", r.URL.Query().Get("list_id"))
		}
		page := r.URL.Query().Get("page")
		batch := make([]Dashboard, 0, 100)
		n := 100
		if page == "2" {
			n = 3
		}
		for i := 0; i < n; i++ {
			batch = append(batch, Dashboard{ID: page + "-" + string(rune('a'+i%26))})
		}
		data, _ := json.Marshal(batch)
		_, _ = io.WriteString(w, `{"error":false,"count":103,"data":`+string(data)+`}`)
	})
	got, err := c.ListDashboards(context.Background(), DashboardQuery{ListID: "L1"})
	if err != nil || len(got) != 103 {
		t.Fatalf("got %d dashboards, err %v; want 103", len(got), err)
	}
}

func TestNewAppendsAPIPrefixOnce(t *testing.T) {
	for _, in := range []string{"https://k.example.com", "https://k.example.com/", "https://k.example.com/api"} {
		if got := New(in, "", "").baseURL; got != "https://k.example.com/api" {
			t.Errorf("New(%q).baseURL = %q", in, got)
		}
	}
}
