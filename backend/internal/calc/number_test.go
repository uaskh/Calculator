package calc

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func mustDecimal(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(s)
	if err != nil {
		t.Fatalf("NewFromString(%q): %v", s, err)
	}
	return d
}

func TestRounding_HalfAwayFromZero(t *testing.T) {
	t.Parallel()

	cases := []struct {
		value  string
		places int32
		want   string
	}{
		{"0.5", 0, "1"},
		{"-0.5", 0, "-1"},
		{"1.5", 0, "2"},
		{"2.5", 0, "3"},
		{"-2.5", 0, "-3"},
		{"0.4", 0, "0"},
		{"-0.4", 0, "0"},
		{"0.00000000000000005", ResultPlaces, "0.0000000000000001"},
		{"-0.00000000000000005", ResultPlaces, "-0.0000000000000001"},
		{"0.00000000000000004", ResultPlaces, "0"},
		{"-0.00000000000000004", ResultPlaces, "0"},
		{"0.12345678901234565", ResultPlaces, "0.1234567890123457"},
		{"0." + strings.Repeat("0", 32) + "5", IntermediatePlaces, "0." + strings.Repeat("0", 31) + "1"},
		{"-0." + strings.Repeat("0", 32) + "5", IntermediatePlaces, "-0." + strings.Repeat("0", 31) + "1"},
		{"0." + strings.Repeat("0", 32) + "4", IntermediatePlaces, "0"},
		{"1.5", ResultPlaces, "1.5"},
		{"123", IntermediatePlaces, "123"},
		{"-0.0", ResultPlaces, "0"},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			t.Parallel()
			got := roundHalfAwayFromZero(mustDecimal(t, tc.value), tc.places)
			if got.String() != tc.want {
				t.Errorf("roundHalfAwayFromZero(%s, %d) = %s, want %s", tc.value, tc.places, got.String(), tc.want)
			}
			if got.Exponent() < -tc.places {
				t.Errorf("roundHalfAwayFromZero(%s, %d) kept %d decimal places", tc.value, tc.places, -got.Exponent())
			}
		})
	}
}

func TestIntegerDigits(t *testing.T) {
	t.Parallel()

	cases := []struct {
		value string
		want  int
	}{
		{"0", 1},
		{"0.5", 1},
		{"1", 1},
		{"9.99", 1},
		{"10", 2},
		{"-123.4", 3},
		{"1" + strings.Repeat("0", 99), 100},
		{"0.000000000000000000000000000000001", 1},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			t.Parallel()
			if got := integerDigits(mustDecimal(t, tc.value)); got != tc.want {
				t.Errorf("integerDigits(%s) = %d, want %d", tc.value, got, tc.want)
			}
		})
	}
}

func TestPow_ContextCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	num := newNumbers()
	for _, exponent := range []string{"1000", "999.5", "-1000"} {
		_, err := num.pow(ctx, decimal.NewFromInt(2), mustDecimal(t, exponent))
		if !errors.Is(err, context.Canceled) {
			t.Errorf("pow(cancelled, 2, %s) = %v, want context.Canceled", exponent, err)
		}
	}
}

func TestSqrt_ScaledCoefficient(t *testing.T) {
	t.Parallel()

	num := newNumbers()
	cases := []struct {
		value decimal.Decimal
		want  string
	}{
		{decimal.New(1, -150), "0"},
		{decimal.New(1, -64), "0." + strings.Repeat("0", 31) + "1"},
		{decimal.New(1, 98), "1" + strings.Repeat("0", 49)},
		{decimal.New(4, 2), "20"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			got, err := num.sqrt(tc.value)
			if err != nil {
				t.Fatalf("sqrt(%s) returned error %v", tc.value, err)
			}
			if got.String() != tc.want {
				t.Errorf("sqrt(%s) = %s, want %s", tc.value, got.String(), tc.want)
			}
		})
	}
}

func TestLiteral_Malformed(t *testing.T) {
	t.Parallel()

	_, err := newNumbers().literal("1.2.3")
	if err == nil {
		t.Fatal("literal(\"1.2.3\") returned no error")
	}
	var verr *ValidationError
	var aerr *ArithmeticError
	if errors.As(err, &verr) || errors.As(err, &aerr) {
		t.Errorf("literal(\"1.2.3\") returned a domain error %v, want an internal error", err)
	}
}

func TestCanonical(t *testing.T) {
	t.Parallel()

	num := newNumbers()
	cases := []struct {
		value string
		want  string
	}{
		{"0", "0"},
		{"-0.0", "0"},
		{"1.500", "1.5"},
		{"-0.00000000000000004", "0"},
		{"-0.00000000000000005", "-0.0000000000000001"},
		{"1267650600228229401496703205376", "1267650600228229401496703205376"},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			t.Parallel()
			if got := num.canonical(mustDecimal(t, tc.value)); got != tc.want {
				t.Errorf("canonical(%s) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// fourthRoot computes x^(1/4) independently of exp/ln: two integer square roots of the
// coefficient scaled to 4*guardPlaces, then the usual 32-place rounding.
func fourthRoot(t *testing.T, x decimal.Decimal) decimal.Decimal {
	t.Helper()
	scaled := scaleCoefficient(x, 4*guardPlaces)
	root := scaled.Sqrt(scaled)
	root.Sqrt(root)
	return roundHalfAwayFromZero(decimal.NewFromBigInt(root, -guardPlaces), IntermediatePlaces)
}

// TestFractionalPower_MatchesSquareRoot checks x^0.5 and x^0.25 against integer square
// roots, an independent computation, at the full 32 intermediate places.
func TestFractionalPower_MatchesSquareRoot(t *testing.T) {
	t.Parallel()

	num := newNumbers()
	half := mustDecimal(t, "0.5")
	quarter := mustDecimal(t, "0.25")
	for _, base := range []string{"2", "3", "10", "0.5", "0.001", "123456.789", "1.0001", "99", "1" + strings.Repeat("0", 60), "0." + strings.Repeat("0", 20) + "7"} {
		t.Run(base, func(t *testing.T) {
			t.Parallel()
			x := mustDecimal(t, base)
			root, err := num.sqrt(x)
			if err != nil {
				t.Fatalf("sqrt(%s): %v", base, err)
			}
			got, err := num.pow(t.Context(), x, half)
			if err != nil {
				t.Fatalf("pow(%s, 0.5): %v", base, err)
			}
			if !got.Equal(root) {
				t.Errorf("pow(%s, 0.5) = %s, sqrt = %s", base, got, root)
			}
			fourth := fourthRoot(t, x)
			got, err = num.pow(t.Context(), x, quarter)
			if err != nil {
				t.Fatalf("pow(%s, 0.25): %v", base, err)
			}
			if !got.Equal(fourth) {
				t.Errorf("pow(%s, 0.25) = %s, sqrt(sqrt) = %s", base, got, fourth)
			}
		})
	}
}

// TestFractionalPower_MatchesLibrary compares the in-package exp/ln power with the
// library's PowWithPrecision at 32 places. The library call is not safe for concurrent use
// (it appends to a package-level cache), so this test is deliberately not parallel.
func TestFractionalPower_MatchesLibrary(t *testing.T) {
	num := newNumbers()
	cases := []struct{ base, exponent string }{
		{"2", "0.5"}, {"2", "0.02"}, {"2", "0.999"}, {"9", "0.5"}, {"1.0001", "0.5"},
		{"0.25", "0.5"}, {"0.5", "0.5"}, {"7", "0.123456789"}, {"1.5", "0.75"},
		{"10", "0.3"}, {"123.456", "0.111"}, {"0.001", "0.9"}, {"1" + strings.Repeat("0", 50), "0.5"},
		{"3", "0.0000000000000000000000000000001"}, {"2", "0.5000000000000000000000000000000001"},
	}
	for _, tc := range cases {
		t.Run(tc.base+"^"+tc.exponent, func(t *testing.T) {
			x, f := mustDecimal(t, tc.base), mustDecimal(t, tc.exponent)
			got, err := num.pow(context.Background(), x, f)
			if err != nil {
				t.Fatalf("pow(%s, %s): %v", tc.base, tc.exponent, err)
			}
			want, err := x.PowWithPrecision(f, guardPlaces)
			if err != nil {
				t.Fatalf("PowWithPrecision(%s, %s): %v", tc.base, tc.exponent, err)
			}
			want = want.Round(IntermediatePlaces)
			if !got.Equal(want) {
				t.Errorf("pow(%s, %s) = %s, library = %s", tc.base, tc.exponent, got, want)
			}
		})
	}
}

func TestFixedPoint_Ln2(t *testing.T) {
	t.Parallel()

	// ln 2 = 0.693147180559945309417232121458176568075500134360255254120680009493393621969694715605863326996418687...
	const ln2 = "0.6931471805599453094172321214581765680755001343602552541206800094933936219696947156058633269964186875"
	fp := newFixedPoint(guardPlaces)
	got := fp.toDecimal(fp.ln2).Truncate(guardPlaces).String()
	want := mustDecimal(t, ln2).Truncate(guardPlaces).String()
	if got != want {
		t.Errorf("ln2 = %s, want %s", got, want)
	}
	if got := fp.toDecimal(fp.exp(fp.ln2)).Round(IntermediatePlaces).String(); got != "2" {
		t.Errorf("exp(ln 2) = %s, want 2", got)
	}
	if got := fp.toDecimal(fp.exp(big.NewInt(0))).String(); got != "1" {
		t.Errorf("exp(0) = %s, want 1", got)
	}
	if got := fp.toDecimal(fp.ln(fp.one)).String(); got != "0" {
		t.Errorf("ln(1) = %s, want 0", got)
	}
	// exp(-ln 2) = 0.5 exercises the reciprocal branch.
	if got := fp.toDecimal(fp.exp(new(big.Int).Neg(fp.ln2))).Round(IntermediatePlaces).String(); got != "0.5" {
		t.Errorf("exp(-ln 2) = %s, want 0.5", got)
	}
}

func TestFixedPoint_FromDecimal(t *testing.T) {
	t.Parallel()

	fp := newFixedPoint(guardPlaces)
	cases := []struct{ value, want string }{
		{"-1.5", "-1.5"},
		{"0.25", "0.25"},
		{"0", "0"},
		{"-0." + strings.Repeat("0", 100) + "1", "0"}, // beyond the working scale: truncated
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			t.Parallel()
			if got := fp.toDecimal(fp.fromDecimal(mustDecimal(t, tc.value))).String(); got != tc.want {
				t.Errorf("fromDecimal(%s) round-trips to %s, want %s", tc.value, got, tc.want)
			}
		})
	}
}
