# 0003. Compute with exact decimals and bounded precision

- Status: Accepted
- Date: 2026-09-18

## Context

A calculator that answers `0.30000000000000004` to `0.1+0.2` is wrong for its users, and
JSON numbers cannot carry arbitrary precision or the values `NaN` and `Infinity`. The
service must also stay fast and bounded for hostile input such as `9^999.5` or
`(1.1^1000)^1000`, and it must be safe under concurrent requests.

Options considered: `float64` (rejected: binary rounding, `Inf`/`NaN`); `math/big.Rat`
(rejected: exact rationals grow without bound and cannot represent `sqrt(2)`); a decimal
library with a fixed precision policy (chosen).

## Decision

- Arithmetic uses `github.com/shopspring/decimal`, the single approved module (ADR 0001):
  literals are parsed exactly, every literal and every intermediate result is rounded half
  away from zero to 32 decimal places, and the final result is rounded to 16 places and
  returned as a canonical JSON **string** (optional `-`, digits, optional fraction with no
  trailing zeros, no exponent notation, never `-0`).
- Inexact operations carry 72 guard places before the 32-place rounding: division uses a
  truncating quotient (`QuoRem`), the square root is the integer square root of the scaled
  coefficient (`math/big`, Newton iteration), and the fractional part of an exponent is
  computed as `exp(f · ln x)` in fixed point on `math/big` integers at 172 places (100
  integer digits + 32 places + 40 guard places, with an exact `2^k` split in `exp` so the
  argument reduction amplifies no error). Truncation
  followed by rounding yields the correctly rounded 32-place value.
- The library's `PowWithPrecision` is **not** used (spec decision 35): under concurrent
  calls it races on a package-level factorial cache (`go test -race` fails) and it seeds
  its logarithm from a `float64`. The in-package implementation is deterministic,
  race-free and about 60 times faster.
- Integer exponents use square-and-multiply with every partial product rounded and
  magnitude-checked; negative exponents take the reciprocal first (`(1/x)^|n|`). Before
  computing `x^n` the result is rejected when `(d − 1) · ⌊|n|⌋ ≥ 100`, where `d` is the
  number of integer digits of `|x|`, so oversized powers fail in microseconds.
- Limits are errors, not overflow: `|exponent| ≤ 1000` (`EXPONENT_TOO_LARGE`), every
  literal, intermediate and final `|value| < 10^100` (`RESULT_TOO_LARGE`), `0^negative` and
  division by a value that rounds to zero are `DIVISION_BY_ZERO`, a negative base with a
  non-integer exponent is `INVALID_POWER`, and `sqrt` of a negative is
  `NEGATIVE_SQUARE_ROOT`. Checks on `^` run in that order.

## Consequences

- Results are exact for `+ − × %` and integer powers whenever the true value fits 32
  places (`0.1+0.2` = `0.3`, `1.1^2` = `1.21`, `2^100` exact), and correctly rounded to
  16 places otherwise (`1/3*3` = `1`, `sqrt(2)*sqrt(2)` = `2`).
- Magnitudes below `5·10^-33` become `0` (`0.5^1000` = `0`); the README documents this and
  the case `(-2)^(1/3*3)`, whose exponent `0.99…9` is not an integer.
- The longest possible result is 118 characters; the UI scrolls it horizontally rather
  than truncating it.
- Every worst case in the benchmark corpus runs well under the 5 ms budget
  (`docs/coverage.md`).
