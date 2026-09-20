---
name: verify
description: Run this repository's quality gates and report the results with evidence - formatting, lint, type-check, unit, component and API tests with the race detector, dependency policy, coverage thresholds and report, production builds, end-to-end tests, vulnerability scans and container builds. Use after changes, before committing, or when asked whether the project is healthy.
argument-hint: "[quick | full] [backend | frontend]"
allowed-tools:
  - Read
  - Write
  - Edit
  - Glob
  - Grep
  - Bash(make *)
  - Bash(go *)
  - Bash(gofmt *)
  - Bash(golangci-lint *)
  - Bash(govulncheck *)
  - Bash(npm *)
  - Bash(npx *)
  - Bash(docker compose *)
  - Bash(docker info *)
  - Bash(curl *)
  - Bash(git status *)
  - Bash(git diff *)
  - Bash(bash scripts/coverage-report.sh *)
---

# /verify

Arguments: `$ARGUMENTS` (level `quick` by default; optional scope `backend` or `frontend`)

Run the checks instead of reasoning about them, and report exactly what ran and what it printed.

## Steps

| Step | quick | full | Command | Without a Makefile |
|---|:-:|:-:|---|---|
| Format (writes files) | ✓ | ✓ | `make fmt` | `cd backend && gofmt -w .`; `cd frontend && npm run format` |
| Lint | ✓ | ✓ | `make lint` | `cd backend && golangci-lint run ./...` (else `go vet ./...`); `cd frontend && npm run lint` |
| Type-check | ✓ | ✓ | `make typecheck` | `cd frontend && npm run typecheck` |
| Tests | ✓ | via coverage | `make test` | `cd backend && go test -race -count=1 ./...`; `cd frontend && npm test` |
| Dependency policy | ✓ | ✓ | see below | see below |
| Coverage, thresholds (80% total, 90% per domain package), `docs/coverage.md` | | ✓ | `make coverage` | the recipe in the Makefile |
| Production build | ✓ | ✓ | `make build` | `cd backend && go build ./...`; `cd frontend && npm run build` |
| End-to-end | | ✓ | `make test-e2e` | `cd backend && go test ./test/e2e/...`; `cd frontend && npm run test:e2e` |
| Vulnerabilities | | ✓ | `make vuln` | same commands; report "not run" when offline |
| Containers | | ✓ | see below | — |

- **Dependency policy**: `backend/go.mod` has no `require`, `replace` or `tool` directive,
  and `cd frontend && npm ls --omit=dev --depth=0` lists only `react`, `react-dom` and
  packages the user approved in the plan.
- **Containers** (only when `docker info` succeeds and Dockerfiles exist):
  `docker compose up --build --detach --wait`, then
  `curl -fsS http://localhost:3000/ >/dev/null` and
  `curl -fsS http://localhost:3000/api/v1/does-not-exist` (expect a 404 problem document),
  then `docker compose down`.

Formatting runs first because the repository formats on commit; lint then reports real
problems only.

## Rules

- Keep going after a failing step, so the report is complete, unless the build itself is broken.
- A tool that is missing is reported as **not run**, never as passed. Give the install
  command:
  - golangci-lint: `brew install golangci-lint`, or the official install script.
  - govulncheck: `go install golang.org/x/vuln/cmd/govulncheck@latest`.
  - Playwright browser: `cd frontend && npx playwright install chromium`.
  - Docker: start Docker Desktop.
- If a test looks flaky, re-run only that test, up to twice. Flakiness is still a failure.
- Change nothing except formatting unless the caller asked you to fix failures.
- After `full`, mention that `docs/coverage.md` was regenerated and quote its totals.

## Report

```text
Verify <quick|full> (<scope>): PASS | FAIL

| Step        | Result  | Evidence                                              |
|-------------|---------|-------------------------------------------------------|
| Format      | ✓       | 2 files reformatted                                   |
| Lint        | ✗       | backend/internal/httpapi/x.go:12 errcheck: …          |
| Type-check  | ✓       |                                                       |
| Tests       | ✓       | go: 12 packages ok; vitest: 41 passed                 |
| Deps        | ✓       | go.mod clean; runtime deps: react, react-dom          |
| Coverage    | ✓       | backend 91.2% (min 80); frontend lines 88.4% (min 80) |
| Build       | ✓       |                                                       |
| E2E         | not run | Playwright browser missing: npx playwright install chromium |

Next actions: …
```
