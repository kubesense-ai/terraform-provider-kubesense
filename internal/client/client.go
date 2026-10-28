// Package client talks to the KubeSense API over HTTP. It knows the response
// envelope, the error shapes and the retry policy; the resources on top of it
// know nothing about HTTP.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client is a KubeSense API client authenticated with a service-account key.
type Client struct {
	baseURL    string
	apiKey     string
	userAgent  string
	http       *http.Client
	maxRetries int

	// sleep waits between retries. A field so tests do not wait for real.
	sleep func(ctx context.Context, d time.Duration) error

	// schemaVersions is sent as SchemaVersionsHeader on every request: the
	// versions this provider was built against, so a newer server answers in
	// them. See WithSchemaVersions.
	schemaVersions string

	// serverVersions caches ServerSchemaVersions for the life of the client,
	// which is one Terraform run.
	serverVersionsOnce sync.Once
	serverVersions     map[string]int
	serverVersionsErr  error
}

// SchemaVersionsHeader names the header carrying the client's schema versions.
// The value is schema.VersionsHeader in kubeapi; it is repeated here because
// this package knows nothing about the schema, only about HTTP.
const SchemaVersionsHeader = "KubeSense-Schema-Versions"

// WithSchemaVersions sets the SchemaVersionsHeader value sent on every
// request, e.g. "alert-rule=1, dashboard-preset=1", and returns c.
func (c *Client) WithSchemaVersions(header string) *Client {
	c.schemaVersions = header
	return c
}

// New returns a client for the deployment at baseURL, e.g.
// "https://kubesense.acme.com". The /api prefix is added here, so a URL that
// already ends in /api is accepted as well.
func New(baseURL, apiKey, userAgent string) *Client {
	base := strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(base, "/api") {
		base += "/api"
	}
	return &Client{
		baseURL:    base,
		apiKey:     apiKey,
		userAgent:  userAgent,
		http:       &http.Client{Timeout: 2 * time.Minute},
		maxRetries: 3,
		sleep:      sleepCtx,
	}
}

// envelope is the shape of every KubeSense API response. RBAC refusals set
// Warn instead of Error, and a system-dashboard refusal adds Reason.
type envelope struct {
	Error   bool            `json:"error"`
	Warn    bool            `json:"warn"`
	Message string          `json:"message"`
	Reason  string          `json:"reason"`
	Data    json.RawMessage `json:"data"`
	Count   int64           `json:"count"`
}

// APIError is a non-2xx answer, or a 2xx one whose envelope says error.
type APIError struct {
	Status  int
	Message string
	Reason  string

	// Method and Path name the request, for the endpoint-missing message.
	Method string
	Path   string
}

// endpointMissingMessage is what kubeapi's catch-all route answers for a path
// it does not serve. Every build since the first sends exactly this text (see
// app.NoRoute in kubeapi's cmd/main.go), which is what makes it safe to match.
const endpointMissingMessage = "Endpoint doesn't exists"

// endpointMissing reports a 404 that came from no route at all, as opposed
// to a route that looked for an object and did not find it.
func (e *APIError) endpointMissing() bool {
	return e.Status == http.StatusNotFound && e.Message == endpointMissingMessage
}

func (e *APIError) Error() string {
	if e.endpointMissing() {
		return fmt.Sprintf("this KubeSense server has no %s %s endpoint: it is older than this provider "+
			"version supports. Upgrade KubeSense, or pin the provider to the version you used with this server.",
			e.Method, e.Path)
	}
	if e.Message == "" {
		return fmt.Sprintf("KubeSense API returned HTTP %d", e.Status)
	}
	return fmt.Sprintf("KubeSense API returned HTTP %d: %s", e.Status, e.Message)
}

// IsNotFound reports a 404 for an object: it was deleted, or the key cannot
// see it. A 404 for an endpoint the server does not have is NOT one.
//
// The distinction is the one that matters most when the provider is newer
// than the server. Read answers "not found" by dropping the resource from
// state, so a server too old to have a resource's endpoint would otherwise
// make every such resource vanish from state and be planned for creation —
// against a server that cannot create it either.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound && !apiErr.endpointMissing()
}

// IsEndpointMissing reports a request to an endpoint the server does not
// have: it predates the feature.
func IsEndpointMissing(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.endpointMissing()
}

// IsConflict reports a 409: a duplicate name, or a concurrent change.
func IsConflict(err error) bool { return hasStatus(err, http.StatusConflict) }

func hasStatus(err error, status int) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == status
}

// do sends one request and decodes the envelope's data into out (when out is
// not nil). It retries 429 on every method, since the rate limiter answers
// before any handler runs, and 502/503/504 only on methods that are safe to
// repeat.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) (*envelope, error) {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
	}

	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	for attempt := 0; ; attempt++ {
		resp, err := c.send(ctx, method, target, payload)
		if err != nil {
			return nil, err
		}
		env, apiErr, readErr := decode(resp)
		if readErr != nil {
			return nil, readErr
		}
		if apiErr != nil {
			apiErr.Method, apiErr.Path = method, path
		}
		if apiErr == nil {
			if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
				if err := json.Unmarshal(env.Data, out); err != nil {
					return nil, fmt.Errorf("decoding %s %s response: %w", method, path, err)
				}
			}
			return env, nil
		}
		if attempt >= c.maxRetries || !retryable(method, resp.StatusCode) {
			return nil, apiErr
		}
		if err := c.sleep(ctx, backoff(attempt, resp.Header.Get("Retry-After"))); err != nil {
			return nil, err
		}
	}
}

func (c *Client) send(ctx context.Context, method, target string, payload []byte) (*http.Response, error) {
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if c.schemaVersions != "" {
		req.Header.Set(SchemaVersionsHeader, c.schemaVersions)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.http.Do(req)
}

// decode reads the envelope. A body that is not an envelope (a proxy's HTML
// error page, say) still yields an APIError carrying the status.
func decode(resp *http.Response) (*envelope, *APIError, error) {
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("reading response: %w", err)
	}
	var env envelope
	jsonErr := json.Unmarshal(raw, &env)

	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	if ok && jsonErr == nil && !env.Error && !env.Warn {
		return &env, nil, nil
	}
	if ok && jsonErr != nil {
		return nil, nil, fmt.Errorf("unexpected response body (HTTP %d): %w", resp.StatusCode, jsonErr)
	}
	return &env, &APIError{Status: resp.StatusCode, Message: env.Message, Reason: env.Reason}, nil
}

func retryable(method string, status int) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	if method == http.MethodPost {
		return false
	}
	return status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

// backoff honours a Retry-After in seconds, else doubles from one second with
// up to 50% jitter so parallel resources do not retry in lockstep.
func backoff(attempt int, retryAfter string) time.Duration {
	if secs, err := strconv.Atoi(retryAfter); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	base := time.Second << attempt
	return base + time.Duration(rand.Int64N(int64(base/2)))
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
