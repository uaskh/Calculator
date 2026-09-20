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
	maxRequestIDLength  = 64
	generatedIDBytes    = 16 // 32 lower-case hex characters
	corsAllowedMethods  = "GET, POST"
	corsAllowedHeaders  = "Content-Type, " + requestIDHeader
	corsPreflightMaxAge = 600 // seconds
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
	status int
	bytes  int
	err    string
}

func stateFrom(ctx context.Context) *requestState {
	s, _ := ctx.Value(stateKey).(*requestState)
	return s
}

func withState(ctx context.Context, s *requestState) context.Context {
	return context.WithValue(ctx, stateKey, s)
}

// setError records why the request failed; it is reported as the access log's "error".
func (s *requestState) setError(msg string) {
	if s == nil {
		return
	}
	s.err = msg
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

// accessLog logs one structured line per request (NFR-6).
func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		state := stateFrom(r.Context())
		if state == nil {
			state = &requestState{}
			r = r.WithContext(withState(r.Context(), state))
		}
		next.ServeHTTP(&statusRecorder{ResponseWriter: w, state: state}, r)
		if state.status == 0 {
			state.status = http.StatusOK
		}
		level := slog.LevelInfo
		attrs := []slog.Attr{
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", state.status),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.Int("bytes", state.bytes),
			slog.String("remote_addr", r.RemoteAddr),
			slog.String("user_agent", r.UserAgent()),
		}
		if state.status >= http.StatusInternalServerError || state.err != "" {
			level = slog.LevelError
		}
		if state.err != "" {
			attrs = append(attrs, slog.String("error", state.err))
		}
		loggerFrom(r.Context()).LogAttrs(r.Context(), level, "http request", attrs...)
	})
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
