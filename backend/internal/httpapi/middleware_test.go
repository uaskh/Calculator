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

// fakeClock advances by step on every call, so durations are deterministic.
func fakeClock(step time.Duration) func() time.Time {
	now := time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
	return func() time.Time {
		now = now.Add(step)
		return now
	}
}

// logged runs h through the request ID and access log middleware with a JSON logger at
// DEBUG level and returns the access-log line for req.
func logged(t *testing.T, h http.Handler, req *http.Request, clock func() time.Time) map[string]any {
	t.Helper()
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	chain(h, requestID(logger), accessLog(clock)).ServeHTTP(httptest.NewRecorder(), req)
	return logLine(t, &logs)
}

func TestAccessLog(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("short and stout"))
	})
	req := httptest.NewRequest(http.MethodGet, "/brew?strong=1", http.NoBody)
	req.Header.Set(requestIDHeader, "trace-1")
	req.Header.Set("User-Agent", "kettle/2.0")
	req.RemoteAddr = "192.0.2.10:4242"

	entry := logged(t, h, req, fakeClock(1500*time.Microsecond))

	want := map[string]any{
		"level":       "INFO",
		"request_id":  "trace-1",
		"method":      "GET",
		"path":        "/brew",
		"status":      float64(http.StatusTeapot),
		"duration_ms": 1.5,
		"bytes":       float64(15),
		"remote_addr": "192.0.2.10:4242",
		"user_agent":  "kettle/2.0",
	}
	for key, value := range want {
		if entry[key] != value {
			t.Errorf("log[%s] = %v (%T), want %v", key, entry[key], entry[key], value)
		}
	}
	for _, key := range []string{"error", "code", "error_code", "cancelled", "forwarded_for"} {
		if _, present := entry[key]; present {
			t.Errorf("a plain 418 response logged %s: %v", key, entry[key])
		}
	}
}

func TestAccessLog_DurationHasSubMillisecondResolution(t *testing.T) {
	t.Parallel()
	entry := logged(t, ok, httptest.NewRequest(http.MethodGet, "/", http.NoBody), fakeClock(250*time.Microsecond))

	if entry["duration_ms"] != 0.25 {
		t.Errorf("duration_ms = %v, want 0.25 (a 250µs request must not be logged as 0)", entry["duration_ms"])
	}
}

func TestAccessLog_HealthChecksAreDebug(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		path      string
		handler   http.Handler
		wantLevel string
	}{
		{"healthy liveness", "/healthz", ok, "DEBUG"},
		{"healthy readiness", "/readyz", ok, "DEBUG"},
		{"api request", "/api/v1/evaluate", ok, "INFO"},
		{"not ready", "/readyz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeProblem(w, r, Problem{Status: http.StatusServiceUnavailable, Code: CodeNotReady})
		}), "INFO"},
		{"wrong method on health", "/healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeProblem(w, r, Problem{Status: http.StatusMethodNotAllowed, Code: CodeMethodNotAllowed})
		}), "INFO"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			entry := logged(t, tc.handler, httptest.NewRequest(http.MethodGet, tc.path, http.NoBody), time.Now)
			if entry["level"] != tc.wantLevel {
				t.Errorf("level = %v, want %s: %v", entry["level"], tc.wantLevel, entry)
			}
		})
	}
}

func TestAccessLog_ForwardedFor(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("203.0.113.7, ", 30) // 390 bytes
	tests := []struct {
		name string
		hdr  string
		want any // nil means the attribute is absent
	}{
		{"absent", "", nil},
		{"present", "203.0.113.7, 198.51.100.2", "203.0.113.7, 198.51.100.2"},
		{"truncated to 256 bytes", long, long[:256]},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			req.RemoteAddr = "10.0.0.1:1234"
			if tc.hdr != "" {
				req.Header.Set("X-Forwarded-For", tc.hdr)
			}
			entry := logged(t, ok, req, time.Now)

			got, present := entry["forwarded_for"]
			if present != (tc.want != nil) || (present && got != tc.want) {
				t.Errorf("forwarded_for = %v (present %v), want %v", got, present, tc.want)
			}
			if entry["remote_addr"] != "10.0.0.1:1234" {
				t.Errorf("remote_addr = %v, want the peer address, never the forwarded one", entry["remote_addr"])
			}
		})
	}
}

func TestAccessLog_TruncatesUserAgent(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("Mozilla/5.0 ", 40) // 480 bytes
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("User-Agent", long)

	entry := logged(t, ok, req, time.Now)

	if entry["user_agent"] != long[:256] {
		t.Errorf("user_agent = %q, want the first 256 bytes", entry["user_agent"])
	}
}

func TestAccessLog_DefaultsToStatusOK(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("implicit 200"))
	})

	entry := logged(t, h, httptest.NewRequest(http.MethodGet, "/", http.NoBody), time.Now)

	if entry["status"] != float64(http.StatusOK) || entry["bytes"] != float64(12) {
		t.Errorf("log = %v, want status 200 and 12 bytes", entry)
	}
}

func TestAccessLog_ServerErrorsCarryTheProblem(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		problem   Problem
		wantError string
	}{
		{"timeout", Problem{Status: http.StatusServiceUnavailable, Code: CodeTimeout, Detail: "The calculation took too long."}, "TIMEOUT: The calculation took too long."},
		{"internal error", Problem{Status: http.StatusInternalServerError, Code: CodeInternal}, "INTERNAL_ERROR"},
		{"other 5xx", Problem{Status: http.StatusBadGateway, Code: "UPSTREAM_FAILED", Detail: "no"}, "UPSTREAM_FAILED: no"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeProblem(w, r, tc.problem) })

			entry := logged(t, h, httptest.NewRequest(http.MethodPost, "/", http.NoBody), time.Now)

			if entry["level"] != "ERROR" || entry["status"] != float64(tc.problem.Status) {
				t.Errorf("log = %v, want ERROR level and status %d", entry, tc.problem.Status)
			}
			if entry["error"] != tc.wantError || entry["code"] != tc.problem.Code {
				t.Errorf("error = %v, code = %v; want %q and %q", entry["error"], entry["code"], tc.wantError, tc.problem.Code)
			}
		})
	}
}

func TestAccessLog_RawServerErrorIsStillAnError(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) })

	entry := logged(t, h, httptest.NewRequest(http.MethodGet, "/", http.NoBody), time.Now)

	if entry["level"] != "ERROR" || entry["status"] != float64(http.StatusBadGateway) {
		t.Errorf("log = %v, want ERROR level for a 5xx written without a problem", entry)
	}
}

func TestAccessLog_NotReadyIsNotAnError(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeProblem(w, r, Problem{Status: http.StatusServiceUnavailable, Code: CodeNotReady, Detail: "The service is not ready to handle requests."})
	})

	entry := logged(t, h, httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody), time.Now)

	if entry["level"] != "INFO" || entry["status"] != float64(http.StatusServiceUnavailable) || entry["code"] != CodeNotReady {
		t.Errorf("log = %v, want INFO with status 503 and code NOT_READY", entry)
	}
	if _, present := entry["error"]; present {
		t.Errorf("a planned 503 NOT_READY logged an error: %v", entry["error"])
	}
}

func TestAccessLog_ClientErrorsCarryCodesButNoError(t *testing.T) {
	t.Parallel()
	position := 3
	tests := []struct {
		name          string
		problem       Problem
		wantErrorCode string
	}{
		{"not found", Problem{Status: http.StatusNotFound, Code: CodeNotFound, Detail: "nope"}, ""},
		{"validation failed", Problem{
			Status: http.StatusBadRequest,
			Code:   CodeValidationFailed,
			Detail: "unexpected ')' at character 4",
			Errors: []FieldError{{Field: "expression", Code: "UNEXPECTED_TOKEN", Position: &position, Message: "unexpected ')' at character 4"}},
		}, "UNEXPECTED_TOKEN"},
		{"arithmetic error", Problem{Status: http.StatusUnprocessableEntity, Code: "DIVISION_BY_ZERO", Detail: "Cannot divide by zero."}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeProblem(w, r, tc.problem) })

			entry := logged(t, h, httptest.NewRequest(http.MethodGet, "/", http.NoBody), time.Now)

			if _, present := entry["error"]; present || entry["level"] != "INFO" {
				t.Errorf("log = %v, want INFO without an error attribute", entry)
			}
			if entry["code"] != tc.problem.Code {
				t.Errorf("code = %v, want %q", entry["code"], tc.problem.Code)
			}
			got, present := entry["error_code"]
			if present != (tc.wantErrorCode != "") || (present && got != tc.wantErrorCode) {
				t.Errorf("error_code = %v (present %v), want %q", got, present, tc.wantErrorCode)
			}
		})
	}
}

func TestAccessLog_CancelledRequest(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { stateFrom(r.Context()).setCancelled() })

	entry := logged(t, h, httptest.NewRequest(http.MethodPost, "/", http.NoBody), time.Now)

	// 499 is nginx's status for "client closed request"; it never goes on the wire.
	if entry["level"] != "INFO" || entry["status"] != float64(statusClientClosedRequest) || entry["cancelled"] != true {
		t.Errorf("log = %v, want INFO, status 499 and cancelled=true", entry)
	}
	if _, present := entry["error"]; present {
		t.Errorf("a cancelled request logged an error: %v", entry["error"])
	}
}

func TestProblem_IsFault(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		p    Problem
		want bool
	}{
		{"validation failed", Problem{Status: http.StatusBadRequest, Code: CodeValidationFailed}, false},
		{"not found", Problem{Status: http.StatusNotFound, Code: CodeNotFound}, false},
		{"arithmetic", Problem{Status: http.StatusUnprocessableEntity, Code: "DIVISION_BY_ZERO"}, false},
		{"not ready", Problem{Status: http.StatusServiceUnavailable, Code: CodeNotReady}, false},
		{"timeout", Problem{Status: http.StatusServiceUnavailable, Code: CodeTimeout}, true},
		{"internal", Problem{Status: http.StatusInternalServerError, Code: CodeInternal}, true},
		{"other 5xx", Problem{Status: http.StatusBadGateway, Code: "UPSTREAM"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.p.isFault(); got != tc.want {
				t.Errorf("%+v.isFault() = %v, want %v", tc.p, got, tc.want)
			}
		})
	}
}

func TestRecoverer(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	h := chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}), requestID(slog.New(slog.NewJSONHandler(&logs, nil))), accessLog(time.Now), recoverer)

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
	}), requestID(slog.New(slog.NewJSONHandler(&logs, nil))), accessLog(time.Now), recoverer)
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
	state := stateFrom(req.Context())
	if state != nil {
		t.Errorf("stateFrom() = %+v, want nil without the middleware", state)
	}
	// Every setter tolerates the missing state.
	state.setCancelled()
	state.setError("ignored")
	state.setProblem(Problem{Code: CodeNotFound})
}
