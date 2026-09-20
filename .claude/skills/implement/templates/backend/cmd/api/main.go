// Command api runs the HTTP API service.
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
	"syscall"
	"time"

	"example.com/service/internal/app"
	"example.com/service/internal/config"
	"example.com/service/internal/server"
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
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := server.Run(ctx, cfg.HTTP, app.New(cfg, logger), logger); err != nil {
		logger.ErrorContext(ctx, "service stopped", "error", err)
		return 1
	}
	return 0
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
