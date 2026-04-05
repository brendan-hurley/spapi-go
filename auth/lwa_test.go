// ABOUTME: Tests for the LWA OAuth client and RoundTripper used by SP-API.
// ABOUTME: Uses httptest.Server so we exercise real HTTP/JSON paths (no mocks).

package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeLWAServer stands up an httptest.Server that behaves like the real
// LWA token endpoint at https://api.amazon.com/auth/o2/token. Returning
// the number of hits lets tests assert caching behavior.
type fakeLWAServer struct {
	*httptest.Server
	hits *int32
}

func newFakeLWAServer(t *testing.T, tokenValue string, expiresIn int) *fakeLWAServer {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
			t.Errorf("expected form content-type, got %q", ct)
		}
		body, _ := io.ReadAll(r.Body)
		form, err := url.ParseQuery(string(body))
		if err != nil {
			t.Errorf("parse form: %v", err)
		}
		// All flows must present credentials.
		if form.Get("client_id") == "" || form.Get("client_secret") == "" {
			http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": tokenValue,
			"token_type":   "bearer",
			"expires_in":   expiresIn,
		})
	}))
	return &fakeLWAServer{Server: srv, hits: &hits}
}

func (f *fakeLWAServer) Hits() int32 { return atomic.LoadInt32(f.hits) }

func testCreds(endpoint string) Credentials {
	return Credentials{
		ClientID:     "amzn1.application-oa2-client.abc",
		ClientSecret: "super-secret",
		RefreshToken: "Atzr|refresh",
		Endpoint:     endpoint,
	}
}

func TestClient_FetchesTokenWithRefreshGrant(t *testing.T) {
	srv := newFakeLWAServer(t, "Atza|real-token", 3600)
	defer srv.Close()

	// Intercept the form to check grant fields.
	var capturedForm url.Values
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedForm, _ = url.ParseQuery(string(body))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "Atza|real-token",
			"token_type":   "bearer",
			"expires_in":   3600,
		})
	})

	c := NewClient(testCreds(srv.URL))
	tok, err := c.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok != "Atza|real-token" {
		t.Errorf("token = %q, want Atza|real-token", tok)
	}
	if capturedForm.Get("grant_type") != "refresh_token" {
		t.Errorf("grant_type = %q, want refresh_token", capturedForm.Get("grant_type"))
	}
	if capturedForm.Get("refresh_token") != "Atzr|refresh" {
		t.Errorf("refresh_token = %q", capturedForm.Get("refresh_token"))
	}
}

func TestClient_GrantlessFlowUsesScope(t *testing.T) {
	srv := newFakeLWAServer(t, "Atza|grantless", 3600)
	defer srv.Close()

	var capturedForm url.Values
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedForm, _ = url.ParseQuery(string(body))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "Atza|grantless",
			"token_type":   "bearer",
			"expires_in":   3600,
		})
	})

	creds := testCreds(srv.URL)
	creds.RefreshToken = "" // grantless
	creds.Scopes = []string{ScopeNotifications, ScopeMigration}
	c := NewClient(creds)

	if _, err := c.Token(context.Background()); err != nil {
		t.Fatalf("Token: %v", err)
	}
	if capturedForm.Get("grant_type") != "client_credentials" {
		t.Errorf("grant_type = %q, want client_credentials", capturedForm.Get("grant_type"))
	}
	gotScope := capturedForm.Get("scope")
	// Order isn't guaranteed; check both present.
	if !strings.Contains(gotScope, ScopeNotifications) || !strings.Contains(gotScope, ScopeMigration) {
		t.Errorf("scope = %q, want both scopes present", gotScope)
	}
}

func TestClient_CachesTokenUntilExpiry(t *testing.T) {
	srv := newFakeLWAServer(t, "Atza|cached", 3600)
	defer srv.Close()

	c := NewClient(testCreds(srv.URL))
	for i := range 5 {
		if _, err := c.Token(context.Background()); err != nil {
			t.Fatalf("Token #%d: %v", i, err)
		}
	}
	if got := srv.Hits(); got != 1 {
		t.Errorf("server hit %d times, want 1 (subsequent calls should be cached)", got)
	}
}

func TestClient_RefreshesWhenTokenNearExpiry(t *testing.T) {
	// expires_in=60 means the token is within the 5-min safety margin, so
	// every call should force a refresh.
	srv := newFakeLWAServer(t, "Atza|short-lived", 60)
	defer srv.Close()

	c := NewClient(testCreds(srv.URL))
	for i := range 3 {
		if _, err := c.Token(context.Background()); err != nil {
			t.Fatalf("Token #%d: %v", i, err)
		}
	}
	if got := srv.Hits(); got != 3 {
		t.Errorf("server hit %d times, want 3 (each call should refresh)", got)
	}
}

func TestClient_ReturnsErrorOnUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_grant",
			"error_description": "The refresh token is invalid.",
		})
	}))
	defer srv.Close()

	c := NewClient(testCreds(srv.URL))
	_, err := c.Token(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var lwaErr *Error
	if !asError(err, &lwaErr) {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if lwaErr.Code != "invalid_grant" {
		t.Errorf("Code = %q, want invalid_grant", lwaErr.Code)
	}
}

func TestClient_RespectsContextCancellation(t *testing.T) {
	// Handler sleeps longer than the client context timeout so the
	// client's context deadline must fire first. Using time.Sleep
	// bounded by a select on r.Context().Done() also lets the handler
	// return promptly after cancellation so httptest.Server.Close()
	// doesn't block at teardown.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	c := NewClient(testCreds(srv.URL))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.Token(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRoundTripper_InjectsAccessTokenHeader(t *testing.T) {
	// LWA fake server.
	lwa := newFakeLWAServer(t, "Atza|rt-token", 3600)
	defer lwa.Close()

	// Target SP-API server: asserts it sees the access token header.
	var gotHeader string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("x-amz-access-token")
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	httpClient := &http.Client{Transport: NewRoundTripper(NewClient(testCreds(lwa.URL)), nil)}
	resp, err := httpClient.Get(api.URL + "/orders/v0/orders")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if gotHeader != "Atza|rt-token" {
		t.Errorf("x-amz-access-token = %q, want Atza|rt-token", gotHeader)
	}
}

func TestRoundTripper_UsesContextOverrideToken(t *testing.T) {
	// No LWA server needed — override bypasses the fetch entirely.
	var gotHeader string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("x-amz-access-token")
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	rt := NewRoundTripper(NewClient(testCreds("http://invalid.example")), nil)
	httpClient := &http.Client{Transport: rt}

	req, _ := http.NewRequestWithContext(
		WithTokenOverride(context.Background(), "Atzr|rdt-token-12345"),
		http.MethodGet, api.URL+"/orders/v0/orders/X", nil,
	)
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if gotHeader != "Atzr|rdt-token-12345" {
		t.Errorf("x-amz-access-token = %q, want override", gotHeader)
	}
}

// asError is a tiny local errors.As without importing errors in test file header.
func asError(err error, target **Error) bool {
	for e := err; e != nil; {
		if t, ok := e.(*Error); ok {
			*target = t
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := e.(unwrapper)
		if !ok {
			return false
		}
		e = u.Unwrap()
	}
	return false
}
