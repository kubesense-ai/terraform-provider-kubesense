package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// Integration is a platform integration (Jira, Confluence, Atlassian, a custom
// MCP server) as the API returns it. The API key never comes back; only its
// last characters, as APIKeyHint.
type Integration struct {
	ID              string         `json:"integration_id"`
	Provider        string         `json:"provider"`
	IntegrationType string         `json:"integration_type"`
	Name            string         `json:"name"`
	APIKeyHint      *string        `json:"api_key_hint"`
	AuthScheme      string         `json:"auth_scheme"`
	AuthHeader      *string        `json:"auth_header"`
	Config          map[string]any `json:"config"`
	Enabled         bool           `json:"enabled"`
	Status          string         `json:"status"`
}

// IntegrationCreate is the body of POST /settings/integrations.
type IntegrationCreate struct {
	Provider   string         `json:"provider"`
	Name       string         `json:"name"`
	APIKey     string         `json:"api_key,omitempty"`
	AuthScheme string         `json:"auth_scheme,omitempty"`
	AuthHeader string         `json:"auth_header,omitempty"`
	Config     map[string]any `json:"config"`
	Enabled    *bool          `json:"enabled,omitempty"`
}

// IntegrationUpdate is the body of PATCH /settings/integrations/:id. A nil
// field is left untouched; Config, when sent, replaces the whole map.
type IntegrationUpdate struct {
	Name       *string        `json:"name,omitempty"`
	APIKey     *string        `json:"api_key,omitempty"`
	AuthScheme *string        `json:"auth_scheme,omitempty"`
	AuthHeader *string        `json:"auth_header,omitempty"`
	Config     map[string]any `json:"config,omitempty"`
	Enabled    *bool          `json:"enabled,omitempty"`
}

type integrationWrite struct {
	Integration Integration `json:"integration"`
}

// CreateIntegration connects an integration. The server probes the target
// with the credentials first and refuses (400) when the probe fails; the
// probe's error is in the message.
func (c *Client) CreateIntegration(ctx context.Context, in IntegrationCreate) (*Integration, error) {
	var out integrationWrite
	if _, err := c.do(ctx, http.MethodPost, "/settings/integrations", nil, in, &out); err != nil {
		return nil, err
	}
	return &out.Integration, nil
}

// GetIntegration reads one integration.
func (c *Client) GetIntegration(ctx context.Context, id string) (*Integration, error) {
	var it Integration
	if _, err := c.do(ctx, http.MethodGet, integrationPath(id), nil, nil, &it); err != nil {
		return nil, err
	}
	return &it, nil
}

// ListIntegrations lists integrations, narrowed to one provider when given.
func (c *Client) ListIntegrations(ctx context.Context, provider string) ([]Integration, error) {
	var query url.Values
	if provider != "" {
		query = url.Values{"provider": {provider}}
	}
	var out []Integration
	_, err := c.do(ctx, http.MethodGet, "/settings/integrations", query, nil, &out)
	return out, err
}

// UpdateIntegration patches an integration. A change to the key, auth or
// config is probed again and refused when the probe fails.
func (c *Client) UpdateIntegration(ctx context.Context, id string, in IntegrationUpdate) (*Integration, error) {
	var out integrationWrite
	if _, err := c.do(ctx, http.MethodPatch, integrationPath(id), nil, in, &out); err != nil {
		return nil, err
	}
	return &out.Integration, nil
}

// DeleteIntegration removes an integration (a soft delete that frees the name).
func (c *Client) DeleteIntegration(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, integrationPath(id), nil, nil, nil)
	return err
}

func integrationPath(id string) string { return "/settings/integrations/" + url.PathEscape(id) }

// ConfigStrings renders an integration's config as strings: strings as they
// are, anything else (the extra_headers object, say) as compact JSON, which
// the server accepts back in either form.
func ConfigStrings(cfg map[string]any) map[string]string {
	out := make(map[string]string, len(cfg))
	for k, v := range cfg {
		if s, ok := v.(string); ok {
			out[k] = s
			continue
		}
		b, err := json.Marshal(v)
		if err != nil {
			continue
		}
		out[k] = string(b)
	}
	return out
}
