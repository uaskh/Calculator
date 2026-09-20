// Package calc is the calculator domain: it lexes, normalizes, parses and evaluates
// arithmetic expressions with exact decimal arithmetic. It knows nothing about HTTP or
// JSON; the transport layer maps its results and errors to the API contract.
package calc

// Result is a successful evaluation.
type Result struct {
	// Expression is the normalized expression that was evaluated (trimmed, trailing
	// operators dropped, unmatched parentheses closed).
	Expression string
	// Value is the result in canonical decimal form: optional leading "-", digits, and an
	// optional fraction without trailing zeros; never "-0" and never exponent notation.
	Value string
}
