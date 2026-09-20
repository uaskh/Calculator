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
	if cfg.HTTP.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", cfg.HTTP.Addr)
	}
	if cfg.HTTP.MaxBodyBytes != 1<<20 {
		t.Errorf("MaxBodyBytes = %d, want %d", cfg.HTTP.MaxBodyBytes, 1<<20)
	}
	if cfg.LogLevel != slog.LevelInfo || cfg.LogFormat != "json" {
		t.Errorf("logging = %v/%s, want INFO/json", cfg.LogLevel, cfg.LogFormat)
	}
	if cfg.AllowedOrigins != nil {
		t.Errorf("AllowedOrigins = %v, want none", cfg.AllowedOrigins)
	}
}

func TestLoad_Overrides(t *testing.T) {
	t.Parallel()

	cfg, err := Load(env(map[string]string{
		"HTTP_ADDR":            "127.0.0.1:9000",
		"HTTP_REQUEST_TIMEOUT": "2s",
		"HTTP_MAX_BODY_BYTES":  "2048",
		"LOG_LEVEL":            "debug",
		"LOG_FORMAT":           "TEXT",
		"CORS_ALLOWED_ORIGINS": " http://localhost:5173 , ,https://app.example.com",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTP.Addr != "127.0.0.1:9000" || cfg.HTTP.RequestTimeout != 2*time.Second || cfg.HTTP.MaxBodyBytes != 2048 {
		t.Errorf("HTTP = %+v", cfg.HTTP)
	}
	if cfg.LogLevel != slog.LevelDebug || cfg.LogFormat != "text" {
		t.Errorf("logging = %v/%s, want DEBUG/text", cfg.LogLevel, cfg.LogFormat)
	}
	want := []string{"http://localhost:5173", "https://app.example.com"}
	if !slices.Equal(cfg.AllowedOrigins, want) {
		t.Errorf("AllowedOrigins = %v, want %v", cfg.AllowedOrigins, want)
	}
}

func TestLoad_Invalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		vars    map[string]string
		wantErr string
	}{
		{"bad duration", map[string]string{"HTTP_READ_TIMEOUT": "soon"}, "HTTP_READ_TIMEOUT"},
		{"negative duration", map[string]string{"HTTP_IDLE_TIMEOUT": "-1s"}, "HTTP_IDLE_TIMEOUT"},
		{"zero body limit", map[string]string{"HTTP_MAX_BODY_BYTES": "0"}, "HTTP_MAX_BODY_BYTES"},
		{"bad log level", map[string]string{"LOG_LEVEL": "loud"}, "LOG_LEVEL"},
		{"bad log format", map[string]string{"LOG_FORMAT": "xml"}, "LOG_FORMAT"},
		{"request timeout too long", map[string]string{"HTTP_REQUEST_TIMEOUT": "30s"}, "HTTP_REQUEST_TIMEOUT"},
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

func TestLoad_ReportsAllProblems(t *testing.T) {
	t.Parallel()

	_, err := Load(env(map[string]string{"LOG_LEVEL": "loud", "LOG_FORMAT": "xml"}))
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
	for _, key := range []string{"LOG_LEVEL", "LOG_FORMAT"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not mention %s", err, key)
		}
	}
}
