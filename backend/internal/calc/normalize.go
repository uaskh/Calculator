package calc

import (
	"slices"
	"strings"
)

// normalized is the outcome of lenient normalization: the expression text that is
// evaluated and returned to the caller, and the parser's token stream (whitespace removed,
// unmatched parentheses closed with synthetic tokens).
type normalized struct {
	expression string
	tokens     []token
}

// normalize applies the specification's repair rules to the token stream: trim leading
// and trailing whitespace; repeatedly drop a trailing binary operator, a trailing "."
// (lone or ending a number), or a trailing "(" together with a preceding function name;
// then close every unmatched "(". The expression text is cut from the original input so
// interior spacing is preserved. An input with nothing left is EMPTY.
func normalize(input []rune, tokens []token, ops operatorTable) (normalized, error) {
	toks := dropTrailing(trimWhitespace(slices.Clone(tokens)), ops)
	if len(toks) == 0 {
		return normalized{}, emptyExpression()
	}

	open := unmatchedOpens(toks)
	first, last := toks[0], toks[len(toks)-1]
	expression := string(input[first.pos:last.end]) + strings.Repeat(")", open)

	stream := make([]token, 0, len(toks)+open)
	for _, tok := range toks {
		if tok.kind != tokenWhitespace {
			stream = append(stream, tok)
		}
	}
	closer := token{kind: tokenCloseParen, text: ")", pos: len(input), end: len(input), synthetic: true}
	for range open {
		stream = append(stream, closer)
	}
	return normalized{expression: expression, tokens: stream}, nil
}

// dropTrailing removes droppable tokens from the end of the stream until the last token
// can end an expression.
func dropTrailing(toks []token, ops operatorTable) []token {
	for len(toks) > 0 {
		n := len(toks)
		last := toks[n-1]
		switch {
		case last.kind == tokenOperator && ops[last.text].kind == kindBinary:
			toks = toks[:n-1]
		case last.kind == tokenNumber && last.text == ".":
			toks = toks[:n-1]
		case last.kind == tokenNumber && strings.HasSuffix(last.text, "."):
			toks[n-1].text = strings.TrimSuffix(last.text, ".")
			toks[n-1].end--
		case last.kind == tokenOpenParen:
			toks = trimTrailingWhitespace(toks[:n-1])
			if m := len(toks); m > 0 && toks[m-1].kind == tokenIdentifier {
				toks = toks[:m-1]
			}
		default:
			return toks
		}
		toks = trimTrailingWhitespace(toks)
	}
	return toks
}

func trimWhitespace(toks []token) []token {
	for len(toks) > 0 && toks[0].kind == tokenWhitespace {
		toks = toks[1:]
	}
	return trimTrailingWhitespace(toks)
}

func trimTrailingWhitespace(toks []token) []token {
	for len(toks) > 0 && toks[len(toks)-1].kind == tokenWhitespace {
		toks = toks[:len(toks)-1]
	}
	return toks
}

// unmatchedOpens counts the "(" tokens left open at the end of the stream. A ")" without
// an open group is ignored here; the parser reports it as UNBALANCED_PARENTHESIS.
func unmatchedOpens(toks []token) int {
	depth := 0
	for _, tok := range toks {
		switch tok.kind {
		case tokenOpenParen:
			depth++
		case tokenCloseParen:
			if depth > 0 {
				depth--
			}
		case tokenNumber, tokenIdentifier, tokenOperator, tokenWhitespace, tokenInvalid, tokenEnd:
			// not a parenthesis
		}
	}
	return depth
}

// checkDepth rejects a normalized stream whose parentheses nest deeper than MaxDepth.
// Function calls count through their "(".
func checkDepth(toks []token) error {
	depth, deepest := 0, 0
	for _, tok := range toks {
		switch tok.kind {
		case tokenOpenParen:
			depth++
			deepest = max(deepest, depth)
		case tokenCloseParen:
			if depth > 0 {
				depth--
			}
		case tokenNumber, tokenIdentifier, tokenOperator, tokenWhitespace, tokenInvalid, tokenEnd:
			// not a parenthesis
		}
	}
	if deepest > MaxDepth {
		return tooDeep()
	}
	return nil
}
