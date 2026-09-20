# Engineering guide

Full-stack web application: a **React + TypeScript** single-page app in `frontend/` that
consumes a **Go REST microservice** in `backend/`. Product requirements live in `specs/`.
This file defines *how* we build; the specs define *what*.

## Non-negotiables

1. **Specs are the source of truth.** Build what `specs/<name>.md` asks, nothing more.
   When a requirement is ambiguous, incomplete, contradictory, or silent on something that
   changes behaviour, the API contract, UX, data handling, security, dependencies or
   deployment: **stop and ask the user** with `AskUserQuestion` (concrete options, the
   recommended one first, trade-offs stated). Never invent product behaviour. Record every
   answer in the plan's decision log, and user-visible ones in the README.
2. **Backend = Go standard library plus one approved module.** `net/http` (method +
   wildcard `ServeMux` patterns), `encoding/json`, `log/slog`, `context`, `errors`,
   `testing`, `net/http/httptest`. No web frameworks, routers, ORMs, DI containers,
   assertion or mocking libraries. The only permitted module dependency is
   `github.com/shopspring/decimal` (exact decimal arithmetic; spec `calculator` C-1,
   decision 14), pinned in `go.mod` with `go.sum` committed. Dev tools (golangci-lint,
   govulncheck) are binaries, never module dependencies. Hooks and `depguard` enforce this.
3. **Frontend = React + strict TypeScript on Vite**, styled with CSS Modules and CSS custom
   properties; no UI kit, no CSS framework, no state-management library. Runtime
   dependencies stay at `react` + `react-dom`; any other dependency needs a one-line
   justification in the plan and the user's approval.
4. **Tests ship with the code.** Test-first (red → green → refactor). Every acceptance
   criterion has a test. Layers: unit, component, HTTP handler, black-box API and browser
   end-to-end (`.claude/rules/testing.md`). Coverage ≥ 80% per side, ≥ 90% for domain
   logic. Never skip, delete or weaken a test, and never lower a threshold, to get green.
5. **SOLID with clean boundaries.** Domain logic is pure and framework-free. Transport
   (HTTP handlers, React components) depends on the domain, never the reverse.
   Dependencies are injected through small interfaces owned by the consumer and wired only
   in the composition root. New behaviour extends through registration, not by editing
   switch statements scattered across layers.
6. **Production quality by default:** validated input, explicit error contract
   (RFC 9457 problem details), timeouts, graceful shutdown, structured logs, health
   checks, configuration via environment with safe defaults, security headers, no secrets
   in code, accessible (WCAG 2.2 AA) and responsive UI.
7. **Docs are part of done.** The README stays accurate; every command and API example in
   it has been executed. Significant decisions get an ADR in `docs/adr/`.
8. Write code, comments, docs and commit messages the way a professional product team
   would for a service it runs in production. No TODOs, commented-out code, placeholder
   text or unexplained magic values left behind.

## Repository layout

```
backend/                 Go module (REST microservice)
  cmd/<service>/         main package: flags/env → app → server (thin)
  internal/app/          composition root: builds the http.Handler from config
  internal/config/       typed configuration from environment, validated at startup
  internal/<domain>/     pure business logic, domain types and errors
  internal/httpapi/      routing, handlers, DTOs, validation, problem responses, middleware
  internal/server/       http.Server lifecycle and graceful shutdown
  api/openapi.yaml       API contract (OpenAPI 3.1)
  test/e2e/              black-box API tests against the fully wired handler
frontend/                Vite + React + TypeScript
  src/api/               typed HTTP client, DTOs, error mapping
  src/features/<name>/   feature components, hooks, pure logic, tests
  src/components/        shared presentational components
  src/styles/            design tokens and global styles
  src/test/              test setup, MSW handlers, render helpers
  e2e/                   Playwright specs
specs/                   product specs (<name>.md) and implementation plans (<name>.plan.md)
docs/                    ADRs, coverage report, diagrams
Makefile                 single entry point for every developer task
```

Brownfield code keeps its existing structure unless it violates a non-negotiable (then ask).

## Commands

The root `Makefile` is the source of truth (created by `/implement` on a fresh repo).

| Command | Purpose |
|---|---|
| `make setup` | Install dependencies and the Playwright browser |
| `make dev` | Run backend and frontend together with live reload |
| `make run-backend` / `make run-frontend` | Run one side |
| `make fmt` / `make fmt-check` | Format (gofmt/goimports, Prettier) / check formatting |
| `make lint` | golangci-lint, ESLint |
| `make typecheck` | TypeScript project check |
| `make test` | Go tests with the race detector + Vitest |
| `make test-e2e` | Black-box API tests + Playwright journeys |
| `make coverage` | Coverage with thresholds; HTML in `coverage/`, summary in `docs/coverage.md` |
| `make build` | Production builds |
| `make vuln` | govulncheck + npm audit |
| `make verify` | Everything CI runs except the container smoke test (`make docker-up`) |
| `make docker-up` / `make docker-down` | Full stack in containers |

Before the Makefile exists, use the underlying `go` / `npm` commands from the rules files.

## Workflow

- `/spec <name>` turns notes into `specs/<name>.md`, asking about gaps.
- `/implement <name>` clarifies → plans → scaffolds (empty repo) → builds backend and
  frontend test-first → end-to-end tests → verifies → reviews → documents → commits each
  phase. Progress and decisions live in `specs/<name>.plan.md`, so it can resume.
- `/verify [quick|full]` runs the quality gates and writes the coverage report.
- `/review [scope] [--fix]` runs a multi-agent review and bug hunt; every finding needs
  evidence. Reports go to `.reviews/` (git-ignored).
- `/readme` rewrites the README from the code and runs every example in it.

Subagents live in `.claude/agents/`: `spec-analyst`, `go-backend-engineer`,
`react-frontend-engineer`, `e2e-test-engineer`, `code-reviewer`, `bug-hunter`,
`docs-writer`. Give them complete briefs (requirement IDs, acceptance criteria, contract
excerpts, decisions, files, done criteria). Subagents cannot ask the user anything: they
return open questions, and you ask.

## Git

- `/implement` commits at the end of every completed phase **without asking**, once its
  checks pass: `git add -A && git commit -m "<type>(<scope>): <summary>"` (Conventional
  Commits, imperative, ≤ 72 characters; the body explains why). Keep commits focused.
- Never push, force, amend, rebase, or change git configuration other than
  `core.hooksPath`. If `user.name`/`user.email` are missing, ask the user to set them.
- Formatting happens automatically on commit (`.claude/githooks/pre-commit`, enabled
  through `core.hooksPath`), and `/verify` formats first. Don't hand-format.
- Never commit secrets, `.env` files, `node_modules/`, build output or `coverage/`.

## Hooks (automatic, configured in `.claude/hooks/hooks.conf`)

- **Guardrails** block: module dependencies and web frameworks in the backend,
  destructive shell and git commands, edits to `.env*`, lockfiles, generated output,
  `.git/` and the hook configuration itself. When blocked, read the reason and adapt; if
  the action is really needed, ask the user.
- **Quality gate**: after backend/frontend edits you cannot finish a turn until build,
  vet, type-check, lint and unit tests pass (3 attempts). Need a decision while checks
  fail? Ask with `AskUserQuestion`: waiting for the answer does not end the turn.
- **Protected paths**: Claude Code asks the user before any write under `.claude/`, so
  never write files there (review reports go to `.reviews/`, screenshots to
  `frontend/test-results/`). Start servers as background tasks and stop them with the
  background-task tools, not `kill`.
- **Session snapshot**: toolchain, specs, plan progress and git state are injected at
  session start. Re-run with `bash .claude/hooks/session-context.sh </dev/null`.

## Definition of done (per spec)

- Every requirement is implemented, tested and traceable in the plan's matrix.
- `make verify` is green, coverage thresholds hold, the production build works.
- `/review` leaves no open critical, high or medium findings.
- README covers setup, running each side, configuration, API examples (executed),
  testing and coverage, design decisions and assumptions.
- Work is committed; the final report lists anything not done, explicitly.
