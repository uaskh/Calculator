// Package httpapi is the HTTP transport layer: routing, request decoding and validation,
// response encoding, problem details and middleware. It translates between HTTP and the
// domain and contains no business rules.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// Deps are the collaborators of the HTTP layer. Domain capabilities are added here as
// small interfaces declared in this package (the consumer), and implemented by domain
// packages; the composition root (internal/app) wires them together.
type Deps struct {
	Logger         *slog.Logger
	MaxBodyBytes   int64
	RequestTimeout time.Duration
	AllowedOrigins []string
	// Ready reports whether the service can take traffic; nil means always ready.
	Ready func(context.Context) error
}

// NewRouter returns the service's root handler with all routes and middleware.
func NewRouter(d Deps) http.Handler {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("GET /readyz", readiness(d.Ready))
	// Feature routes, one line each, for example:
	//   items := newItemsHandler(d.Items, d.MaxBodyBytes)
	//   mux.HandleFunc("POST /api/v1/items", items.create)

	return chain(problemMux{mux},
		requestID(d.Logger),
		accessLog,
		recoverer,
		securityHeaders,
		cors(d.AllowedOrigins),
		requestTimeout(d.RequestTimeout),
	)
}

// problemMux serves the mux, replacing its plain-text 404/405 replies with problem details
// (keeping the Allow header).
type problemMux struct{ mux *http.ServeMux }

func (p problemMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, pattern := p.mux.Handler(r); pattern != "" {
		p.mux.ServeHTTP(w, r)
		return
	}
	probe := &headerOnlyWriter{header: http.Header{}}
	p.mux.ServeHTTP(probe, r)
	if probe.status == http.StatusMethodNotAllowed {
		w.Header().Set("Allow", probe.header.Get("Allow"))
		writeProblem(w, r, Problem{
			Status: http.StatusMethodNotAllowed,
			Code:   CodeMethodNotAllowed,
			Detail: "Method " + r.Method + " is not allowed for this resource.",
		})
		return
	}
	writeProblem(w, r, Problem{
		Status: http.StatusNotFound,
		Code:   CodeNotFound,
		Detail: "No resource matches this path.",
	})
}

// headerOnlyWriter records the status and headers of a response and discards its body.
type headerOnlyWriter struct {
	header http.Header
	status int
}

func (h *headerOnlyWriter) Header() http.Header         { return h.header }
func (h *headerOnlyWriter) Write(p []byte) (int, error) { return len(p), nil }
func (h *headerOnlyWriter) WriteHeader(code int)        { h.status = code }

type statusBody struct {
	Status string `json:"status"`
}

func health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, http.StatusOK, statusBody{Status: "ok"})
}

func readiness(check func(context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if check != nil {
			if err := check(r.Context()); err != nil {
				loggerFrom(r.Context()).WarnContext(r.Context(), "not ready", "error", err)
				writeProblem(w, r, Problem{
					Status: http.StatusServiceUnavailable,
					Code:   CodeNotReady,
					Detail: "The service is not ready to handle requests.",
				})
				return
			}
		}
		writeJSON(w, r, http.StatusOK, statusBody{Status: "ready"})
	}
}
