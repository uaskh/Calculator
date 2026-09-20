# 0005. Give `%` calculator semantics as a table-driven postfix operator

- Status: Accepted
- Date: 2026-09-18

## Context

Users expect `200+10%` to mean "200 plus ten percent of 200" (220), as on handheld
calculators, while `50%` alone means `0.5` and `50*10%` means `5`. A purely mathematical
`x/100` would make `200+10%` equal `200.1`, which surprises people. At the same time the
service must be extensible: adding an operator should not require editing the parser or
the evaluator.

## Decision

- `%` is a postfix operator with the highest precedence (above `^`), evaluated as `x/100`,
  **except** when the `%` node is the direct right operand of `+` or `-` with left value
  `L`; then it evaluates to `L · x / 100`.
- "Direct" is structural: a parenthesised group as the right operand breaks it
  (`200+(10%)` = 200.1), an intervening operator breaks it (`200+10%*2` = 200.2), unary
  minus breaks it (`200+-10%` = 199.9), a group _inside_ the `%` node does not
  (`200+(10)%` = 220), and for repeated `%` only the outermost node receives the base
  (`100+50%%` = 100.5).
- The rule lives in the operator table: the `+` and `-` entries carry a
  `percentRelativeRight` flag, and the `%` entry's evaluate function takes an optional base.
  The parser and evaluator read the table; they contain no knowledge of `%`.
- This is the Strategy pattern realised with Go function values: each table entry holds
  an interchangeable evaluation function (`binaryFunc` or `postfixFunc`) that the
  evaluator calls without knowing which operator it is, in the same way `http.HandlerFunc`
  and `slices.SortFunc` take a function instead of a one-method interface. Precedence and
  associativity are plain data on the entry because the parser reads them rather than
  calls them. An interface with one type per operator would give the same
  interchangeability at the cost of four methods per operator, three returning constants;
  it becomes worth it only if operators gain per-instance state or come from outside the
  package.
- The parser keeps parentheses as group nodes precisely because they change this rule.

## Consequences

- Every example in spec section 5 holds and is a test row (`2^50%` = `2^0.5`,
  `2%^2` = 0.0004, `sqrt(16)%` = 0.04, `-50%` = -0.5).
- Adding another binary or postfix operator is one table entry plus tests; adding a
  function is one entry in the function table.
- The semantics are intentionally not those of a spreadsheet; the README states the rule
  with examples so users are not surprised.
