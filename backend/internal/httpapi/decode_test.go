package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type samplePayload struct {
	Name  string  `json:"name"`
	Count int     `json:"count"`
	Ratio float64 `json:"ratio"`
	Note  *string `json:"note"`
}

func TestDecodeJSON(t *testing.T) {
	t.Parallel()

	const limit = 64
	tests := []struct {
		name        string
		contentType string
		body        string
		wantStatus  int // 0 means success
		wantCode    string
		wantField   string
	}{
		{name: "valid object", contentType: "application/json", body: `{"name":"a","count":2,"ratio":0.5}`},
		{name: "charset parameter", contentType: "application/json; charset=utf-8", body: `{"name":"a"}`},
		{name: "media type is case-insensitive", contentType: "Application/JSON", body: `{}`},
		{name: "structured syntax suffix is not json", contentType: "application/merge-patch+json", body: `{}`, wantStatus: http.StatusUnsupportedMediaType, wantCode: CodeUnsupportedMediaType},
		{name: "problem json is not accepted", contentType: "application/problem+json", body: `{}`, wantStatus: http.StatusUnsupportedMediaType, wantCode: CodeUnsupportedMediaType},
		{name: "unparsable content type", contentType: "application/json; charset", body: `{}`, wantStatus: http.StatusUnsupportedMediaType, wantCode: CodeUnsupportedMediaType},
		{name: "leading whitespace", contentType: "application/json", body: " \n\t{\"name\":\"a\"}"},
		{name: "missing content type", body: `{}`, wantStatus: http.StatusUnsupportedMediaType, wantCode: CodeUnsupportedMediaType},
		{name: "wrong content type", contentType: "text/plain", body: `{}`, wantStatus: http.StatusUnsupportedMediaType, wantCode: CodeUnsupportedMediaType},
		{name: "empty body", contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: CodeMalformedRequest},
		{name: "truncated json", contentType: "application/json", body: `{"name":`, wantStatus: http.StatusBadRequest, wantCode: CodeMalformedRequest},
		{name: "invalid json", contentType: "application/json", body: `{name: 1}`, wantStatus: http.StatusBadRequest, wantCode: CodeMalformedRequest},
		{name: "wrong field type", contentType: "application/json", body: `{"count":"two"}`, wantStatus: http.StatusBadRequest, wantCode: CodeMalformedRequest, wantField: "count"},
		{name: "fraction for integer", contentType: "application/json", body: `{"count":1.5}`, wantStatus: http.StatusBadRequest, wantCode: CodeMalformedRequest, wantField: "count"},
		{name: "whitespace only", contentType: "application/json", body: " \n ", wantStatus: http.StatusBadRequest, wantCode: CodeMalformedRequest},
		{name: "array instead of object", contentType: "application/json", body: `[]`, wantStatus: http.StatusBadRequest, wantCode: CodeMalformedRequest},
		{name: "null instead of object", contentType: "application/json", body: `null`, wantStatus: http.StatusBadRequest, wantCode: CodeMalformedRequest},
		{name: "string instead of object", contentType: "application/json", body: `"a"`, wantStatus: http.StatusBadRequest, wantCode: CodeMalformedRequest},
		{name: "unknown field", contentType: "application/json", body: `{"extra":true}`, wantStatus: http.StatusBadRequest, wantCode: CodeMalformedRequest, wantField: "extra"},
		{name: "null for a pointer field is accepted", contentType: "application/json", body: `{"note":null}`},
		{name: "wrong type for a pointer field", contentType: "application/json", body: `{"note":1}`, wantStatus: http.StatusBadRequest, wantCode: CodeMalformedRequest, wantField: "note"},
		{name: "two objects", contentType: "application/json", body: `{} {}`, wantStatus: http.StatusBadRequest, wantCode: CodeMalformedRequest},
		{name: "trailing garbage", contentType: "application/json", body: `{}x`, wantStatus: http.StatusBadRequest, wantCode: CodeMalformedRequest},
		{name: "body over limit", contentType: "application/json", body: `{"name":"` + strings.Repeat("a", limit) + `"}`, wantStatus: http.StatusRequestEntityTooLarge, wantCode: CodePayloadTooLarge},
		{name: "limit crossed after the object", contentType: "application/json", body: `{}` + strings.Repeat(" ", limit), wantStatus: http.StatusRequestEntityTooLarge, wantCode: CodePayloadTooLarge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			var dst samplePayload
			p := decodeJSON(httptest.NewRecorder(), req, limit, &dst)

			if tc.wantStatus == 0 {
				if p != nil {
					t.Fatalf("decodeJSON() = %+v, want success", *p)
				}
				return
			}
			if p == nil {
				t.Fatalf("decodeJSON() = nil, want status %d", tc.wantStatus)
			}
			if p.Status != tc.wantStatus || p.Code != tc.wantCode || p.Detail == "" {
				t.Errorf("problem = %+v, want status %d code %s and a detail", *p, tc.wantStatus, tc.wantCode)
			}
			if tc.wantField != "" && (len(p.Errors) != 1 || p.Errors[0].Field != tc.wantField) {
				t.Errorf("field errors = %+v, want one for %q", p.Errors, tc.wantField)
			}
		})
	}
}

// FuzzDecodeJSON checks that arbitrary bodies and media types never panic and always end
// either in success or in one of the three problem statuses the contract documents.
func FuzzDecodeJSON(f *testing.F) {
	for _, seed := range []struct{ contentType, body string }{
		{"application/json", `{"name":"a","count":2,"ratio":0.5}`},
		{"application/json; charset=utf-8", `{"note":null}`},
		{"text/plain", `{}`},
		{"", ``},
		{"application/json", `{"count":"two"}`},
		{"application/json", `{} {}`},
		{"application/json", `[]`},
		{"application/json", strings.Repeat("{", 100)},
		{"application/json", "\x00"},
	} {
		f.Add(seed.contentType, seed.body)
	}
	f.Fuzz(func(t *testing.T, contentType, body string) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", contentType)
		var dst samplePayload

		p := decodeJSON(httptest.NewRecorder(), req, 64, &dst)

		if p == nil {
			return
		}
		switch p.Status {
		case http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType:
		default:
			t.Fatalf("decodeJSON(%q, %q) status = %d", contentType, body, p.Status)
		}
		if p.Code == "" || p.Detail == "" {
			t.Fatalf("decodeJSON(%q, %q) = %+v, want a code and a detail", contentType, body, *p)
		}
	})
}
