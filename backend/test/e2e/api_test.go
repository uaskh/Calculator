// Package e2e holds black-box tests that drive the fully wired service over real HTTP.
package e2e

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/aksh/calculator/backend/internal/app"
	"github.com/aksh/calculator/backend/internal/calc"
	"github.com/aksh/calculator/backend/internal/config"
)

// newApp wires the real handler graph, including the real calculator domain, and serves
// it on a loopback port.
func newApp(t *testing.T, env map[string]string) (*app.App, *httptest.Server) {
	t.Helper()
	cfg, err := config.Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	a := app.New(cfg, slog.New(slog.DiscardHandler), calc.New())
	srv := httptest.NewServer(a.Handler)
	t.Cleanup(srv.Close)
	return a, srv
}

func newService(t *testing.T, env map[string]string) *httptest.Server {
	t.Helper()
	_, srv := newApp(t, env)
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

var generatedRequestID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// securityHeaders are promised on every response (spec §6, plan A-23).
var securityHeaders = map[string]string{
	"X-Content-Type-Options":     "nosniff",
	"X-Frame-Options":            "DENY",
	"Referrer-Policy":            "no-referrer",
	"Permissions-Policy":         "camera=(), microphone=(), geolocation=()",
	"Cross-Origin-Opener-Policy": "same-origin",
	"Content-Security-Policy":    "default-src 'none'; frame-ancestors 'none'",
}

func assertCommonHeaders(t *testing.T, h http.Header) {
	t.Helper()
	if id := h.Get("X-Request-ID"); !generatedRequestID.MatchString(id) {
		t.Errorf("X-Request-ID = %q, want 32 lower-case hex characters", id)
	}
	if got := h.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	for header, want := range securityHeaders {
		if got := h.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
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
		if len(r.body) != 1 || r.body["status"] != "ok" {
			t.Errorf("GET %s body = %v, want {\"status\":\"ok\"}", path, r.body)
		}
		assertCommonHeaders(t, r.header)
	}
}

func TestReadinessDuringShutdown(t *testing.T) {
	t.Parallel()
	a, srv := newApp(t, nil)

	a.BeginShutdown()

	r := send(t, srv, newRequest(t, srv, http.MethodGet, "/readyz", http.NoBody))
	if r.status != http.StatusServiceUnavailable || r.body["code"] != "NOT_READY" {
		t.Errorf("GET /readyz during shutdown = %d %v, want 503 NOT_READY", r.status, r.body)
	}
	if ct := r.header.Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q", ct)
	}
	assertCommonHeaders(t, r.header)

	if live := send(t, srv, newRequest(t, srv, http.MethodGet, "/healthz", http.NoBody)); live.status != http.StatusOK {
		t.Errorf("GET /healthz during shutdown = %d, want 200", live.status)
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
		wantAllow  string
	}{
		{"unknown path", http.MethodGet, "/api/v1/unknown", http.StatusNotFound, "NOT_FOUND", ""},
		{"wrong method on health", http.MethodDelete, "/healthz", http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET, HEAD"},
		{"wrong method on evaluate", http.MethodGet, "/api/v1/evaluate", http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "POST"},
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
			if r.body["code"] != tc.wantCode || r.body["requestId"] != r.header.Get("X-Request-ID") ||
				r.body["status"] != float64(tc.wantStatus) || r.body["instance"] != tc.path {
				t.Errorf("body = %v", r.body)
			}
			if got := r.header.Get("Allow"); got != tc.wantAllow {
				t.Errorf("Allow = %q, want %q", got, tc.wantAllow)
			}
			assertCommonHeaders(t, r.header)
		})
	}
}

func TestRequestID(t *testing.T) {
	t.Parallel()
	srv := newService(t, nil)
	tests := []struct {
		name     string
		incoming string
		echoed   bool
	}{
		{"valid id is echoed", "trace.42_A-b", true},
		{"too long id is replaced", strings.Repeat("x", 65), false},
		{"id with a space is replaced", "trace 42", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := newRequest(t, srv, http.MethodGet, "/healthz", http.NoBody)
			req.Header.Set("X-Request-ID", tc.incoming)

			got := send(t, srv, req).header.Get("X-Request-ID")

			if tc.echoed && got != tc.incoming {
				t.Errorf("X-Request-ID = %q, want %q echoed", got, tc.incoming)
			}
			if !tc.echoed && !generatedRequestID.MatchString(got) {
				t.Errorf("X-Request-ID = %q, want a generated ID", got)
			}
		})
	}
}

func TestCORS(t *testing.T) {
	t.Parallel()
	const allowed = "http://localhost:5173"

	t.Run("off by default", func(t *testing.T) {
		t.Parallel()
		srv := newService(t, nil)
		req := newRequest(t, srv, http.MethodGet, "/healthz", http.NoBody)
		req.Header.Set("Origin", allowed)

		r := send(t, srv, req)
		if r.header.Get("Access-Control-Allow-Origin") != "" || r.header.Get("Vary") != "" {
			t.Errorf("CORS headers present without configuration: %v", r.header)
		}
	})

	t.Run("allowed origin", func(t *testing.T) {
		t.Parallel()
		srv := newService(t, map[string]string{"CORS_ALLOWED_ORIGINS": allowed})
		req := newRequest(t, srv, http.MethodGet, "/healthz", http.NoBody)
		req.Header.Set("Origin", allowed)

		r := send(t, srv, req)
		if r.header.Get("Access-Control-Allow-Origin") != allowed || r.header.Get("Vary") != "Origin" ||
			r.header.Get("Access-Control-Expose-Headers") != "X-Request-ID" {
			t.Errorf("CORS headers = %v", r.header)
		}
	})

	t.Run("other origin gets nothing", func(t *testing.T) {
		t.Parallel()
		srv := newService(t, map[string]string{"CORS_ALLOWED_ORIGINS": allowed})
		req := newRequest(t, srv, http.MethodGet, "/healthz", http.NoBody)
		req.Header.Set("Origin", "https://evil.example")

		r := send(t, srv, req)
		for _, header := range []string{"Access-Control-Allow-Origin", "Access-Control-Expose-Headers"} {
			if got := r.header.Get(header); got != "" {
				t.Errorf("%s = %q for a refused origin", header, got)
			}
		}
	})

	t.Run("preflight", func(t *testing.T) {
		t.Parallel()
		srv := newService(t, map[string]string{"CORS_ALLOWED_ORIGINS": allowed})
		req := newRequest(t, srv, http.MethodOptions, "/api/v1/evaluate", http.NoBody)
		req.Header.Set("Origin", allowed)
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)

		r := send(t, srv, req)
		want := map[string]string{
			"Access-Control-Allow-Origin":  allowed,
			"Access-Control-Allow-Methods": "GET, POST",
			"Access-Control-Allow-Headers": "Content-Type, X-Request-ID",
			"Access-Control-Max-Age":       "600",
		}
		if r.status != http.StatusNoContent {
			t.Errorf("preflight status = %d, want 204", r.status)
		}
		for header, value := range want {
			if got := r.header.Get(header); got != value {
				t.Errorf("%s = %q, want %q", header, got, value)
			}
		}
	})
}
