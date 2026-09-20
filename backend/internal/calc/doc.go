// Package calc is the calculator domain: it lexes, normalizes, parses and evaluates
// arithmetic expressions with exact decimal arithmetic. It knows nothing about HTTP or
// JSON; the transport layer maps its results and errors to the API contract.
//
// The package is deliberately flat, one file per concern, in the order an expression
// flows through Evaluate:
//
//	calc.go           Calculator, New and Evaluate: the pipeline and the validation order
//	                  (TOO_LONG → lex → normalize → EMPTY → TOO_DEEP → parse → evaluate)
//	result.go         Result, the successful outcome
//	errors.go         ValidationError (400 cases), ArithmeticError (422 cases), their codes
//
//	token.go          token kinds and their code-point spans in the original input
//	lexer.go          input → tokens; INVALID_CHARACTER, INVALID_NUMBER, UNKNOWN_FUNCTION
//	validation.go     the fixed message templates and ValidationError constructors
//	normalize.go      trims, drops trailing operators/dots/opens, closes parentheses,
//	                  produces the echoed expression, checks nesting depth
//
//	operators.go      the operator table: symbol, kind, precedence, associativity and the
//	                  evaluation function of every binary and postfix operator
//	functions.go      the function table (sqrt)
//	ast.go            the node types the parser builds
//	parser.go         precedence climbing over the operator table; UNEXPECTED_TOKEN,
//	                  UNBALANCED_PARENTHESIS
//
//	eval.go           walks the tree, honours the context, applies the percent-base rule
//	number.go         decimal arithmetic: literals, rounding policy, magnitude cap,
//	                  division, square root, integer and fractional powers, canonical output
//	transcendental.go fixed-point ln and exp on math/big for fractional exponents
//
// Adding an operator or a function is one entry in operators.go or functions.go plus its
// arithmetic in number.go and its test rows; the lexer, parser and evaluator read the
// tables and need no change. Tests mirror the files they cover; calc_test.go holds the
// end-to-end tables over Evaluate, bench_test.go the worst-case corpus.
package calc
