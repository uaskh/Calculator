package calc

import (
	"context"

	"github.com/shopspring/decimal"
)

// node is an expression tree node. Nodes are evaluated through evaluator.eval, which
// checks the context before every visit.
type node interface {
	eval(ctx context.Context, ev *evaluator) (decimal.Decimal, error)
}

// numberNode is a literal, kept as source text so that parse errors take precedence over
// a literal beyond the magnitude limit.
type numberNode struct {
	text string
}

// groupNode is a parenthesized expression. It is kept in the tree because parentheses
// change percent semantics ("200+(10%)" is not "200+10%").
type groupNode struct {
	inner node
}

// callNode applies a function from the function table.
type callNode struct {
	fn       *function
	argument node
}

// negateNode is unary minus.
type negateNode struct {
	operand node
}

// postfixNode applies a postfix operator from the operator table.
type postfixNode struct {
	op      *operator
	operand node
}

// binaryNode applies a binary operator from the operator table.
type binaryNode struct {
	op          *operator
	left, right node
}
