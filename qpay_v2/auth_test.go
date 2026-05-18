package qpay_v2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"resty.dev/v3"
)

// newMockServer creates a test server that returns configurable auth responses.
func newMockServer(t *testing.T, authCalls *atomic.Int32, refreshCalls *atomic.Int32, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	if handler != nil {
		return httptest.NewServer(handler)
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := qpayLoginResponse{
			TokenType:        "bearer",
			AccessToken:      "test-access-token",
			RefreshToken:     "test-refresh-token",
			ExpiresIn:        time.Now().Add(24 * time.Hour).Unix(),
			RefreshExpiresIn: time.Now().Add(48 * time.Hour).Unix(),
			Scope:            "get_token",
			SessionState:     "test",
		}

		switch r.URL.Path {
		case "/auth/token":
			authCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		case "/auth/refresh":
			refreshCalls.Add(1)
			resp.AccessToken = "refreshed-access-token"
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func newTestQPay(endpoint string) *qpay {
	q := &qpay{
		endpoint:    endpoint,
		username:    "test-user",
		password:    "test-pass",
		invoiceCode: "TEST",
		client:      resty.New().SetTimeout(5 * time.Second),
		plugins:     map[string]Plugin{},
	}
	q.cbs = initializeCallbacks(q)
	registerDefaultCallbacks(q)
	return q
}

func TestTokenValid_CachedWhenFresh(t *testing.T) {
	var authCalls, refreshCalls atomic.Int32
	srv := newMockServer(t, &authCalls, &refreshCalls, nil)
	defer srv.Close()

	q := newTestQPay(srv.URL)

	// First call — should hit /auth/token
	_, err := q.authQPayV2()
	if err != nil {
		t.Fatalf("first auth failed: %v", err)
	}
	if authCalls.Load() != 1 {
		t.Fatalf("expected 1 auth call, got %d", authCalls.Load())
	}

	// Second call — should use cache, no network
	_, err = q.authQPayV2()
	if err != nil {
		t.Fatalf("cached auth failed: %v", err)
	}
	if authCalls.Load() != 1 {
		t.Fatalf("expected still 1 auth call (cached), got %d", authCalls.Load())
	}
}

func TestTokenExpired_ReauthCalled(t *testing.T) {
	var authCalls, refreshCalls atomic.Int32
	srv := newMockServer(t, &authCalls, &refreshCalls, nil)
	defer srv.Close()

	q := newTestQPay(srv.URL)

	// First auth
	_, err := q.authQPayV2()
	if err != nil {
		t.Fatalf("first auth failed: %v", err)
	}

	// Force token expiry
	q.mu.Lock()
	q.loginObject.ExpiresIn = time.Now().Add(-1 * time.Hour).Unix()
	q.mu.Unlock()

	// Should use refresh token (refresh_expires_in is still valid)
	_, err = q.authQPayV2()
	if err != nil {
		t.Fatalf("refresh auth failed: %v", err)
	}
	if refreshCalls.Load() != 1 {
		t.Fatalf("expected 1 refresh call, got %d", refreshCalls.Load())
	}
}

func TestRefreshExpired_FullAuthFallback(t *testing.T) {
	var authCalls, refreshCalls atomic.Int32
	srv := newMockServer(t, &authCalls, &refreshCalls, nil)
	defer srv.Close()

	q := newTestQPay(srv.URL)

	// First auth
	_, err := q.authQPayV2()
	if err != nil {
		t.Fatalf("first auth failed: %v", err)
	}

	// Force both tokens expired
	q.mu.Lock()
	q.loginObject.ExpiresIn = time.Now().Add(-1 * time.Hour).Unix()
	q.loginObject.RefreshExpiresIn = time.Now().Add(-1 * time.Hour).Unix()
	q.mu.Unlock()

	// Should fallback to full auth
	_, err = q.authQPayV2()
	if err != nil {
		t.Fatalf("fallback auth failed: %v", err)
	}
	if authCalls.Load() != 2 {
		t.Fatalf("expected 2 auth calls (initial + fallback), got %d", authCalls.Load())
	}
	if refreshCalls.Load() != 0 {
		t.Fatalf("expected 0 refresh calls, got %d", refreshCalls.Load())
	}
}

func TestRefreshFails_FallsBackToFullAuth(t *testing.T) {
	var authCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := qpayLoginResponse{
			TokenType:        "bearer",
			AccessToken:      "test-access-token",
			RefreshToken:     "test-refresh-token",
			ExpiresIn:        time.Now().Add(24 * time.Hour).Unix(),
			RefreshExpiresIn: time.Now().Add(48 * time.Hour).Unix(),
			Scope:            "get_token",
		}
		switch r.URL.Path {
		case "/auth/token":
			authCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		case "/auth/refresh":
			// Refresh always fails
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"invalid_token"}`))
		}
	}))
	defer srv.Close()

	q := newTestQPay(srv.URL)

	// First auth
	_, err := q.authQPayV2()
	if err != nil {
		t.Fatalf("first auth failed: %v", err)
	}

	// Force access token expiry (refresh token still valid)
	q.mu.Lock()
	q.loginObject.ExpiresIn = time.Now().Add(-1 * time.Hour).Unix()
	q.mu.Unlock()

	// Refresh will fail → should fallback to full auth
	_, err = q.authQPayV2()
	if err != nil {
		t.Fatalf("fallback auth failed: %v", err)
	}
	if authCalls.Load() != 2 {
		t.Fatalf("expected 2 full auth calls (initial + fallback after refresh fail), got %d", authCalls.Load())
	}
}

func TestSingleflight_ConcurrentCallsMakeOneRequest(t *testing.T) {
	var authCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authCalls.Add(1)
		// Simulate slow auth
		time.Sleep(100 * time.Millisecond)
		resp := qpayLoginResponse{
			TokenType:        "bearer",
			AccessToken:      "test-access-token",
			RefreshToken:     "test-refresh-token",
			ExpiresIn:        time.Now().Add(24 * time.Hour).Unix(),
			RefreshExpiresIn: time.Now().Add(48 * time.Hour).Unix(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	q := newTestQPay(srv.URL)

	// Launch 50 concurrent auth calls
	var wg sync.WaitGroup
	errors := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := q.authQPayV2()
			if err != nil {
				errors <- err
			}
		}()
	}
	wg.Wait()
	close(errors)

	for err := range errors {
		t.Fatalf("concurrent auth failed: %v", err)
	}

	// Singleflight should collapse all 50 into 1 call
	if authCalls.Load() != 1 {
		t.Fatalf("expected 1 auth call (singleflight), got %d", authCalls.Load())
	}
}

func TestAuthServerDown_ReturnsError(t *testing.T) {
	// Point to a closed server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	q := newTestQPay(srv.URL)

	_, err := q.authQPayV2()
	if err == nil {
		t.Fatal("expected error when server is down, got nil")
	}
}
