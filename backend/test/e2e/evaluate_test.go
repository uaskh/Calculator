package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const evaluatePath = "/api/v1/evaluate"

// resultPattern is the canonical result form promised by api/openapi.yaml
// (`^-?(0|[1-9][0-9]*)(\.[0-9]*[1-9])?$`); RE2's `\d` is ASCII-only, so it is equivalent.
var resultPattern = regexp.MustCompile(`^-?(0|[1-9]\d*)(\.\d*[1-9])?$`)

// postEvaluate sends body as-is with the given content type.
func postEvaluate(t *testing.T, srv *httptest.Server, contentType, body string) response {
	t.Helper()
	req := newRequest(t, srv, http.MethodPost, evaluatePath, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return send(t, srv, req)
}

// evaluate posts {"expression": expr} as JSON.
func evaluate(t *testing.T, srv *httptest.Server, expr string) response {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"expression": expr})
	if err != nil {
		t.Fatal(err)
	}
	return postEvaluate(t, srv, "application/json", string(raw))
}

func assertSuccess(t *testing.T, r response, wantExpression, wantResult string) {
	t.Helper()
	if r.status != http.StatusOK {
		t.Fatalf("status = %d, body = %v, want 200", r.status, r.body)
	}
	if ct := r.header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}
	if len(r.body) != 2 || r.body["expression"] != wantExpression || r.body["result"] != wantResult {
		t.Errorf("body = %v, want {expression:%q result:%q}", r.body, wantExpression, wantResult)
	}
	assertCommonHeaders(t, r.header)
}

// assertProblem checks the RFC 9457 envelope every error response must have.
func assertProblem(t *testing.T, r response, wantStatus int, wantCode string) {
	t.Helper()
	if r.status != wantStatus {
		t.Fatalf("status = %d, body = %v, want %d", r.status, r.body, wantStatus)
	}
	if ct := r.header.Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
	if r.body["type"] != "about:blank" || r.body["title"] != http.StatusText(wantStatus) ||
		r.body["status"] != float64(wantStatus) || r.body["instance"] != evaluatePath {
		t.Errorf("problem envelope = %v", r.body)
	}
	if r.body["code"] != wantCode {
		t.Errorf("code = %v, want %s", r.body["code"], wantCode)
	}
	if r.body["requestId"] != r.header.Get("X-Request-ID") {
		t.Errorf("requestId = %v, want the X-Request-ID header %q", r.body["requestId"], r.header.Get("X-Request-ID"))
	}
	assertCommonHeaders(t, r.header)
}

// fieldError returns the single entry of errors; it fails when there is not exactly one.
func fieldError(t *testing.T, r response) map[string]any {
	t.Helper()
	list, ok := r.body["errors"].([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("errors = %v, want exactly one entry", r.body["errors"])
	}
	entry, ok := list[0].(map[string]any)
	if !ok {
		t.Fatalf("errors[0] = %v, want an object", list[0])
	}
	return entry
}

func TestEvaluate_Examples(t *testing.T) {
	t.Parallel()
	srv := newService(t, nil)

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name           string
			input          string
			wantExpression string
			wantResult     string
		}{
			{"precedence", "2+3*4", "2+3*4", "14"},
			{"unclosed parenthesis is closed", "2*(3+4", "2*(3+4)", "14"},
			{"surrounding whitespace is trimmed", "  2 + 2  ", "2 + 2", "4"},
			{"exact decimals", "0.1+0.2", "0.1+0.2", "0.3"},
			{"large integer power", "2^100", "2^100", "1267650600228229401496703205376"},
			{"square root round trip", "sqrt(2)*sqrt(2)", "sqrt(2)*sqrt(2)", "2"},
			{"percent of the left operand", "200+10%", "200+10%", "220"},
			{"power binds tighter than unary minus", "-2^2", "-2^2", "-4"},
			{"division rounded to 16 places", "1/3", "1/3", "0.3333333333333333"},
			{"fractional power", "2^0.5", "2^0.5", "1.414213562373095"},
			{"trailing operator is dropped", "3+4*", "3+4", "7"},
			{"trailing dot is dropped", "2.", "2", "2"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				assertSuccess(t, evaluate(t, srv, tc.input), tc.wantExpression, tc.wantResult)
			})
		}
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name         string
			input        string
			wantCode     string
			wantPosition int
			hasPosition  bool
			wantMessage  string
		}{
			{"empty", "", "EMPTY", 0, false, "expression is empty"},
			{"whitespace only", "   ", "EMPTY", 0, false, "expression is empty"},
			{"two numbers", "2 3", "UNEXPECTED_TOKEN", 2, true, "unexpected '3' at character 3"},
			{"position indexes the untrimmed input", "  2 3", "UNEXPECTED_TOKEN", 4, true, "unexpected '3' at character 5"},
			{"end of input", "sqrt", "UNEXPECTED_TOKEN", 4, true, "unexpected end of input"},
			{"extra close parenthesis", "2+3)", "UNBALANCED_PARENTHESIS", 3, true, "unbalanced ')' at character 4"},
			{"two dots", "1.2.3", "INVALID_NUMBER", 0, true, "invalid number '1.2.3' at character 1"},
			{"dollar", "2$3", "INVALID_CHARACTER", 1, true, "invalid character '$' at character 2"},
			{"unknown function", "foo(1)", "UNKNOWN_FUNCTION", 0, true, "unknown function 'foo' at character 1"},
			{"1,025 characters", strings.Repeat("1", 1025), "TOO_LONG", 0, false, "expression exceeds 1,024 characters"},
			{"33 nested parentheses", strings.Repeat("(", 33) + "1" + strings.Repeat(")", 33), "TOO_DEEP", 0, false, "expression is nested deeper than 32 levels"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				r := evaluate(t, srv, tc.input)
				assertProblem(t, r, http.StatusBadRequest, "VALIDATION_FAILED")
				fe := fieldError(t, r)
				if fe["field"] != "expression" || fe["code"] != tc.wantCode || fe["message"] != tc.wantMessage {
					t.Errorf("errors[0] = %v, want field expression, code %s, message %q", fe, tc.wantCode, tc.wantMessage)
				}
				position, present := fe["position"]
				if present != tc.hasPosition {
					t.Errorf("position present = %v, want %v (errors[0] = %v)", present, tc.hasPosition, fe)
				}
				if tc.hasPosition && position != float64(tc.wantPosition) {
					t.Errorf("position = %v, want %d", position, tc.wantPosition)
				}
				if r.body["detail"] != fe["message"] {
					t.Errorf("detail = %v, want errors[0].message %q", r.body["detail"], fe["message"])
				}
			})
		}
	})

	t.Run("arithmetic error", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name       string
			input      string
			wantCode   string
			wantDetail string
		}{
			{"division by zero", "1/0", "DIVISION_BY_ZERO", "Cannot divide by zero."},
			{"negative square root", "sqrt(-4)", "NEGATIVE_SQUARE_ROOT", "Cannot take the square root of a negative number."},
			{"negative base with fractional exponent", "(-8)^0.5", "INVALID_POWER", "Cannot raise a negative number to a fractional power."},
			{"exponent too large", "2^1001", "EXPONENT_TOO_LARGE", "Exponent must be between -1000 and 1000."},
			{"result too large", "10^100", "RESULT_TOO_LARGE", "Result is too large to calculate."},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				r := evaluate(t, srv, tc.input)
				assertProblem(t, r, http.StatusUnprocessableEntity, tc.wantCode)
				if r.body["detail"] != tc.wantDetail {
					t.Errorf("detail = %v, want %q", r.body["detail"], tc.wantDetail)
				}
				if _, present := r.body["errors"]; present {
					t.Errorf("errors present on a 422: %v", r.body["errors"])
				}
			})
		}
	})

	t.Run("missing expression", func(t *testing.T) {
		t.Parallel()
		for _, body := range []string{`{}`, `{"expression":null}`} {
			t.Run(body, func(t *testing.T) {
				t.Parallel()
				r := postEvaluate(t, srv, "application/json", body)
				assertProblem(t, r, http.StatusBadRequest, "VALIDATION_FAILED")
				fe := fieldError(t, r)
				if fe["field"] != "expression" || fe["code"] != "REQUIRED" || fe["message"] != "expression is required" {
					t.Errorf("errors[0] = %v, want REQUIRED", fe)
				}
				if _, present := fe["position"]; present {
					t.Errorf("position present for REQUIRED: %v", fe)
				}
				if r.body["detail"] != "expression is required" {
					t.Errorf("detail = %v, want %q", r.body["detail"], "expression is required")
				}
			})
		}
	})

	t.Run("malformed request", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name        string
			contentType string
			body        string
			wantDetail  string
			wantField   string
			wantCode    string
		}{
			{"wrong type", "application/json", `{"expression":2}`, "Request body contains a value of the wrong type.", "expression", "INVALID_TYPE"},
			{"unknown field", "application/json", `{"expression":"1","precision":2}`, "Request body contains an unknown field.", "precision", "UNKNOWN_FIELD"},
			{"invalid JSON", "application/json", `{"expression":`, "Request body is not valid JSON.", "", ""},
			{"array instead of object", "application/json", `["1+1"]`, "Request body must be a JSON object.", "", ""},
			{"empty body", "application/json", ``, "Request body must not be empty.", "", ""},
			{"two JSON values", "application/json", `{"expression":"1"}{"expression":"2"}`, "Request body must contain a single JSON object.", "", ""},
			{"charset parameter is accepted", "application/json; charset=utf-8", `{"expression":true}`, "Request body contains a value of the wrong type.", "expression", "INVALID_TYPE"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				r := postEvaluate(t, srv, tc.contentType, tc.body)
				assertProblem(t, r, http.StatusBadRequest, "MALFORMED_REQUEST")
				if r.body["detail"] != tc.wantDetail {
					t.Errorf("detail = %v, want %q", r.body["detail"], tc.wantDetail)
				}
				if tc.wantField == "" {
					if _, present := r.body["errors"]; present {
						t.Errorf("errors present: %v", r.body["errors"])
					}
					return
				}
				fe := fieldError(t, r)
				if fe["field"] != tc.wantField || fe["code"] != tc.wantCode {
					t.Errorf("errors[0] = %v, want field %q code %s", fe, tc.wantField, tc.wantCode)
				}
			})
		}
	})

	t.Run("payload too large", func(t *testing.T) {
		t.Parallel()
		const prefix, suffix = `{"expression":"`, `"}`
		body := prefix + strings.Repeat("1", 4097-len(prefix)-len(suffix)) + suffix
		if len(body) != 4097 {
			t.Fatalf("body is %d bytes, want 4097", len(body))
		}
		r := postEvaluate(t, srv, "application/json", body)
		assertProblem(t, r, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE")
		if r.body["detail"] != "Request body must not exceed 4096 bytes." {
			t.Errorf("detail = %v", r.body["detail"])
		}
	})

	t.Run("largest accepted body", func(t *testing.T) {
		t.Parallel()
		const prefix, suffix = `{"expression":"`, `"}`
		body := prefix + strings.Repeat("1", 4096-len(prefix)-len(suffix)) + suffix
		r := postEvaluate(t, srv, "application/json", body)
		// 4,096 bytes pass the body limit; the 4,079-character expression is then TOO_LONG.
		assertProblem(t, r, http.StatusBadRequest, "VALIDATION_FAILED")
		if fe := fieldError(t, r); fe["code"] != "TOO_LONG" {
			t.Errorf("errors[0] = %v, want TOO_LONG", fe)
		}
	})

	t.Run("unsupported media type", func(t *testing.T) {
		t.Parallel()
		for _, contentType := range []string{"text/plain", "application/problem+json", ""} {
			t.Run("content type "+strconv.Quote(contentType), func(t *testing.T) {
				t.Parallel()
				r := postEvaluate(t, srv, contentType, `{"expression":"1+1"}`)
				assertProblem(t, r, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE")
				if r.body["detail"] != "Content-Type must be application/json." {
					t.Errorf("detail = %v", r.body["detail"])
				}
			})
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		t.Parallel()
		for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
			t.Run(method, func(t *testing.T) {
				t.Parallel()
				r := send(t, srv, newRequest(t, srv, method, evaluatePath, http.NoBody))
				assertProblem(t, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
				if allow := r.header.Get("Allow"); allow != "POST" {
					t.Errorf("Allow = %q, want POST", allow)
				}
			})
		}
	})

	t.Run("client request id is echoed", func(t *testing.T) {
		t.Parallel()
		req := newRequest(t, srv, http.MethodPost, evaluatePath, strings.NewReader(`{"expression":"1/0"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "calc-trace.1")
		r := send(t, srv, req)
		if r.status != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", r.status)
		}
		if r.header.Get("X-Request-ID") != "calc-trace.1" || r.body["requestId"] != "calc-trace.1" {
			t.Errorf("X-Request-ID = %q, requestId = %v, want calc-trace.1 in both", r.header.Get("X-Request-ID"), r.body["requestId"])
		}
	})
}

func TestEvaluate_Concurrent(t *testing.T) {
	t.Parallel()
	srv := newService(t, nil)

	type job struct {
		input string
		want  string
	}
	base := []job{
		{"2^0.5", "1.414213562373095"},
		{"sqrt(2)", "1.414213562373095"},
		{"sqrt(2)*sqrt(2)", "2"},
		{"(-8)^3", "-512"},
		{"2^100", "1267650600228229401496703205376"},
		{"1/3", "0.3333333333333333"},
		{"200+10%", "220"},
		{"2^-1", "0.5"},
		{"3^-2", "0.1111111111111111"},
		{"1.1^2", "1.21"},
	}
	// 50 distinct expressions: each base case is offset by a different constant so a
	// response delivered to the wrong request is caught.
	jobs := make([]job, 0, 50)
	for i := range 50 {
		b := base[i%len(base)]
		offset := i / len(base)
		if offset == 0 {
			jobs = append(jobs, b)
			continue
		}
		jobs = append(jobs, job{
			input: fmt.Sprintf("%s+%d-%d", b.input, offset*1000, offset*1000),
			want:  b.want,
		})
	}

	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Go(func() {
			r := evaluate(t, srv, j.input)
			if r.status != http.StatusOK {
				t.Errorf("%q: status = %d, body = %v", j.input, r.status, r.body)
				return
			}
			if r.body["result"] != j.want || r.body["expression"] != j.input {
				t.Errorf("%q: body = %v, want result %q", j.input, r.body, j.want)
			}
		})
	}
	wg.Wait()
}

func TestEvaluate_ResultPattern(t *testing.T) {
	t.Parallel()
	srv := newService(t, nil)

	inputs := []string{
		"14", "0*-1", "-0.0", "007", "00.5", "0.1^20", "2^100", "10^99", "-10^99",
		"1/3", "2/3", "2^0.5", "-0.00000000000000005", "-0.00000000000000004",
		"1.10*3", "50%", "2^-1000", "-2^2", "(-2)^-3", "0^0",
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			r := evaluate(t, srv, input)
			if r.status != http.StatusOK {
				t.Fatalf("status = %d, body = %v", r.status, r.body)
			}
			result, _ := r.body["result"].(string)
			if !resultPattern.MatchString(result) {
				t.Errorf("result %q does not match %s", result, resultPattern)
			}
			if result == "-0" || strings.HasPrefix(result, "+") || strings.ContainsAny(result, "eE") {
				t.Errorf("result %q is not canonical", result)
			}
		})
	}

	t.Run("maximal result is 118 characters", func(t *testing.T) {
		t.Parallel()
		r := evaluate(t, srv, "-10^99-0.1234567890123456")
		if r.status != http.StatusOK {
			t.Fatalf("status = %d, body = %v", r.status, r.body)
		}
		result, _ := r.body["result"].(string)
		want := "-1" + strings.Repeat("0", 99) + ".1234567890123456"
		if result != want {
			t.Errorf("result = %q, want %q", result, want)
		}
		if len(result) != 118 {
			t.Errorf("len(result) = %d, want 118", len(result))
		}
		if !resultPattern.MatchString(result) {
			t.Errorf("result %q does not match %s", result, resultPattern)
		}
	})
}
