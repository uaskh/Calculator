package calc

import (
	"context"
	"fmt"
	"math/big"

	"github.com/shopspring/decimal"
)

// Limits of the calculator, documented in the README.
const (
	// MaxLength is the longest accepted expression, in Unicode code points, counted before
	// normalization.
	MaxLength = 1024
	// MaxDepth is the deepest accepted nesting of "(" and "sqrt(".
	MaxDepth = 32
	// MaxExponent bounds |n| in x^n; larger exponents are EXPONENT_TOO_LARGE.
	MaxExponent = 1000
	// MaxIntegerDigits bounds every literal, intermediate and final value:
	// |value| < 10^MaxIntegerDigits, otherwise RESULT_TOO_LARGE.
	MaxIntegerDigits = 100
	// IntermediatePlaces is the number of decimal places every literal and intermediate
	// value is rounded to (half away from zero).
	IntermediatePlaces = 32
	// ResultPlaces is the number of decimal places of the final result (half away from zero).
	ResultPlaces = 16
)

// guardPlaces is the number of decimal places carried by inexact operations (division,
// square root, fractional powers) before they are rounded to IntermediatePlaces.
const guardPlaces = 72

// numbers implements the arithmetic of the specification on decimal.Decimal: exact
// operations, 32-place rounding of every intermediate, the magnitude cap, and the guarded
// inexact operations. It is immutable after construction and safe for concurrent use.
type numbers struct {
	one            decimal.Decimal
	magnitudeLimit decimal.Decimal // 10^MaxIntegerDigits
	maxExponent    decimal.Decimal
	fixed          *fixedPoint // exp and ln for fractional powers
}

// newNumbers builds the arithmetic. The fixed-point context for fractional powers is
// sized so that the largest reachable result (MaxIntegerDigits integer digits) is still
// correct in all of its IntermediatePlaces decimals after the guard margin.
func newNumbers() *numbers {
	return &numbers{
		one:            decimal.NewFromInt(1),
		magnitudeLimit: decimal.New(1, MaxIntegerDigits),
		maxExponent:    decimal.NewFromInt(MaxExponent),
		fixed:          newFixedPoint(MaxIntegerDigits + IntermediatePlaces),
	}
}

// literal converts a number token to a value: parsed exactly, rejected when
// |value| >= 10^100, then rounded to 32 places.
func (n *numbers) literal(text string) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(text)
	if err != nil {
		return decimal.Zero, fmt.Errorf("parse literal %q: %w", text, err)
	}
	return n.intermediate(d)
}

// intermediate rounds a computed value to IntermediatePlaces and enforces the magnitude
// cap. Every operation returns through it.
func (n *numbers) intermediate(d decimal.Decimal) (decimal.Decimal, error) {
	d = roundHalfAwayFromZero(d, IntermediatePlaces)
	if err := n.checkMagnitude(d); err != nil {
		return decimal.Zero, err
	}
	return d, nil
}

func (n *numbers) checkMagnitude(d decimal.Decimal) error {
	if d.Abs().Cmp(n.magnitudeLimit) >= 0 {
		return arithmetic(CodeResultTooLarge)
	}
	return nil
}

// canonical rounds the final value to ResultPlaces, enforces the magnitude cap on the
// rounded value (a result just below 10^100 can round up to it) and formats it: optional
// "-", digits, optional fraction without trailing zeros, no exponent, never "-0".
func (n *numbers) canonical(d decimal.Decimal) (string, error) {
	d = roundHalfAwayFromZero(d, ResultPlaces)
	if err := n.checkMagnitude(d); err != nil {
		return "", err
	}
	return d.String(), nil
}

func (n *numbers) add(_ context.Context, left, right decimal.Decimal) (decimal.Decimal, error) {
	return n.intermediate(left.Add(right))
}

func (n *numbers) sub(_ context.Context, left, right decimal.Decimal) (decimal.Decimal, error) {
	return n.intermediate(left.Sub(right))
}

func (n *numbers) mul(_ context.Context, left, right decimal.Decimal) (decimal.Decimal, error) {
	return n.intermediate(left.Mul(right))
}

// div computes the quotient truncated at guardPlaces and rounds it to 32 places.
func (n *numbers) div(_ context.Context, left, right decimal.Decimal) (decimal.Decimal, error) {
	if right.IsZero() {
		return decimal.Zero, arithmetic(CodeDivisionByZero)
	}
	quotient, _ := left.QuoRem(right, guardPlaces)
	return n.intermediate(quotient)
}

// negate flips the sign; a rounded value stays rounded and within the cap.
func (n *numbers) negate(d decimal.Decimal) decimal.Decimal {
	return d.Neg()
}

// percent computes operand/100, or base*operand/100 when the node is the direct right
// operand of an additive operator.
func (n *numbers) percent(operand decimal.Decimal, base *decimal.Decimal) (decimal.Decimal, error) {
	p := operand.Shift(-2)
	if base != nil {
		p = base.Mul(p)
	}
	return n.intermediate(p)
}

// sqrt computes the square root truncated at guardPlaces (integer square root of the
// coefficient scaled to 2*guardPlaces) and rounds it to 32 places.
func (n *numbers) sqrt(x decimal.Decimal) (decimal.Decimal, error) {
	if x.Sign() < 0 {
		return decimal.Zero, arithmetic(CodeNegativeSquareRoot)
	}
	if x.IsZero() {
		return decimal.Zero, nil
	}
	// x = c * 10^e, so floor(sqrt(x) * 10^guard) = floor(sqrt(c * 10^(e + 2*guard))).
	scaled := scaleCoefficient(x, 2*guardPlaces)
	root := scaled.Sqrt(scaled)
	return n.intermediate(decimal.NewFromBigInt(root, -guardPlaces))
}

// pow computes base^exponent with the checks of the specification, in order:
// EXPONENT_TOO_LARGE, DIVISION_BY_ZERO, INVALID_POWER, RESULT_TOO_LARGE.
func (n *numbers) pow(ctx context.Context, base, exponent decimal.Decimal) (decimal.Decimal, error) {
	if exponent.Abs().Cmp(n.maxExponent) > 0 {
		return decimal.Zero, arithmetic(CodeExponentTooLarge)
	}
	if base.IsZero() {
		switch exponent.Sign() {
		case -1:
			return decimal.Zero, arithmetic(CodeDivisionByZero)
		case 0:
			return n.one, nil
		default:
			return decimal.Zero, nil
		}
	}
	if base.Sign() < 0 && !exponent.IsInteger() {
		return decimal.Zero, arithmetic(CodeInvalidPower)
	}
	if exponent.Sign() < 0 {
		// x^-n = (1/x)^n, with the reciprocal taking the 32-place division path.
		reciprocal, err := n.div(ctx, n.one, base)
		if err != nil {
			return decimal.Zero, err
		}
		if reciprocal.IsZero() {
			return decimal.Zero, nil
		}
		base, exponent = reciprocal, exponent.Abs()
	}

	integerPart := exponent.Truncate(0)
	steps := integerPart.IntPart()
	if int64(integerDigits(base)-1)*steps >= MaxIntegerDigits {
		return decimal.Zero, arithmetic(CodeResultTooLarge)
	}
	result, err := n.integerPower(ctx, base, steps)
	if err != nil {
		return decimal.Zero, err
	}
	fraction := exponent.Sub(integerPart)
	if fraction.IsZero() {
		return result, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return decimal.Zero, ctxErr
	}
	fractionalPower, err := n.intermediate(n.fixed.pow(base, fraction))
	if err != nil {
		return decimal.Zero, err
	}
	return n.mul(ctx, result, fractionalPower)
}

// integerPower is square-and-multiply with every partial product rounded to 32 places
// and checked against the magnitude cap; the context is checked at every step.
func (n *numbers) integerPower(ctx context.Context, base decimal.Decimal, exponent int64) (decimal.Decimal, error) {
	result := n.one
	var err error
	for exponent > 0 {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return decimal.Zero, ctxErr
		}
		if exponent&1 == 1 {
			if result, err = n.intermediate(result.Mul(base)); err != nil {
				return decimal.Zero, err
			}
		}
		exponent >>= 1
		if exponent > 0 {
			if base, err = n.intermediate(base.Mul(base)); err != nil {
				return decimal.Zero, err
			}
		}
	}
	return result, nil
}

// roundHalfAwayFromZero rounds to the given number of decimal places; values with fewer
// places are returned unchanged so coefficients never grow needlessly.
func roundHalfAwayFromZero(d decimal.Decimal, places int32) decimal.Decimal {
	if d.Exponent() >= -places {
		return d
	}
	return d.Round(places)
}

// integerDigits returns the number of digits of the integer part of |d|; 1 for |d| < 1.
func integerDigits(d decimal.Decimal) int {
	whole := d.Abs().BigInt()
	if whole.Sign() == 0 {
		return 1
	}
	return len(whole.Text(10))
}

// scaleCoefficient returns floor(|d| * 10^places) as an integer.
func scaleCoefficient(d decimal.Decimal, places int32) *big.Int {
	shift := int64(d.Exponent()) + int64(places)
	c := d.Abs().Coefficient()
	if shift >= 0 {
		return c.Mul(c, pow10(shift))
	}
	return c.Quo(c, pow10(-shift))
}

// pow10 returns 10^n as an integer.
func pow10(n int64) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(n), nil)
}
