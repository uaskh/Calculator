package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
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
