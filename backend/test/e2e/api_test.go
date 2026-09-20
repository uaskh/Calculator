// Package e2e holds black-box tests that drive the fully wired service over real HTTP.
package e2e

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aksh/calculator/backend/internal/app"
	"github.com/aksh/calculator/backend/internal/config"
)

// newService starts the real handler graph on a loopback port.
func newService(t *testing.T, env map[string]string) *httptest.Server {
	t.Helper()
	cfg, err := config.Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	srv := httptest.NewServer(app.New(cfg, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	return srv
}

type response struct {
	status int
	header http.Header
	body   map[string]any
}

func newRequest(t *testing.T, srv *httptest.Server, method, path string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func send(t *testing.T, srv *httptest.Server, req *http.Request) response {
	t.Helper()
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := response{status: resp.StatusCode, header: resp.Header}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out.body); err != nil {
			t.Fatalf("%s %s: body is not JSON: %s", req.Method, req.URL.Path, raw)
		}
	}
	return out
}

func TestHealthEndpoints(t *testing.T) {
	t.Parallel()
	srv := newService(t, nil)

	for _, path := range []string{"/healthz", "/readyz"} {
		r := send(t, srv, newRequest(t, srv, http.MethodGet, path, http.NoBody))
		if r.status != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, r.status)
		}
		if ct := r.header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Errorf("GET %s Content-Type = %q", path, ct)
		}
	}
}

func TestErrorsUseProblemDetails(t *testing.T) {
	t.Parallel()
	srv := newService(t, nil)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantCode   string
	}{
		{"unknown path", http.MethodGet, "/api/v1/unknown", http.StatusNotFound, "NOT_FOUND"},
		{"wrong method", http.MethodDelete, "/healthz", http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := send(t, srv, newRequest(t, srv, tc.method, tc.path, http.NoBody))
			if r.status != tc.wantStatus {
				t.Fatalf("status = %d, want %d", r.status, tc.wantStatus)
			}
			if ct := r.header.Get("Content-Type"); ct != "application/problem+json" {
				t.Errorf("Content-Type = %q", ct)
			}
			if r.body["code"] != tc.wantCode || r.body["requestId"] != r.header.Get("X-Request-ID") {
				t.Errorf("body = %v", r.body)
			}
		})
	}
}

func TestRequestIDIsPropagated(t *testing.T) {
	t.Parallel()
	srv := newService(t, nil)
	req := newRequest(t, srv, http.MethodGet, "/healthz", http.NoBody)
	req.Header.Set("X-Request-ID", "trace-42")

	if got := send(t, srv, req).header.Get("X-Request-ID"); got != "trace-42" {
		t.Errorf("X-Request-ID = %q, want trace-42", got)
	}
}

func TestCORSPreflight(t *testing.T) {
	t.Parallel()
	srv := newService(t, map[string]string{"CORS_ALLOWED_ORIGINS": "http://localhost:5173"})
	req := newRequest(t, srv, http.MethodOptions, "/api/v1/anything", http.NoBody)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)

	r := send(t, srv, req)
	if r.status != http.StatusNoContent || r.header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Errorf("preflight = %d %v", r.status, r.header)
	}
}
