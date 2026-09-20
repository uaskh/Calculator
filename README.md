# Calculator

A web calculator: a React + TypeScript single-page app (`frontend/`) that sends arithmetic
expressions to a Go REST microservice (`backend/`), which lexes, normalizes, parses and
evaluates them with exact decimal arithmetic. The result appears while you type. The API
has a strict OpenAPI contract with RFC 9457 problem details, every layer is tested (unit,
handler, fuzz, black-box API, component, browser end-to-end), and the stack runs with one
command in containers.

## Features

- Operators `+ - * /`, exponentiation `^` (right-associative, binds tighter than unary
  minus: `-2^2` = `-4`), unary minus, parentheses and `sqrt(x)`.
- Calculator-style postfix percent: `x%` is `x/100`, except as the direct right operand of
  `+` or `-`, where `200+10%` = `220`.
- Exact decimal arithmetic: `0.1+0.2` = `0.3`; results are returned as JSON strings, so no
  precision is lost in transit.
- Live result 150 ms after typing stops; superseded requests are cancelled; "Calculating…"
  appears after 300 ms without a response.
- Enter or `=` commits: the result replaces the input (negatives as `(-5)`) and the
  calculation joins an in-memory history of the last 20 entries. `⌫` deletes one character;
  `C` or Escape clears.
- Keypad with 44 px keys; on touch devices the keypad is the input method and the soft
  keyboard stays hidden. From 48 rem wide the calculator is one centred panel with the
  display across the top, the keypad on the left and History on the right, and keys and
  type that scale with the window; narrower screens keep a single column.
- The input is focused when the page opens, and printable keys, Backspace and Escape reach
  it from anywhere on the page, so you can type without clicking first.
- Lenient normalization: trailing operators and a trailing `.` are dropped, unmatched `(`
  are closed, and the evaluated expression is echoed ("Evaluated as 2*(3+4)").
- Every error is machine-readable: RFC 9457 problem details with a stable `code` and, for
  validation errors, a 0-based character position.
- Light and dark themes, WCAG 2.2 AA (checked with axe), responsive from 320 px.

## Architecture

```mermaid
flowchart LR
  user([Browser]) --> web["Web app<br/>React + TypeScript (Vite)"]
  web -- "POST /api/v1/evaluate" --> mw["httpapi middleware<br/>request ID · access log · recover · headers · CORS · timeout"]
  mw --> handler["evaluate handler<br/>decode · validate · map errors"]
  handler --> calc["calc (domain)<br/>lex → normalize → parse → eval"]
  calc --> decimal["shopspring/decimal"]
```

The web app calls `/api/v1` on its own origin: in development Vite proxies `/api` to the Go
service on port 8080; in containers nginx serves the static build and proxies `/api/` to
`backend:8080`. In the backend, `cmd/api` reads the environment into a typed config,
`internal/app` (the composition root) wires the domain `calc.Calculator` into the
`internal/httpapi` handler, and `internal/server` runs the `http.Server` with timeouts and
graceful shutdown. The handler decodes and validates the body, calls the domain, and maps
domain errors to problem documents: 400 `VALIDATION_FAILED` with one `errors[]` entry for
expressions that cannot be parsed, 422 with a domain code for expressions that have no
value within the limits. Every response carries `X-Request-ID`, `Cache-Control: no-store`
and the security headers; every request produces one structured `log/slog` line.

### Project structure

```text
backend/                     Go module github.com/aksh/calculator/backend
  cmd/api/                   main: flags, environment, signals, -healthcheck
  internal/app/              composition root: config → http.Handler
  internal/calc/             pure domain: lexer, normalizer, parser, AST, evaluator, tables
  internal/config/           typed configuration from the environment, validated at startup
  internal/httpapi/          router, evaluate handler, DTOs, problem responses, middleware
  internal/server/           http.Server lifecycle and graceful shutdown
  test/e2e/                  black-box API tests over real HTTP
  api/openapi.yaml           API contract (OpenAPI 3.1)
  Dockerfile, .golangci.yml
frontend/                    Vite + React + TypeScript
  src/api/                   typed HTTP client, DTOs, response validation, error mapping
  src/app/                   App shell and error boundary
  src/features/calculator/   reducer (model.ts), hook (useCalculator.ts), components
  src/lib/                   environment parsing, colour-contrast maths
  src/styles/                design tokens and global styles
  src/test/                  Vitest setup, MSW handlers, render helpers
  e2e/                       Playwright specs (desktop and mobile)
  nginx.conf, Dockerfile, playwright.config.ts, vitest.config.ts, vite.config.ts
scripts/                     dev.sh (make dev), coverage-report.sh (docs/coverage.md)
docs/                        adr/ (decision records), coverage.md, prompts.md
specs/                       product spec and implementation plan
compose.yaml                 two-container stack (backend + web)
Makefile                     every developer task (make help)
.github/workflows/ci.yml     CI: hygiene, backend, frontend, e2e, docker
```

## Tech stack

| Area     | Choice                                                                                                            | Why                                                                                                                   |
| -------- | ----------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| Backend  | Go 1.27, standard library (`net/http`, `encoding/json`, `log/slog`) plus `shopspring/decimal` 1.4.0               | Explicit routing, middleware and validation with nothing to audit but one module; exact decimals instead of `float64` |
| Frontend | React 19.2, TypeScript 6.0, Vite 8.3, CSS Modules with custom properties                                          | Strict types, fast builds, no UI kit or state library to learn; runtime dependencies are `react` and `react-dom` only |
| Testing  | Go `testing` (unit, fuzz, benchmark, `httptest`), Vitest 5.0, Testing Library, MSW 2.15, Playwright 1.63 with axe | One tool per layer; the network is faked at the HTTP boundary and journeys run against the real backend               |
| Delivery | Make, Docker (distroless static image, unprivileged nginx), Compose v2, GitHub Actions                            | One entry point for every task; production-shaped containers; CI mirrors `make verify`                                |

## Getting started

### Prerequisites

| Tool          | Version                    | Needed for                 |
| ------------- | -------------------------- | -------------------------- |
| Go            | ≥ 1.27 (`backend/go.mod`)  | backend                    |
| Node.js / npm | ≥ 26.8 / 11 (see `.nvmrc`) | frontend                   |
| Docker        | recent, with Compose v2    | containers (optional)      |
| golangci-lint | v2.13 (optional)           | `make lint`, `make verify` |

### Quick start with Docker

```bash
make docker-up        # web on http://localhost:3000, API on http://localhost:8080
make docker-down
```

`make docker-up` builds both images, starts the API (published on 8080) and the web
container (nginx on 3000), and waits for the health checks. Open
<http://localhost:3000>.

### Local development

```bash
make setup            # go mod verify, npm ci, Playwright Chromium
make dev              # API on :8080 and web app on :5173 (proxies /api); Ctrl+C stops both
```

Open <http://localhost:5173>. `make dev` runs the backend with `LOG_FORMAT=text` so the
access log is readable in a terminal. `make help` lists every target.

### Running the backend and frontend separately

```bash
make run-backend      # or: cd backend && go run ./cmd/api
make run-frontend     # or: cd frontend && npm run dev
```

The backend listens on `:8080`; the Vite dev server listens on `:5173` and proxies `/api`
to `API_PROXY_TARGET` (default `http://localhost:8080`). To point the dev server at
another backend: `API_PROXY_TARGET=http://localhost:9090 make run-frontend`.

## Configuration

### Backend

All variables are optional. Invalid values are reported together and stop the process at
startup.

| Variable                   | Default | Description                                                                                                |
| -------------------------- | ------- | ---------------------------------------------------------------------------------------------------------- |
| `HTTP_ADDR`                | `:8080` | Listen address                                                                                             |
| `HTTP_READ_HEADER_TIMEOUT` | `5s`    | `http.Server` ReadHeaderTimeout                                                                            |
| `HTTP_READ_TIMEOUT`        | `10s`   | `http.Server` ReadTimeout                                                                                  |
| `HTTP_WRITE_TIMEOUT`       | `15s`   | `http.Server` WriteTimeout; must be longer than `HTTP_REQUEST_TIMEOUT`                                     |
| `HTTP_IDLE_TIMEOUT`        | `60s`   | Keep-alive idle timeout                                                                                    |
| `HTTP_SHUTDOWN_TIMEOUT`    | `10s`   | Grace period for in-flight requests on SIGINT/SIGTERM                                                      |
| `HTTP_REQUEST_TIMEOUT`     | `5s`    | Handler deadline; when exceeded the response is 503 `TIMEOUT`                                              |
| `HTTP_MAX_BODY_BYTES`      | `4096`  | Request body limit; larger bodies get 413 `PAYLOAD_TOO_LARGE`                                              |
| `LOG_LEVEL`                | `info`  | `debug`, `info`, `warn` or `error`                                                                         |
| `LOG_FORMAT`               | `json`  | `json` or `text` (`make dev` and `make run-backend` use `text`)                                            |
| `CORS_ALLOWED_ORIGINS`     | empty   | Comma-separated exact origins (`https://app.example.com`). Empty disables CORS; `*` is rejected at startup |

### Frontend

`VITE_*` variables are read at build time (see [`frontend/.env.example`](frontend/.env.example)).

| Variable              | Default                 | Description                                                                             |
| --------------------- | ----------------------- | --------------------------------------------------------------------------------------- |
| `VITE_APP_NAME`       | `Calculator`            | Application title                                                                       |
| `VITE_API_BASE_URL`   | empty                   | Origin of the API. Empty means same origin; the code appends `/api/v1` either way       |
| `VITE_API_TIMEOUT_MS` | `10000`                 | Client-side fetch timeout; after it the UI shows the network message                    |
| `API_PROXY_TARGET`    | `http://localhost:8080` | Shell variable read by `vite.config.ts`: where the dev and preview servers proxy `/api` |

## Expression language

```text
expression := term { ("+" | "-") term }
term       := unary { ("*" | "/") unary }
unary      := "-" unary | power
power      := postfix [ "^" unary ]          right-associative; -2^2 = -(2^2)
postfix    := primary { "%" }
primary    := number | "(" expression ")" | "sqrt" "(" expression ")"
number     := digits [ "." digits ] | "." digits
```

Precedence, high to low:

| Level | Operators     | Notes                                    |
| ----- | ------------- | ---------------------------------------- |
| 1     | `%` (postfix) | `2%^2` = `0.0004`, `2^2%` = `2^0.02`     |
| 2     | `^`           | Right-associative: `2^3^2` = `512`       |
| 3     | unary `-`     | Below `^`: `-2^2` = `-4`, `(-2)^2` = `4` |
| 4     | `*` `/`       | Left-associative: `8/2/2` = `2`          |
| 5     | `+` `-`       | Left-associative: `10-4-3` = `3`         |

Rules:

- Only ASCII operators are accepted; `×`, `÷` and `−` are `INVALID_CHARACTER` (the keypad
  inserts `*`, `/`, `-`). Whitespace between tokens (space, tab) is ignored. `sqrt` is
  case-sensitive; any other identifier is `UNKNOWN_FUNCTION`. Implicit multiplication
  (`2(3)`) and unary plus (`+3`) are `UNEXPECTED_TOKEN`. Leading zeros are allowed
  (`007` = `7`).
- Percent: `x%` is `x/100` (`50%` = `0.5`, `50*10%` = `5`), except when the `%` node is the
  direct right operand of `+` or `-`: then it is a percentage of the left operand
  (`200+10%` = `220`, `200-10%` = `180`). Parentheses, an intervening operator or a unary
  minus break directness: `200+(10%)` = `200.1`, `200+10%*2` = `200.2`, `200+-10%` =
  `199.9`. Repeated `%` applies the rule to the outermost node only: `50%%` = `0.005`,
  `100+50%%` = `100.5`.
- Normalization (before parsing): leading and trailing whitespace is trimmed; a trailing
  binary operator, a trailing `.` or a trailing `(` / `sqrt(` is dropped, repeatedly;
  every unmatched `(` is closed. The evaluated text is returned as `expression`. Examples:
  `3+4*` → `3+4` = `7`; `2*(3+4` → `2*(3+4)` = `14`; `2.` → `2`; `sqrt(` and `(` → 400
  `EMPTY`. A `.` is only repairable at the very end: `2.+3` is `INVALID_NUMBER`.
- Precision: literals and every intermediate result are rounded half away from zero to 32
  decimal places; the final result to 16 places. Division, `sqrt` and fractional powers
  carry 72 guard places, so `1/3*3` = `1` and `sqrt(2)*sqrt(2)` = `2`. Magnitudes below
  `5·10^-33` become `0` (`0.5^1000` = `0`). Because exponents are evaluated values,
  `(-2)^(1/3*3)` is `INVALID_POWER`: the exponent is `0.99…9`, not `1`.
- Validation stops at the first failure in this order: `TOO_LONG` → lexing
  (`INVALID_CHARACTER`, `INVALID_NUMBER`, `UNKNOWN_FUNCTION`) → normalization → `EMPTY` →
  `TOO_DEEP` → parsing (`UNEXPECTED_TOKEN`, `UNBALANCED_PARENTHESIS`). `errors[]` always
  has exactly one entry. `position` is 0-based; the message counts from 1 (`2+()` →
  position `3`, "unexpected ')' at character 4").

Limits (constants in the code):

| Limit                                     | Value                | Error                    |
| ----------------------------------------- | -------------------- | ------------------------ |
| Expression length                         | ≤ 1,024 code points  | 400 `TOO_LONG`           |
| Nesting depth (`(` and `sqrt(`)           | ≤ 32                 | 400 `TOO_DEEP`           |
| Exponent                                  | `\|n\|` ≤ 1000       | 422 `EXPONENT_TOO_LARGE` |
| Literal, intermediate and final magnitude | `\|value\|` < 10^100 | 422 `RESULT_TOO_LARGE`   |
| Request body                              | ≤ 4 KiB              | 413 `PAYLOAD_TOO_LARGE`  |
| Handler time                              | 5 s                  | 503 `TIMEOUT`            |

The longest possible result is 118 characters; the normalized expression can reach 1,056
characters (closing parentheses added to a 1,024-character input).

## Keyboard and accessibility

- The expression input has focus when the page opens. Type the expression directly; Enter
  commits, Escape clears. When focus is on the page background or on a keypad or history
  button, printable characters, Backspace and Escape are routed to the input (appended at
  the end) and Enter on the background commits; Enter and Space on a button activate that
  button as usual. Tab order is unchanged: Tab reaches every key, and focus returns to the
  input after keypad taps, `=`, `C` and history activation. Keys with Ctrl, Alt or Cmd are
  left to the browser. There are no other shortcuts.
- Every keypad key is at least 44 × 44 px. Non-digit keys have accessible names ("divide",
  "multiply", "backspace", …); the `sqrt` key is named "sqrt, square root" so the visible
  label is part of the name (WCAG 2.5.3).
- The live result is an `<output aria-live="polite">`; while typing, errors are polite
  status text. On commit, errors are announced with `role="alert"` and linked to the input
  with `aria-describedby`; the input is marked `aria-invalid` only for 400 and 422 responses.
- Light and dark themes follow `prefers-color-scheme` with ≥ 4.5:1 text contrast in both
  (checked by a unit test over the design tokens and by axe in the browser suite).
  Transitions are disabled under `prefers-reduced-motion`.
- Long inputs and results scroll horizontally inside their fields; the page never scrolls
  sideways at 320 px, 412 px, 1280 px or 1920 px.

## API

Base URL: `http://localhost:8080/api/v1` (through the web app: `/api/v1`). The full
contract is in [`backend/api/openapi.yaml`](backend/api/openapi.yaml). Errors use
[RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) problem details
(`Content-Type: application/problem+json`):

```json
{
  "type": "about:blank",
  "title": "Bad Request",
  "status": 400,
  "detail": "unexpected ')' at character 4",
  "instance": "/api/v1/evaluate",
  "code": "VALIDATION_FAILED",
  "requestId": "090245b6808236d510cbf63c73f653d8",
  "errors": [
    {
      "field": "expression",
      "code": "UNEXPECTED_TOKEN",
      "position": 3,
      "message": "unexpected ')' at character 4"
    }
  ]
}
```

| Status | Code                     | When                                                                                                                                                                                                              |
| ------ | ------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 200    | —                        | Evaluated; body `{ "expression", "result" }`                                                                                                                                                                      |
| 400    | `MALFORMED_REQUEST`      | Invalid JSON, wrong type, unknown field, several JSON values, empty body; `errors[]` may carry `INVALID_TYPE` or `UNKNOWN_FIELD`                                                                                  |
| 400    | `VALIDATION_FAILED`      | Expression missing or unparsable; `errors[0].code` is one of `REQUIRED`, `EMPTY`, `TOO_LONG`, `TOO_DEEP`, `INVALID_CHARACTER`, `INVALID_NUMBER`, `UNEXPECTED_TOKEN`, `UNBALANCED_PARENTHESIS`, `UNKNOWN_FUNCTION` |
| 404    | `NOT_FOUND`              | Unknown path                                                                                                                                                                                                      |
| 405    | `METHOD_NOT_ALLOWED`     | Wrong method; `Allow` header lists the accepted ones                                                                                                                                                              |
| 413    | `PAYLOAD_TOO_LARGE`      | Body larger than 4 KiB                                                                                                                                                                                            |
| 415    | `UNSUPPORTED_MEDIA_TYPE` | `Content-Type` is not `application/json`                                                                                                                                                                          |
| 422    | `DIVISION_BY_ZERO`       | "Cannot divide by zero." (also `0^-1` and division by a value that rounds to 0)                                                                                                                                   |
| 422    | `NEGATIVE_SQUARE_ROOT`   | "Cannot take the square root of a negative number."                                                                                                                                                               |
| 422    | `INVALID_POWER`          | "Cannot raise a negative number to a fractional power."                                                                                                                                                           |
| 422    | `EXPONENT_TOO_LARGE`     | "Exponent must be between -1000 and 1000."                                                                                                                                                                        |
| 422    | `RESULT_TOO_LARGE`       | "Result is too large to calculate." (any literal, intermediate or final value ≥ 10^100)                                                                                                                           |
| 500    | `INTERNAL_ERROR`         | Unexpected failure; details are logged with the request ID                                                                                                                                                        |
| 503    | `TIMEOUT`                | Handler exceeded `HTTP_REQUEST_TIMEOUT`                                                                                                                                                                           |
| 503    | `NOT_READY`              | `GET /readyz` once shutdown has started                                                                                                                                                                           |

Every response carries `X-Request-ID` (a client value matching `^[A-Za-z0-9._-]{1,64}$` is
echoed; otherwise 32 hex characters are generated), `Cache-Control: no-store` and the
security headers shown in the first example below.

### POST /api/v1/evaluate

Evaluates one expression. The body must be `application/json` (a `charset` parameter is
accepted); unknown fields are rejected.

Request:

| Field        | Type   | Rules                                                |
| ------------ | ------ | ---------------------------------------------------- |
| `expression` | string | Required; ≤ 1,024 code points; see the grammar above |

Response (200):

| Field        | Type   | Meaning                                                                                                             |
| ------------ | ------ | ------------------------------------------------------------------------------------------------------------------- |
| `expression` | string | The normalized expression that was evaluated                                                                        |
| `result`     | string | Canonical decimal: optional `-`, digits, optional fraction without trailing zeros, no exponent notation, never `-0` |

Success, with normalization of an unclosed parenthesis:

```bash
curl -s -i -X POST http://localhost:8080/api/v1/evaluate \
  -H 'Content-Type: application/json' \
  -d '{"expression":"2*(3+4"}'
```

```http
HTTP/1.1 200 OK
Cache-Control: no-store
Content-Security-Policy: default-src 'none'; frame-ancestors 'none'
Content-Type: application/json; charset=utf-8
Cross-Origin-Opener-Policy: same-origin
Permissions-Policy: camera=(), microphone=(), geolocation=()
Referrer-Policy: no-referrer
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
X-Request-Id: ec3b943519a86b591b3249518b5e03c1
Content-Length: 39

{"expression":"2*(3+4)","result":"14"}
```

More successful evaluations (`curl -s -X POST … -d '{"expression":"…"}'`, body only):

| Expression    | Response                                                                                                                     |
| ------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `200+10%`     | `{"expression":"200+10%","result":"220"}`                                                                                    |
| `2^0.5`       | `{"expression":"2^0.5","result":"1.414213562373095"}`                                                                        |
| `0.1+0.2`     | `{"expression":"0.1+0.2","result":"0.3"}`                                                                                    |
| `2^100`       | `{"expression":"2^100","result":"1267650600228229401496703205376"}`                                                          |
| `-2^2`        | `{"expression":"-2^2","result":"-4"}`                                                                                        |
| `3+4*`        | `{"expression":"3+4","result":"7"}`                                                                                          |
| `2.`          | `{"expression":"2","result":"2"}`                                                                                            |
| `(10^90)^0.9` | `{"expression":"(10^90)^0.9","result":"1000000000000000000000000000000000000000000000000000000000000000000000000000000000"}` |

Validation error (400) with a position:

```bash
curl -s -i -X POST http://localhost:8080/api/v1/evaluate \
  -H 'Content-Type: application/json' \
  -d '{"expression":"2+()"}'
```

```http
HTTP/1.1 400 Bad Request
Content-Type: application/problem+json

{"type":"about:blank","title":"Bad Request","status":400,"detail":"unexpected ')' at character 4","instance":"/api/v1/evaluate","code":"VALIDATION_FAILED","requestId":"090245b6808236d510cbf63c73f653d8","errors":[{"field":"expression","code":"UNEXPECTED_TOKEN","position":3,"message":"unexpected ')' at character 4"}]}
```

Empty expression (400, no `position`):

```bash
curl -s -X POST http://localhost:8080/api/v1/evaluate \
  -H 'Content-Type: application/json' \
  -d '{"expression":""}'
```

```json
{
  "type": "about:blank",
  "title": "Bad Request",
  "status": 400,
  "detail": "expression is empty",
  "instance": "/api/v1/evaluate",
  "code": "VALIDATION_FAILED",
  "requestId": "53fcee38…",
  "errors": [
    { "field": "expression", "code": "EMPTY", "message": "expression is empty" }
  ]
}
```

Arithmetic error (422):

```bash
curl -s -i -X POST http://localhost:8080/api/v1/evaluate \
  -H 'Content-Type: application/json' \
  -d '{"expression":"1/0"}'
```

```http
HTTP/1.1 422 Unprocessable Entity
Content-Type: application/problem+json

{"type":"about:blank","title":"Unprocessable Entity","status":422,"detail":"Cannot divide by zero.","instance":"/api/v1/evaluate","code":"DIVISION_BY_ZERO","requestId":"150b103e2cb6f8a24a345ef40ea7561d"}
```

Other 422s return the same shape: `10^100` → `RESULT_TOO_LARGE` ("Result is too large to
calculate."); `(-2)^(1/3*3)` → `INVALID_POWER` ("Cannot raise a negative number to a
fractional power.").

Malformed body (400 `MALFORMED_REQUEST`), here with a number instead of a string:

```bash
curl -s -X POST http://localhost:8080/api/v1/evaluate \
  -H 'Content-Type: application/json' \
  -d '{"expression":2}'
```

```json
{
  "type": "about:blank",
  "title": "Bad Request",
  "status": 400,
  "detail": "Request body contains a value of the wrong type.",
  "instance": "/api/v1/evaluate",
  "code": "MALFORMED_REQUEST",
  "requestId": "488a7429…",
  "errors": [
    {
      "field": "expression",
      "code": "INVALID_TYPE",
      "message": "must be a string"
    }
  ]
}
```

A body without the field (`-d '{}'`) is 400 `VALIDATION_FAILED` with `detail`
"expression is required" and `errors[0].code` `REQUIRED`.

Wrong media type (415):

```bash
curl -s -i -X POST http://localhost:8080/api/v1/evaluate \
  -H 'Content-Type: text/plain' \
  -d '2+2'
```

```http
HTTP/1.1 415 Unsupported Media Type
Content-Type: application/problem+json

{"type":"about:blank","title":"Unsupported Media Type","status":415,"detail":"Content-Type must be application/json.","instance":"/api/v1/evaluate","code":"UNSUPPORTED_MEDIA_TYPE","requestId":"06753495…"}
```

Wrong method (405, with `Allow`):

```bash
curl -s -i http://localhost:8080/api/v1/evaluate
```

```http
HTTP/1.1 405 Method Not Allowed
Allow: POST
Content-Type: application/problem+json

{"type":"about:blank","title":"Method Not Allowed","status":405,"detail":"Method GET is not allowed for this resource.","instance":"/api/v1/evaluate","code":"METHOD_NOT_ALLOWED","requestId":"99ead9ed…"}
```

Unknown path (404):

```bash
curl -s http://localhost:8080/api/v1/nope
```

```json
{
  "type": "about:blank",
  "title": "Not Found",
  "status": 404,
  "detail": "No resource matches this path.",
  "instance": "/api/v1/nope",
  "code": "NOT_FOUND",
  "requestId": "a2e550f8…"
}
```

Request IDs: a well-formed client value is echoed, so logs can be correlated across
services.

```bash
curl -s -D - -o /dev/null -X POST http://localhost:8080/api/v1/evaluate \
  -H 'Content-Type: application/json' \
  -H 'X-Request-ID: docs-example-1' \
  -d '{"expression":"1+1"}' | grep -i request-id
```

```text
X-Request-Id: docs-example-1
```

### Health

`GET /healthz` is liveness; `GET /readyz` is readiness and answers 503 `NOT_READY` once
shutdown has started (there are no external dependencies to check). Both carry the same
headers as every other response.

```bash
curl -s http://localhost:8080/healthz
curl -s http://localhost:8080/readyz
```

```json
{ "status": "ok" }
```

### Observability

Each request produces one `log/slog` line with `request_id`, `method`, `path`, `status`,
`duration_ms`, `bytes`, `remote_addr` and `user_agent`; 5xx responses, recovered panics
and client cancellations add `error`. A panic before the response is written becomes a
500 `INTERNAL_ERROR`. The process shuts down gracefully on SIGINT or SIGTERM: `/readyz`
flips to 503, in-flight requests get `HTTP_SHUTDOWN_TIMEOUT` to finish.

## Testing

| Layer                                | Tooling                                                               | Command                                           |
| ------------------------------------ | --------------------------------------------------------------------- | ------------------------------------------------- |
| Backend unit, fuzz and handler tests | Go `testing`, `httptest`, race detector                               | `make test-backend`                               |
| API black-box tests                  | Go, real HTTP against the wired handler                               | `cd backend && go test ./test/e2e/...`            |
| Frontend unit and component tests    | Vitest, Testing Library, MSW                                          | `make test-frontend`                              |
| Browser end-to-end                   | Playwright (`desktop-chromium`, `mobile-chromium` = Pixel 7) with axe | `make test-e2e` (starts both apps on 18080/14173) |
| Unit, component and API tests        |                                                                       | `make test`                                       |
| Everything CI runs                   |                                                                       | `make verify`                                     |

What the suites contain (at commit `b74872e`):

- Go: 940 test cases including subtests, across domain tables, fuzz targets
  (`FuzzEvaluate`, `FuzzDecodeJSON`, `FuzzValidRequestID`), handler tests and black-box
  API tests.
- Vitest: 273 tests in 14 files (reducer, hook with fake timers, API client with MSW,
  components, contrast maths).
- Playwright: 90 tests in 6 spec files, run on both projects: journeys, errors and
  recovery via `page.route`, keyboard-only use, axe WCAG 2.2 AA in light and dark themes,
  44 px targets and no horizontal overflow at 320, 412 and 1280 px.

Coverage (from [`docs/coverage.md`](docs/coverage.md)): backend 97.0% of statements
(`cmd/api` 92.5, `app` 100, `calc` 98.8, `config` 100, `httpapi` 94.4, `server` 92.3);
frontend 100% lines, 98.6% statements, 100% functions, 96.0% branches. Thresholds are 80%
per side and 90% for the domain package. Run `make coverage` and open
`coverage/backend/index.html` or `coverage/frontend/index.html`.

`make coverage` also runs the evaluator benchmark over the worst-case corpus and fails if
any case exceeds 5 ms; the slowest case, a 255-step chain of fractional powers
(`pow_fractional_chain_0.7^0.7_x255`, the longest the length limit admits), takes 2.2 ms.

CI ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) runs five jobs: `hygiene`
(the assessment brief must not be tracked), `backend` (gofmt, golangci-lint, race tests
with the coverage thresholds and the benchmark limit, govulncheck, build), `frontend`
(format check, lint, typecheck, coverage, build, npm audit), `e2e` (API black-box and
Playwright) and `docker` (compose build, `up --wait`, smoke test on port 3000).

## Design decisions

- **Standard library plus one module** ([ADR 0001](docs/adr/0001-architecture-and-dependency-policy.md)).
  The backend uses `net/http` routing, `encoding/json` and `log/slog`; the only module is
  `shopspring/decimal`. Routing, middleware and validation are explicit code with tests,
  and there is one module to audit. The trade-off is that helpers a framework would
  provide are written and maintained in-house. The frontend keeps `react` and `react-dom`
  as its only runtime dependencies.
- **One endpoint, RFC 9457 errors** ([ADR 0002](docs/adr/0002-api-shape-and-error-model.md)).
  `POST /api/v1/evaluate` is the whole API; a resource-style API was rejected because
  nothing is stored. Errors are problem documents with a stable `code`: 400 for input the
  user must fix (with a position), 422 for well-formed expressions without a value.
  Clients branch on codes, never on text.
- **Exact decimals with bounded precision** ([ADR 0003](docs/adr/0003-number-representation.md)).
  `float64` was rejected (binary rounding, `Inf`/`NaN`) and `big.Rat` too (unbounded
  growth, no `sqrt(2)`). Literals and intermediates are rounded to 32 places, results to
  16, inexact operations carry 72 guard places, and limits are errors rather than
  overflow. Fractional powers are computed in-package (`exp(f·ln x)` in binary fixed
  point on `math/big`, at 80, 112 or 172 decimal places depending on the size of the
  base) because the library's `PowWithPrecision` races under concurrent use and seeds
  from a `float64`. The costs: magnitudes below `5·10^-33`
  collapse to `0`, and `(-2)^(1/3*3)` is `INVALID_POWER`.
- **Lenient normalization** ([ADR 0004](docs/adr/0004-lenient-normalization.md)). Live
  evaluation means most requests are incomplete (`3+4*`, `2*(3+4`). A token-level
  normalization step repairs trailing operators, a trailing `.` and unmatched `(`, and the
  evaluated text is echoed so the UI never guesses. A permissive grammar was rejected
  because it blurs valid and invalid input.
- **Calculator-style percent as a table entry** ([ADR 0005](docs/adr/0005-percent-semantics.md)).
  `200+10%` = `220` matches handheld calculators, not spreadsheets. The rule lives in the
  operator table (`+`/`-` flag their right operand; `%` takes an optional base), so the
  parser and evaluator contain no knowledge of `%`, and a new operator or function is one
  table entry.
- **Debounced live preview with cancellation** ([ADR 0006](docs/adr/0006-live-preview-strategy.md)).
  One request 150 ms after the last edit, at most one in flight, superseded requests
  aborted, "Calculating…" after 300 ms, live errors as polite status text, commits as
  authoritative round trips with alerts. Evaluating in the browser was rejected because
  the backend owns the grammar and precision policy. The logic is a pure reducer driven by
  a hook that owns the timers.
- **Two containers behind nginx** ([ADR 0007](docs/adr/0007-container-topology.md)). The
  API runs on a distroless static image as a non-root user with a `-healthcheck` flag; the
  web build is served by unprivileged nginx, which proxies `/api/` and adds the security
  headers once. Same-origin calls keep CORS off. Bundling the SPA into the Go binary was
  rejected because it couples the release cycles of the two tiers.

## Assumptions

Where the requirements were silent, the implementation behaves as follows.

- A number ending in `.` is valid only as the last token of the trimmed input (`2.` → `2`,
  `(2.` → `(2)`); anywhere else it is `INVALID_NUMBER` at the number's position (`2.+3`).
  Two dots in one token are always `INVALID_NUMBER`.
- `UNEXPECTED_TOKEN` messages quote the token's source text (`1 2` → "unexpected '2' at
  character 3"); an input that ends early, or a `)` that normalization appended, reports
  "unexpected end of input" with `position` equal to the input length. `INVALID_CHARACTER`
  quotes control characters Go-style (`'\n'`).
- A `)` where an operand is expected (`()`, `2+)`) is `UNEXPECTED_TOKEN` at the `)`;
  `UNBALANCED_PARENTHESIS` is reported when a complete expression is followed by `)` with
  no open group (`2+3)`).
- The media type must be exactly `application/json` (parameters such as `charset` are
  accepted). `MALFORMED_REQUEST` carries `errors[]` with `INVALID_TYPE` or `UNKNOWN_FIELD`
  where applicable; field names match case-insensitively and the last duplicate key wins,
  as in Go's `encoding/json`.
- The evaluator takes a `context.Context` and checks it per node and per multiplication
  step; a deadline maps to 503 `TIMEOUT`. Operator and function tables reject invalid or
  duplicate registrations at construction time.
- Division truncates the quotient at 72 places and rounds to 32; `sqrt` is the integer
  square root of the scaled coefficient truncated at 72 places and rounded to 32; a
  negative exponent of any kind first takes the reciprocal through the division path.
- The benchmark corpus is: a 1,024-digit literal, `0.` followed by 1,022 digits,
  `1.0001^1000`, `(1.0001^1000)^1000`, `(1.1^1000)^1000` (pre-check reject), 32-deep
  `2^0.5` and `sqrt` chains, 32-deep parentheses, `2^332`, `99^50` and `9^999.5`.
- API responses also carry `Content-Security-Policy: default-src 'none'; frame-ancestors
'none'`, and `Access-Control-Expose-Headers: X-Request-ID` when CORS applies. nginx hides
  the API's security headers on proxied responses and serves the HTML shell with
  `default-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'self';
form-action 'self'; frame-ancestors 'none'`. nginx accepts 8 KiB bodies so oversized
  requests reach the API and get its 413 problem document.
- `LOG_FORMAT=text` is set by the `run-backend` target, which `make dev` calls. The access
  log's `duration_ms` is an integer; `error` holds the problem code and detail for 5xx and
  the panic value for panics.
- Frontend: `VITE_API_BASE_URL` defaults to empty with `/api/v1` appended;
  `VITE_API_TIMEOUT_MS` defaults to 10 s. An empty or whitespace-only input sends no
  request. A 400 `EMPTY` never sets `aria-invalid`. A response the client cannot parse is
  shown as "Something went wrong. Try again."; an aborted request shows nothing. After a
  failed commit the result area is blank and the alert is the only message. While a new
  live request is pending, the previous status text stays visible; "Calculating…" after
  300 ms replaces the result, the status text and "Evaluated as …".
- Activating a history entry counts as an edit even when the text is unchanged (it clears
  alerts and restarts the debounce). A history entry longer than 1,024 characters (closing
  parentheses added by normalization) loads and yields the backend's `TOO_LONG` status
  text. Any edit during an in-flight commit aborts the commit, re-enables `=` and follows
  the debounced path.
- Escape clears only while focus is in the expression input; keypad and history buttons
  are reached with Tab and act on Enter or Space. Keypad buttons keep focus on the input
  (`mousedown` is prevented; `=`, `C` and history activation re-focus it explicitly).
- Playwright keeps two projects (`desktop-chromium` = Desktop Chrome at 1280 × 720,
  `mobile-chromium` = Pixel 7) and checks 320 px by resizing the viewport inside the
  desktop project; the suite uses ports 18080 and 14173 so it never collides with
  `make dev`.
- CI mirrors `make verify` in four jobs plus the hygiene check; ESLint is pinned to major
  9 because `eslint-plugin-jsx-a11y` declares support up to ESLint 9. The default branch
  is `main`.

## Limitations and next steps

Out of scope by design:

- No persistence, accounts, authentication or rate limiting. History lives in the page and
  is lost on reload.
- `sqrt` is the only function; there are no variables, scientific notation input or
  complex numbers.
- English only, with `.` as the decimal point; only ASCII operators are accepted.
- Magnitudes below `5·10^-33` become `0`, and `(-2)^(1/3*3)` is `INVALID_POWER` because
  the exponent is evaluated before the check (see the expression language section).
- A body of non-ASCII characters can exceed the 4 KiB body limit (413) before it reaches
  the 1,024-code-point check (`TOO_LONG`); such input would be rejected as
  `INVALID_CHARACTER` anyway.

Possible next steps:

- More functions (`abs`, `ln`, trigonometry) through the function table, each one entry
  plus tests.
- Server-side history as a new resource, leaving `POST /api/v1/evaluate` unchanged.
- Internationalisation of the UI wording and the decimal separator.

## Troubleshooting

| Symptom                                                                   | Fix                                                                                                                                                |
| ------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| `address already in use` on 8080, 5173 or 3000                            | Find the process (`lsof -i :8080`) and stop it, or set `HTTP_ADDR` for the API; Vite and the containers use fixed ports                            |
| Playwright cannot find a browser                                          | `cd frontend && npx playwright install chromium` (also done by `make setup`)                                                                       |
| `make lint` fails with `golangci-lint: command not found`                 | Install golangci-lint v2 (`brew install golangci-lint`)                                                                                            |
| `make docker-up` fails with `Cannot connect to the Docker daemon`         | Start Docker Desktop, confirm with `docker info`, retry                                                                                            |
| 502 from `/api/` on http://localhost:3000 after rebuilding only `backend` | nginx resolves the `backend` hostname once at startup; restart the web container (`docker compose restart web`) or use `make docker-up` for both   |
| 413 `PAYLOAD_TOO_LARGE` for an expression shorter than 1,024 characters   | Non-ASCII characters take several bytes each; the 4 KiB body limit applies before the length check. Such characters are `INVALID_CHARACTER` anyway |
| `make coverage` fails on the benchmark limit on a slow machine            | Raise the limit for the local run (`make coverage BENCH_MAX_MS=10`); CI keeps 5 ms                                                                 |
| `make coverage` or `make verify` fails a coverage threshold               | Open `coverage/backend/index.html` or `coverage/frontend/index.html` to find the uncovered lines; thresholds are not lowered                       |
