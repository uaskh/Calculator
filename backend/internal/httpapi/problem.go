package httpapi

import (
	"encoding/json"
	"net/http"
)

// Stable, machine-readable error codes. Clients branch on these, never on messages.
const (
	CodeMalformedRequest     = "MALFORMED_REQUEST"
	CodeValidationFailed     = "VALIDATION_FAILED"
	CodeUnsupportedMediaType = "UNSUPPORTED_MEDIA_TYPE"
	CodePayloadTooLarge      = "PAYLOAD_TOO_LARGE"
	CodeNotFound             = "NOT_FOUND"
	CodeMethodNotAllowed     = "METHOD_NOT_ALLOWED"
	CodeNotReady             = "NOT_READY"
	CodeInternal             = "INTERNAL_ERROR"
)

const problemContentType = "application/problem+json"

// Problem is an RFC 9457 "problem details" response body with two extensions:
// a stable Code and, for input errors, per-field Errors.
type Problem struct {
	Type      string       `json:"type"`
	Title     string       `json:"title"`
	Status    int          `json:"status"`
	Detail    string       `json:"detail,omitempty"`
	Instance  string       `json:"instance,omitempty"`
	Code      string       `json:"code"`
	RequestID string       `json:"requestId,omitempty"`
	Errors    []FieldError `json:"errors,omitempty"`
}

// FieldError describes one invalid input field.
type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeProblem sends p, filling in the type, title, instance and request ID.
func writeProblem(w http.ResponseWriter, r *http.Request, p Problem) {
	if p.Type == "" {
		p.Type = "about:blank"
	}
	if p.Title == "" {
		p.Title = http.StatusText(p.Status)
	}
	p.Instance = r.URL.Path
	p.RequestID = RequestIDFrom(r.Context())

	body, err := json.Marshal(p)
	if err != nil { // cannot happen for this type; keep the response well-formed anyway
		body = []byte(`{"type":"about:blank","title":"Internal Server Error","status":500,"code":"INTERNAL_ERROR"}`)
		p.Status = http.StatusInternalServerError
	}
	h := w.Header()
	h.Set("Content-Type", problemContentType)
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(p.Status)
	_, _ = w.Write(append(body, '\n'))
}

// writeJSON marshals v before writing anything, so an encoding failure (for example a NaN
// float) becomes a clean 500 problem instead of a truncated success response.
func writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		loggerFrom(r.Context()).ErrorContext(r.Context(), "encode response", "error", err)
		writeProblem(w, r, Problem{Status: http.StatusInternalServerError, Code: CodeInternal})
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
}
