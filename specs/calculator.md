# Calculator

> Status: Ready for implementation
> Owner: aksh · Last updated: 2026-09-18

## 1. Summary

A web calculator made of a React + TypeScript single-page app and a Go REST microservice.
The user types or taps an arithmetic expression; the frontend sends it to the backend,
which tokenizes it, parses it into an abstract syntax tree (AST) and evaluates it, returning
the result as JSON. The purpose is a senior backend engineer assessment: production-grade
code, explicit error handling, tests at every layer and clear documentation, without
over-engineering.

## 2. Goals and non-goals

**Goals**

- Correct evaluation of `+ - * /`, unary minus, parentheses, `^`, `sqrt(x)` and `%` with
  well-defined precedence and associativity.
- Live result while typing (debounced), plus explicit commit with `=`.
- Every invalid input and arithmetic edge case produces a precise, machine-readable error.
- Clean boundaries: pure domain (lexer → parser → AST → evaluator) with no HTTP knowledge;
  thin transport layers on both sides.
- Unit, handler, black-box API, component and browser end-to-end tests; coverage report.
- README with setup, run, API examples, design decisions; ADRs; Docker Compose; CI.

**Non-goals**

- Persistence, accounts, authentication, rate limiting, multi-user features.
- Server-side history; variables; functions other than `sqrt`; scientific notation input;
  localisation (English, `.` decimal point); complex numbers.

## 3. Users and main use cases

- As a user, I want to type `2+3*4` and see `14` as I type, so I don't have to press `=`.
- As a mobile user, I want a keypad so I can enter expressions without a hardware keyboard.
- As a user, I want to press `=` and continue calculating from the result.
- As a user, I want to understand what went wrong (e.g. division by zero) and fix it.
- As a user, I want to reuse an earlier calculation from this session.

## 4. Functional requirements

| ID | Requirement | Priority |
|---|---|---|
| FR-1 | Evaluate expressions with `+`, `-`, `*`, `/`, decimal numbers and parentheses, with standard precedence and left associativity | Must |
| FR-2 | Unary minus: `-3`, `2*-3`, `-(2+3)`, `--3` | Must |
| FR-3 | Exponentiation `^`, right-associative, above `*`/`/` and above unary minus (`-2^2` = -4, `2^-1` = 0.5, `2^3^2` = 512) | Must (Optional in brief; in scope per decision 4) |
| FR-4 | Square root as `sqrt(<expression>)` | Must (Optional in brief; in scope per decision 4) |
| FR-5 | Postfix percent `%` with calculator semantics (section 5) | Must (Optional in brief; in scope per decision 4) |
| FR-6 | Lenient normalization (token-based): trailing binary operators, a trailing `.`, and a trailing `(`/`sqrt(` are dropped, unclosed parentheses are closed, and the evaluated expression is returned | Must |
| FR-7 | Every invalid expression yields 400 with a field error naming the problem and, where one exists, its 0-based position in the original input | Must |
| FR-8 | Arithmetic errors (division by zero, negative square root, negative base with fractional exponent, results or exponents beyond the size limits) yield 422 with a stable code, checked in a fixed order | Must |
| FR-9 | Frontend: one labelled expression input, editable by keyboard; live result shown ≈150 ms after the last change; an in-flight request is aborted when the input changes; a 400 `EMPTY` is shown as a blank result, not an error | Must |
| FR-10 | Frontend: keypad with `0–9 . ( ) + − × ÷ ^ % sqrt ⌫ C =`; taps append to the expression (ASCII tokens `* / -` are inserted; `sqrt` inserts `sqrt(`) and keep focus on the input | Must |
| FR-11 | Frontend: `=` button or Enter commits with a fresh request: the result replaces the expression (negative results wrapped as `(-5)`), the normalized expression and result are added to history, and live preview stays quiet until the next edit | Must |
| FR-12 | Frontend: `C` button or Escape clears expression, result and error; `⌫` removes the last character | Must |
| FR-13 | Frontend: validation, arithmetic, network and unexpected-status errors are shown in plain language next to the input; input is preserved | Must |
| FR-14 | Frontend: in-memory session history of the last 20 committed calculations (`<normalized expression> = <result>`), newest first; activating an entry loads its expression | Should |
| FR-15 | Backend: `GET /healthz` (liveness) and `GET /readyz` (readiness) | Must |

### Acceptance criteria

- **FR-1**: Given `2+3*4`, when evaluated, then the result is `"14"`. Given `(2+3)*4` →
  `"20"`. `10-4-3` → `"3"`. `8/2/2` → `"2"`. `1.5*2` → `"3"`. `.5+.5` → `"1"`.
  `0.1+0.2` → `"0.3"`. `1/3` → `"0.3333333333333333"`. `1/3*3` → `"1"`. `2/3` →
  `"0.6666666666666667"`. `1.10*3` → `"3.3"`. `007` → `"7"`. `00.5` → `"0.5"`.
  `sqrt (16)` → `"4"` (whitespace between tokens). `1 2` → 400 `UNEXPECTED_TOKEN` at 2.
- **FR-2**: `-3+5` → `"2"`; `2*-3` → `"-6"`; `-(2+3)` → `"-5"`; `--3` → `"3"`.
- **FR-3**: `2^10` → `"1024"`; `2^3^2` → `"512"`; `-2^2` → `"-4"`; `(-2)^2` → `"4"`;
  `2^-1` → `"0.5"`; `3^-1` → `"0.3333333333333333"`; `(-2)^-3` → `"-0.125"`; `2^0.5` →
  `"1.414213562373095"`; `1.1^2` → `"1.21"`; `2^100` →
  `"1267650600228229401496703205376"`; `(-8)^(1/3)` → 422 `INVALID_POWER`.
- **FR-4**: `sqrt(16)` → `"4"`; `sqrt(2)` → `"1.414213562373095"`; `sqrt(2)*sqrt(2)` →
  `"2"`; `sqrt(0.25)` → `"0.5"`; `sqrt(-1)` → 422 `NEGATIVE_SQUARE_ROOT`; `sqrt 16` and
  `sqrt()` → 400.
- **FR-5**: `50%` → `"0.5"`; `200+10%` → `"220"`; `200-10%` → `"180"`; `50*10%` → `"5"`;
  `50/10%` → `"500"`; `10%+5` → `"5.1"`; `200+(10%)` → `"200.1"`; `200+10%*2` → `"200.2"`;
  `2^50%` → `"1.414213562373095"`; `50%%` → `"0.005"`; `100+50%%` → `"100.5"`; `-50%` →
  `"-0.5"`; `200+-10%` → `"199.9"` and `200--10%` → `"200.1"` (unary minus breaks
  directness); `(200+10)%` → `"2.1"`; `200+(10)%` → `"220"` (the `%` node itself is direct);
  `2%^2` → `"0.0004"`; `2^2%` → `"1.0139594797900291"` (2^0.02); `sqrt(16)%` → `"0.04"`.
- **FR-6**: `3+4*` → `{ "expression": "3+4", "result": "7" }`; `3+*` → `"3"`; `2*(3+4` →
  `"14"` with `"expression": "2*(3+4)"`; `sqrt(` → 400 `EMPTY`; `2*(` → `"2"`; `2.` →
  `"expression": "2"`, `"result": "2"`; `2+.` → `"2"`; `.` → 400 `EMPTY`; `2+sqrt (` →
  `"2"`; `2..` and `2.5.` → 400 `INVALID_NUMBER` at 0 (only a single trailing `.` is
  repairable); `  2 + 2  ` →
  `"expression": "2 + 2"` (only trailing/leading whitespace trimmed).
- **FR-7**: `` (empty), `   ` and `+` → 400, field `expression`, code `EMPTY`, no
  `position`; `2 3` → `UNEXPECTED_TOKEN`, `position: 2`; `  2 3` → `position: 4` (positions
  index the original, untrimmed input); `*3` → `UNEXPECTED_TOKEN` at 0; `2+3)` →
  `UNBALANCED_PARENTHESIS` at 3; `1.2.3` → `INVALID_NUMBER` at 0; `2.+3` → `INVALID_NUMBER`
  at 0; `2$3` → `INVALID_CHARACTER` at 1; `sqrt` → `UNEXPECTED_TOKEN` at 4 (end of input);
  `sqrt 16` → `UNEXPECTED_TOKEN` at 5; `sqrt()` → `UNEXPECTED_TOKEN` at 5; `foo(1)` →
  `UNKNOWN_FUNCTION` at 0; `2+()` → `UNEXPECTED_TOKEN` at 3; 1,025 code points →
  `TOO_LONG`; `1` wrapped in 33 `(`…`)` pairs → `TOO_DEEP` (both without `position`; 33 bare
  `(` normalise to nothing and are `EMPTY`). Every response also carries
  `code: "VALIDATION_FAILED"` and `status: 400`, exactly one entry in `errors`, and a
  `message` built from the templates in section 6 (`2+3)` → `unbalanced ')' at character 4`;
  `2+()` → `unexpected ')' at character 4`; `sqrt` → `unexpected end of input`).
- **FR-8**: `1/0`, `0/0`, `5/0%` and `0^-1` → 422 `DIVISION_BY_ZERO`; `sqrt(-4)` → 422
  `NEGATIVE_SQUARE_ROOT`; `(-8)^0.5` → 422 `INVALID_POWER`; `10^100`, `10^99*10` and
  `2^1000` → 422 `RESULT_TOO_LARGE`; `1.0001^1001` and `2^-1001` → 422
  `EXPONENT_TOO_LARGE`; `10^99`, `2^300` and `1.0001^1000` → 200. An intermediate value
  beyond the limit is an error even if the final result would be small (`10^100/10^100` →
  `RESULT_TOO_LARGE`). `0^0` → `"1"`; `0.1^20` → `"0"` (below 16 decimal places after
  final rounding). `(-8)^1001.5` → `EXPONENT_TOO_LARGE` (exponent checked first); a literal
  whose value is ≥ 10^100 (`1` followed by 100 zeros) → `RESULT_TOO_LARGE`, while `0`
  followed by 100 zeros → `"0"` (rejection is by value, not digit count); `0.5^1000` and
  `(0.5^1000)^1000` → `"0"` (intermediates rounded to 32 places); `(1.1^1000)^1000` →
  `RESULT_TOO_LARGE` without computing it (pre-check). Negative integer exponents:
  `2^-1000` → `"0"`; `10^-100` → `"0"`; `0.5^-1000` → `RESULT_TOO_LARGE`; `3^-2` →
  `"0.1111111111111111"`. Literal rounding: `0.` followed by 32 zeros and `5` → `"0"`
  (rounded to 32 places on parse, then to 16). Final rounding: `-0.00000000000000005` →
  `"-0.0000000000000001"`; `-0.00000000000000004` → `"0"`.
- **FR-9**: Given the user types `2+2`, when they pause 150 ms, then exactly one request is
  sent and `4` appears in the live result; typing `*3` within 150 ms of the previous
  keystroke sends no request for `2+2`. Given the input changes while a request is in flight,
  then that request is aborted and its response never reaches the UI. Given the input is
  empty, or the backend answers 400 `EMPTY` (e.g. `(`, `-`, `sqrt(`), then the result area
  is blank and no message is shown. Given a commit has just completed, then no live request
  is sent until the input changes. While a request is pending, the previous result stays
  visible; if the response has not arrived 300 ms after the request was sent, the result
  text is replaced by "Calculating…" until it does. "Evaluated as …" is shown only when
  the last response was 200 and its `expression` differs from the trimmed input; it is
  hidden on any error and after a commit.
- **FR-10**: Tapping `7`, `×`, `(`, `2`, `+`, `1`, `)` produces `7*(2+1)` in the input and
  `21` as the live result; focus stays on the input. Tapping `sqrt`, `1`, `6`, `)` produces
  `sqrt(16)` and `4`. Every key is ≥ 44×44 px and reachable with Tab.
- **FR-11**: With `2+2` entered, pressing Enter or `=` shows `4` in the input, adds
  `2+2 = 4` to history, and typing `*3` then Enter yields `12`. With `1/0` entered,
  pressing `=` shows "Cannot divide by zero." as an alert and keeps `1/0` in the input.
  With `2*(3+4` entered, `=` shows `14` and adds `2*(3+4) = 14` to history. While a
  commit request is in flight, `=` is disabled and Enter is ignored; a commit cancels the
  pending debounce, aborts any live request and always sends its own. Enter on an empty or
  whitespace-only input does nothing. A commit answered with 400 `EMPTY` (e.g. `(`) leaves
  the input, shows a blank result and adds no history entry. After a commit the result
  area is blank until the input changes. History stores the raw result string: committing
  `2-7` adds `2-7 = -5` while the input shows `(-5)`. Committing `4` adds `4 = 4`; every
  commit appends, duplicates included. Focus stays on the input after `=`.
- **FR-12**: With `12+3` entered, `⌫` yields `12+`; with `sqrt(` entered, `⌫` yields `sqrt`
  (one character); `C` or Escape empties input, result and error, aborts any in-flight
  request and keeps focus on the input; history is unchanged.
- **FR-13**: Given the backend is unreachable or returns 5xx, when the user commits, then
  "The calculator service is unavailable. Try again." is announced (`role="alert"`) and the
  input is preserved; while typing, the same message is shown as status text, not an alert.
  Given a 400 with code `VALIDATION_FAILED`, then the first `errors[].message` is shown
  verbatim for any of the nine validation codes. Given a 400 `MALFORMED_REQUEST`, any other
  unexpected status (413, 415, 404, other 4xx) or an unknown `code`, then "Something went
  wrong. Try again." is shown the same way. Given no response within 10 s, then the network
  message is shown. `aria-invalid` is set only for 400 and 422 responses, not for network
  errors; any edit clears the alert.
- **FR-14**: After committing `2+2` and `3*3`, history shows `3*3 = 9` above `2+2 = 4`;
  activating `2+2 = 4` loads `2+2` into the input, places the caret at the end, returns
  focus to the input and shows its live result through the normal debounced path. A 21st
  commit drops the oldest entry. Committing `2+2` twice shows two identical entries.
  Reloading the page empties history.
- **FR-15**: `GET /healthz` → 200 `{ "status": "ok" }`; `GET /readyz` → 200 while serving,
  503 `NOT_READY` once shutdown has started (readiness checks only the shutdown flag; there
  are no dependencies). The flag flips on SIGTERM/SIGINT before `Shutdown` runs, and the
  503 is asserted at handler level.

## 5. Business rules and edge cases

### Grammar (accepted language)

```
expression := term { ("+" | "-") term }
term       := unary { ("*" | "/") unary }
unary      := "-" unary | power
power      := postfix [ "^" unary ]          right-associative; -2^2 = -(2^2)
postfix    := primary { "%" }
primary    := number | "(" expression ")" | "sqrt" "(" expression ")"
number     := digits [ "." digits ] | "." digits      digits := 1*DIGIT (ASCII)
```

Leading zeros are allowed (`007` = 7). The lexer takes a number token as the maximal run of
digits and dots, so `1.2.3` is one `INVALID_NUMBER` token at 0. Identifiers are maximal runs
of ASCII letters; anything other than `sqrt` is `UNKNOWN_FUNCTION`.

- Whitespace (ASCII space, tab) between tokens is ignored and trimmed at both ends; any
  other character, including `\n` and `\r`, is `INVALID_CHARACTER`. Only ASCII operators are accepted (`×`, `÷`, `−` are rejected; the
  keypad inserts `*`, `/`, `-`). `sqrt` is case-sensitive; other identifiers are
  `UNKNOWN_FUNCTION`.
- Implicit multiplication (`2(3)`, `2sqrt(4)`) is `UNEXPECTED_TOKEN`. Unary plus (`+3`) is
  `UNEXPECTED_TOKEN`. `()` is `UNEXPECTED_TOKEN` at the `)`.
- Precedence, high to low: `%` (postfix) · `^` · unary `-` · `*` `/` · `+` `-`.
- Parentheses are retained in the AST (a Group node) because they change `%` semantics.
- Validation runs in this order and stops at the first failure, so `errors` always has
  exactly one entry: `TOO_LONG` → full lex (`INVALID_CHARACTER`, `INVALID_NUMBER`,
  `UNKNOWN_FUNCTION`, at the first offending token) → normalization → `EMPTY` → `TOO_DEEP`
  (counted on the normalized token stream) → parse errors (`UNEXPECTED_TOKEN`,
  `UNBALANCED_PARENTHESIS`).

### Normalization (applied to the token stream before parsing, in this order)

1. Trim leading and trailing whitespace (space, tab).
2. Repeatedly drop a trailing binary operator token (`+ - * / ^`), a trailing `.` (a lone
   `.` or one that ends a number, so `2.` → `2`), or a trailing `(` or `sqrt` `(` pair
   (whitespace before the dropped token is dropped too; `2+sqrt (` → `2`).
3. Append `)` for every unmatched `(`.
4. If the result is empty → `EMPTY`.

The evaluated string is returned as `expression`. Nothing else is rewritten (spacing and
number formatting are preserved). Error positions always refer to the original input, so
the backend keeps the trim offset (`  2 3` → position 4).

### Percent semantics (calculator-style)

`x%` evaluates to `x/100`, except when the `%` node is the direct right operand of `+` or
`-` with left operand `L`: then it evaluates to `L * x/100`. Parentheses break directness
(`200+(10%)` = 200.1); an intervening operator breaks it (`200+10%*2` = 200.2). Repeated
`%` applies the rule to the outermost node only (`100+50%%` = 100.5). Unary minus is an
intervening node (`200+-10%` = 199.9; `-50%` = -0.5). A Group inside the `%` node does not
break directness (`200+(10)%` = 220), and `%` binds tighter than `^` (`2%^2` = 0.0004,
`2^2%` = 2^0.02).

### Numbers

- Arbitrary-precision decimal arithmetic (`github.com/shopspring/decimal`), never binary
  floating point: `0.1+0.2` = `0.3`. Literals are parsed exactly and then rounded to 32
  decimal places (half away from zero), the same rule as every intermediate, so no value in
  the evaluator ever has more than 100 integer digits plus 32 decimals (132 digits).
- `+`, `-`, `*`, `%` (÷100) and `^` with a non-negative integer exponent are exact whenever
  the true result has at most 32 decimal places. A negative integer exponent is evaluated
  as `(1/x)^|n|`: the reciprocal takes the 32-place division path and the power is then the
  ordinary integer power (`3^-1` = `0.3333333333333333`, `3^-2` = `0.1111111111111111`,
  `2^-1000` = `0`, `0.5^-1000` = `RESULT_TOO_LARGE`, `0^-1` = `DIVISION_BY_ZERO`).
- Inexact operations — `/`, `^` with a non-integer exponent, `sqrt` — are computed with at
  least 40 guard digits (Newton iteration for `sqrt`; for the fractional part of `^`,
  `exp(f·ln x)` evaluated in fixed point on `math/big` integers with 88 places, because the
  library's `PowWithPrecision` has a data race under concurrent use and seeds its logarithm
  from a `float64`, see decision 35) and rounded half away from zero to 32 places, so that
  the 32-place value is correctly rounded and guard digits absorb intermediate error
  (`1/3*3` = `1`, `sqrt(2)*sqrt(2)` = `2`).
- Every intermediate result is rounded to 32 decimal places (half away from zero).
  Integer powers are evaluated by square-and-multiply with every partial product rounded to
  32 places and rejected as `RESULT_TOO_LARGE` the moment it reaches 10^100, so no value
  ever exceeds 132 digits and no single operation touches more than a few hundred digits.
  Consequence: magnitudes below 5·10^-33 become 0 (`0.5^1000` = `0`, `1/10^33*10^33` = `0`,
  and `10^99/10^-99` is `DIVISION_BY_ZERO` because `10^-99` rounds to 0).
- Before computing `x^n`, the result is rejected as `RESULT_TOO_LARGE` when
  `(d - 1) · floor(|n|) ≥ 100`, where `d` is the number of integer digits of `|x|` (for a
  negative `n`, `x` is the reciprocal); this bounds the work of exponentiation
  (`(1.1^1000)^1000` and `99^999.5` fail without computing).
- The final result is rounded to 16 decimal places (half away from zero).
- Results are JSON **strings** in canonical form: optional leading `-`, digits, optional
  `.` and fraction with no trailing zeros, no exponent notation, no leading `+`, `-0` → `0`
  (`"14"`, `"0.3"`, `"-6"`, `"1267650600228229401496703205376"`).
- Errors (all 422): division by zero (`DIVISION_BY_ZERO`; also `0^<negative>`); `sqrt` of a
  negative (`NEGATIVE_SQUARE_ROOT`); negative base with a non-integer exponent
  (`INVALID_POWER`); `|exponent| > 1000` (`EXPONENT_TOO_LARGE`, checked before computing);
  any literal, intermediate or final `|value| ≥ 10^100` (`RESULT_TOO_LARGE`). `0^0` = 1.
  For `^` the checks run in this order: `EXPONENT_TOO_LARGE`, `DIVISION_BY_ZERO`,
  `INVALID_POWER`, `RESULT_TOO_LARGE` (`(-8)^1001.5` → `EXPONENT_TOO_LARGE`). Because
  exponents are evaluated values, `(-2)^(1/3*3)` is `INVALID_POWER` (the exponent is
  `0.99…9`, not `1`); the README documents this.
- The UI displays the string as returned. A committed result re-enters the expression
  verbatim, wrapped in parentheses when negative (`(-5)`), so `^2` after committing `-5`
  gives `25`.

### Limits

Expression ≤ 1,024 Unicode code points, counted before normalization (`TOO_LONG`); nesting
depth ≤ 32 counting `(` and `sqrt(` only (`TOO_DEEP`; unary and `^` chains are bounded by
the length limit); request body ≤ 4 KiB (413; because non-ASCII input is rejected anyway,
a body that trips 413 before `TOO_LONG` is acceptable and documented); `|exponent| ≤ 1000`;
`|value| < 10^100`; 32 decimal places per literal and intermediate; handler timeout 5 s
(503 `TIMEOUT`). Limits are constants documented in the README.

## 6. API requirements

Base path `/api/v1`; contract in `backend/api/openapi.yaml` (OpenAPI 3.1); errors are RFC
9457 problem details per `.claude/rules/api-contract.md`. Every response has
`X-Request-ID` and `Cache-Control: no-store`.

- **Request ID**: 32 lower-case hex characters from `crypto/rand`. A client-supplied
  `X-Request-ID` matching `^[A-Za-z0-9._-]{1,64}$` is echoed; anything else is replaced.
- **Content type**: parsed with `mime.ParseMediaType`, so `application/json; charset=utf-8`
  is accepted; any other media type is 415.
- **Security headers** on every response (backend and nginx): `X-Content-Type-Options:
  nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, `Permissions-Policy:
  camera=(), microphone=(), geolocation=()`, `Cross-Origin-Opener-Policy: same-origin`; the
  HTML shell additionally gets `Content-Security-Policy: default-src 'self'`.
- **CORS**: off unless `CORS_ALLOWED_ORIGINS` is set to a comma-separated list of exact
  origins (`*` is rejected at startup). Allowed origins get `Access-Control-Allow-Origin`
  echoed with `Vary: Origin`; preflights answer `Access-Control-Allow-Methods: GET, POST`,
  `Access-Control-Allow-Headers: Content-Type, X-Request-ID`, `Access-Control-Max-Age: 600`.

### `POST /api/v1/evaluate`

Request (`Content-Type: application/json`, unknown fields rejected):

```json
{ "expression": "2*(3+4" }
```

200:

```json
{ "expression": "2*(3+4)", "result": "14" }
```

`result` is a string (exact decimal, canonical form; see section 5) so that no precision
is lost in JSON or JavaScript numbers. `"expression": null` is `VALIDATION_FAILED` /
`REQUIRED`; a non-string value is `MALFORMED_REQUEST`.

Errors:

| Status | `code` | When | `errors[].code` (field `expression`) |
|---|---|---|---|
| 400 | `MALFORMED_REQUEST` | invalid JSON, wrong type, unknown field, multiple JSON values | — |
| 400 | `VALIDATION_FAILED` | expression missing or unparsable | `REQUIRED`, `EMPTY`, `TOO_LONG`, `TOO_DEEP`, `INVALID_CHARACTER`, `INVALID_NUMBER`, `UNEXPECTED_TOKEN`, `UNBALANCED_PARENTHESIS`, `UNKNOWN_FUNCTION` |
| 413 | `PAYLOAD_TOO_LARGE` | body > 4 KiB | — |
| 415 | `UNSUPPORTED_MEDIA_TYPE` | not `application/json` | — |
| 422 | `DIVISION_BY_ZERO` / `NEGATIVE_SQUARE_ROOT` / `INVALID_POWER` / `EXPONENT_TOO_LARGE` / `RESULT_TOO_LARGE` | arithmetic error (section 5); `detail` is the matching sentence from section 7 | — |
| 503 | `TIMEOUT` | handler exceeded 5 s | — |
| 503 | `NOT_READY` | `GET /readyz` during shutdown | — |
| 404 / 405 | `NOT_FOUND` / `METHOD_NOT_ALLOWED` | unknown route / method (`Allow` header) | — |
| 500 | `INTERNAL_ERROR` | unexpected failure (details logged with the request ID) | — |

Validation errors carry a structured position (this example is the response to `2+()`):

```json
{ "field": "expression", "code": "UNEXPECTED_TOKEN", "position": 3, "message": "unexpected ')' at character 4" }
```

`position` is the 0-based code-point index into the original (untrimmed) input and is
omitted for `REQUIRED`, `EMPTY`, `TOO_LONG` and `TOO_DEEP`; `detail` repeats the message.
`message` is built from these templates (`N` is `position + 1`):

| Code | Message |
|---|---|
| `REQUIRED` | `expression is required` |
| `EMPTY` | `expression is empty` |
| `TOO_LONG` | `expression exceeds 1,024 characters` |
| `TOO_DEEP` | `expression is nested deeper than 32 levels` |
| `INVALID_CHARACTER` | `invalid character '$' at character N` |
| `INVALID_NUMBER` | `invalid number at character N` |
| `UNKNOWN_FUNCTION` | `unknown function 'foo' at character N` |
| `UNEXPECTED_TOKEN` | `unexpected ')' at character N`, or `unexpected end of input` when the input ends early (`position` = input length) |
| `UNBALANCED_PARENTHESIS` | `unbalanced ')' at character N` | `result` matches
`^-?(0|[1-9][0-9]*)(\.[0-9]*[1-9])?$` (the pattern goes into the OpenAPI schema).

### Health

`GET /healthz` → 200 `{ "status": "ok" }`. `GET /readyz` → 200 `{ "status": "ok" }` or 503
problem `NOT_READY` during shutdown.

## 7. User interface requirements

Single screen, mobile-first, usable from 320 px to wide desktop.

- **Header**: title "Calculator".
- **Display region**: labelled expression `<input>` (`maxLength=1024`, `autoComplete="off"`,
  `spellCheck=false`; `inputMode="none"` on touch devices detected with `(pointer: coarse)`
  so the keypad is the input method and the soft keyboard stays hidden, `inputMode="text"`
  otherwise), below it the live result in an `<output aria-live="polite">`; both are
  monospace, single-line and scroll horizontally so a full-length result (up to 118
  characters) is never truncated; the region shows the normalized expression in muted text
  when it differs from the input (e.g. "Evaluated as 2*(3+4)").
- **Message region**: validation/arithmetic/network messages. While typing: status text
  (polite live region, replaces the result). On commit: `role="alert"`, input marked
  `aria-invalid` and linked with `aria-describedby`.
- **Keypad**: 4-column grid of `<button type="button">` elements with visible labels
  `C ⌫ ( )` / `7 8 9 ÷` / `4 5 6 ×` / `1 2 3 −` / `0 . % +` / `sqrt ^ =`; `=` is
  `type="submit"` in the form wrapping the input. Non-digit keys have `aria-label`s:
  "clear", "backspace", "open parenthesis", "close parenthesis", "divide", "multiply",
  "subtract", "add", "decimal point", "percent", "square root", "power", "equals".
  `sqrt` inserts `sqrt(`; every key appends at the end of the expression and places the
  caret there, and an insert that would exceed 1,024 characters is ignored (`maxLength`
  does not cover programmatic changes). `=` spans two columns in the last row. The document
  `<title>` is "Calculator".
- **History**: list (`<ul>`) of buttons "2+2 = 4", newest first, max 20, heading "History";
  hidden when empty; entries show the normalized expression and the raw result in full
  (no truncation); every commit appends, duplicates included.
- **States**: empty (blank result), typing (debounced request; previous result stays
  visible; "Calculating…" replaces it only if no response has arrived 300 ms after the
  request was sent), success, error, committing (`=` disabled, Enter ignored, same
  "Calculating…" rule), committed (input holds the result, result area blank, no request
  until the next edit). Client-side fetch timeout 10 s → network message.
- **Keyboard**: all characters typed directly; Enter commits; Escape clears; these are the
  only shortcuts, and the keyboard Backspace keeps native caret behaviour; Tab reaches every
  button; visible `:focus-visible` ring; no keyboard traps.
- **Focus**: stays on the input after keypad taps, `=`, `C` and after activating a history
  entry; errors never move focus (the alert is linked via `aria-describedby`).
- **Wording**: `DIVISION_BY_ZERO` → "Cannot divide by zero."; `NEGATIVE_SQUARE_ROOT` →
  "Cannot take the square root of a negative number."; `INVALID_POWER` → "Cannot raise a
  negative number to a fractional power."; `EXPONENT_TOO_LARGE` → "Exponent must be between
  -1000 and 1000."; `RESULT_TOO_LARGE` → "Result is too large to calculate."; validation →
  server message; network/5xx → "The calculator service is unavailable. Try again."; any
  other status or unknown code → "Something went wrong. Try again."
- **Theme**: honours `prefers-color-scheme`; text contrast ≥ 4.5:1 for every token pair
  in both schemes (asserted by an axe check in the Playwright suite and a unit test over
  the design tokens). No animation is specified; any transition that is added must be
  disabled under `prefers-reduced-motion`.

## 8. Non-functional requirements

| ID | Category | Requirement |
|---|---|---|
| NFR-1 | Accessibility | WCAG 2.2 AA: labelled controls, live regions, alerts, keyboard operable; targets ≥ 44×44 px measured by bounding box in Playwright (stricter than the AA minimum of 24 px); zero axe violations on both Playwright projects |
| NFR-2 | Responsiveness | Usable from 320 px; no page-level horizontal scrolling (`document.scrollWidth ≤ viewport width`; the input and result elements scroll internally); tested at 320 px, Pixel 7 (412 px) and 1280 px |
| NFR-3 | Quality | Test-first; Go domain unit + fuzz tests, handler tests, black-box API tests; Vitest unit/component/MSW tests; Playwright projects "Desktop Chrome" (1280×720) and `devices['Pixel 7']`. Coverage ≥ 80% per side, ≥ 90% for domain packages; report in `docs/coverage.md` |
| NFR-4 | Performance | Parsing is single-pass over the token stream (verified by review). A Go benchmark over a named worst-case corpus (a 1,024-character literal, `1.000…01^1000` chains, `2^0.5` and `sqrt(2)` chains, 32-deep nesting, the largest legal integer power) runs each case in < 5 ms on the CI runner; the numbers are quoted in `docs/coverage.md`. Live preview never sends more than one request per 150 ms pause |
| NFR-5 | Security | Server-side validation of everything; input limits; the security headers and CORS rules of section 6 (asserted by handler tests); no secrets; same-origin `/api` |
| NFR-6 | Observability | `log/slog` structured logs per request with `request_id`, `method`, `path`, `status`, `duration_ms`, `bytes`, `remote_addr`, `user_agent` (5xx and panics add `error`); health endpoints; a panic before the response is written is recovered into a 500 `INTERNAL_ERROR` and logged (panics after headers are written can only be logged) |
| NFR-7 | Operability | Configuration via environment with these defaults: `HTTP_ADDR=:8080`, `LOG_LEVEL=info`, `LOG_FORMAT=json` (`scripts/dev.sh` sets `text`), `HTTP_SHUTDOWN_TIMEOUT=10s`, `CORS_ALLOWED_ORIGINS=` (empty = off); server timeouts ReadHeader 5 s, Read 10 s, Write 15 s (longer than the 5 s handler timeout), Idle 60 s, MaxHeaderBytes 1 MiB; graceful shutdown; `VITE_API_BASE_URL` default empty (same origin, `/api/v1` appended in code), Vite dev proxy `/api` → `http://localhost:8080` |
| NFR-8 | Maintainability | Adding a binary operator = one entry in the operator table (token, precedence, associativity, evaluate function); adding a function = one entry in the function table (name, evaluate function). `%` and `^` are entries in that table whose evaluate functions own their special rules. Verified by review: no `switch` on operator tokens outside the domain package |

## 9. Constraints

| ID | Constraint |
|---|---|
| C-1 | Backend in Go (≥ 1.24) with the standard library for HTTP, JSON, logging and testing (no web frameworks, routers, ORMs, DI, assertion or mocking libraries). The single permitted module dependency is `github.com/shopspring/decimal` ≥ v1.4.0 (needed for `PowWithPrecision`) for exact arithmetic (pinned, `go.sum` committed). The first phase of `/implement` relaxes the guardrails, each edit approved by the user: `GO_DEPENDENCY_POLICY="no-frameworks"` in `.claude/hooks/hooks.conf`, and the "standard library only" wording in `.claude/CLAUDE.md`, `.claude/rules/backend-go.md`, `.claude/skills/implement/references/definition-of-done.md` and the depguard config amended to allow this one module (`go.sum` is written by `go mod tidy`, since the file guard blocks direct edits) |
| C-2 | Frontend in React + strict TypeScript on Vite; runtime dependencies `react` and `react-dom` only; CSS Modules + custom properties, no UI/CSS frameworks |
| C-3 | Backend parses expressions into an AST and evaluates the tree (no `eval`, no regex-based arithmetic) |
| C-4 | Dev/test tooling (Vitest, Testing Library, MSW, Playwright, `@axe-core/playwright`, ESLint, Prettier, golangci-lint, govulncheck) are dev dependencies or binaries only |
| C-5 | Repository root is `Assessment/`; layout per `.claude/CLAUDE.md` (`backend/`, `frontend/`, `specs/`, `docs/`, `Makefile`) |

## 10. Deliverables

- Git repository with `backend/` and `frontend/`, `Makefile`, `scripts/dev.sh`.
- `README.md`: setup, running each side and together, configuration, executed `curl`
  examples (success and errors), testing and coverage, project structure, design
  decisions, assumptions, limitations, troubleshooting.
- `backend/api/openapi.yaml`.
- Tests at every layer; `docs/coverage.md` with real numbers.
- `docs/adr/`: dependency policy, API shape and error model, number representation,
  lenient normalization, percent semantics, live-preview strategy, container topology.
- `docs/prompts.md`: the `/spec` and `/implement` prompts used to produce the work,
  verbatim (the brief asks for them). The brief itself lives in `docs/brief.md`, which is
  listed in `.gitignore` and never committed (a CI step fails if it is tracked).
- `Dockerfile`s with a `-healthcheck` flag on the backend binary for the container
  `HEALTHCHECK`; `compose.yaml` (`docker compose up --build --wait`): `web` on port 3000
  (nginx serving the SPA and proxying `/api/`), `backend` on port 8080 (published so the
  README `curl` examples work); `.github/workflows/ci.yml` running `make verify` and the
  compose smoke test.

## 11. Decisions

| # | Question | Decision | Date |
|---|---|---|---|
| 1 | Trailing operator | Dropped; evaluated expression returned | 2026-09-17 |
| 2 | Unclosed parentheses | Auto-closed by the backend | 2026-09-17 |
| 3 | API surface and path | Single `POST /api/v1/evaluate` | 2026-09-17 |
| 4 | Optional operations | `^`, `sqrt`, calculator-style `%` all in scope | 2026-09-17 |
| 5 | Number representation | Exact decimals via `shopspring/decimal` (user's call: floats cause errors); result as JSON string; 32 guard places for inexact ops, final rounding to 16 places; size limits instead of overflow | 2026-09-17 |
| 6 | Live evaluation | 150 ms debounce, cancel stale requests | 2026-09-17 |
| 7 | UI | Expression input + keypad; `=` commits result into input | 2026-09-17 |
| 8 | History | UI-only, in-memory, 20 entries | 2026-09-17 |
| 9 | Delivery | Docker Compose and GitHub Actions CI in scope | 2026-09-17 |
| 10 | `-2^2` | = -4 (unary minus below `^`, as in Python/Google) | 2026-09-17 |
| 11 | Limits | 1,024 chars, depth 32, body 4 KiB, exponent ±1000, magnitude < 10^100, timeout 5 s | 2026-09-17 |
| 12 | Precision policy | Exact for `+ - * %` and integer `^`; 32 decimal places for `/`, fractional `^`, `sqrt`; final result rounded to 16 places, half away from zero | 2026-09-17 |
| 13 | Committed negatives | Re-entered as `(-5)` so a following `^` applies to the whole value | 2026-09-17 |
| 14 | Guardrails vs. `shopspring/decimal` | Dependency stays; Claude edits `hooks.conf`, `CLAUDE.md`, `backend-go.md` and depguard in `/implement`'s first phase, each edit approved by the user | 2026-09-17 |
| 15 | Intermediate precision | Every intermediate rounded to 32 places (≤ 132 digits); magnitude cap stays `10^100`; `^` pre-checked from the base's digit count | 2026-09-17 |
| 16 | Long results | Shown in full; input and result are monospace and scroll horizontally | 2026-09-17 |
| 17 | Error positions | 0-based index into the original input in a structured `position` field (omitted for `EMPTY`, `REQUIRED`, `TOO_LONG`, `TOO_DEEP`); `message` says it 1-based | 2026-09-17 |
| 18 | Live-typing errors | 400 `EMPTY` shown as a blank result; a trailing `.` is dropped by normalization; other validation errors stay as status text | 2026-09-17 |
| 19 | `sqrt` key | Inserts `sqrt(`; `⌫` removes one character | 2026-09-17 |
| 20 | History text | Stores and reloads the normalized expression | 2026-09-17 |
| 21 | After commit | Result area blank and no live request until the next edit | 2026-09-17 |
| 22 | Stale requests | Aborted with `AbortController` when the input changes | 2026-09-17 |
| 23 | Commit edge cases | Enter and `=` ignored while committing; a commit cancels the debounce, aborts the live request and sends its own; empty input ignored | 2026-09-17 |
| 24 | Length unit | 1,024 Unicode code points; input `maxLength=1024`; unexpected statuses show "Something went wrong. Try again." | 2026-09-17 |
| 25 | Mobile input | `inputMode="none"` on coarse-pointer devices, `text` elsewhere | 2026-09-17 |
| 26 | Depth and check order | Depth counts `(`/`sqrt(` only; `^` checks exponent, zero base, negative base, then magnitude; literals ≥ `10^100` are `RESULT_TOO_LARGE` | 2026-09-17 |
| 27 | Negative integer exponents | Take the 32-place division path, not the exact path | 2026-09-17 |
| 28 | Brief | Kept in git-ignored `docs/brief.md`; `docs/prompts.md` holds the prompts only | 2026-09-17 |
| 29 | Literals | Rounded to 32 decimal places on parse, same rule as intermediates | 2026-09-17 |
| 30 | Negative integer exponents | Tiny results are `0`, huge ones `RESULT_TOO_LARGE`; implemented as `(1/x)^\|n\|` so every partial product stays within 32 places (`2^-1000` → 0, `0.5^-1000` → error) | 2026-09-17 |
| 31 | History duplicates | Always append | 2026-09-17 |
| 32 | Validation messages | Fixed templates per code (section 6), shown verbatim by the UI | 2026-09-17 |
| 33 | Handler timeout | 503 problem `TIMEOUT` | 2026-09-17 |
| 35 | Fractional `^` | Computed in-package (`exp(f·ln x)` in fixed point on `math/big`, 88 places) instead of `decimal.PowWithPrecision`, which races under concurrent use (package-level factorial cache, `go test -race` fails) and seeds `Ln` from a `float64`; the library still parses, rounds, multiplies and divides | 2026-09-18 |
| 34 | Audit assumptions | The audit's low-risk assumptions (request ID, CORS, security headers, config defaults, log fields, focus and status behaviour, percent edge cases, validation order, Playwright projects, compose ports, `=` key span) are written into sections 4–10 and testability rewrites into NFR-1/2/4/6/8 | 2026-09-17 |

## 12. Open questions

- [x] None. Decisions 1–34 are confirmed; `/implement` builds from this spec as written.

## 13. References

- Brief: the assessment text (kept in git-ignored `docs/brief.md`, not in the repository).
- `.claude/rules/api-contract.md`, `.claude/rules/testing.md`, `.claude/CLAUDE.md`.
- RFC 9457 Problem Details for HTTP APIs.
