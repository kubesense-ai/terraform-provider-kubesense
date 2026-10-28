package client

import (
	"context"
	"net/http"
	"net/url"
)

// DashboardList is a dashboard list (the UI's folders) as the API returns it.
type DashboardList struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type dashboardListInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CreateDashboardList creates a list and returns its id.
func (c *Client) CreateDashboardList(ctx context.Context, name, description string) (string, error) {
	var id string
	_, err := c.do(ctx, http.MethodPost, "/dashboard-lists", nil, dashboardListInput{name, description}, &id)
	return id, err
}

// GetDashboardList reads one list.
func (c *Client) GetDashboardList(ctx context.Context, id string) (*DashboardList, error) {
	var l DashboardList
	if _, err := c.do(ctx, http.MethodGet, listPath(id), nil, nil, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// UpdateDashboardList renames a list or changes its description.
func (c *Client) UpdateDashboardList(ctx context.Context, id, name, description string) error {
	_, err := c.do(ctx, http.MethodPut, listPath(id), nil, dashboardListInput{name, description}, nil)
	return err
}

// DeleteDashboardList deletes a list. Its dashboards are kept.
func (c *Client) DeleteDashboardList(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, listPath(id), nil, nil, nil)
	return err
}

// DashboardListMembers returns the ids of the dashboards filed under a list.
// The list endpoint does not carry them, so this reads the dashboards page
// narrowed to the list.
func (c *Client) DashboardListMembers(ctx context.Context, id string) ([]string, error) {
	dashboards, err := c.ListDashboards(ctx, DashboardQuery{ListID: id})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(dashboards))
	for _, d := range dashboards {
		ids = append(ids, d.ID)
	}
	return ids, nil
}

// AddDashboardsToList files dashboards under a list. Re-adding one already
// filed is a no-op on the server.
func (c *Client) AddDashboardsToList(ctx context.Context, id string, dashboardIDs []string) error {
	return c.membership(ctx, http.MethodPost, id, dashboardIDs)
}

// RemoveDashboardsFromList unfiles dashboards from a list.
func (c *Client) RemoveDashboardsFromList(ctx context.Context, id string, dashboardIDs []string) error {
	return c.membership(ctx, http.MethodDelete, id, dashboardIDs)
}

func (c *Client) membership(ctx context.Context, method, id string, dashboardIDs []string) error {
	if len(dashboardIDs) == 0 {
		return nil
	}
	body := map[string][]string{"dashboard_ids": dashboardIDs}
	_, err := c.do(ctx, method, listPath(id)+"/dashboards", nil, body, nil)
	return err
}

func listPath(id string) string { return "/dashboard-lists/" + url.PathEscape(id) }
