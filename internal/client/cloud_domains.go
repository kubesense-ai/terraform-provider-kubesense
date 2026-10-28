package client

import (
	"context"
	"net/http"
	"net/url"
)

// MaskedSecret is what the API returns in place of a stored secret credential.
const MaskedSecret = "******"

// CloudDomain is one cloud account integration (AWS account, GCP project,
// Azure subscription, ...). Kuid is its id: a deterministic hash of provider
// and account, so creating the same account again updates it.
type CloudDomain struct {
	Kuid        string `json:"kuid"`
	Provider    string `json:"provider"`
	AccountID   string `json:"account_id"`
	DisplayName string `json:"display_name"`
	Active      bool   `json:"active"`
	Status      string `json:"status"`
	ErrorMsg    string `json:"error_msg"`
}

// CloudFlag is one resource type's collection settings.
type CloudFlag struct {
	ResourceType  string   `json:"resource_type"`
	Collect       bool     `json:"collect"`
	MetricEnabled bool     `json:"metric_enabled"`
	MetricTier    string   `json:"metric_tier"`
	MetricExtras  []string `json:"metric_extras"`
}

// CloudDomainCreate is the body of POST /settings/cloud-domains.
type CloudDomainCreate struct {
	Provider    string            `json:"provider"`
	AccountID   string            `json:"account_id"`
	Credentials map[string]string `json:"credentials"`
	Flags       []CloudFlag       `json:"flags"`
}

// CreateCloudDomain creates (or, for an existing account, updates) a cloud
// domain and returns its kuid. Credentials are validated against the cloud
// first: 400 when they don't work, 409 when they belong to another account.
func (c *Client) CreateCloudDomain(ctx context.Context, in CloudDomainCreate) (string, error) {
	var out struct {
		Kuid string `json:"kuid"`
	}
	_, err := c.do(ctx, http.MethodPost, "/settings/cloud-domains", nil, in, &out)
	return out.Kuid, err
}

// GetCloudDomain reads one cloud domain.
func (c *Client) GetCloudDomain(ctx context.Context, kuid string) (*CloudDomain, error) {
	var d CloudDomain
	if _, err := c.do(ctx, http.MethodGet, domainPath(kuid), nil, nil, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// GetCloudCredentials reads a domain's credentials, secrets as MaskedSecret.
func (c *Client) GetCloudCredentials(ctx context.Context, kuid string) (map[string]string, error) {
	out := map[string]string{}
	_, err := c.do(ctx, http.MethodGet, domainPath(kuid)+"/credentials", nil, nil, &out)
	return out, err
}

// UpdateCloudCredentials merges credentials into the stored ones.
func (c *Client) UpdateCloudCredentials(ctx context.Context, kuid string, creds map[string]string) error {
	if len(creds) == 0 {
		return nil
	}
	body := map[string]map[string]string{"credentials": creds}
	_, err := c.do(ctx, http.MethodPatch, domainPath(kuid)+"/credentials", nil, body, nil)
	return err
}

// GetCloudFlags reads every resource type's settings. The server seeds every
// type the provider supports, disabled.
func (c *Client) GetCloudFlags(ctx context.Context, kuid string) ([]CloudFlag, error) {
	var out []CloudFlag
	_, err := c.do(ctx, http.MethodGet, domainPath(kuid)+"/config", nil, nil, &out)
	return out, err
}

// UpdateCloudFlags upserts the given resource types' settings.
func (c *Client) UpdateCloudFlags(ctx context.Context, kuid string, flags []CloudFlag) error {
	if len(flags) == 0 {
		return nil
	}
	body := map[string][]CloudFlag{"flags": flags}
	_, err := c.do(ctx, http.MethodPatch, domainPath(kuid)+"/config", nil, body, nil)
	return err
}

// SetCloudDomainActive resumes or pauses collection for a domain.
func (c *Client) SetCloudDomainActive(ctx context.Context, kuid string, active bool) error {
	action := "/deactivate"
	if active {
		action = "/activate"
	}
	_, err := c.do(ctx, http.MethodPost, domainPath(kuid)+action, nil, nil, nil)
	return err
}

// DeleteCloudDomain removes a domain with its credentials and settings.
func (c *Client) DeleteCloudDomain(ctx context.Context, kuid string) error {
	_, err := c.do(ctx, http.MethodDelete, domainPath(kuid), nil, nil, nil)
	return err
}

func domainPath(kuid string) string { return "/settings/cloud-domains/" + url.PathEscape(kuid) }
