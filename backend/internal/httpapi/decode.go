package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strings"
)

// decodeJSON decodes exactly one JSON object from the request body into dst. It enforces
// the media type and body size limit and rejects unknown fields and trailing data.
// On failure it returns the problem to send; the caller must stop handling the request.
func decodeJSON(w http.ResponseWriter, r *http.Request, maxBytes int64, dst any) *Problem {
	if !isJSON(r.Header.Get("Content-Type")) {
		return &Problem{
			Status: http.StatusUnsupportedMediaType,
			Code:   CodeUnsupportedMediaType,
			Detail: "Content-Type must be application/json.",
		}
	}
	// Read the whole (bounded) body first, so an oversized body is always a 413, whichever
	// part of it crosses the limit.
	body, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBytes))
	if readErr != nil {
		return decodeProblem(readErr)
	}
	switch trimmed := bytes.TrimLeft(body, " \t\r\n"); {
	case len(trimmed) == 0:
		return malformed("Request body must not be empty.")
	case trimmed[0] != '{':
		return malformed("Request body must be a JSON object.")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return decodeProblem(err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return malformed("Request body must contain a single JSON object.")
	}
	return nil
}

// isJSON accepts exactly the application/json media type, with any parameters (such as
// charset=utf-8). Structured-syntax suffixes like application/problem+json are refused.
func isJSON(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "application/json"
}

func decodeProblem(err error) *Problem {
	var (
		syntaxErr *json.SyntaxError
		typeErr   *json.UnmarshalTypeError
		sizeErr   *http.MaxBytesError
	)
	switch {
	case errors.As(err, &sizeErr):
		return &Problem{
			Status: http.StatusRequestEntityTooLarge,
			Code:   CodePayloadTooLarge,
			Detail: fmt.Sprintf("Request body must not exceed %d bytes.", sizeErr.Limit),
		}
	case errors.Is(err, io.EOF):
		return malformed("Request body must not be empty.")
	case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF):
		return malformed("Request body is not valid JSON.")
	case errors.As(err, &typeErr):
		p := malformed("Request body contains a value of the wrong type.")
		p.Errors = []FieldError{{
			Field:   typeErr.Field,
			Code:    "INVALID_TYPE",
			Message: "must be " + jsonTypeName(typeErr.Type),
		}}
		return p
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		field := strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)
		p := malformed("Request body contains an unknown field.")
		p.Errors = []FieldError{{Field: field, Code: "UNKNOWN_FIELD", Message: "is not supported"}}
		return p
	default:
		return malformed("Request body could not be decoded.")
	}
}

func malformed(detail string) *Problem {
	return &Problem{Status: http.StatusBadRequest, Code: CodeMalformedRequest, Detail: detail}
}

func jsonTypeName(t reflect.Type) string {
	if t == nil {
		return "a valid value"
	}
	switch t.Kind() {
	case reflect.Bool:
		return "a boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "an integer"
	case reflect.Float32, reflect.Float64:
		return "a number"
	case reflect.String:
		return "a string"
	case reflect.Slice, reflect.Array:
		return "an array"
	case reflect.Map, reflect.Struct:
		return "an object"
	default:
		return "a valid value"
	}
}
