package calc

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
)

// Throw-away operator and function implementations used by TestOperatorTable to prove
// that the parser and evaluator are driven by the tables alone.

// concatDigits is a binary operator: l # r = l*10 + r.
func (n *numbers) concatDigits(_ context.Context, left, right decimal.Decimal) (decimal.Decimal, error) {
	return n.intermediate(left.Mul(decimal.NewFromInt(10)).Add(right))
}

// double is a postfix operator: x! = 2x. It has no relativeTo, so it never gets a base.
func (n *numbers) double(operand decimal.Decimal, _ *decimal.Decimal) (decimal.Decimal, error) {
	return n.intermediate(operand.Mul(decimal.NewFromInt(2)))
}

// cube is a function: cube(x) = x^3.
func (n *numbers) cube(x decimal.Decimal) (decimal.Decimal, error) {
	return n.intermediate(x.Mul(x).Mul(x))
}

func TestTableRegistration_RejectsInvalidEntries(t *testing.T) {
	t.Parallel()

	operators := map[string]*operator{
		"multi-rune symbol":    {symbol: "<<", kind: kindBinary, binary: (*numbers).concatDigits},
		"empty symbol":         {symbol: "", kind: kindBinary, binary: (*numbers).concatDigits},
		"digit symbol":         {symbol: "7", kind: kindBinary, binary: (*numbers).concatDigits},
		"letter symbol":        {symbol: "x", kind: kindBinary, binary: (*numbers).concatDigits},
		"parenthesis symbol":   {symbol: "(", kind: kindBinary, binary: (*numbers).concatDigits},
		"binary without func":  {symbol: "#", kind: kindBinary},
		"postfix without func": {symbol: "!", kind: kindPostfix},
	}
	for name, op := range operators {
		t.Run("operator "+name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if recover() == nil {
					t.Errorf("register(%+v) did not panic", op)
				}
			}()
			defaultOperators().register(op)
		})
	}

	functions := map[string]*function{
		"empty name":     {name: "", evaluate: (*numbers).cube},
		"digit in name":  {name: "log2", evaluate: (*numbers).cube},
		"without func":   {name: "cube"},
		"duplicate name": {name: "sqrt", evaluate: (*numbers).cube},
	}
	for name, fn := range functions {
		t.Run("function "+name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if recover() == nil {
					t.Errorf("register(%+v) did not panic", fn)
				}
			}()
			defaultFunctions().register(fn)
		})
	}
}
