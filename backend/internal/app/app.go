// Package app is the composition root: it builds the HTTP layer from configuration and
// the domain services and wires them together. Nothing else constructs dependencies.
package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"

	"github.com/aksh/calculator/backend/internal/calc"
	"github.com/aksh/calculator/backend/internal/config"
	"github.com/aksh/calculator/backend/internal/httpapi"
)

// errShuttingDown is what readiness reports once BeginShutdown has been called.
var errShuttingDown = errors.New("shutdown in progress")

// App is the wired service: the root HTTP handler plus the readiness flag the server
// flips before it stops accepting connections.
type App struct {
	// Handler is the fully wired root handler, including middleware.
	Handler http.Handler

	shuttingDown atomic.Bool
}

// New builds the calculator domain and wires it into the HTTP layer according to cfg.
func New(cfg config.Config, logger *slog.Logger) *App {
	return NewWithEvaluator(cfg, logger, calc.New())
}

// NewWithEvaluator wires evaluator into the HTTP layer according to cfg. Tests use it to
// substitute the domain; production code uses New.
func NewWithEvaluator(cfg config.Config, logger *slog.Logger, evaluator httpapi.Evaluator) *App {
	a := &App{}
	a.Handler = httpapi.NewRouter(httpapi.Deps{
		Logger:         logger,
		MaxBodyBytes:   cfg.HTTP.MaxBodyBytes,
		RequestTimeout: cfg.HTTP.RequestTimeout,
		AllowedOrigins: cfg.AllowedOrigins,
		Ready:          a.ready,
		Evaluator:      evaluator,
	})
	return a
}

// BeginShutdown makes GET /readyz answer 503 NOT_READY from now on, so that load
// balancers stop routing new traffic while in-flight requests drain. It is safe to call
// more than once and from any goroutine.
func (a *App) BeginShutdown() {
	a.shuttingDown.Store(true)
}

// ready is the readiness check: the service has no dependencies, so the only reason to
// refuse traffic is an ongoing shutdown (spec FR-15).
func (a *App) ready(context.Context) error {
	if a.shuttingDown.Load() {
		return errShuttingDown
	}
	return nil
}
