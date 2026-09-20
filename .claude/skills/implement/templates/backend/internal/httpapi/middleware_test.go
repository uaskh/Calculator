package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

var ok = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestRequestID(t *testing.T) {
	t.Parallel()
	generated := regexp.MustCompile(`^[0-9a-f]{32}$`)
	tests := []struct {
		name     string
		incoming string
		keep     bool
	}{
		{"generated when absent", "", false},
		{"propagated when valid", "abc-123_XYZ", true},
		{"replaced when too long", strings.Repeat("a", 129), false},
		{"replaced when it has spaces", "abc 123", false},
		{"replaced when it has control characters", "abc\x01", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var seen string
			h := requestID(discardLogger())(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				seen = RequestIDFrom(r.Context())
			}))
			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			if tc.incoming != "" {
				req.Header.Set(requestIDHeader, tc.incoming)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if got := rec.Header().Get(requestIDHeader); got != seen {
				t.Fatalf("header %q != context %q", got, seen)
			}
			if tc.keep && seen != tc.incoming {
				t.Errorf("request ID = %q, want %q", seen, tc.incoming)
			}
			if !tc.keep && !generated.MatchString(seen) {
				t.Errorf("request ID = %q, want a generated 32-char hex ID", seen)
			}
		})
	}
}

func TestAccessLog(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	h := chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("short and stout"))
	}), requestID(slog.New(slog.NewJSONHandler(&logs, nil))), accessLog)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/brew", http.NoBody))

	for _, want := range []string{`"msg":"http request"`, `"path":"/brew"`, `"status":418`, `"bytes":15`, `"request_id":`} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log %s does not contain %s", logs.String(), want)
		}
	}
}

func TestAccessLog_ServerErrorsLogAtErrorLevel(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	h := chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}), requestID(slog.New(slog.NewJSONHandler(&logs, nil))), accessLog)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", http.NoBody))

	if !strings.Contains(logs.String(), `"level":"ERROR"`) {
		t.Errorf("log %s is not at ERROR level", logs.String())
	}
}

func TestRecoverer(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	h := chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}), requestID(slog.New(slog.NewTextHandler(&logs, nil))), recoverer)

	rec := serve(h, http.MethodGet, "/")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if p := decodeProblemBody(t, rec); p.Code != CodeInternal || p.Detail != "" {
		t.Errorf("problem = %+v, want generic internal error without details", p)
	}
	if !strings.Contains(logs.String(), "boom") {
		t.Errorf("panic was not logged: %s", logs.String())
	}
}

func TestRecoverer_RepanicsAbortHandler(t *testing.T) {
	t.Parallel()
	defer func() {
		if v := recover(); v != http.ErrAbortHandler { //nolint:errorlint // identity comparison, as in net/http
			t.Errorf("recover() = %v, want http.ErrAbortHandler", v)
		}
	}()
	h := recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) }))
	serve(h, http.MethodGet, "/")
}

func TestCORS(t *testing.T) {
	t.Parallel()
	const allowed = "http://localhost:5173"
	tests := []struct {
		name        string
		allowList   []string
		method      string
		origin      string
		preflight   bool
		wantStatus  int
		wantOrigin  string
		wantMethods bool
		wantVary    bool
	}{
		{"same origin request", []string{allowed}, http.MethodGet, "", false, http.StatusOK, "", false, true},
		{"allowed origin", []string{allowed}, http.MethodGet, allowed, false, http.StatusOK, allowed, false, true},
		{"disallowed origin", []string{allowed}, http.MethodGet, "https://evil.example", false, http.StatusOK, "", false, true},
		{"allowed preflight", []string{allowed}, http.MethodOptions, allowed, true, http.StatusNoContent, allowed, true, true},
		{"disallowed preflight", []string{allowed}, http.MethodOptions, "https://evil.example", true, http.StatusOK, "", false, true},
		{"wildcard", []string{"*"}, http.MethodGet, "https://any.example", false, http.StatusOK, "https://any.example", false, true},
		{"empty allow-list", nil, http.MethodGet, allowed, false, http.StatusOK, "", false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(tc.method, "/", http.NoBody)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.preflight {
				req.Header.Set("Access-Control-Request-Method", http.MethodPost)
			}
			rec := httptest.NewRecorder()
			cors(tc.allowList)(ok).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tc.wantOrigin {
				t.Errorf("Allow-Origin = %q, want %q", got, tc.wantOrigin)
			}
			if got := rec.Header().Get("Access-Control-Allow-Methods") != ""; got != tc.wantMethods {
				t.Errorf("Allow-Methods present = %v, want %v", got, tc.wantMethods)
			}
			if got := slices.Contains(rec.Header().Values("Vary"), "Origin"); got != tc.wantVary {
				t.Errorf("Vary: Origin present = %v, want %v", got, tc.wantVary)
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	t.Parallel()
	rec := serve(securityHeaders(ok), http.MethodGet, "/")
	for header, want := range map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestRequestTimeout(t *testing.T) {
	t.Parallel()
	var deadline time.Time
	h := requestTimeout(time.Minute)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		deadline, _ = r.Context().Deadline()
	}))
	serve(h, http.MethodGet, "/")

	if remaining := time.Until(deadline); remaining <= 0 || remaining > time.Minute {
		t.Errorf("deadline in %s, want within 1m", remaining)
	}
}

func TestRequestTimeout_ZeroSetsNoDeadline(t *testing.T) {
	t.Parallel()
	var (
		hasDeadline bool
		ctxErr      error
	)
	h := requestTimeout(0)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, hasDeadline = r.Context().Deadline()
		ctxErr = r.Context().Err()
	}))
	serve(h, http.MethodGet, "/")

	if hasDeadline || ctxErr != nil {
		t.Errorf("deadline set = %v, context error = %v; want neither", hasDeadline, ctxErr)
	}
}

func TestChainOrder(t *testing.T) {
	t.Parallel()
	var order []string
	mark := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	serve(chain(ok, mark("outer"), mark("inner")), http.MethodGet, "/")

	if strings.Join(order, ",") != "outer,inner" {
		t.Errorf("order = %v, want [outer inner]", order)
	}
}

func TestLoggerFrom_DefaultsWithoutMiddleware(t *testing.T) {
	t.Parallel()
	if loggerFrom(httptest.NewRequest(http.MethodGet, "/", http.NoBody).Context()) == nil {
		t.Fatal("loggerFrom() = nil")
	}
}
