package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aksh/calculator/backend/internal/config"
)

func testConfig(addr string) config.HTTP {
	return config.HTTP{
		Addr:              addr,
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       time.Second,
		WriteTimeout:      2 * time.Second,
		IdleTimeout:       time.Second,
		ShutdownTimeout:   2 * time.Second,
	}
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

func noop() {}

func listen(t *testing.T) net.Listener {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func get(ctx context.Context, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func TestServe_GracefulShutdownDrainsInFlightRequests(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	started := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusAccepted)
	})

	ln := listen(t)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, testConfig(ln.Addr().String()), handler, discard(), noop) }()

	var (
		wg     sync.WaitGroup
		status int
		reqErr error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		status, reqErr = get(t.Context(), "http://"+ln.Addr().String()+"/")
	}()

	<-started
	cancel() // begin shutdown while the request is in flight
	close(release)
	wg.Wait()

	if reqErr != nil || status != http.StatusAccepted {
		t.Fatalf("in-flight request: status %d, err %v; want 202", status, reqErr)
	}
	if err := <-done; err != nil {
		t.Fatalf("Serve() = %v, want nil", err)
	}
}

// TestServe_BeforeShutdownRunsWhileStillServing proves the hook runs after ctx ends and
// before srv.Shutdown: a request issued from inside the hook still gets a response, which
// is impossible once Shutdown has closed the listener.
func TestServe_BeforeShutdownRunsWhileStillServing(t *testing.T) {
	t.Parallel()

	var (
		mu     sync.Mutex
		events []string
	)
	record := func(event string) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		record("request")
		w.WriteHeader(http.StatusOK)
	})

	ln := listen(t)
	url := "http://" + ln.Addr().String() + "/"
	ctx, cancel := context.WithCancel(t.Context())
	var (
		hookStatus int
		hookErr    error
	)
	hook := func() {
		record("hook")
		hookStatus, hookErr = get(t.Context(), url)
	}
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, testConfig(ln.Addr().String()), handler, discard(), hook) }()

	if status, err := get(t.Context(), url); err != nil || status != http.StatusOK {
		t.Fatalf("warm-up request: status %d, err %v", status, err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Serve() = %v, want nil", err)
	}

	if hookErr != nil || hookStatus != http.StatusOK {
		t.Fatalf("request from the hook: status %d, err %v; want 200 (the server must still be serving)", hookStatus, hookErr)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{"request", "hook", "request"}
	if len(events) != len(want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("events = %v, want %v", events, want)
		}
	}
	if _, err := get(t.Context(), url); err == nil {
		t.Error("server still accepting connections after Serve returned")
	}
}

// TestServe_ShutdownDelayKeepsServingAfterReadinessFlips proves the drain window: once the
// hook has flipped readiness, the listener still accepts connections for ShutdownDelay
// (so load balancers see /readyz fail and stop routing before the socket closes) and only
// then does the server stop.
func TestServe_ShutdownDelayKeepsServingAfterReadinessFlips(t *testing.T) {
	t.Parallel()

	const delay = 300 * time.Millisecond
	var shuttingDown atomic.Bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/readyz" && shuttingDown.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	hookCalled := make(chan struct{})
	hook := func() {
		shuttingDown.Store(true)
		close(hookCalled)
	}

	ln := listen(t)
	base := "http://" + ln.Addr().String()
	cfg := testConfig(ln.Addr().String())
	cfg.ShutdownDelay = delay
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, cfg, handler, discard(), hook) }()

	if status, err := get(t.Context(), base+"/readyz"); err != nil || status != http.StatusOK {
		t.Fatalf("GET /readyz before shutdown: status %d, err %v; want 200", status, err)
	}
	shutdownStarted := time.Now()
	cancel()
	<-hookCalled

	if status, err := get(t.Context(), base+"/readyz"); err != nil || status != http.StatusServiceUnavailable {
		t.Errorf("GET /readyz during the delay: status %d, err %v; want 503 from a still-open listener", status, err)
	}
	if status, err := get(t.Context(), base+"/healthz"); err != nil || status != http.StatusOK {
		t.Errorf("GET /healthz during the delay: status %d, err %v; want 200", status, err)
	}
	select {
	case err := <-done:
		t.Fatalf("Serve() returned %v during the delay", err)
	default:
	}

	if err := <-done; err != nil {
		t.Fatalf("Serve() = %v, want nil", err)
	}
	if elapsed := time.Since(shutdownStarted); elapsed < delay {
		t.Errorf("shutdown took %s, want at least the %s delay", elapsed, delay)
	}
	if _, err := get(t.Context(), base+"/healthz"); err == nil {
		t.Error("server still accepting connections after Serve returned")
	}
}

// TestServe_DrainTimeoutClosesConnections proves that a handler which never finishes
// cannot keep the process alive: after ShutdownTimeout the server closes the open
// connections and Serve returns nil, so the process exits cleanly.
func TestServe_DrainTimeoutClosesConnections(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	started := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
	})

	ln := listen(t)
	cfg := testConfig(ln.Addr().String())
	cfg.ShutdownTimeout = 100 * time.Millisecond
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, cfg, handler, logger, noop) }()

	requestDone := make(chan error, 1)
	go func() {
		_, err := get(t.Context(), "http://"+ln.Addr().String()+"/")
		requestDone <- err
	}()
	<-started
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve() = %v, want nil after a timed-out drain", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve() did not return after the shutdown timeout")
	}
	if err := <-requestDone; err == nil {
		t.Error("the hung request completed, want its connection closed")
	}
	if !strings.Contains(logs.String(), "shutdown timed out; closing open connections") || !strings.Contains(logs.String(), `"level":"WARN"`) {
		t.Errorf("timed-out drain was not logged at WARN: %s", logs.String())
	}
}

func TestServe_UsesConfiguredHeaderLimit(t *testing.T) {
	t.Parallel()

	ln := listen(t)
	cfg := testConfig(ln.Addr().String())
	cfg.MaxHeaderBytes = 1024
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, cfg, http.NotFoundHandler(), discard(), noop) }()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+ln.Addr().String()+"/", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	// net/http grants 4 KiB on top of MaxHeaderBytes for the request line, so go well past it.
	req.Header.Set("X-Padding", strings.Repeat("a", 8192))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request with oversized headers: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestHeaderFieldsTooLarge {
		t.Errorf("status = %d, want 431 with MaxHeaderBytes=1024", resp.StatusCode)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Serve() = %v, want nil", err)
	}
}

func TestServe_LogsShutdownTimingsAsDurations(t *testing.T) {
	t.Parallel()

	ln := listen(t)
	cfg := testConfig(ln.Addr().String())
	cfg.ShutdownDelay = time.Millisecond
	var logs bytes.Buffer
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, ln, cfg, http.NotFoundHandler(), slog.New(slog.NewJSONHandler(&logs, nil)), noop)
	}()
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Serve() = %v, want nil", err)
	}

	var entry map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(logs.String()), "\n") {
		var candidate map[string]any
		if err := json.Unmarshal([]byte(line), &candidate); err != nil {
			t.Fatalf("log line is not JSON: %s", line)
		}
		if candidate["msg"] == "shutting down http server" {
			entry = candidate
		}
	}
	if entry == nil {
		t.Fatalf("no shutdown line in: %s", logs.String())
	}
	if entry["timeout"] != "2s" || entry["delay"] != "1ms" {
		t.Errorf("shutdown line = %v, want timeout \"2s\" and delay \"1ms\" as duration strings", entry)
	}
}

func TestServe_NilHookIsAllowed(t *testing.T) {
	t.Parallel()

	ln := listen(t)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, testConfig(ln.Addr().String()), http.NotFoundHandler(), discard(), nil) }()

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Serve() = %v, want nil", err)
	}
}

func TestRun_InvalidAddress(t *testing.T) {
	t.Parallel()
	err := Run(t.Context(), testConfig("256.0.0.1:http-alt-invalid"), http.NotFoundHandler(), discard(), noop)
	if err == nil {
		t.Fatal("Run() = nil, want listen error")
	}
}

func TestRun_ServesUntilCancelled(t *testing.T) {
	t.Parallel()
	ln := listen(t)
	addr := ln.Addr().String()
	_ = ln.Close() // free the port for Run()

	ctx, cancel := context.WithCancel(t.Context())
	hookCalled := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, testConfig(addr), http.NotFoundHandler(), discard(), func() { close(hookCalled) })
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := get(t.Context(), "http://"+addr+"/"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server never started listening")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	if err := <-done; err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	select {
	case <-hookCalled:
	default:
		t.Error("beforeShutdown hook was not called")
	}
}

func TestServe_ListenerFailure(t *testing.T) {
	t.Parallel()
	ln := listen(t)
	_ = ln.Close() // Serve fails immediately on a closed listener

	if err := Serve(t.Context(), ln, testConfig(""), http.NotFoundHandler(), discard(), noop); err == nil {
		t.Fatal("Serve() = nil, want error")
	}
}
