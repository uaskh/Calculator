package calc

// tokenKind classifies a lexeme.
type tokenKind int

const (
	tokenNumber     tokenKind = iota // maximal run of ASCII digits and dots
	tokenIdentifier                  // maximal run of ASCII letters; a registered function name once lexed
	tokenOperator                    // a symbol registered in the operator table
	tokenOpenParen                   // "("
	tokenCloseParen                  // ")"
	tokenWhitespace                  // ASCII spaces and tabs; removed before parsing
	tokenInvalid                     // a code point that belongs to no token; never leaves the lexer
	tokenEnd                         // end of the stream; produced by the parser, never stored
)

// token is a lexeme together with its code-point span [pos, end) in the original input.
// Synthetic tokens are the closing parentheses appended by normalization; they carry the
// input length as their position and are reported as "end of input" when unexpected.
type token struct {
	kind      tokenKind
	text      string
	pos       int
	end       int
	synthetic bool
}

// atEnd reports whether the token stands for the end of the original input.
func (t token) atEnd() bool { return t.kind == tokenEnd || t.synthetic }
