# 0004. Normalize incomplete expressions before parsing

- Status: Accepted
- Date: 2026-09-18

## Context

The result is computed live while the user types, so most requests carry an incomplete
expression: `3+4*` on the way to `3+4*2`, `2*(3+4` before the closing parenthesis, `2.`
before the fraction digits. Reporting each of these as an error would make the live
preview flicker with messages that describe the user's own typing, not a mistake.

Options considered: client-side heuristics only (rejected: the backend is the authority
and the frontend would duplicate grammar knowledge); a permissive grammar that accepts
dangling operators (rejected: it blurs the line between valid and invalid input); a
separate normalization step on the token stream with the evaluated text echoed back
(chosen).

## Decision

After lexing and before parsing, the token stream is normalized in this order:

1. leading and trailing whitespace (space, tab) is trimmed;
2. repeatedly, a trailing binary operator (`+ - * / ^`), a trailing `.` (a lone dot or one
   that ends a number, so `2.` becomes `2`) or a trailing `(` / `sqrt (` pair is dropped,
   together with the whitespace before it;
3. a `)` is appended for every unmatched `(`;
4. an empty remainder is the validation error `EMPTY`.

Nothing else is rewritten: spacing and number formatting are preserved, and the evaluated
text is returned as `expression` so the UI can show "Evaluated as 2*(3+4)". Error
positions always index the original, untrimmed input. A number token ending in `.` is
valid only when it is the last token of the trimmed input; anywhere else it is
`INVALID_NUMBER` (plan decision D-2), and two dots in one token are always invalid.

## Consequences

- Typing `2*(3+4` shows `14` immediately; committing it records `2*(3+4) = 14`.
- Inputs such as `(`, `-` or `sqrt(` normalize to nothing and are `EMPTY`, which the UI
  renders as a blank result rather than an error.
- Because 33 bare `(` normalize to nothing, `EMPTY` is reported before `TOO_DEEP`; the
  validation order is fixed and tested.
- The frontend never guesses: it always sends what the user typed and displays what the
  backend evaluated.
