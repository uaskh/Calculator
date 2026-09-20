package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"slices"
	"strconv"
	"time"
)

type ctxKey int

const (
	requestIDKey ctxKey = iota
	loggerKey
	stateKey
)

const (
	requestIDHeader     = "X-Request-ID"
	forwardedForHeader  = "X-Forwarded-For"
	maxRequestIDLength  = 64
	generatedIDBytes    = 16 // 32 lower-case hex characters
	corsAllowedMethods  = "GET, POST"
	corsAllowedHeaders  = "Content-Type, " + requestIDHeader
	corsPreflightMaxAge = 600 // seconds
	// maxLoggedHeaderBytes bounds client-controlled header values in the access log.
	maxLoggedHeaderBytes = 256
	// statusClientClosedRequest is the status the access log records for a request the
	// client abandoned before a response was written. It follows nginx's convention
	// (499 "client closed request"); it never goes on the wire, since nobody is listening.
	statusClientClosedRequest = 499
)

// RequestIDFrom returns the request ID stored by the requestID middleware.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

func loggerFrom(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// requestState is the per-request record behind the access log: what was sent and, for
// failures, why. It lives in the request context and is only touched from the goroutine
// serving the request, like the http.ResponseWriter it mirrors. Every method tolerates a
// nil receiver so helpers such as writeProblem work outside the middleware chain.
type requestState struct {
	status    int
	bytes     int
	code      string // the problem's code, for 4xx and 5xx responses
	errorCode string // the first field error's code, when the problem has any
	err       string // why the request failed; only faults set it
	cancelled bool   // the client went away before a response was written
}

func stateFrom(ctx context.Context) *requestState {
	s, _ := ctx.Value(stateKey).(*requestState)
	return s
}

func withState(ctx context.Context, s *requestState) context.Context {
	return context.WithValue(ctx, stateKey, s)
}

// setError records why the request failed; it is reported as the access log's "error"
// and raises the line to ERROR.
func (s *requestState) setError(msg string) {
	if s == nil {
		return
	}
	s.err = msg
}

// setProblem records the codes of the problem sent to the client.
func (s *requestState) setProblem(p Problem) {
	if s == nil {
		return
	}
	s.code = p.Code
	if len(p.Errors) > 0 {
		s.errorCode = p.Errors[0].Code
	}
}

// setCancelled records that the client abandoned the request before any response was
// written; the access log reports it as 499 at INFO rather than as a 200 or a fault.
func (s *requestState) setCancelled() {
	if s == nil {
		return
	}
	s.cancelled = true
}

// isFault reports whether the request failed on the service's side: a recorded error
// (a fault problem or a panic) or a 5xx written without going through writeProblem.
func (s *requestState) isFault() bool {
	return s.err != "" || (s.status >= http.StatusInternalServerError && s.code == "")
}

// responseStarted reports whether a status line has already gone out, after which no
// other response can be written.
func (s *requestState) responseStarted() bool {
	return s != nil && s.status != 0
}

// chain applies middleware so that the first one listed is the outermost.
func chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for _, mw := range slices.Backward(mws) {
		h = mw(h)
	}
	return h
}

// requestID propagates a caller-supplied X-Request-ID (if well-formed) or generates one,
// and stores a request-scoped logger carrying it together with the request state.
func requestID(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(requestIDHeader)
			if !validRequestID(id) {
				id = newRequestID()
			}
			w.Header().Set(requestIDHeader, id)
			ctx := context.WithValue(r.Context(), requestIDKey, id)
			ctx = context.WithValue(ctx, loggerKey, base.With("request_id", id))
			ctx = withState(ctx, &requestState{})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// validRequestID reports whether id matches ^[A-Za-z0-9._-]{1,64}$ (spec §6).
func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLength {
		return false
	}
	for i := 0; i < len(id); i++ {
		if !isRequestIDByte(id[i]) {
			return false
		}
	}
	return true
}

func isRequestIDByte(c byte) bool {
	switch {
	case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		return true
	case c == '.', c == '_', c == '-':
		return true
	default:
		return false
	}
}

func newRequestID() string {
	var b [generatedIDBytes]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never returns an error (Go 1.24+)
	return hex.EncodeToString(b[:])
}

// statusRecorder mirrors the status code and body size into the request state.
type statusRecorder struct {
	http.ResponseWriter
	state *requestState
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.state.status == 0 {
		s.state.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(p []byte) (int, error) {
	if s.state.status == 0 {
		s.state.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(p)
	s.state.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// accessLog logs one structured line per request (NFR-6). now is the clock used to
// measure the request's duration.
func accessLog(now func() time.Time) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := now()
			state := stateFrom(r.Context())
			if state == nil {
				state = &requestState{}
				r = r.WithContext(withState(r.Context(), state))
			}
			next.ServeHTTP(&statusRecorder{ResponseWriter: w, state: state}, r)
			state.finish()
			ctx := r.Context()
			loggerFrom(ctx).LogAttrs(ctx, accessLogLevel(r, state), "http request", accessLogAttrs(r, state, now().Sub(start))...)
		})
	}
}

// finish settles the status of a request whose handler never wrote one.
func (s *requestState) finish() {
	switch {
	case s.status != 0:
	case s.cancelled:
		s.status = statusClientClosedRequest
	default:
		s.status = http.StatusOK
	}
}

// accessLogLevel picks the line's level: faults are errors, successful health probes are
// debug noise, everything else (including cancelled requests and client errors) is info.
func accessLogLevel(r *http.Request, state *requestState) slog.Level {
	switch {
	case state.isFault():
		return slog.LevelError
	case isHealthPath(r.URL.Path) && state.status < http.StatusMultipleChoices:
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}

func accessLogAttrs(r *http.Request, state *requestState, elapsed time.Duration) []slog.Attr {
	attrs := []slog.Attr{
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.Int("status", state.status),
		slog.Float64("duration_ms", float64(elapsed)/float64(time.Millisecond)),
		slog.Int("bytes", state.bytes),
		slog.String("remote_addr", r.RemoteAddr),
		slog.String("user_agent", truncate(r.UserAgent(), maxLoggedHeaderBytes)),
	}
	// X-Forwarded-For is logged for correlation behind a proxy but is never trusted as
	// the peer address: remote_addr stays the connection's.
	if forwardedFor := r.Header.Get(forwardedForHeader); forwardedFor != "" {
		attrs = append(attrs, slog.String("forwarded_for", truncate(forwardedFor, maxLoggedHeaderBytes)))
	}
	if state.code != "" {
		attrs = append(attrs, slog.String("code", state.code))
	}
	if state.errorCode != "" {
		attrs = append(attrs, slog.String("error_code", state.errorCode))
	}
	if state.cancelled {
		attrs = append(attrs, slog.Bool("cancelled", true))
	}
	if state.err != "" {
		attrs = append(attrs, slog.String("error", state.err))
	}
	return attrs
}

// truncate cuts s to at most n bytes.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// recoverer turns a panic into a logged 500 problem instead of a dropped connection.
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer recoverPanic(w, r)
		next.ServeHTTP(w, r)
	})
}

// recoverPanic must be called directly by defer for recover to take effect. A panic that
// happens after the status line went out can only be logged: the client already has a
// response and a second one would corrupt it.
func recoverPanic(w http.ResponseWriter, r *http.Request) {
	v := recover()
	if v == nil {
		return
	}
	if v == http.ErrAbortHandler { //nolint:errorlint // sentinel panic value, compared by identity as net/http does
		panic(v)
	}
	ctx := r.Context()
	state := stateFrom(ctx)
	loggerFrom(ctx).ErrorContext(ctx, "panic serving request",
		"panic", v,
		"response_started", state.responseStarted(),
		"stack", string(debug.Stack()),
	)
	if !state.responseStarted() {
		writeProblem(w, r, Problem{Status: http.StatusInternalServerError, Code: CodeInternal})
	}
	state.setError(fmt.Sprint(v))
}

// securityHeaders sets the defensive headers of spec §6 on every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// cors allows cross-origin calls from an explicit list of exact origins. There is no
// wildcard: config.Load refuses "*", and an origin that is not listed gets no
// Access-Control headers at all.
func cors(allowed []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			if len(allowed) > 0 {
				// With CORS enabled every response depends on Origin, including those for
				// missing or refused origins: caches must keep the variants apart.
				h.Add("Vary", "Origin")
			}
			origin := r.Header.Get("Origin")
			if origin == "" || !slices.Contains(allowed, origin) {
				next.ServeHTTP(w, r)
				return
			}
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Expose-Headers", requestIDHeader)
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				h.Set("Access-Control-Allow-Methods", corsAllowedMethods)
				h.Set("Access-Control-Allow-Headers", corsAllowedHeaders)
				h.Set("Access-Control-Max-Age", strconv.Itoa(corsPreflightMaxAge))
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requestTimeout bounds the time handlers may spend; they must honour ctx.Done().
// A duration of zero or less sets no deadline.
func requestTimeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if d <= 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
