# Clarification guide

Ask when the answer changes what is built or how it behaves. Decide yourself, and record an
assumption, when the choice is conventional, internal and cheap to change.

## Always ask when the spec is silent or ambiguous

**Domain behaviour**
- Exact semantics of every business rule, and how rules combine when several apply.
- Edge cases: empty, zero, negative, very large or very small values, boundaries,
  duplicates, invalid combinations, and the expected outcome for each (error or result).
- When the domain handles numbers (money, quantities, measurements): representation
  (integer, decimal or floating point), precision, rounding, allowed range, overflow and
  display format.
- Units, locales, time zones, text normalisation (trimming, case, Unicode).

**API**
- Shape: resources and their names, methods, collection behaviour (paging, sorting,
  filtering), versioning.
- Fields, required versus optional, limits (sizes, counts, ranges).
- Error model beyond the default (RFC 9457 with codes; the 400 versus 422 split) and which
  messages clients may show.
- Idempotency, pagination, rate limiting, authentication and authorisation, CORS origins.

**User experience**
- Interaction model (forms, lists, wizards, inline editing…), layout, and what is shown
  in each state (idle, loading, empty, error, success); what persists across reloads;
  keyboard support beyond the defaults.
- Validation timing and wording; tone of copy; languages.
- Accessibility level (default WCAG 2.2 AA), supported browsers and devices, breakpoints.

**Non-functional and delivery**
- Persistence (none, in memory, database) and data retention.
- Performance targets, concurrency, observability (logs, metrics, tracing).
- Container topology (two services with Compose, or one image serving both), CI,
  target platforms, ports.
- Coverage thresholds if they differ from 80% overall and 90% for domain code.
- Which optional features are in scope.
- Any new dependency (backend: never without approval; frontend: anything beyond
  `react` and `react-dom`).

## Decide yourself and record as assumptions

Package and file names within the reference layout, internal type names, test names, lint
rules within the reference configuration, CSS token names, default ports (8080 API, 5173
web dev server, 3000 containerised web), log field names, commit granularity within a phase.

## How to ask

- Batch related questions: up to 4 per `AskUserQuestion` call, 2–4 options each.
- Put the recommended option first, labelled "(Recommended)", and state each option's
  trade-off in its description. Options are concrete ("Return 422 with code
  RULE_VIOLATION"), never vague ("Handle it gracefully").
- Say why a question matters when it isn't obvious; quote the spec when a statement is
  ambiguous. Never ask what the spec already states.
- Delivery items that only need confirming (container topology, CI, ports, coverage
  thresholds) go into one question whose recommended option is the documented defaults.
- Record every answer immediately in the plan: `D-n | question | decision | consequence`.

Illustrative example (adapt to the actual spec):

> **How should the API report input that is well-formed but breaks a business rule?**
> - *422 with a domain error code (Recommended)*: separates malformed payloads (400) from
>   rule violations, so clients can react precisely.
> - *400 for every input error*: simpler, but clients cannot tell the two cases apart.
