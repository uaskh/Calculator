# 0002. Expose one evaluation endpoint with RFC 9457 problem details

- Status: Accepted
- Date: 2026-09-18

## Context

The calculator has a single use case: evaluate an expression and return its value. The
brief asks for a REST microservice with explicit, machine-readable error handling. The
browser client needs to tell apart "the expression is not valid" (the user should fix the
input), "the expression cannot be computed" (a mathematical limit) and transport problems,
and it needs positions to point at the offending character.

Options considered: a resource-style API (`POST /calculations` creating a record) was
rejected because nothing is stored; separate endpoints per operation were rejected because
the expression grammar already composes operations; a bare `{ "error": "…" }` body was
rejected because clients would have to parse prose.

## Decision

- One operation, `POST /api/v1/evaluate`, with a JSON body `{ "expression": string }` and a
  JSON result `{ "expression": string, "result": string }`. `expression` in the response is
  the normalized text that was evaluated (see ADR 0004); `result` is an exact decimal
  string (see ADR 0003).
- Every error is an RFC 9457 problem document (`application/problem+json`) with a stable
  `code` in `UPPER_SNAKE_CASE`. The 400/422 split follows the repository's API rules:
  - 400 `MALFORMED_REQUEST` for bodies that are not the documented shape (invalid JSON,
    wrong types, unknown fields, several JSON values, empty body);
  - 400 `VALIDATION_FAILED` for an expression that cannot be parsed, with exactly one entry
    in `errors[]`: `field: "expression"`, one of nine codes (`REQUIRED`, `EMPTY`,
    `TOO_LONG`, `TOO_DEEP`, `INVALID_CHARACTER`, `INVALID_NUMBER`, `UNEXPECTED_TOKEN`,
    `UNBALANCED_PARENTHESIS`, `UNKNOWN_FUNCTION`), a `message` built from fixed templates,
    and, where one exists, a 0-based code-point `position` into the original input;
  - 422 with a domain code (`DIVISION_BY_ZERO`, `NEGATIVE_SQUARE_ROOT`, `INVALID_POWER`,
    `EXPONENT_TOO_LARGE`, `RESULT_TOO_LARGE`) for a well-formed expression that has no
    value within the limits; `detail` carries the user-facing sentence;
  - 413, 415, 404, 405 (with `Allow`), 500 `INTERNAL_ERROR`, 503 `TIMEOUT` (request
    deadline) and 503 `NOT_READY` (readiness during a stop) as in the contract.
- Validation stops at the first failure in a fixed order (length → lexing → normalization
  → emptiness → depth → parsing), so `errors` always has one entry and messages are
  deterministic.
- Every response carries `X-Request-ID` (echoed when the client's value matches
  `^[A-Za-z0-9._-]{1,64}$`, otherwise 32 hex characters from `crypto/rand`),
  `Cache-Control: no-store` and the security headers of spec section 6. CORS is off unless
  an explicit origin allow-list is configured; `*` is refused at startup.

The contract lives in `backend/api/openapi.yaml`; the frontend mirrors it in
`frontend/src/api/`.

## Consequences

- Clients branch on `code` and `errors[0].code`, never on text; the UI shows validation
  messages verbatim and maps arithmetic codes to its own wording.
- Adding an arithmetic rule is one 422 code plus one wording entry; adding a validation
  rule is one code plus one message template.
- The single endpoint cannot be cached and is not idempotent in the REST sense, which is
  acceptable for a pure computation. A future history or persistence feature would add
  resources rather than change this operation.
