package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

func envMap(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func TestRun_ServesUntilCancelled(t *testing.T) {
	t.Parallel()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // free the port for run()

	ctx, cancel := context.WithCancel(t.Context())
	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, nil, envMap(map[string]string{"HTTP_ADDR": addr}), &stdout, &stderr)
	}()

	waitHealthy(t, "http://"+addr+"/healthz")
	cancel()

	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("run() = %d, want 0; stderr: %s", code, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not stop after cancellation")
	}
	if !strings.Contains(stdout.String(), "http server stopped") {
		t.Errorf("expected shutdown log, got: %s", stdout.String())
	}
}

func waitHealthy(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s never became healthy", url)
}

func TestRun_Healthcheck(t *testing.T) {
	t.Parallel()
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(healthy.Close)
	unhealthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(unhealthy.Close)

	tests := []struct {
		name string
		addr string
		want int
	}{
		{"healthy instance", strings.TrimPrefix(healthy.URL, "http://"), 0},
		{"unhealthy instance", strings.TrimPrefix(unhealthy.URL, "http://"), 1},
		{"nothing listening", "127.0.0.1:1", 1},
		{"malformed address", "no-port", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer
			got := run(t.Context(), []string{"-healthcheck"}, envMap(map[string]string{"HTTP_ADDR": tc.addr}), &bytes.Buffer{}, &stderr)
			if got != tc.want {
				t.Errorf("run(-healthcheck) = %d, want %d; stderr: %s", got, tc.want, stderr.String())
			}
		})
	}
}

func TestRun_StartupErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
		env  map[string]string
		want int
	}{
		{"invalid configuration", nil, map[string]string{"LOG_LEVEL": "loud"}, 1},
		{"unknown flag", []string{"-nope"}, nil, 2},
		{"help", []string{"-h"}, nil, 0},
		{"port in use or invalid", nil, map[string]string{"HTTP_ADDR": "256.256.256.256:0"}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := run(t.Context(), tc.args, envMap(tc.env), &bytes.Buffer{}, &bytes.Buffer{})
			if got != tc.want {
				t.Errorf("run() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestRun_LogsConfigurationAtStartup asserts the one-line startup summary. The listen
// address is invalid so run returns right after logging it, without opening a socket.
func TestRun_LogsConfigurationAtStartup(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	env := map[string]string{
		"HTTP_ADDR":            "256.256.256.256:0",
		"HTTP_SHUTDOWN_DELAY":  "3s",
		"LOG_LEVEL":            "debug",
		"CORS_ALLOWED_ORIGINS": "http://localhost:5173",
	}

	if code := run(t.Context(), nil, envMap(env), &stdout, &stderr); code != 1 {
		t.Fatalf("run() = %d, want 1 for an unusable address", code)
	}

	var entry map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(stdout.String()), "\n") {
		var candidate map[string]any
		if err := json.Unmarshal([]byte(line), &candidate); err != nil {
			t.Fatalf("log line is not JSON: %s", line)
		}
		if candidate["msg"] == "starting" {
			entry = candidate
		}
	}
	if entry == nil {
		t.Fatalf("no \"starting\" line in: %s", stdout.String())
	}
	want := map[string]any{
		"level":                "INFO",
		"addr":                 "256.256.256.256:0",
		"log_level":            "debug",
		"log_format":           "json",
		"read_header_timeout":  "5s",
		"read_timeout":         "10s",
		"write_timeout":        "15s",
		"idle_timeout":         "1m0s",
		"shutdown_timeout":     "10s",
		"shutdown_delay":       "3s",
		"request_timeout":      "5s",
		"max_body_bytes":       float64(4096),
		"max_header_bytes":     float64(1 << 20),
		"cors_allowed_origins": []any{"http://localhost:5173"},
		"go_version":           runtime.Version(),
	}
	for key, value := range want {
		if !reflect.DeepEqual(entry[key], value) {
			t.Errorf("starting[%s] = %v (%T), want %v", key, entry[key], entry[key], value)
		}
	}
	for _, key := range []string{"vcs_revision", "vcs_modified"} {
		if v, ok := entry[key].(string); !ok || v == "" {
			t.Errorf("starting[%s] = %v, want a non-empty string (\"unknown\" without build info)", key, entry[key])
		}
	}
}

func TestBuildInfo_ReportsUnknownWithoutVCSSettings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		info         *debug.BuildInfo
		wantRevision string
		wantModified string
	}{
		{"no build info", nil, "unknown", "unknown"},
		{"no vcs settings", &debug.BuildInfo{}, "unknown", "unknown"},
		{"vcs settings", &debug.BuildInfo{Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abc123"},
			{Key: "vcs.modified", Value: "true"},
		}}, "abc123", "true"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			revision, modified := vcsInfo(tc.info)
			if revision != tc.wantRevision || modified != tc.wantModified {
				t.Errorf("vcsInfo() = %q, %q; want %q, %q", revision, modified, tc.wantRevision, tc.wantModified)
			}
		})
	}
}

func TestNewLogger_TextFormat(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	cfg := defaultConfig(t)
	cfg.LogFormat = "text"
	newLogger(&out, cfg).Info("hello")
	if !strings.Contains(out.String(), "msg=hello") {
		t.Errorf("text log = %q", out.String())
	}
}
