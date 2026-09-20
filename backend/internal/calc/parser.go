package calc

// parser is a single-pass precedence-climbing parser over the normalized token stream.
// Binary and postfix operators come from the operator table; unary minus, grouping and
// function calls are grammar.
type parser struct {
	tokens []token
	next   int
	ops    operatorTable
	fns    functionTable
	end    token // stands for the end of the original input
}

// parse builds the expression tree for a normalized stream. inputLength is the code-point
// length of the original input, reported as the position of "unexpected end of input".
func parse(tokens []token, ops operatorTable, fns functionTable, inputLength int) (node, error) {
	p := &parser{
		tokens: tokens,
		ops:    ops,
		fns:    fns,
		end:    token{kind: tokenEnd, pos: inputLength, end: inputLength},
	}
	root, err := p.expression(precedenceLowest)
	if err != nil {
		return nil, err
	}
	if tok := p.peek(); tok.kind != tokenEnd {
		if tok.kind == tokenCloseParen {
			return nil, unbalancedParenthesis(tok)
		}
		return nil, unexpectedToken(tok)
	}
	return root, nil
}

// expression parses an operand followed by every operator whose precedence is at least
// minPrecedence.
func (p *parser) expression(minPrecedence int) (node, error) {
	left, err := p.operand()
	if err != nil {
		return nil, err
	}
	for {
		tok := p.peek()
		if tok.kind != tokenOperator {
			return left, nil
		}
		op := p.ops[tok.text]
		if op.precedence < minPrecedence {
			return left, nil
		}
		p.advance()
		if op.kind == kindPostfix {
			left = &postfixNode{op: op, operand: left}
			continue
		}
		nextMinimum := op.precedence + 1
		if op.assoc == assocRight {
			nextMinimum = op.precedence
		}
		right, err := p.expression(nextMinimum)
		if err != nil {
			return nil, err
		}
		left = &binaryNode{op: op, left: left, right: right}
	}
}

// operand parses a number, a parenthesized expression, a function call or a negated
// operand. Anything else, including unary plus and a ")" where a value is expected, is
// UNEXPECTED_TOKEN.
func (p *parser) operand() (node, error) {
	tok := p.advance()
	switch tok.kind {
	case tokenNumber:
		return &numberNode{text: tok.text}, nil
	case tokenOpenParen:
		inner, err := p.expression(precedenceLowest)
		if err != nil {
			return nil, err
		}
		if err := p.expect(tokenCloseParen); err != nil {
			return nil, err
		}
		return &groupNode{inner: inner}, nil
	case tokenIdentifier:
		return p.call(p.fns[tok.text])
	case tokenOperator:
		if tok.text == unaryMinusSymbol {
			operand, err := p.expression(precedenceUnaryMinus)
			if err != nil {
				return nil, err
			}
			return &negateNode{operand: operand}, nil
		}
	case tokenCloseParen, tokenWhitespace, tokenInvalid, tokenEnd:
		// reported below
	}
	return nil, unexpectedToken(tok)
}

// call parses "(" expression ")" after a function name.
func (p *parser) call(fn *function) (node, error) {
	if err := p.expect(tokenOpenParen); err != nil {
		return nil, err
	}
	argument, err := p.expression(precedenceLowest)
	if err != nil {
		return nil, err
	}
	if err := p.expect(tokenCloseParen); err != nil {
		return nil, err
	}
	return &callNode{fn: fn, argument: argument}, nil
}

// expect consumes a token of the given kind or reports it as unexpected.
func (p *parser) expect(kind tokenKind) error {
	tok := p.advance()
	if tok.kind != kind {
		return unexpectedToken(tok)
	}
	return nil
}

func (p *parser) peek() token {
	if p.next < len(p.tokens) {
		return p.tokens[p.next]
	}
	return p.end
}

func (p *parser) advance() token {
	tok := p.peek()
	if p.next < len(p.tokens) {
		p.next++
	}
	return tok
}
