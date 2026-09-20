package calc

import (
	"fmt"
	"strconv"
	"strings"
)

// Constructors for the domain errors. Messages follow the fixed templates of the
// specification (section 6): positions are 0-based code-point indexes into the original
// input, and the message quotes them 1-based as "character N".

func emptyExpression() *ValidationError {
	return &ValidationError{Code: CodeEmpty, Message: "expression is empty"}
}

func tooLong() *ValidationError {
	return &ValidationError{Code: CodeTooLong, Message: fmt.Sprintf("expression exceeds %s characters", groupThousands(MaxLength))}
}

func tooDeep() *ValidationError {
	return &ValidationError{Code: CodeTooDeep, Message: fmt.Sprintf("expression is nested deeper than %d levels", MaxDepth)}
}

func invalidCharacter(r rune, pos int) *ValidationError {
	return positioned(CodeInvalidCharacter, pos, fmt.Sprintf("invalid character %s at character %d", quoteRune(r), pos+1))
}

// invalidNumber quotes the whole number token so the reader sees what is wrong even
// though the position points at the token's first character (spec decision 39).
func invalidNumber(text string, pos int) *ValidationError {
	return positioned(CodeInvalidNumber, pos, fmt.Sprintf("invalid number '%s' at character %d", text, pos+1))
}

func unknownFunction(name string, pos int) *ValidationError {
	return positioned(CodeUnknownFunction, pos, fmt.Sprintf("unknown function '%s' at character %d", name, pos+1))
}

func unexpectedToken(tok token) *ValidationError {
	if tok.atEnd() {
		return positioned(CodeUnexpectedToken, tok.pos, "unexpected end of input")
	}
	return positioned(CodeUnexpectedToken, tok.pos, fmt.Sprintf("unexpected '%s' at character %d", tok.text, tok.pos+1))
}

func unbalancedParenthesis(tok token) *ValidationError {
	return positioned(CodeUnbalancedParenthesis, tok.pos, fmt.Sprintf("unbalanced ')' at character %d", tok.pos+1))
}

func positioned(code string, pos int, message string) *ValidationError {
	return &ValidationError{Code: code, Position: pos, HasPosition: true, Message: message}
}

func arithmetic(code string) *ArithmeticError {
	return &ArithmeticError{Code: code}
}

// quoteRune shows printable characters verbatim between single quotes and Go-quotes
// control and other non-printable characters ('\n', '\x00', ' ').
func quoteRune(r rune) string {
	if strconv.IsPrint(r) {
		return "'" + string(r) + "'"
	}
	return strconv.QuoteRune(r)
}

// groupThousands formats a non-negative integer with comma separators (1024 -> "1,024").
func groupThousands(n int) string {
	digits := strconv.Itoa(n)
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	return b.String()
}
