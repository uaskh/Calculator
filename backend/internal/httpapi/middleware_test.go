package httpapi

import (
	"bytes"
	"encoding/json"
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
		{"propagated when valid", "abc.DEF_1-2", true},
		{"propagated at 64 characters", strings.Repeat("a", 64), true},
		{"replaced at 65 characters", strings.Repeat("a", 65), false},
		{"replaced when it has spaces", "abc 123", false},
		{"replaced when it has non-ascii letters", "café", false},
		{"replaced when it has control characters", "abc\x01", false},
		{"replaced when it has other punctuation", "abc/123", false},
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

func TestRequestID_GeneratedIDsAreUnique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool)
	for range 64 {
		id := newRequestID()
		if seen[id] {
			t.Fatalf("duplicate generated request ID %q", id)
		}
		seen[id] = true
	}
}

// FuzzValidRequestID pins the hand-written validator to the contract's pattern.
func FuzzValidRequestID(f *testing.F) {
	for _, seed := range []string{"", "abc.DEF_1-2", strings.Repeat("a", 64), strings.Repeat("a", 65), "abc 123", "café", "\x00", "a/b"} {
		f.Add(seed)
	}
	pattern := regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	f.Fuzz(func(t *testing.T, id string) {
		if got, want := validRequestID(id), pattern.MatchString(id); got != want {
			t.Fatalf("validRequestID(%q) = %v, want %v", id, got, want)
		}
	})
}

// logLine decodes the single access-log line written by a request.
func logLine(t *testing.T, logs *bytes.Buffer) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	var entry map[string]any
	for _, line := range lines {
		var candidate map[string]any
		if err := json.Unmarshal([]byte(line), &candidate); err != nil {
			t.Fatalf("log line is not JSON: %s", line)
		}
		if candidate["msg"] == "http request" {
			if entry != nil {
				t.Fatalf("more than one access-log line: %s", logs.String())
			}
			entry = candidate
		}
	}
	if entry == nil {
		t.Fatalf("no access-log line in: %s", logs.String())
	}
	return entry
}

func TestAccessLog(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	h := chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("short and stout"))
	}), requestID(slog.New(slog.NewJSONHandler(&logs, nil))), accessLog)
	req := httptest.NewRequest(http.MethodGet, "/brew?strong=1", http.NoBody)
	req.Header.Set(requestIDHeader, "trace-1")
	req.Header.Set("User-Agent", "kettle/2.0")
	req.RemoteAddr = "192.0.2.10:4242"

	h.ServeHTTP(httptest.NewRecorder(), req)

	entry := logLine(t, &logs)
	want := map[string]any{
		"level":       "INFO",
		"request_id":  "trace-1",
		"method":      "GET",
		"path":        "/brew",
		"status":      float64(http.StatusTeapot),
		"bytes":       float64(15),
		"remote_addr": "192.0.2.10:4242",
		"user_agent":  "kettle/2.0",
	}
	for key, value := range want {
		if entry[key] != value {
			t.Errorf("log[%s] = %v (%T), want %v", key, entry[key], entry[key], value)
		}
	}
	if ms, isNumber := entry["duration_ms"].(float64); !isNumber || ms < 0 || ms != float64(int64(ms)) {
		t.Errorf("duration_ms = %v (%T), want a non-negative integer", entry["duration_ms"], entry["duration_ms"])
	}
	if _, present := entry["error"]; present {
		t.Errorf("a 418 response logged an error: %v", entry["error"])
	}
}

func TestAccessLog_DefaultsToStatusOK(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	h := chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("implicit 200"))
	}), requestID(slog.New(slog.NewJSONHandler(&logs, nil))), accessLog)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", http.NoBody))

	if entry := logLine(t, &logs); entry["status"] != float64(http.StatusOK) || entry["bytes"] != float64(12) {
		t.Errorf("log = %v, want status 200 and 12 bytes", entry)
	}
}

func TestAccessLog_ServerErrorsCarryTheProblem(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	h := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeProblem(w, r, Problem{Status: http.StatusServiceUnavailable, Code: CodeTimeout, Detail: "The calculation took too long."})
	}), requestID(slog.New(slog.NewJSONHandler(&logs, nil))), accessLog)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", http.NoBody))

	entry := logLine(t, &logs)
	if entry["level"] != "ERROR" || entry["status"] != float64(http.StatusServiceUnavailable) {
		t.Errorf("log = %v, want ERROR level and status 503", entry)
	}
	if entry["error"] != "TIMEOUT: The calculation took too long." {
		t.Errorf("error = %v, want the problem code and detail", entry["error"])
	}
}

func TestAccessLog_ClientErrorsCarryNoError(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	h := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeProblem(w, r, Problem{Status: http.StatusNotFound, Code: CodeNotFound, Detail: "nope"})
	}), requestID(slog.New(slog.NewJSONHandler(&logs, nil))), accessLog)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", http.NoBody))

	entry := logLine(t, &logs)
	if _, present := entry["error"]; present || entry["level"] != "INFO" {
		t.Errorf("log = %v, want INFO without an error attribute", entry)
	}
}

func TestRecoverer(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	h := chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}), requestID(slog.New(slog.NewJSONHandler(&logs, nil))), accessLog, recoverer)

	rec := serve(h, http.MethodGet, "/")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if p := decodeProblemBody(t, rec); p.Code != CodeInternal || p.Detail != "" {
		t.Errorf("problem = %+v, want generic internal error without details", p)
	}
	if strings.Contains(rec.Body.String(), "boom") || strings.Contains(rec.Body.String(), "goroutine") {
		t.Errorf("panic details leaked to the client: %s", rec.Body)
	}
	if !strings.Contains(logs.String(), `"msg":"panic serving request"`) || !strings.Contains(logs.String(), "middleware_test.go") {
		t.Errorf("panic was not logged with a stack: %s", logs.String())
	}
	entry := logLine(t, &logs)
	if entry["status"] != float64(http.StatusInternalServerError) || entry["error"] != "boom" || entry["level"] != "ERROR" {
		t.Errorf("access log = %v, want status 500 and error \"boom\"", entry)
	}
}

// countingWriter records how many times a status line was written.
type countingWriter struct {
	http.ResponseWriter
	writeHeaderCalls int
}

func (c *countingWriter) WriteHeader(code int) {
	c.writeHeaderCalls++
	c.ResponseWriter.WriteHeader(code)
}

func TestRecoverer_PanicAfterHeadersIsOnlyLogged(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	h := chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		panic("late boom")
	}), requestID(slog.New(slog.NewJSONHandler(&logs, nil))), accessLog, recoverer)
	rec := httptest.NewRecorder()
	counting := &countingWriter{ResponseWriter: rec}

	h.ServeHTTP(counting, httptest.NewRequest(http.MethodGet, "/", http.NoBody))

	if counting.writeHeaderCalls != 1 {
		t.Errorf("WriteHeader called %d times, want 1", counting.writeHeaderCalls)
	}
	if rec.Code != http.StatusOK || rec.Body.String() != "partial" {
		t.Errorf("response = %d %q, want the partial 200 left untouched", rec.Code, rec.Body)
	}
	if !strings.Contains(logs.String(), "late boom") {
		t.Errorf("panic was not logged: %s", logs.String())
	}
	entry := logLine(t, &logs)
	if entry["status"] != float64(http.StatusOK) || entry["error"] != "late boom" || entry["level"] != "ERROR" {
		t.Errorf("access log = %v, want status 200 with the panic as error at ERROR level", entry)
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
		wantVary    bool
		wantPreAuth bool
	}{
		{"same origin request", []string{allowed}, http.MethodGet, "", false, http.StatusOK, "", true, false},
		{"allowed origin", []string{allowed}, http.MethodGet, allowed, false, http.StatusOK, allowed, true, false},
		{"second allowed origin", []string{"https://app.example", allowed}, http.MethodGet, allowed, false, http.StatusOK, allowed, true, false},
		{"disallowed origin", []string{allowed}, http.MethodGet, "https://evil.example", false, http.StatusOK, "", true, false},
		{"origin differing only by scheme", []string{allowed}, http.MethodGet, "https://localhost:5173", false, http.StatusOK, "", true, false},
		{"allowed preflight", []string{allowed}, http.MethodOptions, allowed, true, http.StatusNoContent, allowed, true, true},
		{"disallowed preflight", []string{allowed}, http.MethodOptions, "https://evil.example", true, http.StatusOK, "", true, false},
		{"wildcard is not an allow-all", []string{"*"}, http.MethodGet, "https://any.example", false, http.StatusOK, "", true, false},
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
			wantExpose := ""
			if tc.wantOrigin != "" {
				wantExpose = requestIDHeader
			}
			if got := rec.Header().Get("Access-Control-Expose-Headers"); got != wantExpose {
				t.Errorf("Expose-Headers = %q, want %q", got, wantExpose)
			}
			if got := slices.Contains(rec.Header().Values("Vary"), "Origin"); got != tc.wantVary {
				t.Errorf("Vary: Origin present = %v, want %v", got, tc.wantVary)
			}
			preflightHeaders := map[string]string{
				"Access-Control-Allow-Methods": "GET, POST",
				"Access-Control-Allow-Headers": "Content-Type, X-Request-ID",
				"Access-Control-Max-Age":       "600",
			}
			for header, value := range preflightHeaders {
				want := ""
				if tc.wantPreAuth {
					want = value
				}
				if got := rec.Header().Get(header); got != want {
					t.Errorf("%s = %q, want %q", header, got, want)
				}
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	t.Parallel()
	rec := serve(securityHeaders(ok), http.MethodGet, "/")
	for header, want := range securityHeaderValues {
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

func TestRequestState_NilSafeWithoutMiddleware(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)

	// writeProblem records 5xx details in the request state; without the requestID
	// middleware there is none and the call must still produce the response.
	writeProblem(rec, req, Problem{Status: http.StatusInternalServerError, Code: CodeInternal})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if state := stateFrom(req.Context()); state != nil {
		t.Errorf("stateFrom() = %+v, want nil without the middleware", state)
	}
}
