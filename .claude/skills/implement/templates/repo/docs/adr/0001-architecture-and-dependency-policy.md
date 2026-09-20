# 0001. Architecture and dependency policy

- Status: Accepted
- Date: __DATE__

## Context

The product consists of a browser application and an HTTP API. Both must be easy to
review, test and operate, with a small supply-chain surface.

## Decision

- **Backend**: a Go service built only on the standard library (`net/http` routing with
  method patterns, `encoding/json`, `log/slog`). Layers: domain packages with the business
  rules; an HTTP adapter (`internal/httpapi`) that decodes, validates and maps errors to
  RFC 9457 problem details; a composition root (`internal/app`) that wires dependencies
  through small consumer-owned interfaces; `cmd/api` as a thin entry point.
- **Frontend**: React with strict TypeScript on Vite, CSS Modules and design tokens; the
  only runtime dependencies are `react` and `react-dom`. All HTTP goes through a typed
  client injected with React context.
- **Contract**: JSON over `/api/v1`, described in `backend/api/openapi.yaml`.
- **Quality**: test-first development; unit, component, API black-box and browser
  end-to-end tests; coverage thresholds enforced in the build.

## Consequences

- No framework conventions to learn and no transitive dependencies to audit on the
  backend; routing, middleware and validation are explicit code with their own tests.
- Some helpers (JSON decoding, problem responses, middleware) are written in-house and
  must be maintained.
- Adding a dependency requires a new decision record.
