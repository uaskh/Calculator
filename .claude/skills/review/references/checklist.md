# Review checklist

Use the sections that match the scope. Every finding needs a location and evidence.

## Correctness and edge cases (all code)

- Does the behaviour match the spec and plan (decisions `D-n`) exactly, including error cases?
- Boundaries: empty, zero, one, maximum, just above/below limits, negative; for numbers
  also very large or very small values, precision and rounding, overflow, non-finite
  values and `-0`.
- Text: whitespace, Unicode (multi-byte, combining characters, emoji), very long strings,
  control characters, case sensitivity, locale-specific formats.
- Input shape: missing vs `null` vs zero values, unknown fields, wrong types, duplicate keys,
  arrays instead of objects, nested depth, oversized bodies, wrong content type.
- State: double submission, retries, out-of-order responses, stale state after errors,
  cancellation, unmount during requests.

## Go backend

- Only the standard library is imported; `go.mod` has no requirements.
- Layering: domain packages don't import `net/http` or know JSON; handlers contain no
  business rules; dependencies are injected via small consumer-owned interfaces; wiring
  happens only in `internal/app`.
- Errors: wrapped with context (`%w`), compared with `errors.Is/As`, mapped to HTTP in one
  place, never swallowed, never logged and returned; 5xx responses leak nothing.
- HTTP semantics: correct methods and status codes (400 vs 422, 404, 405 with `Allow`,
  413, 415), `Content-Type` on every response, `Location` for created resources,
  `Cache-Control` for dynamic results, problem details with stable codes.
- Decoding: body limit, unknown fields, trailing data, type errors mapped to fields;
  validation reports all field errors.
- Encoding: values that `encoding/json` rejects (NaN, ±Inf, unsupported types) cannot reach
  `writeJSON`; numeric formatting matches the contract.
- Concurrency: shared state guarded; no goroutine leaks; contexts propagated and honoured;
  tests pass with `-race`.
- Server: timeouts set, graceful shutdown, health endpoints, configuration validated at
  startup, no secrets in code or logs.
- Security: CORS allow-list is explicit, security headers set, input sizes bounded, no
  reflection of raw input into logs without structure, errors don't expose internals.
- Design: single responsibility per type/function, extension by registration rather than
  editing switches, no premature abstraction, names describe intent, exported identifiers
  documented.

## React / TypeScript frontend

- Types: no `any`, unchecked casts, `!` assertions or `@ts-ignore`; API data parsed from
  `unknown`; discriminated unions for state.
- Data flow: all HTTP via `src/api`; client injected through context; requests cancelled
  on unmount and when superseded; stale responses ignored; errors mapped by `kind`/`code`.
- Hooks: dependency arrays complete and stable; no state updates after unmount; no effects
  used for derived state; refs not read during render.
- Forms: labels, `aria-invalid`, `aria-describedby`, errors in `role="alert"`, results in a
  live region; client validation mirrors server rules; submit disabled while pending; input
  kept on error; number parsing pitfalls (`Number('')`, whitespace, locale) handled.
- Accessibility: semantic elements and landmarks, heading order, keyboard operability,
  visible focus, contrast (text ≥ 4.5:1), reduced motion, no colour-only meaning.
- Responsive: no horizontal scroll at 320 px, touch targets ≥ 44 px, layout at 375 and
  1280 px checked (screenshots when possible).
- Styling: tokens instead of raw values, CSS Modules, no inline styles or `!important`.
- Robustness: error boundary present; no `dangerouslySetInnerHTML`; configuration via
  `src/config.ts`; no secrets; no console noise in production paths.
- Dependencies: runtime deps limited to `react`, `react-dom` and approved packages.

## API contract consistency

- `backend/api/openapi.yaml`, handlers, frontend types and parsers, e2e tests and README
  examples all agree on paths, fields, types, status codes and error codes.
- Every documented error response can actually be produced, and every produced one is
  documented.

## Tests

- Each acceptance criterion has a test that would fail if the behaviour broke.
- Tests assert behaviour (outputs, rendered text, roles), not implementation details.
- Error paths, edge cases and cancellation are covered; fuzz targets exist for parsers.
- No sleeps, order dependence, real network (beyond localhost e2e), or shared mutable
  state between parallel tests; no skipped or focused tests.
- Coverage thresholds hold and critical code is not in the uncovered list.
- Playwright uses role/label locators and web-first assertions; covers mobile and failures.

## Delivery and docs

- `make verify` mirrors CI; Makefile works with GNU Make 3.81.
- Dockerfiles: multi-stage, pinned bases matching `go.mod`/`.nvmrc`, non-root, health
  checks, small contexts; compose waits for health; nginx proxies `/api/`, sets security
  headers at server level, serves the SPA fallback.
- CI: least-privilege permissions, current action versions, same commands as the Makefile.
- README: every section required by `.claude/rules/documentation.md`, commands that work,
  examples that match real responses, decisions and assumptions stated.
