// Package app is the composition root: it builds the domain services and the HTTP layer
// from configuration and wires them together. Nothing else constructs dependencies.
package app

import (
	"log/slog"
	"net/http"

	"example.com/service/internal/config"
	"example.com/service/internal/httpapi"
)

// New returns the fully wired HTTP handler for the service.
func New(cfg config.Config, logger *slog.Logger) http.Handler {
	// Construct domain services here and pass them to the HTTP layer through the
	// interfaces it declares, for example:
	//   items := item.NewService(item.NewMemoryStore())
	return httpapi.NewRouter(httpapi.Deps{
		Logger:         logger,
		MaxBodyBytes:   cfg.HTTP.MaxBodyBytes,
		RequestTimeout: cfg.HTTP.RequestTimeout,
		AllowedOrigins: cfg.AllowedOrigins,
	})
}
