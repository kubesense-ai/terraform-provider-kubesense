package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// An alert rule is read and written as its definition: the stored rule
// without server state, with channels by name. kubeapi converts it to and from
// the rule; see observability/alertmanager/rule-engine/definition.go.

// CreateAlertRule creates a rule from its definition and returns its id.
func (c *Client) CreateAlertRule(ctx context.Context, definition map[string]any) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	_, err := c.do(ctx, http.MethodPost, "/alerts/rules/definition", nil, definition, &out)
	return out.ID, err
}

// GetAlertRule reads a rule's definition, numbers kept as written.
func (c *Client) GetAlertRule(ctx context.Context, id string) (map[string]any, error) {
	var raw json.RawMessage
	if _, err := c.do(ctx, http.MethodGet, alertRulePath(id)+"/definition", nil, nil, &raw); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var def map[string]any
	if err := dec.Decode(&def); err != nil {
		return nil, err
	}
	return def, nil
}

// UpdateAlertRule replaces a rule with a definition.
func (c *Client) UpdateAlertRule(ctx context.Context, id string, definition map[string]any) error {
	_, err := c.do(ctx, http.MethodPut, alertRulePath(id)+"/definition", nil, definition, nil)
	return err
}

// DeleteAlertRule deletes a rule.
func (c *Client) DeleteAlertRule(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, alertRulePath(id), nil, nil, nil)
	return err
}

func alertRulePath(id string) string { return "/alerts/rules/" + url.PathEscape(id) }
