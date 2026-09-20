package config

import (
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"
)

func env(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func TestLoad_Defaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"HTTP_ADDR", cfg.HTTP.Addr, ":8080"},
		{"HTTP_READ_HEADER_TIMEOUT", cfg.HTTP.ReadHeaderTimeout, 5 * time.Second},
		{"HTTP_READ_TIMEOUT", cfg.HTTP.ReadTimeout, 10 * time.Second},
		{"HTTP_WRITE_TIMEOUT", cfg.HTTP.WriteTimeout, 15 * time.Second},
		{"HTTP_IDLE_TIMEOUT", cfg.HTTP.IdleTimeout, 60 * time.Second},
		{"HTTP_SHUTDOWN_TIMEOUT", cfg.HTTP.ShutdownTimeout, 10 * time.Second},
		{"HTTP_SHUTDOWN_DELAY", cfg.HTTP.ShutdownDelay, time.Duration(0)},
		{"HTTP_REQUEST_TIMEOUT", cfg.HTTP.RequestTimeout, 5 * time.Second},
		{"HTTP_MAX_BODY_BYTES", cfg.HTTP.MaxBodyBytes, int64(4096)},
		{"HTTP_MAX_HEADER_BYTES", cfg.HTTP.MaxHeaderBytes, 1 << 20},
		{"LOG_LEVEL", cfg.LogLevel, slog.LevelInfo},
		{"LOG_FORMAT", cfg.LogFormat, "json"},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("default %s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
	if cfg.AllowedOrigins != nil {
		t.Errorf("AllowedOrigins = %v, want none (CORS off)", cfg.AllowedOrigins)
	}
}

func TestLoad_Overrides(t *testing.T) {
	t.Parallel()

	cfg, err := Load(env(map[string]string{
		"HTTP_ADDR":                "127.0.0.1:9000",
		"HTTP_READ_HEADER_TIMEOUT": "1s",
		"HTTP_READ_TIMEOUT":        "3s",
		"HTTP_WRITE_TIMEOUT":       "4s",
		"HTTP_IDLE_TIMEOUT":        "30s",
		"HTTP_SHUTDOWN_TIMEOUT":    "2s",
		"HTTP_SHUTDOWN_DELAY":      "3s",
		"HTTP_REQUEST_TIMEOUT":     "2s",
		"HTTP_MAX_BODY_BYTES":      "2048",
		"HTTP_MAX_HEADER_BYTES":    "8192",
		"LOG_LEVEL":                "debug",
		"LOG_FORMAT":               "TEXT",
		"CORS_ALLOWED_ORIGINS":     " http://localhost:5173 , ,https://app.example.com",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := HTTP{
		Addr:              "127.0.0.1:9000",
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       3 * time.Second,
		WriteTimeout:      4 * time.Second,
		IdleTimeout:       30 * time.Second,
		ShutdownTimeout:   2 * time.Second,
		ShutdownDelay:     3 * time.Second,
		RequestTimeout:    2 * time.Second,
		MaxBodyBytes:      2048,
		MaxHeaderBytes:    8192,
	}
	if cfg.HTTP != want {
		t.Errorf("HTTP = %+v, want %+v", cfg.HTTP, want)
	}
	if cfg.LogLevel != slog.LevelDebug || cfg.LogFormat != "text" {
		t.Errorf("logging = %v/%s, want DEBUG/text", cfg.LogLevel, cfg.LogFormat)
	}
	wantOrigins := []string{"http://localhost:5173", "https://app.example.com"}
	if !slices.Equal(cfg.AllowedOrigins, wantOrigins) {
		t.Errorf("AllowedOrigins = %v, want %v", cfg.AllowedOrigins, wantOrigins)
	}
}

func TestLoad_Invalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		vars    map[string]string
		wantErr string
	}{
		{"unparsable read header timeout", map[string]string{"HTTP_READ_HEADER_TIMEOUT": "soon"}, "HTTP_READ_HEADER_TIMEOUT"},
		{"zero read header timeout", map[string]string{"HTTP_READ_HEADER_TIMEOUT": "0"}, "HTTP_READ_HEADER_TIMEOUT"},
		{"unparsable read timeout", map[string]string{"HTTP_READ_TIMEOUT": "soon"}, "HTTP_READ_TIMEOUT"},
		{"unparsable write timeout", map[string]string{"HTTP_WRITE_TIMEOUT": "later"}, "HTTP_WRITE_TIMEOUT"},
		{"negative idle timeout", map[string]string{"HTTP_IDLE_TIMEOUT": "-1s"}, "HTTP_IDLE_TIMEOUT"},
		{"unparsable shutdown timeout", map[string]string{"HTTP_SHUTDOWN_TIMEOUT": "10"}, "HTTP_SHUTDOWN_TIMEOUT"},
		{"zero shutdown timeout", map[string]string{"HTTP_SHUTDOWN_TIMEOUT": "0s"}, "HTTP_SHUTDOWN_TIMEOUT"},
		{"unparsable shutdown delay", map[string]string{"HTTP_SHUTDOWN_DELAY": "3"}, "HTTP_SHUTDOWN_DELAY"},
		{"negative shutdown delay", map[string]string{"HTTP_SHUTDOWN_DELAY": "-1s"}, "HTTP_SHUTDOWN_DELAY"},
		{"unparsable request timeout", map[string]string{"HTTP_REQUEST_TIMEOUT": "five"}, "HTTP_REQUEST_TIMEOUT"},
		{"request timeout equal to write timeout", map[string]string{"HTTP_REQUEST_TIMEOUT": "15s"}, "HTTP_REQUEST_TIMEOUT"},
		{"request timeout longer than write timeout", map[string]string{"HTTP_REQUEST_TIMEOUT": "30s"}, "HTTP_REQUEST_TIMEOUT"},
		{"zero body limit", map[string]string{"HTTP_MAX_BODY_BYTES": "0"}, "HTTP_MAX_BODY_BYTES"},
		{"negative body limit", map[string]string{"HTTP_MAX_BODY_BYTES": "-1"}, "HTTP_MAX_BODY_BYTES"},
		{"non-numeric body limit", map[string]string{"HTTP_MAX_BODY_BYTES": "4k"}, "HTTP_MAX_BODY_BYTES"},
		{"zero header limit", map[string]string{"HTTP_MAX_HEADER_BYTES": "0"}, "HTTP_MAX_HEADER_BYTES"},
		{"negative header limit", map[string]string{"HTTP_MAX_HEADER_BYTES": "-1"}, "HTTP_MAX_HEADER_BYTES"},
		{"non-numeric header limit", map[string]string{"HTTP_MAX_HEADER_BYTES": "1M"}, "HTTP_MAX_HEADER_BYTES"},
		{"header limit beyond int range", map[string]string{"HTTP_MAX_HEADER_BYTES": "99999999999999999999"}, "HTTP_MAX_HEADER_BYTES"},
		{"unknown log level", map[string]string{"LOG_LEVEL": "loud"}, "LOG_LEVEL"},
		{"unknown log format", map[string]string{"LOG_FORMAT": "xml"}, "LOG_FORMAT"},
		{"wildcard origin", map[string]string{"CORS_ALLOWED_ORIGINS": "*"}, "CORS_ALLOWED_ORIGINS"},
		{"wildcard among origins", map[string]string{"CORS_ALLOWED_ORIGINS": "http://localhost:5173, *"}, "CORS_ALLOWED_ORIGINS"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := Load(env(tc.vars))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Load() error = %v, want mention of %s", err, tc.wantErr)
			}
		})
	}
}

func TestLoad_ZeroShutdownDelayIsAllowed(t *testing.T) {
	t.Parallel()

	cfg, err := Load(env(map[string]string{"HTTP_SHUTDOWN_DELAY": "0s"}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTP.ShutdownDelay != 0 {
		t.Errorf("ShutdownDelay = %s, want 0s", cfg.HTTP.ShutdownDelay)
	}
}

func TestLoad_RequestTimeoutMayShrinkWithWriteTimeout(t *testing.T) {
	t.Parallel()

	cfg, err := Load(env(map[string]string{"HTTP_WRITE_TIMEOUT": "3s", "HTTP_REQUEST_TIMEOUT": "2s"}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTP.RequestTimeout != 2*time.Second || cfg.HTTP.WriteTimeout != 3*time.Second {
		t.Errorf("timeouts = request %s / write %s, want 2s / 3s", cfg.HTTP.RequestTimeout, cfg.HTTP.WriteTimeout)
	}
}

func TestLoad_ReportsAllProblems(t *testing.T) {
	t.Parallel()

	_, err := Load(env(map[string]string{"LOG_LEVEL": "loud", "LOG_FORMAT": "xml", "CORS_ALLOWED_ORIGINS": "*"}))
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
	for _, key := range []string{"LOG_LEVEL", "LOG_FORMAT", "CORS_ALLOWED_ORIGINS"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not mention %s", err, key)
		}
	}
}
