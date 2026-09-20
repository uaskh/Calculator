package calc

import "strings"

// lex splits the input into tokens and reports the first offending token in input order
// as a *ValidationError: INVALID_CHARACTER for a code point outside the language,
// INVALID_NUMBER for a number with two dots or with a trailing dot that is not the last
// token (decision D-2), UNKNOWN_FUNCTION for an identifier missing from the function
// table. Whitespace tokens are kept so that normalization can trim the ends and reproduce
// the original spacing.
func lex(input []rune, ops operatorTable, fns functionTable) ([]token, error) {
	tokens := make([]token, 0, len(input))
	dangling := token{pos: -1} // a number ending in "." that must remain the last token
	for i := 0; i < len(input); {
		tok := scanToken(input, i, ops)
		i = tok.end
		if tok.kind == tokenWhitespace {
			tokens = append(tokens, tok)
			continue
		}
		if dangling.pos >= 0 {
			return nil, invalidNumber(dangling.text, dangling.pos)
		}
		switch tok.kind {
		case tokenInvalid:
			return nil, invalidCharacter(input[tok.pos], tok.pos)
		case tokenNumber:
			if strings.Count(tok.text, ".") > 1 {
				return nil, invalidNumber(tok.text, tok.pos)
			}
			if strings.HasSuffix(tok.text, ".") {
				dangling = tok
			}
		case tokenIdentifier:
			if _, ok := fns[tok.text]; !ok {
				return nil, unknownFunction(tok.text, tok.pos)
			}
		case tokenOperator, tokenOpenParen, tokenCloseParen, tokenWhitespace, tokenEnd:
			// structural tokens need no further checks
		}
		tokens = append(tokens, tok)
	}
	return tokens, nil
}

// scanToken reads the token starting at code-point index start.
func scanToken(input []rune, start int, ops operatorTable) token {
	r := input[start]
	switch {
	case isSpace(r):
		return spanToken(tokenWhitespace, input, start, isSpace)
	case isDigit(r) || r == '.':
		return spanToken(tokenNumber, input, start, func(r rune) bool { return isDigit(r) || r == '.' })
	case isLetter(r):
		return spanToken(tokenIdentifier, input, start, isLetter)
	case r == '(':
		return token{kind: tokenOpenParen, text: "(", pos: start, end: start + 1}
	case r == ')':
		return token{kind: tokenCloseParen, text: ")", pos: start, end: start + 1}
	case ops.hasSymbol(r):
		return token{kind: tokenOperator, text: string(r), pos: start, end: start + 1}
	default:
		return token{kind: tokenInvalid, text: string(r), pos: start, end: start + 1}
	}
}

// spanToken reads the maximal run of code points accepted by keep, starting at start.
func spanToken(kind tokenKind, input []rune, start int, keep func(rune) bool) token {
	end := start
	for end < len(input) && keep(input[end]) {
		end++
	}
	return token{kind: kind, text: string(input[start:end]), pos: start, end: end}
}

func isSpace(r rune) bool  { return r == ' ' || r == '\t' }
func isDigit(r rune) bool  { return r >= '0' && r <= '9' }
func isLetter(r rune) bool { return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') }
