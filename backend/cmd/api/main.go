// Command api runs the calculator HTTP API service.
//
// Configuration comes from environment variables (see internal/config). The -healthcheck
// flag probes a running instance and is used by container health checks.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/aksh/calculator/backend/internal/app"
	"github.com/aksh/calculator/backend/internal/config"
	"github.com/aksh/calculator/backend/internal/server"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

// run is main without process-level side effects, so it can be tested.
func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("api", flag.ContinueOnError)
	flags.SetOutput(stderr)
	healthcheck := flags.Bool("healthcheck", false, "probe GET /healthz on the configured address and exit")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	cfg, err := config.Load(getenv)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *healthcheck {
		return probe(ctx, cfg.HTTP.Addr, stderr)
	}

	logger := newLogger(stdout, cfg)
	logStartup(ctx, logger, cfg)
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	// The first signal starts the graceful drain; restoring the default signal handling
	// right away lets a second SIGINT/SIGTERM terminate the process immediately instead
	// of being swallowed until the drain finishes.
	context.AfterFunc(ctx, stop)

	a := app.New(cfg, logger)
	if err := server.Run(ctx, cfg.HTTP, a.Handler, logger, a.BeginShutdown); err != nil {
		logger.ErrorContext(ctx, "service stopped", "error", err)
		return 1
	}
	return 0
}

// logStartup writes the one-line summary operators look for first: the effective
// configuration and the build that is running.
func logStartup(ctx context.Context, logger *slog.Logger, cfg config.Config) {
	info, _ := debug.ReadBuildInfo()
	revision, modified := vcsInfo(info)
	logger.InfoContext(ctx, "starting",
		"addr", cfg.HTTP.Addr,
		"log_level", strings.ToLower(cfg.LogLevel.String()),
		"log_format", cfg.LogFormat,
		"read_header_timeout", cfg.HTTP.ReadHeaderTimeout.String(),
		"read_timeout", cfg.HTTP.ReadTimeout.String(),
		"write_timeout", cfg.HTTP.WriteTimeout.String(),
		"idle_timeout", cfg.HTTP.IdleTimeout.String(),
		"shutdown_timeout", cfg.HTTP.ShutdownTimeout.String(),
		"shutdown_delay", cfg.HTTP.ShutdownDelay.String(),
		"request_timeout", cfg.HTTP.RequestTimeout.String(),
		"max_body_bytes", cfg.HTTP.MaxBodyBytes,
		"max_header_bytes", cfg.HTTP.MaxHeaderBytes,
		"cors_allowed_origins", cfg.AllowedOrigins,
		"go_version", runtime.Version(),
		"vcs_revision", revision,
		"vcs_modified", modified,
	)
}

// vcsInfo extracts the commit and dirty flag stamped by the Go toolchain; binaries built
// outside a checkout report "unknown".
func vcsInfo(info *debug.BuildInfo) (revision, modified string) {
	revision, modified = "unknown", "unknown"
	if info == nil {
		return revision, modified
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value
		}
	}
	return revision, modified
}

func newLogger(w io.Writer, cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.LogFormat == "text" {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}

// probe checks the local instance's liveness endpoint; distroless images have no curl.
func probe(ctx context.Context, addr string, stderr io.Writer) int {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		fmt.Fprintf(stderr, "healthcheck: invalid address %q: %v\n", addr, err)
		return 1
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	url := "http://" + net.JoinHostPort(host, port) + "/healthz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		fmt.Fprintf(stderr, "healthcheck: %v\n", err)
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(stderr, "healthcheck: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(stderr, "healthcheck: %s returned %d\n", url, resp.StatusCode)
		return 1
	}
	return 0
}
