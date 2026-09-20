package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aksh/calculator/backend/internal/calc"
)

// fakeEvaluator is a hand-written stand-in for the calculator domain.
type fakeEvaluator struct {
	result calc.Result
	err    error
	// block makes Evaluate wait for the context to end and return its error, which is how
	// the real evaluator behaves when a request times out or the client goes away.
	block         bool
	gotExpression string
}

func (f *fakeEvaluator) Evaluate(ctx context.Context, expression string) (calc.Result, error) {
	f.gotExpression = expression
	if f.block {
		<-ctx.Done()
		return calc.Result{}, ctx.Err()
	}
	if f.err != nil {
		return calc.Result{}, f.err
	}
	return f.result, nil
}

func evaluateRequestWith(t *testing.T, body, contentType string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evaluate", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req
}

func postEvaluate(t *testing.T, d Deps, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	NewRouter(d).ServeHTTP(rec, evaluateRequestWith(t, body, "application/json"))
	return rec
}

// decodeRawBody exposes the JSON body as a map so tests can assert on key presence.
func decodeRawBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v; body %s", err, rec.Body)
	}
	return body
}

func firstFieldError(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	list, ok := body["errors"].([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("errors = %v, want exactly one entry", body["errors"])
	}
	entry, ok := list[0].(map[string]any)
	if !ok {
		t.Fatalf("errors[0] = %v, want an object", list[0])
	}
	return entry
}

func TestEvaluate_Success(t *testing.T) {
	t.Parallel()
	fake := &fakeEvaluator{result: calc.Result{Expression: "2*(3+4)", Value: "14"}}
	d := testDeps()
	d.Evaluator = fake

	rec := postEvaluate(t, d, `{"expression":"2*(3+4"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	if got, want := rec.Body.String(), "{\"expression\":\"2*(3+4)\",\"result\":\"14\"}\n"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if fake.gotExpression != "2*(3+4" {
		t.Errorf("expression passed to the evaluator = %q, want the raw input", fake.gotExpression)
	}
	assertCommonHeaders(t, rec.Header())
}

func TestEvaluate_RequiredExpression(t *testing.T) {
	t.Parallel()
	for _, body := range []string{`{}`, `{"expression":null}`} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()
			d := testDeps()
			d.Evaluator = &fakeEvaluator{}

			rec := postEvaluate(t, d, body)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body)
			}
			p := decodeProblemBody(t, rec)
			if p.Code != CodeValidationFailed || p.Detail != "expression is required" {
				t.Errorf("problem = %+v, want VALIDATION_FAILED with the REQUIRED message as detail", p)
			}
			raw := decodeRawBody(t, rec)
			entry := firstFieldError(t, raw)
			if entry["field"] != "expression" || entry["code"] != "REQUIRED" || entry["message"] != "expression is required" {
				t.Errorf("errors[0] = %v", entry)
			}
			if _, present := entry["position"]; present {
				t.Errorf("errors[0] has a position: %v", entry)
			}
		})
	}
}

func TestEvaluate_MalformedRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		body      string
		wantField string
		wantMsg   string
	}{
		{name: "non-string expression", body: `{"expression":2}`, wantField: "expression", wantMsg: "must be a string"},
		{name: "unknown field", body: `{"expression":"1","extra":true}`, wantField: "extra", wantMsg: "is not supported"},
		{name: "two JSON values", body: `{"expression":"1"} {"expression":"2"}`},
		{name: "array", body: `[]`},
		{name: "empty body", body: ``},
		{name: "invalid JSON", body: `{bad`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeEvaluator{}
			d := testDeps()
			d.Evaluator = fake

			rec := postEvaluate(t, d, tc.body)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body)
			}
			p := decodeProblemBody(t, rec)
			if p.Code != CodeMalformedRequest || p.Detail == "" {
				t.Errorf("problem = %+v, want MALFORMED_REQUEST with a detail", p)
			}
			if tc.wantField != "" && (len(p.Errors) != 1 || p.Errors[0].Field != tc.wantField || p.Errors[0].Message != tc.wantMsg) {
				t.Errorf("errors = %+v, want one for %q saying %q", p.Errors, tc.wantField, tc.wantMsg)
			}
			if fake.gotExpression != "" {
				t.Errorf("evaluator was called with %q for a malformed request", fake.gotExpression)
			}
			assertCommonHeaders(t, rec.Header())
		})
	}
}

func TestEvaluate_BodyLimit(t *testing.T) {
	t.Parallel()
	const limit = 4096
	// {"expression":"<digits>"} is 17 bytes of framing.
	atLimit := `{"expression":"` + strings.Repeat("1", limit-17) + `"}`
	if len(atLimit) != limit {
		t.Fatalf("test body is %d bytes, want %d", len(atLimit), limit)
	}
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"exactly the limit is accepted", atLimit, http.StatusOK},
		{"one byte over is rejected", atLimit + " ", http.StatusRequestEntityTooLarge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := testDeps()
			d.MaxBodyBytes = limit
			d.Evaluator = &fakeEvaluator{result: calc.Result{Expression: "1", Value: "1"}}

			rec := postEvaluate(t, d, tc.body)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tc.wantStatus, rec.Body)
			}
			if tc.wantStatus == http.StatusRequestEntityTooLarge {
				if p := decodeProblemBody(t, rec); p.Code != CodePayloadTooLarge {
					t.Errorf("code = %q, want %q", p.Code, CodePayloadTooLarge)
				}
				// Closing the connection after the reply saves net/http from draining the
				// rest of an oversized body to keep the connection reusable.
				if got := rec.Header().Get("Connection"); got != "close" {
					t.Errorf("Connection = %q, want close on a 413", got)
				}
				assertCommonHeaders(t, rec.Header())
			} else if got := rec.Header().Get("Connection"); got != "" {
				t.Errorf("Connection = %q on an accepted body, want none", got)
			}
		})
	}
}

func TestEvaluate_UnsupportedMediaType(t *testing.T) {
	t.Parallel()
	for _, contentType := range []string{"text/plain", "application/problem+json", ""} {
		t.Run("content type "+contentType, func(t *testing.T) {
			t.Parallel()
			d := testDeps()
			d.Evaluator = &fakeEvaluator{}
			rec := httptest.NewRecorder()

			NewRouter(d).ServeHTTP(rec, evaluateRequestWith(t, `{"expression":"1"}`, contentType))

			if rec.Code != http.StatusUnsupportedMediaType {
				t.Fatalf("status = %d, want 415; body %s", rec.Code, rec.Body)
			}
			if p := decodeProblemBody(t, rec); p.Code != CodeUnsupportedMediaType {
				t.Errorf("code = %q, want %q", p.Code, CodeUnsupportedMediaType)
			}
			assertCommonHeaders(t, rec.Header())
		})
	}
}

func TestEvaluate_ValidationErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  *calc.ValidationError
	}{
		{"empty", &calc.ValidationError{Code: calc.CodeEmpty, Message: "expression is empty"}},
		{"too long", &calc.ValidationError{Code: calc.CodeTooLong, Message: "expression exceeds 1,024 characters"}},
		{"too deep", &calc.ValidationError{Code: calc.CodeTooDeep, Message: "expression is nested deeper than 32 levels"}},
		{"invalid character", &calc.ValidationError{Code: calc.CodeInvalidCharacter, Position: 1, HasPosition: true, Message: "invalid character '$' at character 2"}},
		{"invalid number", &calc.ValidationError{Code: calc.CodeInvalidNumber, Position: 0, HasPosition: true, Message: "invalid number at character 1"}},
		{"unexpected token", &calc.ValidationError{Code: calc.CodeUnexpectedToken, Position: 3, HasPosition: true, Message: "unexpected ')' at character 4"}},
		{"unbalanced parenthesis", &calc.ValidationError{Code: calc.CodeUnbalancedParenthesis, Position: 3, HasPosition: true, Message: "unbalanced ')' at character 4"}},
		{"unknown function", &calc.ValidationError{Code: calc.CodeUnknownFunction, Position: 0, HasPosition: true, Message: "unknown function 'foo' at character 1"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := testDeps()
			d.Evaluator = &fakeEvaluator{err: tc.err}

			rec := postEvaluate(t, d, `{"expression":"x"}`)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body)
			}
			p := decodeProblemBody(t, rec)
			if p.Code != CodeValidationFailed || p.Status != http.StatusBadRequest {
				t.Errorf("problem = %+v, want VALIDATION_FAILED / 400", p)
			}
			raw := decodeRawBody(t, rec)
			entry := firstFieldError(t, raw)
			if entry["field"] != "expression" || entry["code"] != tc.err.Code || entry["message"] != tc.err.Message {
				t.Errorf("errors[0] = %v, want field expression, code %s, message %q", entry, tc.err.Code, tc.err.Message)
			}
			if raw["detail"] != entry["message"] {
				t.Errorf("detail = %v, want errors[0].message %v", raw["detail"], entry["message"])
			}
			position, present := entry["position"]
			if present != tc.err.HasPosition {
				t.Fatalf("position present = %v, want %v (entry %v)", present, tc.err.HasPosition, entry)
			}
			if tc.err.HasPosition && position != float64(tc.err.Position) {
				t.Errorf("position = %v (%T), want %d", position, position, tc.err.Position)
			}
			assertCommonHeaders(t, rec.Header())
		})
	}
}

func TestEvaluate_ArithmeticErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code       string
		wantDetail string
	}{
		{calc.CodeDivisionByZero, "Cannot divide by zero."},
		{calc.CodeNegativeSquareRoot, "Cannot take the square root of a negative number."},
		{calc.CodeInvalidPower, "Cannot raise a negative number to a fractional power."},
		{calc.CodeExponentTooLarge, "Exponent must be between -1000 and 1000."},
		{calc.CodeResultTooLarge, "Result is too large to calculate."},
	}
	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			t.Parallel()
			d := testDeps()
			d.Evaluator = &fakeEvaluator{err: &calc.ArithmeticError{Code: tc.code}}

			rec := postEvaluate(t, d, `{"expression":"1/0"}`)

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422; body %s", rec.Code, rec.Body)
			}
			p := decodeProblemBody(t, rec)
			if p.Code != tc.code || p.Detail != tc.wantDetail || p.Status != http.StatusUnprocessableEntity {
				t.Errorf("problem = %+v, want code %s detail %q", p, tc.code, tc.wantDetail)
			}
			if _, present := decodeRawBody(t, rec)["errors"]; present {
				t.Errorf("422 body has an errors array: %s", rec.Body)
			}
			assertCommonHeaders(t, rec.Header())
		})
	}
}

func TestEvaluate_UnknownArithmeticCodeIsStill422(t *testing.T) {
	t.Parallel()
	d := testDeps()
	d.Evaluator = &fakeEvaluator{err: &calc.ArithmeticError{Code: "FUTURE_CODE"}}

	rec := postEvaluate(t, d, `{"expression":"1"}`)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body %s", rec.Code, rec.Body)
	}
	if p := decodeProblemBody(t, rec); p.Code != "FUTURE_CODE" || p.Detail == "" {
		t.Errorf("problem = %+v, want the domain code and a generic detail", p)
	}
}

func TestEvaluate_Timeout(t *testing.T) {
	t.Parallel()
	d := testDeps()
	d.RequestTimeout = 20 * time.Millisecond
	d.Evaluator = &fakeEvaluator{block: true}

	rec := postEvaluate(t, d, `{"expression":"2^1000"}`)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body %s", rec.Code, rec.Body)
	}
	p := decodeProblemBody(t, rec)
	if p.Code != CodeTimeout || p.Detail != "The calculation took too long." {
		t.Errorf("problem = %+v, want TIMEOUT", p)
	}
	assertCommonHeaders(t, rec.Header())
}

func TestEvaluate_ClientCancellationWritesNothing(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	d := testDeps()
	d.Logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	d.Evaluator = &fakeEvaluator{block: true}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	req := evaluateRequestWith(t, `{"expression":"1"}`, "application/json").WithContext(ctx)
	rec := httptest.NewRecorder()

	NewRouter(d).ServeHTTP(rec, req)

	if rec.Body.Len() != 0 {
		t.Errorf("body written for a cancelled request: %s", rec.Body)
	}
	// The access log must neither count an aborted live preview as a successful 200 nor
	// raise it as a fault: it is recorded as a cancelled request (499) at INFO.
	entry := logLine(t, &logs)
	if entry["level"] != "INFO" || entry["cancelled"] != true || entry["status"] != float64(statusClientClosedRequest) {
		t.Errorf("access log = %v, want INFO with cancelled=true and status 499", entry)
	}
	if _, present := entry["error"]; present {
		t.Errorf("a cancelled request logged an error: %v", entry["error"])
	}
}

// brokenBody fails on the first read, like a connection the client dropped mid-body.
type brokenBody struct{ err error }

func (b brokenBody) Read([]byte) (int, error) { return 0, b.err }
func (brokenBody) Close() error               { return nil }

func TestEvaluate_BodyReadFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		ctx           func(t *testing.T) context.Context
		wantStatus    int // 0 means nothing is written
		wantCode      string
		wantCancelled bool
	}{
		{
			name: "client gone mid-body",
			ctx: func(t *testing.T) context.Context {
				t.Helper()
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				return ctx
			},
			wantCancelled: true,
		},
		{
			name: "deadline passed mid-body",
			ctx: func(t *testing.T) context.Context {
				t.Helper()
				ctx, cancel := context.WithDeadline(t.Context(), time.Unix(0, 0))
				t.Cleanup(cancel)
				return ctx
			},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   CodeTimeout,
		},
		{
			name:       "truncated body with a live connection",
			ctx:        func(t *testing.T) context.Context { t.Helper(); return t.Context() },
			wantStatus: http.StatusBadRequest,
			wantCode:   CodeMalformedRequest,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var logs bytes.Buffer
			d := testDeps()
			d.RequestTimeout = 0 // the test controls the context itself
			d.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
			req := httptest.NewRequest(http.MethodPost, "/api/v1/evaluate", brokenBody{err: io.ErrUnexpectedEOF}).WithContext(tc.ctx(t))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			NewRouter(d).ServeHTTP(rec, req)

			entry := logLine(t, &logs)
			if tc.wantStatus == 0 {
				if rec.Body.Len() != 0 {
					t.Errorf("body written for a cancelled request: %s", rec.Body)
				}
				if entry["cancelled"] != true || entry["status"] != float64(statusClientClosedRequest) || entry["level"] != "INFO" {
					t.Errorf("access log = %v, want cancelled=true, status 499 at INFO", entry)
				}
				return
			}
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tc.wantStatus, rec.Body)
			}
			if p := decodeProblemBody(t, rec); p.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", p.Code, tc.wantCode)
			}
			if _, present := entry["cancelled"]; present {
				t.Errorf("access log = %v, want no cancelled marker", entry)
			}
		})
	}
}

func TestEvaluate_UnexpectedError(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	d := testDeps()
	d.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	d.Evaluator = &fakeEvaluator{err: errors.New("decimal library exploded")}

	rec := postEvaluate(t, d, `{"expression":"1"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body %s", rec.Code, rec.Body)
	}
	if p := decodeProblemBody(t, rec); p.Code != CodeInternal || p.Detail != "" {
		t.Errorf("problem = %+v, want a generic INTERNAL_ERROR", p)
	}
	if strings.Contains(rec.Body.String(), "exploded") {
		t.Error("internal error details leaked to the client")
	}
	requestID := rec.Header().Get(requestIDHeader)
	if !strings.Contains(logs.String(), "decimal library exploded") || !strings.Contains(logs.String(), requestID) {
		t.Errorf("error was not logged with the request ID: %s", logs.String())
	}
	assertCommonHeaders(t, rec.Header())
}

func TestEvaluate_Routing(t *testing.T) {
	t.Parallel()
	d := testDeps()
	d.Evaluator = &fakeEvaluator{}
	h := NewRouter(d)

	rec := serve(h, http.MethodGet, "/api/v1/evaluate")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodPost {
		t.Errorf("Allow = %q, want POST", allow)
	}
	if p := decodeProblemBody(t, rec); p.Code != CodeMethodNotAllowed {
		t.Errorf("code = %q, want %q", p.Code, CodeMethodNotAllowed)
	}
	assertCommonHeaders(t, rec.Header())

	rec = serve(h, http.MethodPost, "/api/v1/evaluat")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown path status = %d, want 404", rec.Code)
	}
	if p := decodeProblemBody(t, rec); p.Code != CodeNotFound {
		t.Errorf("code = %q, want %q", p.Code, CodeNotFound)
	}
	assertCommonHeaders(t, rec.Header())
}
