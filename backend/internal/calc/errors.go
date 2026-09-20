package calc

// Validation codes: the expression cannot be evaluated as written. The transport layer
// reports them as 400 field errors on "expression".
const (
	CodeEmpty                 = "EMPTY"
	CodeTooLong               = "TOO_LONG"
	CodeTooDeep               = "TOO_DEEP"
	CodeInvalidCharacter      = "INVALID_CHARACTER"
	CodeInvalidNumber         = "INVALID_NUMBER"
	CodeUnexpectedToken       = "UNEXPECTED_TOKEN"
	CodeUnbalancedParenthesis = "UNBALANCED_PARENTHESIS"
	CodeUnknownFunction       = "UNKNOWN_FUNCTION"
)

// Arithmetic codes: the expression is well formed but its value cannot be computed. The
// transport layer reports them as 422 problems.
const (
	CodeDivisionByZero     = "DIVISION_BY_ZERO"
	CodeNegativeSquareRoot = "NEGATIVE_SQUARE_ROOT"
	CodeInvalidPower       = "INVALID_POWER"
	CodeExponentTooLarge   = "EXPONENT_TOO_LARGE"
	CodeResultTooLarge     = "RESULT_TOO_LARGE"
)

// ValidationError describes why an expression could not be parsed. Position is the
// 0-based code-point index into the original, untrimmed input and is only meaningful when
// HasPosition is true (EMPTY, TOO_LONG and TOO_DEEP carry no position). Message follows
// the fixed templates of the specification and is safe to show to users.
type ValidationError struct {
	Code        string
	Position    int
	HasPosition bool
	Message     string
}

// Error implements the error interface.
func (e *ValidationError) Error() string { return e.Message }

// ArithmeticError describes a well-formed expression whose value cannot be computed, for
// example a division by zero or a result beyond the magnitude limit.
type ArithmeticError struct {
	Code string
}

// Error implements the error interface.
func (e *ArithmeticError) Error() string { return "cannot evaluate expression: " + e.Code }
