package calc

import "context"

// Calculator evaluates arithmetic expressions: TOO_LONG check, lexing, normalization,
// EMPTY and TOO_DEEP checks, parsing and evaluation, in that order. It is immutable after
// construction and safe for concurrent use.
type Calculator struct {
	operators operatorTable
	functions functionTable
	numbers   *numbers
}

// New returns a calculator with the operators and functions of the specification.
func New() *Calculator {
	return newCalculator(defaultOperators(), defaultFunctions())
}

// newCalculator builds a calculator from explicit tables, so tests can prove that a new
// operator or function is one table entry.
func newCalculator(ops operatorTable, fns functionTable) *Calculator {
	return &Calculator{operators: ops, functions: fns, numbers: newNumbers()}
}

// Evaluate parses and evaluates input. It returns a *ValidationError when the expression
// cannot be evaluated as written, a *ArithmeticError when its value cannot be computed,
// and the context's own error when ctx is done during evaluation.
func (c *Calculator) Evaluate(ctx context.Context, input string) (Result, error) {
	runes := []rune(input)
	if len(runes) > MaxLength {
		return Result{}, tooLong()
	}
	tokens, err := lex(runes, c.operators, c.functions)
	if err != nil {
		return Result{}, err
	}
	norm, err := normalize(runes, tokens, c.operators)
	if err != nil {
		return Result{}, err
	}
	if depthErr := checkDepth(norm.tokens); depthErr != nil {
		return Result{}, depthErr
	}
	tree, err := parse(norm.tokens, c.operators, c.functions, len(runes))
	if err != nil {
		return Result{}, err
	}
	value, err := (&evaluator{num: c.numbers}).eval(ctx, tree)
	if err != nil {
		return Result{}, err
	}
	return Result{Expression: norm.expression, Value: c.numbers.canonical(value)}, nil
}
