package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"example.com/service/internal/config"
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

func TestServe_GracefulShutdownDrainsInFlightRequests(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	started := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusAccepted)
	})

	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, testConfig(ln.Addr().String()), handler, discard()) }()

	var (
		wg     sync.WaitGroup
		status int
		reqErr error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+ln.Addr().String()+"/", http.NoBody)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			reqErr = err
			return
		}
		defer resp.Body.Close()
		status = resp.StatusCode
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

func TestRun_InvalidAddress(t *testing.T) {
	t.Parallel()
	err := Run(t.Context(), testConfig("256.0.0.1:http-alt-invalid"), http.NotFoundHandler(), discard())
	if err == nil {
		t.Fatal("Run() = nil, want listen error")
	}
}

func TestServe_ListenerFailure(t *testing.T) {
	t.Parallel()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close() // Serve fails immediately on a closed listener

	if err := Serve(t.Context(), ln, testConfig(""), http.NotFoundHandler(), discard()); err == nil {
		t.Fatal("Serve() = nil, want error")
	}
}
