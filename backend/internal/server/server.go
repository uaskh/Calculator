// Package server runs an HTTP server with production timeouts and graceful shutdown.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/aksh/calculator/backend/internal/config"
)

// Run listens on cfg.Addr and serves handler until ctx is cancelled. See Serve for the
// role of beforeShutdown.
func Run(ctx context.Context, cfg config.HTTP, handler http.Handler, logger *slog.Logger, beforeShutdown func()) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Addr, err)
	}
	return Serve(ctx, ln, cfg, handler, logger, beforeShutdown)
}

// Serve serves handler on ln until ctx is cancelled, then calls beforeShutdown (if not
// nil) while still accepting connections, keeps serving for cfg.ShutdownDelay, stops
// accepting and waits up to cfg.ShutdownTimeout for in-flight requests to finish.
// beforeShutdown is where the application flips its readiness flag, so load balancers
// see /readyz fail and stop routing traffic before the listener closes. A drain that
// outlives the timeout closes the remaining connections; it is logged, not fatal.
func Serve(ctx context.Context, ln net.Listener, cfg config.HTTP, handler http.Handler, logger *slog.Logger, beforeShutdown func()) error {
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    cfg.MaxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.InfoContext(ctx, "http server listening", "addr", ln.Addr().String())
		serveErr <- srv.Serve(ln)
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}

	if beforeShutdown != nil {
		beforeShutdown()
	}
	logger.InfoContext(ctx, "shutting down http server",
		"delay", cfg.ShutdownDelay.String(),
		"timeout", cfg.ShutdownTimeout.String(),
	)
	waitForDrain(cfg.ShutdownDelay)
	if err := shutdown(ctx, srv, cfg.ShutdownTimeout, logger); err != nil {
		return err
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server: %w", err)
	}
	logger.InfoContext(ctx, "http server stopped")
	return nil
}

// waitForDrain blocks for delay while the listener stays open, giving load balancers
// time to act on the failing readiness check.
func waitForDrain(delay time.Duration) {
	if delay <= 0 {
		return
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	<-timer.C
}

// shutdown stops accepting connections and waits up to timeout for in-flight requests.
// When they outlive the timeout, the remaining connections are closed so the process can
// still exit cleanly; a second signal during the drain is handled by the caller.
func shutdown(ctx context.Context, srv *http.Server, timeout time.Duration, logger *slog.Logger) error {
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	err := srv.Shutdown(shutdownCtx)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.DeadlineExceeded):
		logger.WarnContext(ctx, "shutdown timed out; closing open connections", "timeout", timeout.String())
		if closeErr := srv.Close(); closeErr != nil {
			return fmt.Errorf("close http server: %w", closeErr)
		}
		return nil
	default:
		return fmt.Errorf("graceful shutdown: %w", err)
	}
}
