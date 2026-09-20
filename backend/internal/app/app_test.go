package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aksh/calculator/backend/internal/calc"
	"github.com/aksh/calculator/backend/internal/config"
)

type fakeEvaluator struct{}

func (fakeEvaluator) Evaluate(_ context.Context, expression string) (calc.Result, error) {
	return calc.Result{Expression: expression, Value: "4"}, nil
}

func newTestApp(t *testing.T) *App {
	t.Helper()
	cfg, err := config.Load(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	return New(cfg, slog.New(slog.DiscardHandler), fakeEvaluator{})
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, http.NoBody))
	return rec
}

func TestApp_Readiness(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)

	if rec := get(a.Handler, "/readyz"); rec.Code != http.StatusOK || rec.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Fatalf("GET /readyz before shutdown = %d %q, want 200 {\"status\":\"ok\"}", rec.Code, rec.Body)
	}

	a.BeginShutdown()
	a.BeginShutdown() // idempotent

	rec := get(a.Handler, "/readyz")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /readyz after BeginShutdown = %d, want 503; body %s", rec.Code, rec.Body)
	}
	var problem struct {
		Code   string `json:"code"`
		Status int    `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem: %v; body %s", err, rec.Body)
	}
	if problem.Code != "NOT_READY" || problem.Status != http.StatusServiceUnavailable {
		t.Errorf("problem = %+v, want NOT_READY / 503", problem)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if rec := get(a.Handler, "/healthz"); rec.Code != http.StatusOK {
		t.Errorf("GET /healthz after BeginShutdown = %d, want 200 (liveness ignores shutdown)", rec.Code)
	}
}

func TestApp_WiresTheEvaluator(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evaluate", strings.NewReader(`{"expression":"2+2"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	a.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != "{\"expression\":\"2+2\",\"result\":\"4\"}\n" {
		t.Fatalf("POST /api/v1/evaluate = %d %q", rec.Code, rec.Body)
	}
}

func TestApp_AppliesConfiguredLimits(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load(func(k string) string {
		return map[string]string{"HTTP_MAX_BODY_BYTES": "32", "CORS_ALLOWED_ORIGINS": "http://localhost:5173"}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	a := New(cfg, slog.New(slog.DiscardHandler), fakeEvaluator{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/evaluate", strings.NewReader(`{"expression":"`+strings.Repeat("1", 40)+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()

	a.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413 with HTTP_MAX_BODY_BYTES=32", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("Access-Control-Allow-Origin = %q, want the configured origin", got)
	}
}
