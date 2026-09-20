package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"sync"
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
	cancel()                          // begin shutdown while the request is in flight
	time.Sleep(50 * time.Millisecond) // give Shutdown time to close the listener
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
