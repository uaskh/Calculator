package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strings"
)

// decodeJSON decodes exactly one JSON object from the request body into dst and reports
// whether it succeeded. It enforces the media type and body size limit and rejects
// unknown fields and trailing data. On failure the response has already been written
// (or deliberately not, when the client went away) and the caller must stop handling
// the request.
func decodeJSON(w http.ResponseWriter, r *http.Request, maxBytes int64, dst any) bool {
	p, ctxErr := readJSON(w, r, maxBytes, dst)
	switch {
	case errors.Is(ctxErr, context.Canceled):
		// The client dropped the connection mid-body: nobody is listening for a response.
		stateFrom(r.Context()).setCancelled()
		return false
	case ctxErr != nil:
		writeProblem(w, r, *timeoutProblem())
		return false
	case p == nil:
		return true
	}
	if p.Code == CodePayloadTooLarge {
		// Tell net/http to drop the connection after the reply instead of draining the
		// rest of the oversized body to keep it reusable.
		w.Header().Set("Connection", "close")
	}
	writeProblem(w, r, *p)
	return false
}

// readJSON does the decoding. It returns the problem to send (nil on success), or the
// request context's error when the body read was cut short because the context ended,
// which is not a malformed request.
func readJSON(w http.ResponseWriter, r *http.Request, maxBytes int64, dst any) (*Problem, error) {
	if !isJSON(r.Header.Get("Content-Type")) {
		return &Problem{
			Status: http.StatusUnsupportedMediaType,
			Code:   CodeUnsupportedMediaType,
			Detail: "Content-Type must be application/json.",
		}, nil
	}
	// Read the whole (bounded) body first, so an oversized body is always a 413, whichever
	// part of it crosses the limit.
	body, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBytes))
	if readErr != nil {
		if ctxErr := r.Context().Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return decodeProblem(readErr), nil
	}
	switch trimmed := bytes.TrimLeft(body, " \t\r\n"); {
	case len(trimmed) == 0:
		return malformed("Request body must not be empty."), nil
	case trimmed[0] != '{':
		return malformed("Request body must be a JSON object."), nil
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return decodeProblem(err), nil
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return malformed("Request body must contain a single JSON object."), nil
	}
	return nil, nil
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
