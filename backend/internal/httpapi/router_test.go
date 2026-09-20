package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testDeps() Deps {
	return Deps{
		Logger:         slog.New(slog.DiscardHandler),
		MaxBodyBytes:   1 << 10,
		RequestTimeout: time.Second,
		AllowedOrigins: []string{"http://localhost:5173"},
	}
}

func serve(h http.Handler, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, http.NoBody))
	return rec
}

func decodeProblemBody(t *testing.T, rec *httptest.ResponseRecorder) Problem {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != problemContentType {
		t.Fatalf("Content-Type = %q, want %q", ct, problemContentType)
	}
	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode problem: %v; body %s", err, rec.Body)
	}
	return p
}

func TestRouter_Health(t *testing.T) {
	t.Parallel()
	h := NewRouter(testDeps())

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		rec := serve(h, method, "/healthz")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s /healthz status = %d, want 200", method, rec.Code)
		}
		if rec.Header().Get(requestIDHeader) == "" {
			t.Errorf("%s /healthz: missing %s header", method, requestIDHeader)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s /healthz: missing security headers", method)
		}
	}
	rec := serve(h, http.MethodGet, "/healthz")
	if got := rec.Body.String(); got != "{\"status\":\"ok\"}\n" {
		t.Errorf("body = %q", got)
	}
}

func TestRouter_NotFound(t *testing.T) {
	t.Parallel()
	rec := serve(NewRouter(testDeps()), http.MethodGet, "/does-not-exist")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	p := decodeProblemBody(t, rec)
	if p.Code != CodeNotFound || p.Status != http.StatusNotFound || p.Instance != "/does-not-exist" {
		t.Errorf("problem = %+v", p)
	}
	if p.RequestID == "" || p.RequestID != rec.Header().Get(requestIDHeader) {
		t.Errorf("requestId = %q, header = %q", p.RequestID, rec.Header().Get(requestIDHeader))
	}
}

func TestRouter_MethodNotAllowed(t *testing.T) {
	t.Parallel()
	rec := serve(NewRouter(testDeps()), http.MethodPost, "/healthz")

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != "GET, HEAD" {
		t.Errorf("Allow = %q, want %q", allow, "GET, HEAD")
	}
	if p := decodeProblemBody(t, rec); p.Code != CodeMethodNotAllowed {
		t.Errorf("code = %q", p.Code)
	}
}

func TestRouter_DefaultLogger(t *testing.T) {
	t.Parallel()
	rec := serve(NewRouter(Deps{MaxBodyBytes: 1, RequestTimeout: time.Second}), http.MethodGet, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestReadiness(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		check      func(context.Context) error
		wantStatus int
	}{
		{"no check", nil, http.StatusOK},
		{"healthy", func(context.Context) error { return nil }, http.StatusOK},
		{"failing", func(context.Context) error { return errors.New("warming up") }, http.StatusServiceUnavailable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := testDeps()
			d.Ready = tc.check
			rec := serve(NewRouter(d), http.MethodGet, "/readyz")
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantStatus != http.StatusOK {
				if p := decodeProblemBody(t, rec); p.Code != CodeNotReady {
					t.Errorf("code = %q, want %q", p.Code, CodeNotReady)
				}
			}
		})
	}
}

func TestWriteJSON_EncodingFailureBecomesProblem(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	h := requestID(slog.New(slog.NewTextHandler(&logs, nil)))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, http.StatusOK, map[string]float64{"value": math.NaN()})
	}))
	rec := serve(h, http.MethodGet, "/x")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if p := decodeProblemBody(t, rec); p.Code != CodeInternal {
		t.Errorf("code = %q", p.Code)
	}
	if !bytes.Contains(logs.Bytes(), []byte("encode response")) {
		t.Errorf("encoding error was not logged: %s", logs.String())
	}
}
