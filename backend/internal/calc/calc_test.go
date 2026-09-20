package calc

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
	"unicode/utf8"
)

// evaluate runs the default calculator with the test context.
func evaluate(t *testing.T, input string) (Result, error) {
	t.Helper()
	return New().Evaluate(t.Context(), input)
}

// wantValue asserts a successful evaluation with the expected canonical value.
func wantValue(t *testing.T, input, want string) {
	t.Helper()
	got, err := evaluate(t, input)
	if err != nil {
		t.Fatalf("Evaluate(%q) returned error %v, want value %q", input, err, want)
	}
	if got.Value != want {
		t.Errorf("Evaluate(%q).Value = %q, want %q", input, got.Value, want)
	}
}

// validationError asserts that input fails with a *ValidationError and returns it.
func validationError(t *testing.T, input string) *ValidationError {
	t.Helper()
	_, err := evaluate(t, input)
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("Evaluate(%q) error = %v, want *ValidationError", input, err)
	}
	return verr
}

// wantValidation asserts a *ValidationError with the given code.
func wantValidation(t *testing.T, input, code string) {
	t.Helper()
	if verr := validationError(t, input); verr.Code != code {
		t.Errorf("Evaluate(%q) code = %s, want %s", input, verr.Code, code)
	}
}

// wantArithmetic asserts a *ArithmeticError with the given code.
func wantArithmetic(t *testing.T, input, code string) {
	t.Helper()
	_, err := evaluate(t, input)
	var aerr *ArithmeticError
	if !errors.As(err, &aerr) {
		t.Fatalf("Evaluate(%q) error = %v, want *ArithmeticError %s", input, err, code)
	}
	if aerr.Code != code {
		t.Errorf("Evaluate(%q) code = %s, want %s", input, aerr.Code, code)
	}
}

// bigPow returns base^exp as a decimal string, computed independently of the calculator.
func bigPow(base, exp int64) string {
	return new(big.Int).Exp(big.NewInt(base), big.NewInt(exp), nil).String()
}

type valueCase struct {
	input string
	want  string
}

func TestEvaluate(t *testing.T) {
	t.Parallel()

	groups := map[string][]valueCase{
		"arithmetic": {
			{"2+3*4", "14"},
			{"(2+3)*4", "20"},
			{"10-4-3", "3"},
			{"8/2/2", "2"},
			{"1+2*3-4/2", "5"},
			{"2*(3+4)", "14"},
			{"((2))", "2"},
			{"3-5", "-2"},
			{"2*3*4", "24"},
			{"100-1", "99"},
		},
		"literals": {
			{"1.5*2", "3"},
			{".5+.5", "1"},
			{"0.1+0.2", "0.3"},
			{"1.10*3", "3.3"},
			{"007", "7"},
			{"00.5", "0.5"},
			{"0", "0"},
			{"0.30", "0.3"},
			{"42", "42"},
			{"0.1*3", "0.3"},
			{"1.1*1.1", "1.21"},
		},
		"division": {
			{"1/3", "0.3333333333333333"},
			{"1/3*3", "1"},
			{"2/3", "0.6666666666666667"},
			{"-2/3", "-0.6666666666666667"},
			{"7/2", "3.5"},
			{"1/8", "0.125"},
			{"10/4", "2.5"},
			{"1/7*7", "1"},
			{"0/5", "0"},
			{"1/-4", "-0.25"},
		},
		"whitespace": {
			{"sqrt (16)", "4"},
			{"2 + 3 * 4", "14"},
			{"\t2\t+\t2", "4"},
			{"  2 + 2  ", "4"},
			{" ( 2 + 3 ) * 4 ", "20"},
			{"2 ^ 3", "8"},
			{"50 %", "0.5"},
			{"sqrt\t(4)", "2"},
		},
		"canonical": {
			{"2^100", "1267650600228229401496703205376"},
			{"0*-1", "0"},
			{"-0.0", "0"},
			{"-0", "0"},
			{"1.500", "1.5"},
			{"100", "100"},
			{"0.000", "0"},
			{"-0.000", "0"},
			{"1000000/1", "1000000"},
			{"0.10+0.20", "0.3"},
			{"-1.50", "-1.5"},
		},
		"unary": {
			{"-3+5", "2"},
			{"2*-3", "-6"},
			{"-(2+3)", "-5"},
			{"--3", "3"},
			{"-2*-3", "6"},
			{"2--3", "5"},
			{"-0.5", "-0.5"},
			{"---3", "-3"},
			{"2^-1", "0.5"},
			{"-sqrt(4)", "-2"},
			{"-2*3", "-6"},
		},
		"power": {
			{"2^10", "1024"},
			{"2^3^2", "512"},
			{"-2^2", "-4"},
			{"(-2)^2", "4"},
			{"2*2^3", "16"},
			{"2^-2^2", "0.0625"},
			{"1.1^2", "1.21"},
			{"2^100", "1267650600228229401496703205376"},
			{"0^0", "1"},
			{"0^2", "0"},
			{"0^0.5", "0"},
			{"2^1", "2"},
			{"2^0", "1"},
			{"1^1000", "1"},
			{"(-1)^1000", "1"},
			{"(-1)^999", "-1"},
			{"(-2)^3", "-8"},
			{"(-8)^2.0", "64"},
			{"10^99", bigPow(10, 99)},
			{"2^300", bigPow(2, 300)},
			{"2^332", bigPow(2, 332)},
			{"1.0001^1000", "1.1051653926032327"},
			{"3^4", "81"},
			{"0.5^2", "0.25"},
			{"2^2^3", "256"},
		},
		"negative_power": {
			{"2^-1", "0.5"},
			{"3^-1", "0.3333333333333333"},
			{"(-2)^-3", "-0.125"},
			{"3^-2", "0.1111111111111111"},
			{"2^-1000", "0"},
			{"10^-100", "0"},
			{"4^-0.5", "0.5"},
			{"2^-0.5", "0.7071067811865475"},
			{"0.5^-1", "2"},
			{"0.5^-2", "4"},
			{"(-0.5)^-1", "-2"},
			{"2^-2", "0.25"},
		},
		"fractional_power": {
			{"2^0.5", "1.414213562373095"},
			{"4^0.5", "2"},
			{"9^0.5", "3"},
			{"8^(1/3)", "2"},
			{"2^2.5", "5.6568542494923802"},
			{"(-8)^2.0", "64"},
			{"2^1.5", "2.8284271247461901"},
			{"1^0.5", "1"},
			{"2^0.25", "1.1892071150027211"},
			{"(10^80)^0.9", "1" + strings.Repeat("0", 72)},
			{"(10^90)^0.9", "1" + strings.Repeat("0", 81)},
			{"(10^99)^0.9", "125892541179416721042395410639580060609361740946693106910792301952664761578250202412105096.6275946170388691"},
			{"(10^99)^0.999", "796159350417318744185341970687172307579007314201602764575008496413321708599304314072090503021624071.6058243083988372"},
		},
		"final_rounding": {
			{strings.Repeat("9", 100) + "+0.9999999999999999", strings.Repeat("9", 100) + "." + strings.Repeat("9", 16)},
			{strings.Repeat("9", 100) + "+0.99999999999999994", strings.Repeat("9", 100) + "." + strings.Repeat("9", 16)},
			{"-" + strings.Repeat("9", 100) + "-0.9999999999999999", "-" + strings.Repeat("9", 100) + "." + strings.Repeat("9", 16)},
		},
		"sqrt": {
			{"sqrt(16)", "4"},
			{"sqrt(2)", "1.414213562373095"},
			{"sqrt(2)*sqrt(2)", "2"},
			{"sqrt(0.25)", "0.5"},
			{"sqrt(0)", "0"},
			{"sqrt(-0)", "0"},
			{"sqrt(sqrt(16))", "2"},
			{"sqrt(2+2)", "2"},
			{"sqrt(16)+1", "5"},
			{"sqrt(0.0001)", "0.01"},
			{"sqrt(1)", "1"},
			{"sqrt(3)", "1.7320508075688773"},
			{"sqrt(10^98)", bigPow(10, 49)},
			{"sqrt(9)^2", "9"},
		},
		"percent": {
			{"50%", "0.5"},
			{"200+10%", "220"},
			{"200-10%", "180"},
			{"50*10%", "5"},
			{"50/10%", "500"},
			{"10%+5", "5.1"},
			{"200+(10%)", "200.1"},
			{"200+10%*2", "200.2"},
			{"2^50%", "1.414213562373095"},
			{"50%%", "0.005"},
			{"100+50%%", "100.5"},
			{"-50%", "-0.5"},
			{"200+-10%", "199.9"},
			{"200--10%", "200.1"},
			{"(200+10)%", "2.1"},
			{"200+(10)%", "220"},
			{"2%^2", "0.0004"},
			{"2^2%", "1.0139594797900291"},
			{"sqrt(16)%", "0.04"},
			{"-200+10%", "-220"},
			{"1+2+10%", "3.3"},
			{"200+10%^2", "200.01"},
			{"2%", "0.02"},
			{"200+sqrt(16)%", "208"},
			{"200*10%+5", "25"},
			{"200-50%-50%", "50"},
			{"10%%+5", "5.001"},
			{"0%", "0"},
			{"200+0%", "200"},
			{"(50%)", "0.5"},
		},
		"rounding": {
			{"0.1^20", "0"},
			{"0.5^1000", "0"},
			{"(0.5^1000)^1000", "0"},
			{"1/10^33*10^33", "0"},
			{"0." + strings.Repeat("0", 32) + "5", "0"},
			{"0." + strings.Repeat("0", 31) + "5", "0"},
			{"-0.00000000000000005", "-0.0000000000000001"},
			{"-0.00000000000000004", "0"},
			{"0.00000000000000005", "0.0000000000000001"},
			{"0.00000000000000004", "0"},
			{"2^-0.5", "0.7071067811865475"},
			{"0.12345678901234565", "0.1234567890123457"},
			{"-0.12345678901234565", "-0.1234567890123457"},
			{"0.12345678901234564", "0.1234567890123456"},
			{"1/10^32", "0"},
			{"1/10^32*10^32", "1"},
			{"1/3+1/3+1/3", "1"},
			{"0.1+0.7", "0.8"},
		},
		"limits": {
			{"10^99", bigPow(10, 99)},
			{"2^300", bigPow(2, 300)},
			{"2^332", bigPow(2, 332)},
			{"1.0001^1000", "1.1051653926032327"},
			{"0" + strings.Repeat("0", 100), "0"},
			{"1" + strings.Repeat("0", 99), bigPow(10, 99)},
			{"99^50", bigPow(99, 50)},
			{"0." + strings.Repeat("0", 1022), "0"},
			{"1" + strings.Repeat(" ", 1023), "1"},
			{"-" + "1" + strings.Repeat("0", 99), "-" + bigPow(10, 99)},
			{"10^99+10^99", "2" + strings.Repeat("0", 99)},
			{"2^1000%", "1024"},
			{strings.Repeat("(", 32) + "1" + strings.Repeat(")", 32), "1"},
			{strings.Repeat("sqrt(", 32) + "1" + strings.Repeat(")", 32), "1"},
		},
	}

	for name, cases := range groups {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, tc := range cases {
				t.Run(tc.input, func(t *testing.T) {
					t.Parallel()
					wantValue(t, tc.input, tc.want)
				})
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input      string
		expression string
		value      string
	}{
		{"3+4*", "3+4", "7"},
		{"3+*", "3", "3"},
		{"2^", "2", "2"},
		{"2 +", "2", "2"},
		{"2*(3+4", "2*(3+4)", "14"},
		{"( 2 + 3", "( 2 + 3)", "5"},
		{"2*(", "2", "2"},
		{"2+sqrt (", "2", "2"},
		{"2sqrt(", "2", "2"},
		{"2.", "2", "2"},
		{"2+.", "2", "2"},
		{"  2 + 2  ", "2 + 2", "4"},
		{"\t2\t", "2", "2"},
		{"sqrt(2", "sqrt(2)", "1.414213562373095"},
		{"((2", "((2))", "2"},
		{"2+(3*(4", "2+(3*(4))", "14"},
		{"2+3-", "2+3", "5"},
		{"2%", "2%", "0.02"},
		{"2 (", "2", "2"},
		{"2 .", "2", "2"},
		{"2*-", "2", "2"},
		{"(2+3)*(", "(2+3)", "5"},
		{"2^-", "2", "2"},
		{"2+3*4-", "2+3*4", "14"},
		{"2/", "2", "2"},
		{"2+(", "2", "2"},
		{"2+sqrt(", "2", "2"},
		{"(2.", "(2)", "2"},
		{"sqrt(2.", "sqrt(2)", "1.414213562373095"},
		{"  (2+3", "(2+3)", "5"},
		{"2*(3+", "2*(3)", "6"},
		{"(2+3", "(2+3)", "5"},
		{"sqrt(sqrt(16", "sqrt(sqrt(16))", "2"},
		{"2+3)", "", ""},
		{"2*(3+4)", "2*(3+4)", "14"},
		{"2 + 3 * ( 4 - 1", "2 + 3 * ( 4 - 1)", "11"},
		{"5%-", "5%", "0.05"},
		{"2^(", "2", "2"},
		{"-(2+3", "-(2+3)", "-5"},
		{strings.Repeat("(", 32) + "1", strings.Repeat("(", 32) + "1" + strings.Repeat(")", 32), "1"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := evaluate(t, tc.input)
			if tc.expression == "" {
				if err == nil {
					t.Fatalf("Evaluate(%q) = %+v, want an error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Evaluate(%q) returned error %v", tc.input, err)
			}
			if got.Expression != tc.expression || got.Value != tc.value {
				t.Errorf("Evaluate(%q) = {%q %q}, want {%q %q}", tc.input, got.Expression, got.Value, tc.expression, tc.value)
			}
		})
	}
}

type validationCase struct {
	name     string
	input    string
	code     string
	position int
	message  string
}

func TestValidation(t *testing.T) {
	t.Parallel()

	noPosition := -1
	groups := map[string][]validationCase{
		"empty": {
			{"empty string", "", CodeEmpty, noPosition, "expression is empty"},
			{"spaces", "   ", CodeEmpty, noPosition, "expression is empty"},
			{"tab", "\t", CodeEmpty, noPosition, "expression is empty"},
			{"plus", "+", CodeEmpty, noPosition, "expression is empty"},
			{"minus", "-", CodeEmpty, noPosition, "expression is empty"},
			{"open paren", "(", CodeEmpty, noPosition, "expression is empty"},
			{"dot", ".", CodeEmpty, noPosition, "expression is empty"},
			{"sqrt paren", "sqrt(", CodeEmpty, noPosition, "expression is empty"},
			{"nested sqrt paren", "sqrt(sqrt(", CodeEmpty, noPosition, "expression is empty"},
			{"minus paren", "-(", CodeEmpty, noPosition, "expression is empty"},
			{"33 bare parens", strings.Repeat("(", 33), CodeEmpty, noPosition, "expression is empty"},
			{"operators only", "+-*/^", CodeEmpty, noPosition, "expression is empty"},
		},
		"unary_plus": {
			{"leading plus", "+3", CodeUnexpectedToken, 0, "unexpected '+' at character 1"},
			{"plus after operator", "2*+3", CodeUnexpectedToken, 2, "unexpected '+' at character 3"},
			{"plus in group", "(+3)", CodeUnexpectedToken, 1, "unexpected '+' at character 2"},
		},
		"unexpected_token": {
			{"two numbers", "2 3", CodeUnexpectedToken, 2, "unexpected '3' at character 3"},
			{"two numbers with leading spaces", "  2 3", CodeUnexpectedToken, 4, "unexpected '3' at character 5"},
			{"one two", "1 2", CodeUnexpectedToken, 2, "unexpected '2' at character 3"},
			{"leading star", "*3", CodeUnexpectedToken, 0, "unexpected '*' at character 1"},
			{"bare sqrt", "sqrt", CodeUnexpectedToken, 4, "unexpected end of input"},
			{"sqrt without parens", "sqrt 16", CodeUnexpectedToken, 5, "unexpected '16' at character 6"},
			{"empty sqrt", "sqrt()", CodeUnexpectedToken, 5, "unexpected ')' at character 6"},
			{"empty group", "2+()", CodeUnexpectedToken, 3, "unexpected ')' at character 4"},
			{"bare empty group", "()", CodeUnexpectedToken, 1, "unexpected ')' at character 2"},
			{"implicit multiplication", "2(3)", CodeUnexpectedToken, 1, "unexpected '(' at character 2"},
			{"implicit sqrt multiplication", "2sqrt(4)", CodeUnexpectedToken, 1, "unexpected 'sqrt' at character 2"},
			{"lone close", ")", CodeUnexpectedToken, 0, "unexpected ')' at character 1"},
			{"close after operator", "2+)", CodeUnexpectedToken, 2, "unexpected ')' at character 3"},
			{"lone percent", "%", CodeUnexpectedToken, 0, "unexpected '%' at character 1"},
			{"percent after operator", "2+%", CodeUnexpectedToken, 2, "unexpected '%' at character 3"},
			{"sqrt with trailing space", "  sqrt ", CodeUnexpectedToken, 7, "unexpected end of input"},
			{"sqrt after number", "2 sqrt", CodeUnexpectedToken, 2, "unexpected 'sqrt' at character 3"},
			{"sqrt inside unclosed group", "(sqrt", CodeUnexpectedToken, 5, "unexpected end of input"},
			{"sqrt followed by number in group", "(sqrt 4)", CodeUnexpectedToken, 6, "unexpected '4' at character 7"},
			{"unary plus in sqrt", "sqrt(+3)", CodeUnexpectedToken, 5, "unexpected '+' at character 6"},
			{"star in sqrt", "sqrt(*3)", CodeUnexpectedToken, 5, "unexpected '*' at character 6"},
			{"two numbers in sqrt", "sqrt(2 3)", CodeUnexpectedToken, 7, "unexpected '3' at character 8"},
			{"operator in group", "(2^)", CodeUnexpectedToken, 3, "unexpected ')' at character 4"},
			{"star after minus", "2-*3", CodeUnexpectedToken, 2, "unexpected '*' at character 3"},
			{"number after group", "(1)2", CodeUnexpectedToken, 3, "unexpected '2' at character 4"},
			{"number after sqrt call", "sqrt(4)2", CodeUnexpectedToken, 7, "unexpected '2' at character 8"},
			{"number after percent", "50%2", CodeUnexpectedToken, 3, "unexpected '2' at character 4"},
			{"sqrt at end after operator", "2+sqrt", CodeUnexpectedToken, 6, "unexpected end of input"},
			{"two numbers in group", "(1 2)", CodeUnexpectedToken, 3, "unexpected '2' at character 4"},
		},
		"unbalanced_parenthesis": {
			{"extra close", "2+3)", CodeUnbalancedParenthesis, 3, "unbalanced ')' at character 4"},
			{"close then open", "2)+(3", CodeUnbalancedParenthesis, 1, "unbalanced ')' at character 2"},
			{"double close", "(2))", CodeUnbalancedParenthesis, 3, "unbalanced ')' at character 4"},
			{"number close", "2)", CodeUnbalancedParenthesis, 1, "unbalanced ')' at character 2"},
			{"percent close", "50%)", CodeUnbalancedParenthesis, 3, "unbalanced ')' at character 4"},
			{"sqrt close", "sqrt(4))", CodeUnbalancedParenthesis, 7, "unbalanced ')' at character 8"},
		},
		"invalid_number": {
			{"three parts", "1.2.3", CodeInvalidNumber, 0, "invalid number '1.2.3' at character 1"},
			{"dot before operator", "2.+3", CodeInvalidNumber, 0, "invalid number '2.' at character 1"},
			{"later token", "2+1.2.3", CodeInvalidNumber, 2, "invalid number '1.2.3' at character 3"},
			{"two dots", "..", CodeInvalidNumber, 0, "invalid number '..' at character 1"},
			{"trailing two dots", "2..", CodeInvalidNumber, 0, "invalid number '2..' at character 1"},
			{"decimal trailing dot", "2.5.", CodeInvalidNumber, 0, "invalid number '2.5.' at character 1"},
			{"dot before dropped operator", "2.+", CodeInvalidNumber, 0, "invalid number '2.' at character 1"},
			{"dot inside group", "(2.)", CodeInvalidNumber, 1, "invalid number '2.' at character 2"},
			{"leading dot trailing dot", ".5.", CodeInvalidNumber, 0, "invalid number '.5.' at character 1"},
			{"dot then space then number", "2. 3", CodeInvalidNumber, 0, "invalid number '2.' at character 1"},
			{"dot before invalid character", "2.$", CodeInvalidNumber, 0, "invalid number '2.' at character 1"},
			{"dot before unknown function", "2.x", CodeInvalidNumber, 0, "invalid number '2.' at character 1"},
			{"dot before percent", "2.%", CodeInvalidNumber, 0, "invalid number '2.' at character 1"},
			{"dot before percent after operator", "234*0.%", CodeInvalidNumber, 4, "invalid number '0.' at character 5"},
			{"invalid before invalid character", "1.2.3+$", CodeInvalidNumber, 0, "invalid number '1.2.3' at character 1"},
			{"lone dot in the middle", "2+.+3", CodeInvalidNumber, 2, "invalid number '.' at character 3"},
		},
		"invalid_character": {
			{"dollar", "2$3", CodeInvalidCharacter, 1, "invalid character '$' at character 2"},
			{"multiplication sign", "2×3", CodeInvalidCharacter, 1, "invalid character '×' at character 2"},
			{"newline", "2\n3", CodeInvalidCharacter, 1, "invalid character '\\n' at character 2"},
			{"carriage return", "2\r3", CodeInvalidCharacter, 1, "invalid character '\\r' at character 2"},
			{"leading multiplication sign", "×2", CodeInvalidCharacter, 0, "invalid character '×' at character 1"},
			{"comma", "1,5", CodeInvalidCharacter, 1, "invalid character ',' at character 2"},
			{"division sign", "2÷3", CodeInvalidCharacter, 1, "invalid character '÷' at character 2"},
			{"minus sign", "2−3", CodeInvalidCharacter, 1, "invalid character '−' at character 2"},
			{"accented letter", "é", CodeInvalidCharacter, 0, "invalid character 'é' at character 1"},
			{"after operator", "2+$", CodeInvalidCharacter, 2, "invalid character '$' at character 3"},
			{"lone dollar", "$", CodeInvalidCharacter, 0, "invalid character '$' at character 1"},
			{"spaced", "2 $ 3", CodeInvalidCharacter, 2, "invalid character '$' at character 3"},
			{"nul", "2\x003", CodeInvalidCharacter, 1, "invalid character '\\x00' at character 2"},
			{"apostrophe", "'", CodeInvalidCharacter, 0, "invalid character ''' at character 1"},
			{"emoji", "😀", CodeInvalidCharacter, 0, "invalid character '😀' at character 1"},
			{"first of several", "2+😀+$", CodeInvalidCharacter, 2, "invalid character '😀' at character 3"},
			{"after multibyte", "é$", CodeInvalidCharacter, 0, "invalid character 'é' at character 1"},
			{"code point index after multibyte", "2×$", CodeInvalidCharacter, 1, "invalid character '×' at character 2"},
			{"non-breaking space", "2 3", CodeInvalidCharacter, 1, "invalid character '\\u00a0' at character 2"},
			{"vertical tab", "2\v3", CodeInvalidCharacter, 1, "invalid character '\\v' at character 2"},
			{"invalid utf8", "2\xff3", CodeInvalidCharacter, 1, "invalid character '�' at character 2"},
			{"equals", "2=3", CodeInvalidCharacter, 1, "invalid character '=' at character 2"},
		},
		"unknown_function": {
			{"foo", "foo(1)", CodeUnknownFunction, 0, "unknown function 'foo' at character 1"},
			{"x after number", "2x", CodeUnknownFunction, 1, "unknown function 'x' at character 2"},
			{"scientific notation", "1e5", CodeUnknownFunction, 1, "unknown function 'e' at character 2"},
			{"sqrt prefix", "sqrtx(4)", CodeUnknownFunction, 0, "unknown function 'sqrtx' at character 1"},
			{"upper case", "SQRT(4)", CodeUnknownFunction, 0, "unknown function 'SQRT' at character 1"},
			{"title case", "Sqrt(4)", CodeUnknownFunction, 0, "unknown function 'Sqrt' at character 1"},
			{"after operator", "2+abc", CodeUnknownFunction, 2, "unknown function 'abc' at character 3"},
			{"before invalid character", "abc$", CodeUnknownFunction, 0, "unknown function 'abc' at character 1"},
			{"sqrt then unknown", "sqrt(x)", CodeUnknownFunction, 5, "unknown function 'x' at character 6"},
		},
		"limits": {
			{"1025 characters", strings.Repeat("1+", 512) + "1", CodeTooLong, noPosition, "expression exceeds 1,024 characters"},
			{"1025 spaces and digit", strings.Repeat(" ", 1024) + "1", CodeTooLong, noPosition, "expression exceeds 1,024 characters"},
			{"1025 multibyte code points", strings.Repeat("é", 1025), CodeTooLong, noPosition, "expression exceeds 1,024 characters"},
			{"1024 multibyte code points", strings.Repeat("é", 1024), CodeInvalidCharacter, 0, "invalid character 'é' at character 1"},
			{"1025 characters with invalid character", strings.Repeat("1", 1024) + "$", CodeTooLong, noPosition, "expression exceeds 1,024 characters"},
			{"33 nested parens", strings.Repeat("(", 33) + "1" + strings.Repeat(")", 33), CodeTooDeep, noPosition, "expression is nested deeper than 32 levels"},
			{"33 nested sqrt", strings.Repeat("sqrt(", 33) + "4" + strings.Repeat(")", 33), CodeTooDeep, noPosition, "expression is nested deeper than 32 levels"},
			{"33 nested unclosed", strings.Repeat("(", 33) + "1", CodeTooDeep, noPosition, "expression is nested deeper than 32 levels"},
			{"33 nested with extra close", strings.Repeat("(", 33) + "1" + strings.Repeat(")", 34), CodeTooDeep, noPosition, "expression is nested deeper than 32 levels"},
			{"33 nested mixed", strings.Repeat("(sqrt(", 17) + "1" + strings.Repeat("))", 17), CodeTooDeep, noPosition, "expression is nested deeper than 32 levels"},
		},
		"order": {
			{"too long beats invalid character", "$" + strings.Repeat("1", 1024), CodeTooLong, noPosition, "expression exceeds 1,024 characters"},
			{"invalid character beats parse error", "2+$", CodeInvalidCharacter, 2, "invalid character '$' at character 3"},
			{"invalid character beats unexpected token", "2 3 $", CodeInvalidCharacter, 4, "invalid character '$' at character 5"},
			{"unknown function beats unexpected token", "2 3 foo", CodeUnknownFunction, 4, "unknown function 'foo' at character 5"},
			{"first offending token wins", "$foo", CodeInvalidCharacter, 0, "invalid character '$' at character 1"},
			{"first offending token wins reversed", "foo$", CodeUnknownFunction, 0, "unknown function 'foo' at character 1"},
			{"empty beats too deep", strings.Repeat("(", 33), CodeEmpty, noPosition, "expression is empty"},
			{"too deep beats unbalanced", strings.Repeat("(", 33) + "1" + strings.Repeat(")", 34), CodeTooDeep, noPosition, "expression is nested deeper than 32 levels"},
			{"too deep beats unexpected token", strings.Repeat("(", 33) + "1 2" + strings.Repeat(")", 33), CodeTooDeep, noPosition, "expression is nested deeper than 32 levels"},
			{"unbalanced beats arithmetic", "1/0)", CodeUnbalancedParenthesis, 3, "unbalanced ')' at character 4"},
			{"unexpected token beats arithmetic", "1/0 2", CodeUnexpectedToken, 4, "unexpected '2' at character 5"},
			{"unexpected token beats large literal", "1" + strings.Repeat("0", 100) + " 2", CodeUnexpectedToken, 102, "unexpected '2' at character 103"},
		},
	}

	for name, cases := range groups {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					verr := validationError(t, tc.input)
					if verr.Code != tc.code {
						t.Errorf("Evaluate(%q) code = %s, want %s", tc.input, verr.Code, tc.code)
					}
					wantHasPosition := tc.position != noPosition
					if verr.HasPosition != wantHasPosition {
						t.Errorf("Evaluate(%q) HasPosition = %v, want %v", tc.input, verr.HasPosition, wantHasPosition)
					}
					if wantHasPosition && verr.Position != tc.position {
						t.Errorf("Evaluate(%q) Position = %d, want %d", tc.input, verr.Position, tc.position)
					}
					if verr.Message != tc.message {
						t.Errorf("Evaluate(%q) Message = %q, want %q", tc.input, verr.Message, tc.message)
					}
					if verr.Error() != tc.message {
						t.Errorf("Evaluate(%q) Error() = %q, want %q", tc.input, verr.Error(), tc.message)
					}
				})
			}
		})
	}
}

func TestValidation_AcceptsLimits(t *testing.T) {
	t.Parallel()

	cases := []valueCase{
		{strings.Repeat("1+", 512), "512"},
		{"1" + strings.Repeat(" ", 1023), "1"},
		{strings.Repeat("(", 32) + "1" + strings.Repeat(")", 32), "1"},
		{strings.Repeat("(", 32) + "1", "1"},
		{strings.Repeat("sqrt(", 32) + "16" + strings.Repeat(")", 32), "1.0000000006455436"},
		{strings.Repeat("(1)+", 33) + "1", "34"},
		{strings.Repeat("(", 32) + "1" + strings.Repeat(")", 32) + "+(1)", "2"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			wantValue(t, tc.input, tc.want)
		})
	}
}

func TestArithmeticErrors(t *testing.T) {
	t.Parallel()

	groups := map[string][]struct {
		input string
		code  string
	}{
		"division_by_zero": {
			{"1/0", CodeDivisionByZero},
			{"0/0", CodeDivisionByZero},
			{"5/0%", CodeDivisionByZero},
			{"0^-1", CodeDivisionByZero},
			{"10^99/10^-99", CodeDivisionByZero},
			{"1/(2-2)", CodeDivisionByZero},
			{"0^-0.5", CodeDivisionByZero},
			{"1/0.0", CodeDivisionByZero},
			{"2/(1-1)^2", CodeDivisionByZero},
			{"1/-0", CodeDivisionByZero},
			{"(1/0)", CodeDivisionByZero},
			{"sqrt(1/0)", CodeDivisionByZero},
			{"1/0%", CodeDivisionByZero},
		},
		"negative_square_root": {
			{"sqrt(-4)", CodeNegativeSquareRoot},
			{"sqrt(-1)", CodeNegativeSquareRoot},
			{"sqrt(2-3)", CodeNegativeSquareRoot},
			{"sqrt(-0.0001)", CodeNegativeSquareRoot},
			{"2+sqrt(-4)", CodeNegativeSquareRoot},
		},
		"invalid_power": {
			{"(-8)^0.5", CodeInvalidPower},
			{"(-8)^(1/3)", CodeInvalidPower},
			{"(-2)^(1/3*3)", CodeInvalidPower},
			{"(-1)^0.5", CodeInvalidPower},
			{"(-8)^-0.5", CodeInvalidPower},
			{"(-2)^2.5", CodeInvalidPower},
			{"(-2)^0.0001", CodeInvalidPower},
		},
		"exponent_too_large": {
			{"1.0001^1001", CodeExponentTooLarge},
			{"2^-1001", CodeExponentTooLarge},
			{"(-8)^1001.5", CodeExponentTooLarge},
			{"0^-1001", CodeExponentTooLarge},
			{"1^1001", CodeExponentTooLarge},
			{"0^1001", CodeExponentTooLarge},
			{"2^1000.5", CodeExponentTooLarge},
			{"2^-1000.5", CodeExponentTooLarge},
			{"2^1000.0001", CodeExponentTooLarge},
			{"2^(10^99)", CodeExponentTooLarge},
		},
		"result_too_large": {
			{"10^100", CodeResultTooLarge},
			{"10^99*10", CodeResultTooLarge},
			{"2^1000", CodeResultTooLarge},
			{"10^100/10^100", CodeResultTooLarge},
			{"1" + strings.Repeat("0", 100), CodeResultTooLarge},
			{"(1.1^1000)^1000", CodeResultTooLarge},
			{"99^999.5", CodeResultTooLarge},
			{"9^999.5", CodeResultTooLarge},
			{"0.5^-1000", CodeResultTooLarge},
			{"2^333", CodeResultTooLarge},
			{"10^99*10^99", CodeResultTooLarge},
			{"-10^100", CodeResultTooLarge},
			{"(-10)^100", CodeResultTooLarge},
			{"(1" + strings.Repeat("0", 99) + ")*10", CodeResultTooLarge},
			{"-" + "1" + strings.Repeat("0", 100), CodeResultTooLarge},
			{"10^99+10^99*9", CodeResultTooLarge},
			{"10^70/10^-30", CodeResultTooLarge},
			{"10^99/0.1", CodeResultTooLarge},
			{"sqrt(10^99)*10^51", CodeResultTooLarge},
			{"1" + strings.Repeat("0", 100) + "%", CodeResultTooLarge},
			{"(10^99)^1.1", CodeResultTooLarge},
			{"(10^50)^2", CodeResultTooLarge},
			{"0.1^-100", CodeResultTooLarge},
			{"(1" + strings.Repeat("0", 100) + ".5)", CodeResultTooLarge},
			{"1" + strings.Repeat("0", 100) + " ", CodeResultTooLarge},
			{strings.Repeat("9", 100) + "+0.99999999999999999", CodeResultTooLarge},
			{strings.Repeat("9", 100) + "+0.99999999999999995", CodeResultTooLarge},
			{"-" + strings.Repeat("9", 100) + "-0.99999999999999999", CodeResultTooLarge},
		},
		"order": {
			{"(-8)^1001.5", CodeExponentTooLarge},
			{"0^-1001", CodeExponentTooLarge},
			{"0^-1", CodeDivisionByZero},
			{"(-8)^-0.5", CodeInvalidPower},
			{"(-10)^1001", CodeExponentTooLarge},
			{"0^-0.5", CodeDivisionByZero},
			{"(-10)^100.5", CodeInvalidPower},
			{"1/0+sqrt(-1)", CodeDivisionByZero},
			{"sqrt(-1)/0", CodeNegativeSquareRoot},
			{"10^100+1/0", CodeResultTooLarge},
			{"(1/0)^1001", CodeDivisionByZero},
			{"2^(1/0)", CodeDivisionByZero},
		},
	}

	for name, cases := range groups {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, tc := range cases {
				t.Run(tc.input, func(t *testing.T) {
					t.Parallel()
					wantArithmetic(t, tc.input, tc.code)
				})
			}
		})
	}
}

// TestFractionalPower_HalfMatchesSquareRoot proves x^0.5 and sqrt(x) agree on the largest
// bases the calculator accepts, where the exp/ln path used to lose significant digits.
func TestFractionalPower_HalfMatchesSquareRoot(t *testing.T) {
	t.Parallel()

	for _, base := range []string{"10^99", "10^98", "10^90", "9" + strings.Repeat("9", 98)} {
		t.Run(base, func(t *testing.T) {
			t.Parallel()
			power, err := evaluate(t, "("+base+")^0.5")
			if err != nil {
				t.Fatalf("Evaluate((%s)^0.5) returned error %v", base, err)
			}
			root, err := evaluate(t, "sqrt("+base+")")
			if err != nil {
				t.Fatalf("Evaluate(sqrt(%s)) returned error %v", base, err)
			}
			if power.Value != root.Value {
				t.Errorf("Evaluate((%s)^0.5) = %q, Evaluate(sqrt(%s)) = %q", base, power.Value, base, root.Value)
			}
		})
	}
}

func TestArithmeticError_Error(t *testing.T) {
	t.Parallel()

	_, err := evaluate(t, "1/0")
	if got, want := err.Error(), "cannot evaluate expression: DIVISION_BY_ZERO"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestEvaluate_ContextCancelled(t *testing.T) {
	t.Parallel()

	t.Run("cancelled", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := New().Evaluate(ctx, "1+1")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Evaluate with cancelled ctx = %v, want context.Canceled", err)
		}
		var verr *ValidationError
		var aerr *ArithmeticError
		if errors.As(err, &verr) || errors.As(err, &aerr) {
			t.Errorf("Evaluate with cancelled ctx returned a domain error %v", err)
		}
	})

	t.Run("deadline exceeded", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(t.Context(), 0)
		defer cancel()
		<-ctx.Done()
		_, err := New().Evaluate(ctx, "2^1000")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Evaluate with expired ctx = %v, want context.DeadlineExceeded", err)
		}
	})

	t.Run("validation errors are reported before the context is consulted", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := New().Evaluate(ctx, "1 2")
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("Evaluate(%q) with cancelled ctx = %v, want *ValidationError", "1 2", err)
		}
	})
}

func TestOperatorTable(t *testing.T) {
	t.Parallel()

	ops := defaultOperators()
	ops.register(&operator{
		symbol:     "#",
		kind:       kindBinary,
		precedence: precedenceMultiplicative,
		assoc:      assocLeft,
		binary:     (*numbers).concatDigits,
	})
	ops.register(&operator{
		symbol:     "@",
		kind:       kindBinary,
		precedence: precedenceAdditive,
		assoc:      assocLeft,
		binary:     (*numbers).concatDigits,
	})
	ops.register(&operator{
		symbol:     "!",
		kind:       kindPostfix,
		precedence: precedencePostfix,
		postfix:    (*numbers).double,
	})
	fns := defaultFunctions()
	fns.register(&function{name: "cube", evaluate: (*numbers).cube})
	extended := newCalculator(ops, fns)

	cases := []struct {
		input      string
		expression string
		value      string
	}{
		{"1#2", "1#2", "12"},
		{"1#2#3", "1#2#3", "123"},
		{"1+2#3", "1+2#3", "24"},
		{"1#2*3", "1#2*3", "36"},
		{"2^1#2", "2^1#2", "22"},
		{"2#", "2", "2"},
		{"3!", "3!", "6"},
		{"3!!", "3!!", "12"},
		{"2+3!", "2+3!", "8"},
		{"cube(2)", "cube(2)", "8"},
		{"cube(2)!", "cube(2)!", "16"},
		{"cube(", "", ""},
		{"1#cube(2)", "1#cube(2)", "18"},
		// The percent entry owns its relative rule: any additive-precedence binary operator
		// gets "base*right/100" as its direct right operand, a multiplicative one does not.
		{"200@10%", "200@10%", "2020"},
		{"200#10%", "200#10%", "2000.1"},
		{"200@(10%)", "200@(10%)", "2000.1"},
		{"200@10!", "200@10!", "2020"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := extended.Evaluate(t.Context(), tc.input)
			if tc.expression == "" {
				var verr *ValidationError
				if !errors.As(err, &verr) || verr.Code != CodeEmpty {
					t.Fatalf("extended.Evaluate(%q) = %+v, %v, want EMPTY", tc.input, got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("extended.Evaluate(%q) returned error %v", tc.input, err)
			}
			if got.Expression != tc.expression || got.Value != tc.value {
				t.Errorf("extended.Evaluate(%q) = {%q %q}, want {%q %q}", tc.input, got.Expression, got.Value, tc.expression, tc.value)
			}
		})
	}

	t.Run("default calculator does not know the extra entries", func(t *testing.T) {
		t.Parallel()
		verr := validationError(t, "1#2")
		if verr.Code != CodeInvalidCharacter || verr.Position != 1 {
			t.Errorf("Evaluate(\"1#2\") = %s at %d, want %s at 1", verr.Code, verr.Position, CodeInvalidCharacter)
		}
		wantValidation(t, "3!", CodeInvalidCharacter)
		wantValidation(t, "cube(2)", CodeUnknownFunction)
	})

	t.Run("registering a duplicate symbol panics", func(t *testing.T) {
		t.Parallel()
		defer func() {
			if recover() == nil {
				t.Error("register(duplicate) did not panic")
			}
		}()
		defaultOperators().register(&operator{symbol: "+", kind: kindBinary, binary: (*numbers).concatDigits})
	})

	t.Run("registering a duplicate function panics", func(t *testing.T) {
		t.Parallel()
		defer func() {
			if recover() == nil {
				t.Error("register(duplicate) did not panic")
			}
		}()
		defaultFunctions().register(&function{name: "sqrt", evaluate: (*numbers).cube})
	})
}

// integerDigitsOf counts the digits before the decimal point of a canonical value.
func integerDigitsOf(value string) int {
	whole, _, _ := strings.Cut(strings.TrimPrefix(value, "-"), ".")
	return len(whole)
}

func FuzzEvaluate(f *testing.F) {
	seeds := []string{
		"2+3*4", "(2+3)*4", "1/3*3", "sqrt (16)", "1 2", "  2 + 2  ", "2^100", "0*-1", "-0.0",
		"--3", "2^-2^2", "(-8)^(1/3)", "sqrt(2)*sqrt(2)", "200+10%", "100+50%%", "2^2%",
		"3+4*", "2*(3+4", "sqrt(", "2.", "2+.", ".", "2..", "2+()", "2+3)", "1.2.3", "2$3",
		"sqrt", "foo(1)", "1/0", "0^-1", "10^100", "2^1000", "1.0001^1001", "0.5^-1000",
		"(1.1^1000)^1000", "99^999.5", "-0.00000000000000005", "1" + strings.Repeat("0", 100),
		strings.Repeat("(", 33) + "1" + strings.Repeat(")", 33), strings.Repeat("(", 33),
		"2×3", "2\n3", "é", "😀", "1e5", "2^0.5", "9^999.5", "1/10^33*10^33", "50%%", "2%^2",
		strings.Repeat("9", 100) + "+0.99999999999999999", "(10^99)^0.999",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	calc := New()
	f.Fuzz(func(t *testing.T, input string) {
		got, err := calc.Evaluate(t.Context(), input)
		if err != nil {
			var verr *ValidationError
			var aerr *ArithmeticError
			switch {
			case errors.As(err, &verr):
				if verr.HasPosition && (verr.Position < 0 || verr.Position > utf8.RuneCountInString(input)) {
					t.Errorf("Evaluate(%q) Position = %d, outside the input", input, verr.Position)
				}
				if verr.Message == "" {
					t.Errorf("Evaluate(%q) returned an empty validation message", input)
				}
			case errors.As(err, &aerr):
				if aerr.Code == "" {
					t.Errorf("Evaluate(%q) returned an arithmetic error without a code", input)
				}
			default:
				t.Errorf("Evaluate(%q) returned an unexpected error type: %v", input, err)
			}
			return
		}
		if !canonicalValue.MatchString(got.Value) {
			t.Errorf("Evaluate(%q).Value = %q is not canonical", input, got.Value)
		}
		if len(got.Value) > maxValueLength {
			t.Errorf("Evaluate(%q).Value has %d bytes, want at most %d", input, len(got.Value), maxValueLength)
		}
		if digits := integerDigitsOf(got.Value); digits > MaxIntegerDigits {
			t.Errorf("Evaluate(%q).Value = %q has %d integer digits, want at most %d", input, got.Value, digits, MaxIntegerDigits)
		}
		if got.Expression == "" {
			t.Errorf("Evaluate(%q) returned an empty normalized expression", input)
		}
		if utf8.RuneCountInString(got.Expression) > MaxLength {
			return // closing parentheses can push a maximal input over the length limit
		}
		again, err := calc.Evaluate(t.Context(), got.Expression)
		if err != nil {
			t.Fatalf("Evaluate(%q) (normalized from %q) returned error %v", got.Expression, input, err)
		}
		if again != got {
			t.Errorf("Evaluate(%q) (normalized from %q) = %+v, want %+v", got.Expression, input, again, got)
		}
	})
}
