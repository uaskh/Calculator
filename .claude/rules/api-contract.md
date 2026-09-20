---
paths:
  - "backend/**"
  - "frontend/src/api/**"
  - "frontend/e2e/**"
  - "docs/**"
  - "README.md"
---

# API contract rules

The contract is designed before the code (plan phase), written to
`backend/api/openapi.yaml` (OpenAPI 3.1), implemented by the backend, mirrored by the
frontend types in `frontend/src/api/`, and demonstrated by the README examples. All four
must agree; a change to one is a change to all.

## Resources and URLs

- Versioned base path: `/api/v1`. Health endpoints stay unversioned: `/healthz`, `/readyz`.
- Lower-case, hyphenated, plural nouns for resources (`/api/v1/order-items/{id}`). An
  action that is not a create/read/update/delete on a resource may use `POST` on a
  verb-like path (`POST /api/v1/<action>`). Decide the resource model in the plan and
  confirm it with the user when the spec doesn't say.
- `GET` is safe and idempotent; `POST` creates or computes; `PUT`/`PATCH`/`DELETE` only for
  real resources. No verbs in query strings, no bodies on `GET`.

## Requests

- JSON only, `Content-Type: application/json` (415 otherwise), body size limit (413).
- Field names in `camelCase`; unknown fields are rejected (400). Optional versus required
  is explicit in the schema; missing required fields are validation errors.
- Numeric fields: the representation (JSON number, or string for exact decimals),
  precision, range and rounding are decided in the plan and written into the schema
  (`minimum`, `maximum`, `format`). JSON cannot carry NaN or Infinity.
- Enumerations are strings in `UPPER_SNAKE_CASE` or `camelCase` (pick one, stay consistent),
  validated against the allowed set.

## Responses

- Success bodies are the resource or result object itself (no `{ "data": … }` wrapper
  unless the spec asks for one), `Content-Type: application/json; charset=utf-8`,
  `Cache-Control: no-store` for dynamic results.
- Collections are objects with an `items` array (`{ "items": [...] }`), so paging fields
  can be added later without breaking clients. Sorting and filtering use query parameters
  validated against an explicit allow-list (unknown values are a 400).
- Required fields are always present; absent optional fields are omitted, not `null`.
- Every response carries `X-Request-ID` (echoed if the client sent a valid one).

## Errors (RFC 9457 problem details)

`Content-Type: application/problem+json`:

```json
{
  "type": "about:blank",
  "title": "Bad Request",
  "status": 400,
  "detail": "The request body is invalid.",
  "instance": "/api/v1/items",
  "code": "VALIDATION_FAILED",
  "requestId": "4f9c0e7d8a1b2c3d4e5f60718293a4b5",
  "errors": [{ "field": "name", "code": "REQUIRED", "message": "is required" }]
}
```

- `code` is stable and machine-readable (`UPPER_SNAKE_CASE`); clients branch on it.
  `detail` is safe, human-readable and says what to fix. `errors` lists every invalid field.
- 5xx responses never leak internals (no stack traces, SQL, file paths or raw errors);
  details go to the logs with the same request ID.

| Status | When | Typical `code` |
|---|---|---|
| 200 / 201 / 204 | success (201 with `Location` for created resources) | — |
| 400 | malformed JSON, wrong types, unknown or missing fields, invalid values | `MALFORMED_REQUEST`, `VALIDATION_FAILED` |
| 404 | unknown route or resource | `NOT_FOUND` |
| 405 | known route, wrong method (with `Allow`) | `METHOD_NOT_ALLOWED` |
| 409 | state conflict | `CONFLICT` |
| 413 | body too large | `PAYLOAD_TOO_LARGE` |
| 415 | not JSON | `UNSUPPORTED_MEDIA_TYPE` |
| 422 | well-formed request that breaks a domain rule | domain-specific, e.g. `RULE_VIOLATION` |
| 429 | rate limited (only if the spec asks for limits; include `Retry-After`) | `RATE_LIMITED` |
| 500 | unexpected failure | `INTERNAL_ERROR` |
| 503 | not ready / dependency unavailable | `NOT_READY` |

The 400-vs-422 split above is the default; if the spec or the user prefers another
convention, record the decision in an ADR and apply it everywhere.

## Cross-origin and security

- The browser app calls the API same-origin (`/api` proxied by Vite in development and by
  nginx in containers), so CORS is off by default; `CORS_ALLOWED_ORIGINS` enables an
  explicit allow-list when the frontend is hosted elsewhere.
- No authentication unless the spec requires it; if it does, design it in the plan and ask.
- Validate everything server-side even when the UI validates too.

## Change management

- Additive changes only within `v1` (new optional fields, new endpoints). Breaking changes
  need a new version and the user's approval.
- When the contract changes, update in the same commit: handler + tests, `openapi.yaml`,
  frontend types/parsers + tests, e2e tests, README examples.
