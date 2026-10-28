package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Dashboard is a dashboard as GET /dashboard/:id returns it. Preset is empty
// on list responses.
type Dashboard struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Preset      string    `json:"preset"`
	Type        string    `json:"type"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// DashboardInput is the body of a dashboard create or update. The update is a
// whole replace, so every field is always sent.
type DashboardInput struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Status must be "active": the REST path stores it verbatim, and anything
	// else hides the dashboard from every list.
	Status string `json:"status"`
	Preset string `json:"preset"`
}

// DashboardQuery narrows ListDashboards. Empty fields do not narrow.
type DashboardQuery struct {
	Search string
	ListID string
}

// CreateDashboard creates a dashboard and returns its id.
func (c *Client) CreateDashboard(ctx context.Context, in DashboardInput) (string, error) {
	in.Status = "active"
	var id string
	_, err := c.do(ctx, http.MethodPost, "/dashboard", nil, in, &id)
	return id, err
}

// GetDashboard reads one dashboard, preset included.
func (c *Client) GetDashboard(ctx context.Context, id string) (*Dashboard, error) {
	var d Dashboard
	if _, err := c.do(ctx, http.MethodGet, "/dashboard/"+url.PathEscape(id), nil, nil, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// UpdateDashboard replaces a dashboard. The id travels in the body.
func (c *Client) UpdateDashboard(ctx context.Context, in DashboardInput) error {
	in.Status = "active"
	_, err := c.do(ctx, http.MethodPut, "/dashboard", nil, in, nil)
	return err
}

// DeleteDashboard soft-deletes a dashboard. The id travels in the body.
func (c *Client) DeleteDashboard(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, "/dashboard", nil, map[string]string{"id": id}, nil)
	return err
}

// ListDashboards returns every dashboard matching q, following pages. Presets
// are not included.
func (c *Client) ListDashboards(ctx context.Context, q DashboardQuery) ([]Dashboard, error) {
	const pageSize = 100
	var all []Dashboard
	for page := 1; ; page++ {
		// status is required by the list endpoint; deleted dashboards are not
		// something a configuration can manage.
		query := url.Values{"status": {"active"}, "page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(pageSize)}}
		if q.Search != "" {
			query.Set("search", q.Search)
		}
		if q.ListID != "" {
			query.Set("list_id", q.ListID)
		}
		var batch []Dashboard
		env, err := c.do(ctx, http.MethodGet, "/dashboard", query, nil, &batch)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < pageSize || int64(len(all)) >= env.Count {
			return all, nil
		}
	}
}
