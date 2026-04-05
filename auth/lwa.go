// ABOUTME: Login-With-Amazon OAuth client used to obtain SP-API access tokens.
// ABOUTME: Provides an http.RoundTripper that injects x-amz-access-token on every request.

// Package auth provides the minimal authentication layer required by
// Amazon's Selling Partner API (SP-API). SP-API requires an LWA access
// token in the x-amz-access-token header on every request. Amazon
// deprecated the AWS SigV4 signing requirement in 2023, so a single
// LWA token is sufficient for standard (non-restricted) operations.
//
// Use [Client] to obtain access tokens directly, or wrap it in a
// [RoundTripper] and hand the resulting *http.Client to any generated
// SP-API client in this module. For operations that require a
// Restricted Data Token (RDT), fetch the RDT via the Tokens API and
// pass it through context with [WithTokenOverride].
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultEndpoint is the production LWA token endpoint.
const DefaultEndpoint = "https://api.amazon.com/auth/o2/token"

// Well-known grantless scopes. Grantless operations use
// grant_type=client_credentials with one of these scopes instead of a
// seller-issued refresh token. Full list lives in Amazon's docs.
const (
	ScopeNotifications = "sellingpartnerapi::notifications"
	ScopeMigration     = "sellingpartnerapi::migration"
	ScopeClientAuth    = "sellingpartnerapi::client_credential:rotation"
)

// refreshSkew is how much earlier than expires_in we consider a token
// stale. Five minutes matches the window used by Amazon's reference
// clients and protects against clock skew and request-time latency.
const refreshSkew = 5 * time.Minute

// Credentials holds the LWA application credentials. Fill RefreshToken
// for seller-grant flows, or Scopes (with RefreshToken empty) for
// grantless flows.
type Credentials struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
	Scopes       []string

	// Endpoint overrides the LWA token URL. Leave empty to use DefaultEndpoint.
	Endpoint string
}

// Error is returned by [Client.Token] when LWA responds with a
// non-2xx status. The Code/Description fields come straight from
// Amazon's JSON error body.
type Error struct {
	StatusCode  int
	Code        string `json:"error"`
	Description string `json:"error_description"`
}

func (e *Error) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("lwa: %s: %s (http %d)", e.Code, e.Description, e.StatusCode)
	}
	return fmt.Sprintf("lwa: %s (http %d)", e.Code, e.StatusCode)
}

// Client fetches and caches LWA access tokens. A zero Client is not
// usable; construct with [NewClient].
type Client struct {
	creds      Credentials
	httpClient *http.Client

	mu     sync.Mutex
	token  string
	expiry time.Time
}

// NewClient constructs a Client from the given credentials. It uses
// http.DefaultClient; call [Client.WithHTTPClient] to override.
func NewClient(creds Credentials) *Client {
	if creds.Endpoint == "" {
		creds.Endpoint = DefaultEndpoint
	}
	return &Client{creds: creds, httpClient: http.DefaultClient}
}

// WithHTTPClient swaps the HTTP client used for token requests and
// returns c for chaining. Call before issuing any requests: the
// underlying client carries cached token state.
func (c *Client) WithHTTPClient(h *http.Client) *Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.httpClient = h
	return c
}

// Token returns a valid access token, fetching a new one if the
// cached token is missing or within refreshSkew of its expiry.
func (c *Client) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.token != "" && time.Until(c.expiry) > refreshSkew {
		tok := c.token
		c.mu.Unlock()
		return tok, nil
	}
	c.mu.Unlock()

	// Refresh outside the lock so a slow LWA response doesn't block
	// callers observing an already-valid token. Re-check inside the
	// lock after the fetch to avoid thundering herds.
	tok, exp, err := c.fetch(ctx)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	c.token = tok
	c.expiry = exp
	c.mu.Unlock()
	return tok, nil
}

func (c *Client) fetch(ctx context.Context) (string, time.Time, error) {
	form := url.Values{}
	form.Set("client_id", c.creds.ClientID)
	form.Set("client_secret", c.creds.ClientSecret)
	if c.creds.RefreshToken != "" {
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", c.creds.RefreshToken)
	} else {
		form.Set("grant_type", "client_credentials")
		form.Set("scope", strings.Join(c.creds.Scopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.creds.Endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("lwa: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("lwa: do request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("lwa: read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		lwaErr := &Error{StatusCode: resp.StatusCode}
		_ = json.Unmarshal(body, lwaErr) // body may be empty/garbage; fall through
		if lwaErr.Code == "" {
			lwaErr.Code = "unknown_error"
			lwaErr.Description = string(body)
		}
		return "", time.Time{}, lwaErr
	}

	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", time.Time{}, fmt.Errorf("lwa: decode response: %w", err)
	}
	if payload.AccessToken == "" {
		return "", time.Time{}, fmt.Errorf("lwa: empty access_token in response")
	}
	exp := time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second)
	return payload.AccessToken, exp, nil
}

// --- RoundTripper ---

type ctxKey int

const tokenOverrideKey ctxKey = iota

// WithTokenOverride returns a context that causes [RoundTripper] to
// use the given token instead of calling LWA. Use this to pass a
// Restricted Data Token (RDT) obtained from the Tokens API for
// operations that require one.
func WithTokenOverride(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenOverrideKey, token)
}

// RoundTripper wraps an underlying http.RoundTripper and attaches
// x-amz-access-token on every outgoing request. Pass the resulting
// *http.Client to the generated API Configuration.HTTPClient field.
type RoundTripper struct {
	client *Client
	base   http.RoundTripper
}

// NewRoundTripper constructs a RoundTripper. If base is nil,
// http.DefaultTransport is used.
func NewRoundTripper(client *Client, base http.RoundTripper) *RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &RoundTripper{client: client, base: base}
}

// RoundTrip implements http.RoundTripper.
func (rt *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Don't mutate the caller's request; clone per the RoundTripper contract.
	req = req.Clone(req.Context())

	if override, ok := req.Context().Value(tokenOverrideKey).(string); ok && override != "" {
		req.Header.Set("x-amz-access-token", override)
		return rt.base.RoundTrip(req)
	}

	tok, err := rt.client.Token(req.Context())
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-amz-access-token", tok)
	return rt.base.RoundTrip(req)
}
