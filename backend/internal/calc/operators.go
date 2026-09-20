package calc

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/shopspring/decimal"
)

// operatorKind distinguishes infix operators from postfix ones.
type operatorKind int

const (
	kindBinary  operatorKind = iota // left <op> right
	kindPostfix                     // operand <op>
)

// associativity decides how a chain of equal-precedence binary operators groups.
type associativity int

const (
	assocLeft  associativity = iota // a - b - c = (a - b) - c
	assocRight                      // a ^ b ^ c = a ^ (b ^ c)
)

// Precedence levels; a higher level binds tighter. Unary minus is grammar rather than a
// table entry: it sits between the multiplicative operators and "^", so "-2^2" = -(2^2).
const (
	precedenceLowest         = 0
	precedenceAdditive       = 1
	precedenceMultiplicative = 2
	precedenceUnaryMinus     = 3
	precedencePower          = 4
	precedencePostfix        = 5
)

// unaryMinusSymbol is the operator symbol that also negates the operand that follows it.
const unaryMinusSymbol = "-"

// binaryFunc evaluates a binary operator. Implementations that iterate (exponentiation)
// check the context between steps.
type binaryFunc func(n *numbers, ctx context.Context, left, right decimal.Decimal) (decimal.Decimal, error)

// postfixFunc evaluates a postfix operator. base is the left operand of the enclosing
// binary operator when the postfix node is its direct right operand and the postfix
// entry's relativeTo accepts that operator; it is nil otherwise.
type postfixFunc func(n *numbers, operand decimal.Decimal, base *decimal.Decimal) (decimal.Decimal, error)

// operator is one entry of the operator table. Adding an operator is adding an entry:
// the lexer, normalizer, parser and evaluator read the table and nothing else.
type operator struct {
	symbol     string // exactly one code point that is not a digit, dot, letter, parenthesis or space
	kind       operatorKind
	precedence int
	assoc      associativity // binary operators only
	binary     binaryFunc    // set for kindBinary
	postfix    postfixFunc   // set for kindPostfix
	// relativeTo, optional on postfix operators, reports whether a node of this operator
	// in direct right-operand position of parent receives parent's left operand as its
	// base. The percent entry uses it for calculator-style "200+10%" (spec §5); nothing
	// on the binary entries knows about it.
	relativeTo func(parent *operator) bool
}

// isRelativeTo reports whether a postfix node of op that is the direct right operand of
// parent evaluates relative to parent's left operand.
func (op *operator) isRelativeTo(parent *operator) bool {
	return op.relativeTo != nil && op.relativeTo(parent)
}

// isAdditive reports whether op is a binary operator at additive precedence ("+", "-" and
// any entry registered alongside them).
func isAdditive(op *operator) bool {
	return op.kind == kindBinary && op.precedence == precedenceAdditive
}

// operatorTable maps a symbol to its operator.
type operatorTable map[string]*operator

// register adds an operator to the table. Registration happens at construction time, so
// an invalid or duplicate entry is a programming error and panics.
func (t operatorTable) register(op *operator) {
	if err := validateOperator(t, op); err != nil {
		panic(fmt.Sprintf("calc: register operator %q: %v", op.symbol, err))
	}
	t[op.symbol] = op
}

func validateOperator(t operatorTable, op *operator) error {
	if utf8.RuneCountInString(op.symbol) != 1 {
		return errors.New("symbol must be a single code point")
	}
	if r, _ := utf8.DecodeRuneInString(op.symbol); isDigit(r) || r == '.' || isLetter(r) || r == '(' || r == ')' || isSpace(r) {
		return errors.New("symbol collides with the number, identifier, parenthesis or whitespace syntax")
	}
	if _, exists := t[op.symbol]; exists {
		return errors.New("already registered")
	}
	switch op.kind {
	case kindBinary:
		if op.binary == nil {
			return errors.New("binary operator without an evaluate function")
		}
	case kindPostfix:
		if op.postfix == nil {
			return errors.New("postfix operator without an evaluate function")
		}
	}
	return nil
}

// hasSymbol reports whether the code point is a registered operator symbol.
func (t operatorTable) hasSymbol(r rune) bool {
	_, ok := t[string(r)]
	return ok
}

// defaultOperators is the operator table of the specification.
func defaultOperators() operatorTable {
	t := operatorTable{}
	t.register(&operator{symbol: "+", kind: kindBinary, precedence: precedenceAdditive, assoc: assocLeft, binary: (*numbers).add})
	t.register(&operator{symbol: "-", kind: kindBinary, precedence: precedenceAdditive, assoc: assocLeft, binary: (*numbers).sub})
	t.register(&operator{symbol: "*", kind: kindBinary, precedence: precedenceMultiplicative, assoc: assocLeft, binary: (*numbers).mul})
	t.register(&operator{symbol: "/", kind: kindBinary, precedence: precedenceMultiplicative, assoc: assocLeft, binary: (*numbers).div})
	t.register(&operator{symbol: "^", kind: kindBinary, precedence: precedencePower, assoc: assocRight, binary: (*numbers).pow})
	t.register(&operator{symbol: "%", kind: kindPostfix, precedence: precedencePostfix, postfix: (*numbers).percent, relativeTo: isAdditive})
	return t
}
