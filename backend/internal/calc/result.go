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
