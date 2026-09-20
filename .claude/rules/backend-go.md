---
paths:
  - "backend/**"
---

# Go backend rules

Verified reference code: the skeleton in `.claude/skills/implement/templates/backend/` and the
feature slice in `.claude/skills/implement/references/backend-feature-pattern.md`.

## Dependencies

- Standard library plus exactly one approved module: `github.com/shopspring/decimal`
  (exact decimal arithmetic for the calculator domain; spec `calculator` C-1, decision 14).
  `go.mod` has the `module` and `go` lines and that single `require`; `go.sum` is written
  by `go mod tidy` and committed. Hooks and the `depguard` linter enforce it. If a
  capability seems to need any other module, stop and ask the user.
- Use the toolchain that is installed (`go env GOVERSION`, ≥ 1.24); `go mod init` writes the
  matching `go` directive. Use modern standard library APIs available at that version
  (`http.ServeMux` method/wildcard patterns, `log/slog`, `errors.Join`, `slices`, `maps`,
  `t.Context()`, `b.Loop()`, `slog.DiscardHandler`, `context.WithoutCancel`).

## Layering (dependency rule: arrows point inwards)

| Package | Owns | May import |
|---|---|---|
| `cmd/<service>` | flags, signals, exit codes; calls `app.New` and `server.Run` | app, config, server |
| `internal/app` | composition root: constructs every dependency | everything below |
| `internal/httpapi` | routes, handlers, DTOs, input validation, problem responses, middleware | domain packages |
| `internal/<domain>` | business rules, domain types, sentinel/typed errors | standard library and `github.com/shopspring/decimal` |
| `internal/config` | env → typed `Config`, defaults, validation | standard library only |
| `internal/server` | `http.Server` construction, graceful shutdown | config |

- Domain packages never import `net/http`, never know about JSON field names or status codes.
- Interfaces are declared by the consumer (`httpapi` declares what it needs from the
  domain), are small (1–3 methods) and are satisfied implicitly. Return concrete types.
- Constructors take explicit dependencies: `NewService(store Store, clock func() time.Time) *Service`.
  No package-level mutable state, no `init()` side effects, no singletons, no service locator.
- Open/closed: adding a new behaviour (for example a new variant, policy or format) means adding a
  type or registering an entry in a table/registry, not editing a growing `switch` spread
  across layers. Keep a single place where variants are registered.

## HTTP layer

- Routing: one `http.NewServeMux()` in `NewRouter`, patterns like `"POST /api/v1/items"` and
  `"GET /api/v1/items/{id}"` (`r.PathValue("id")`). Unmatched paths and methods are turned into
  problem responses (405 keeps the `Allow` header).
- Handler shape: decode → validate → call the domain → map the result → encode. Handlers are
  methods on a small struct holding their dependencies; no business rules in handlers.
- Decoding: `decodeJSON` enforces `Content-Type: application/json` (415),
  `http.MaxBytesReader` (413), `DisallowUnknownFields` (400), exactly one JSON value (400),
  and maps type errors to per-field messages. Use pointer fields or `json.RawMessage` when you
  must distinguish "missing" from "zero".
- Validation lives in request DTO methods (`func (r createItemRequest) validate() []FieldError`)
  and reports **all** field errors at once. Domain invariants are enforced again in the domain.
- Encoding: `writeJSON` marshals before writing, so an encoding failure (for example NaN or
  ±Inf in a float, which `encoding/json` rejects) becomes a 500 problem instead of a broken 200.
- Errors: map domain errors to HTTP in exactly one function per handler set
  (`errors.Is` / `errors.As`). 4xx responses explain what to fix; 5xx responses are generic
  and the details are logged with the request ID. Contract: `.claude/rules/api-contract.md`.
- Middleware order (outermost first): request ID → access log → panic recovery → security
  headers → CORS → request timeout → mux. Handlers must honour `r.Context()`.
- Health: `GET /healthz` (liveness, no dependencies) and `GET /readyz` (readiness).

## Server & configuration

- `http.Server` always sets `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`
  and `MaxHeaderBytes`. Shut down gracefully on SIGINT/SIGTERM within `HTTP_SHUTDOWN_TIMEOUT`.
- Configuration comes only from environment variables read in `config.Load(getenv)`; every
  variable has a safe default, is validated at startup (all problems reported together) and
  is documented in the README. No flags for configuration except `-healthcheck`.
- `main` is thin: `os.Exit(run(ctx, args, getenv, stdout, stderr))`, so `run` is testable.

## Errors, logging, concurrency

- Wrap with context: `fmt.Errorf("load item %s: %w", id, err)`. Compare with `errors.Is/As`.
  Sentinel errors are `var ErrX = errors.New("…")`; typed errors end in `Error`.
- Never `panic` for control flow; never ignore an error silently (a deliberate ignore is
  `_ =` with a reason if not obvious). Never log and return the same error.
- `log/slog` only: JSON in production, text locally (`LOG_FORMAT`). Use the request-scoped
  logger (`loggerFrom(ctx)`), `…Context` methods, and key/value attributes in snake_case.
  Never log secrets, full request bodies or personal data.
- No goroutine without an owner and a stop condition; pass `context.Context` as the first
  parameter; protect shared state with `sync` primitives; the suite must pass with `-race`.

## Input edge cases

- Treat all input as hostile: empty, whitespace, huge, deeply nested, unexpected types,
  Unicode, duplicate keys, missing vs zero values.
- When the domain handles numbers (money, quantities, measurements, scores), decide their
  representation with the user: integer, decimal or floating point, precision, rounding,
  range and overflow. Validate before using them; never let NaN or ±Inf reach the encoder.

## Style

- `gofmt`/`goimports` (local module imports grouped last), golangci-lint v2 config in
  `backend/.golangci.yml` with zero findings; `//nolint` only with a specific linter and reason.
- Doc comments on every exported identifier and package. Short, lower-case package names;
  no stutter (`item.Service`, not `item.ItemService`). Early returns over nesting.
- Tests: `.claude/rules/testing.md`.
