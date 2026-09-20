---
paths:
  - "backend/**"
  - "frontend/**"
  - "specs/*.plan.md"
---

# Testing rules

## Layers

| Layer | Tool | Location | Proves |
|---|---|---|---|
| Domain unit | Go `testing` | `backend/internal/<domain>/*_test.go` | business rules and edge cases, exhaustively |
| HTTP handler | `net/http/httptest` + fakes | `backend/internal/httpapi/*_test.go` | decoding, validation, status codes, problem bodies, headers |
| API black-box | `httptest.NewServer(app.New(...))` | `backend/test/e2e/` | the wired service honours the contract end to end |
| Fuzz | `testing.F` | next to decoders/validators/parsers | no panics, invariants hold on arbitrary input |
| Frontend unit | Vitest | `src/**/*.test.ts` | pure logic: reducers, parsers, formatters, validators |
| Component | Vitest + Testing Library + user-event | `src/**/*.test.tsx` | what the user sees and does, including a11y roles |
| API client | Vitest + MSW | `src/api/*.test.ts` | request shape, error mapping, cancellation, timeouts |
| Browser e2e | Playwright (Chromium desktop + mobile viewport) | `frontend/e2e/` | critical journeys against the real backend |

Each acceptance criterion in the plan maps to at least one test; the plan's traceability
matrix names them.

## Workflow

- Test first: write the failing test, watch it fail for the right reason, write the minimum
  code, refactor with the tests green. Bug fixes start with a test that reproduces the bug.
- Cover the unhappy paths as carefully as the happy path: invalid, missing, extra, huge,
  boundary and hostile input; dependency failures; timeouts; cancellation; concurrency.
- Never delete, skip (`t.Skip`, `it.skip`, `test.fixme`), `.only`, or weaken an assertion
  to get green; never lower a coverage threshold. If a test is wrong, fix it and say why.

## Go

- Table-driven tests with `t.Run(tc.name, …)`; names describe behaviour
  (`"rejects unknown fields"`). `t.Parallel()` wherever state isn't shared.
- Standard library only: compare with `==`, `errors.Is`, `slices.Equal`, `maps.Equal`,
  `reflect.DeepEqual` (last resort); print `got`/`want` in failures:
  `t.Errorf("Parse(%q) = %v, want %v", in, got, want)`.
- Hand-written fakes that implement the consumer's small interface; no mocking frameworks.
  Inject clocks, ID generators and randomness.
- `t.Helper()` in helpers, `t.Context()` for contexts, `t.TempDir()` for files,
  `t.Setenv` only in non-parallel tests (prefer injected `getenv`).
- Long-running tests honour `testing.Short()` (the quality gate runs `-short`).
- Everything passes with `go test -race -count=1 ./...`.
- Fuzz targets (`FuzzXxx`) for every parser/decoder/validator, seeded with the table cases;
  run briefly (`go test -run=^$ -fuzz=FuzzXxx -fuzztime=30s ./internal/...`) when they change.
- Benchmarks (`BenchmarkXxx` with `b.Loop()`) only for paths with a performance requirement.

## Frontend

- Import test APIs explicitly from `vitest`. The setup file registers
  `@testing-library/jest-dom/vitest`, starts the MSW server with
  `onUnhandledRequest: 'error'`, and calls `cleanup()`, `server.resetHandlers()` and
  `vi.restoreAllMocks()` after each test.
- Query like a user: `getByRole` (with `name`), `getByLabelText`, `getByText`; `getByTestId`
  only as a last resort. Interact with `userEvent.setup()`; await `findBy…` for async UI.
- Test behaviour, not implementation: no assertions on internal state, hook call counts or
  class names; no snapshot tests for logic.
- Network is mocked at the HTTP boundary with MSW handlers (`src/test/msw/`), including
  error, slow and malformed responses. Components get the API client via the provider.
- Accessibility smoke checks in component tests: labelled controls, `role="alert"` for
  errors, live region for results, keyboard-only flows.

## Browser end-to-end (Playwright)

- `playwright.config.ts` starts the real backend and frontend (`webServer` array) on
  dedicated ports; tests never depend on each other or on order.
- Locators by role/label; web-first assertions (`await expect(locator).toHaveText(…)`); no
  `waitForTimeout`. Use `page.route` only to simulate failures the real backend can't
  produce on demand (network down, 5xx, slow responses).
- Cover: main happy path(s), validation errors, server/network errors and recovery,
  keyboard-only use, and the mobile project (e.g. Pixel 7 viewport).
- Traces and screenshots are kept on failure; the HTML report is not committed.

## Coverage

- Thresholds: backend statements ≥ 80% overall and ≥ 90% for each domain package under
  `internal/` (`make coverage` and CI fail below either; `app`, `config`, `httpapi`,
  `server` and `storage` count toward the total only); frontend lines/statements/functions/branches ≥ 80%, enforced in
  `vitest.config.ts` (with `coverage.include` so untested files count).
- Backend coverage uses `-coverpkg=./...` so black-box tests count. Generated code, `main.tsx`,
  type-only files and test utilities are excluded, nothing else.
- `make coverage` writes HTML to `coverage/` (git-ignored) and the summary to
  `docs/coverage.md` (committed). Quote real numbers from it, never estimates.

## Determinism

- No real network (other than localhost in e2e), no wall-clock sleeps, no dependence on
  test order, time zone or locale; seed randomness; tests pass three times in a row.
