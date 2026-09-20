// Package server runs an HTTP server with production timeouts and graceful shutdown.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

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
// nil) while still accepting connections, stops accepting and waits up to
// cfg.ShutdownTimeout for in-flight requests to finish. beforeShutdown is where the
// application flips its readiness flag, so load balancers see /readyz fail before the
// listener closes.
func Serve(ctx context.Context, ln net.Listener, cfg config.HTTP, handler http.Handler, logger *slog.Logger, beforeShutdown func()) error {
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    1 << 20,
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
	logger.InfoContext(ctx, "shutting down http server", "timeout", cfg.ShutdownTimeout)
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server: %w", err)
	}
	logger.InfoContext(ctx, "http server stopped")
	return nil
}
