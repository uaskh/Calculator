package calc

import (
	"context"

	"github.com/shopspring/decimal"
)

// evaluator walks an expression tree. It consults the context before every node visit
// and returns the context's error unchanged when the context is done.
type evaluator struct {
	num *numbers
}

func (ev *evaluator) eval(ctx context.Context, n node) (decimal.Decimal, error) {
	if err := ctx.Err(); err != nil {
		return decimal.Zero, err
	}
	return n.eval(ctx, ev)
}

func (n *numberNode) eval(_ context.Context, ev *evaluator) (decimal.Decimal, error) {
	return ev.num.literal(n.text)
}

func (g *groupNode) eval(ctx context.Context, ev *evaluator) (decimal.Decimal, error) {
	return ev.eval(ctx, g.inner)
}

func (c *callNode) eval(ctx context.Context, ev *evaluator) (decimal.Decimal, error) {
	argument, err := ev.eval(ctx, c.argument)
	if err != nil {
		return decimal.Zero, err
	}
	return c.fn.evaluate(ev.num, argument)
}

func (u *negateNode) eval(ctx context.Context, ev *evaluator) (decimal.Decimal, error) {
	operand, err := ev.eval(ctx, u.operand)
	if err != nil {
		return decimal.Zero, err
	}
	return ev.num.negate(operand), nil
}

func (p *postfixNode) eval(ctx context.Context, ev *evaluator) (decimal.Decimal, error) {
	return p.evalWithBase(ctx, ev, nil)
}

// evalWithBase evaluates the postfix node, passing the enclosing binary operator's left
// operand when this node is its direct right operand and its entry is relative to that
// operator (see operator.relativeTo).
func (p *postfixNode) evalWithBase(ctx context.Context, ev *evaluator, base *decimal.Decimal) (decimal.Decimal, error) {
	operand, err := ev.eval(ctx, p.operand)
	if err != nil {
		return decimal.Zero, err
	}
	return p.op.postfix(ev.num, operand, base)
}

func (b *binaryNode) eval(ctx context.Context, ev *evaluator) (decimal.Decimal, error) {
	left, err := ev.eval(ctx, b.left)
	if err != nil {
		return decimal.Zero, err
	}
	right, err := b.evalRight(ctx, ev, left)
	if err != nil {
		return decimal.Zero, err
	}
	return b.op.binary(ev.num, ctx, left, right)
}

// evalRight evaluates the right operand. A postfix node that is the direct right operand
// and whose entry is relative to this operator receives the left value as its base; a
// group, a unary minus or any other node in between breaks that directness.
func (b *binaryNode) evalRight(ctx context.Context, ev *evaluator, left decimal.Decimal) (decimal.Decimal, error) {
	if postfix, ok := b.right.(*postfixNode); ok && postfix.op.isRelativeTo(b.op) {
		if err := ctx.Err(); err != nil {
			return decimal.Zero, err
		}
		return postfix.evalWithBase(ctx, ev, &left)
	}
	return ev.eval(ctx, b.right)
}
