// Package config loads and validates the service configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Config is the complete, validated service configuration.
type Config struct {
	HTTP           HTTP
	LogLevel       slog.Level
	LogFormat      string   // "json" (default) or "text"
	AllowedOrigins []string // CORS allow-list of exact origins; empty means same-origin only
}

// HTTP holds listener, limit and timeout settings.
type HTTP struct {
	Addr              string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	RequestTimeout    time.Duration
	MaxBodyBytes      int64
}

// Load reads the configuration through getenv (os.Getenv in production) and validates it.
// All problems are reported together so a misconfigured deployment fails fast and clearly.
func Load(getenv func(string) string) (Config, error) {
	r := reader{getenv: getenv}
	cfg := Config{
		HTTP: HTTP{
			Addr:              r.str("HTTP_ADDR", ":8080"),
			ReadHeaderTimeout: r.duration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
			ReadTimeout:       r.duration("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:      r.duration("HTTP_WRITE_TIMEOUT", 15*time.Second),
			IdleTimeout:       r.duration("HTTP_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout:   r.duration("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second),
			RequestTimeout:    r.duration("HTTP_REQUEST_TIMEOUT", 5*time.Second),
			MaxBodyBytes:      r.positiveInt("HTTP_MAX_BODY_BYTES", 4096),
		},
		LogLevel:       r.logLevel("LOG_LEVEL", slog.LevelInfo),
		LogFormat:      r.oneOf("LOG_FORMAT", "json", "json", "text"),
		AllowedOrigins: r.origins("CORS_ALLOWED_ORIGINS"),
	}
	if cfg.HTTP.RequestTimeout >= cfg.HTTP.WriteTimeout {
		r.fail("HTTP_REQUEST_TIMEOUT (%s) must be shorter than HTTP_WRITE_TIMEOUT (%s)",
			cfg.HTTP.RequestTimeout, cfg.HTTP.WriteTimeout)
	}
	if err := errors.Join(r.errs...); err != nil {
		return Config{}, fmt.Errorf("invalid configuration: %w", err)
	}
	return cfg, nil
}

type reader struct {
	getenv func(string) string
	errs   []error
}

func (r *reader) fail(format string, args ...any) {
	r.errs = append(r.errs, fmt.Errorf(format, args...))
}

func (r *reader) lookup(key string) (string, bool) {
	v := strings.TrimSpace(r.getenv(key))
	return v, v != ""
}

func (r *reader) str(key, def string) string {
	if v, ok := r.lookup(key); ok {
		return v
	}
	return def
}

func (r *reader) duration(key string, def time.Duration) time.Duration {
	raw, ok := r.lookup(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		r.fail("%s must be a positive duration such as 5s, got %q", key, raw)
		return def
	}
	return d
}

func (r *reader) positiveInt(key string, def int64) int64 {
	raw, ok := r.lookup(key)
	if !ok {
		return def
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		r.fail("%s must be a positive integer, got %q", key, raw)
		return def
	}
	return n
}

func (r *reader) logLevel(key string, def slog.Level) slog.Level {
	raw, ok := r.lookup(key)
	if !ok {
		return def
	}
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(raw)); err != nil {
		r.fail("%s must be one of debug, info, warn, error, got %q", key, raw)
		return def
	}
	return lvl
}

func (r *reader) oneOf(key, def string, allowed ...string) string {
	raw, ok := r.lookup(key)
	if !ok {
		return def
	}
	for _, a := range allowed {
		if strings.EqualFold(raw, a) {
			return a
		}
	}
	r.fail("%s must be one of %s, got %q", key, strings.Join(allowed, ", "), raw)
	return def
}

func (r *reader) list(key string) []string {
	raw, ok := r.lookup(key)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// origins reads a comma-separated allow-list of exact origins. The wildcard "*" is
// refused: CORS is either off or limited to origins named explicitly (spec §6).
func (r *reader) origins(key string) []string {
	origins := r.list(key)
	if slices.Contains(origins, "*") {
		r.fail("%s must list exact origins such as https://app.example.com; \"*\" is not allowed", key)
		return nil
	}
	return origins
}
