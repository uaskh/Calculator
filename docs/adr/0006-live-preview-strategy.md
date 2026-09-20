# 0006. Debounce live evaluation and cancel superseded requests

- Status: Accepted
- Date: 2026-09-18

## Context

The result should appear while the user types, without sending a request per keystroke,
without ever showing a stale answer, and without turning every transient error into an
alert. A commit with `=` must be a fresh, authoritative round trip.

Options considered: evaluating in the browser (rejected: the backend owns the grammar and
precision policy); a request per keystroke with last-write-wins (rejected: wasteful and
racy); server-sent updates (rejected: needless complexity for a stateless computation).

## Decision

- The frontend sends one `POST /api/v1/evaluate` 150 ms after the last change; an edit
  before that cancels the pending request. Empty or whitespace-only input sends nothing.
- Exactly one request is in flight at a time: any edit, clear or commit aborts the current
  one with an `AbortController`, and an aborted response is ignored.
- While a request is pending the previous result stays visible; if no response has
  arrived 300 ms after sending, the result region shows "Calculating…".
- Live errors are polite status text; a 400 `EMPTY` is rendered as a blank result.
  Commits (`=` or Enter) cancel the debounce, abort any live request and always send their
  own; their errors are announced with `role="alert"`, and the input is marked invalid only
  for 400/422 responses. During a commit `=` is disabled and Enter is ignored; an edit
  aborts the commit (plan decision D-4).
- A successful commit puts the result into the input (negatives wrapped as `(-5)` so a
  following `^` applies to the whole value), records `expression = result` in a 20-entry
  in-memory history, and stays quiet until the next edit.
- All of this lives in a pure reducer (`model.ts`) driven by a hook (`useCalculator.ts`)
  that owns the timers and the abort controller; components only render.

## Consequences

- One request per pause, never more; stale responses cannot reach the UI; a slow network
  shows progress after 300 ms instead of flickering on every keystroke.
- The reducer is unit-tested exhaustively with fake timers; the browser suite proves the
  journeys against the real backend.
- History is per page load by design (spec non-goal: no persistence); reloading clears it.
