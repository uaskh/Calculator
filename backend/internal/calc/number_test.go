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
		{strings.Repeat("9", 100) + "." + strings.Repeat("9", 16), strings.Repeat("9", 100) + "." + strings.Repeat("9", 16)},
		{strings.Repeat("9", 100) + "." + strings.Repeat("9", 16) + "4", strings.Repeat("9", 100) + "." + strings.Repeat("9", 16)},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			t.Parallel()
			got, err := num.canonical(mustDecimal(t, tc.value))
			if err != nil {
				t.Fatalf("canonical(%s) returned error %v", tc.value, err)
			}
			if got != tc.want {
				t.Errorf("canonical(%s) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// TestCanonical_RoundsIntoMagnitudeCap proves the final 16-place rounding is checked
// against the cap: a value below 10^100 that rounds up to it is RESULT_TOO_LARGE.
func TestCanonical_RoundsIntoMagnitudeCap(t *testing.T) {
	t.Parallel()

	num := newNumbers()
	for _, value := range []string{
		strings.Repeat("9", 100) + "." + strings.Repeat("9", 17),
		"-" + strings.Repeat("9", 100) + "." + strings.Repeat("9", 16) + "5",
	} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			got, err := num.canonical(mustDecimal(t, value))
			var aerr *ArithmeticError
			if !errors.As(err, &aerr) || aerr.Code != CodeResultTooLarge {
				t.Fatalf("canonical(%s) = %q, %v, want %s", value, got, err, CodeResultTooLarge)
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
	bases := []string{
		"2", "3", "10", "0.5", "0.001", "123456.789", "1.0001", "99",
		"1" + strings.Repeat("0", 60), "1" + strings.Repeat("0", 90), "1" + strings.Repeat("0", 99),
		"0." + strings.Repeat("0", 20) + "7",
	}
	for _, base := range bases {
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

// TestTierFor proves the fixed-point tier chosen for a base always has a digit budget
// covering the base's integer digits, works with that budget plus the intermediate and
// guard places, and is the narrowest such tier, so the cost of a fractional power follows
// the magnitude of its base rather than the magnitude cap.
func TestTierFor(t *testing.T) {
	t.Parallel()

	num := newNumbers()
	cases := []struct {
		base       string
		wantDigits int32
	}{
		{"0.5", 8},
		{"1", 8},
		{"9.99", 8},
		{"99999999", 8},
		{"1" + strings.Repeat("0", 8), 40},
		{"1" + strings.Repeat("0", 9), 40},
		{"1" + strings.Repeat("0", 39), 40},
		{"1" + strings.Repeat("0", 40), MaxIntegerDigits},
		{"1" + strings.Repeat("0", 41), MaxIntegerDigits},
		{"1" + strings.Repeat("0", 99), MaxIntegerDigits},
		{strings.Repeat("9", 100) + "." + strings.Repeat("9", 32), MaxIntegerDigits},
	}
	for _, tc := range cases {
		t.Run(tc.base, func(t *testing.T) {
			t.Parallel()
			x := mustDecimal(t, tc.base)
			tier := num.tierFor(x)
			if digits := integerDigits(x); int(tier.digits) < digits {
				t.Errorf("tierFor(%s) serves %d digits, base has %d", tc.base, tier.digits, digits)
			}
			if tier.digits != tc.wantDigits {
				t.Errorf("tierFor(%s).digits = %d, want %d", tc.base, tier.digits, tc.wantDigits)
			}
			if want := tier.digits + IntermediatePlaces + fixedGuardPlaces; tier.places != want {
				t.Errorf("tierFor(%s).places = %d, want %d", tc.base, tier.places, want)
			}
		})
	}
}

// TestTiers_CoverMagnitudeCap proves the widest tier serves every base the magnitude cap
// admits and that the tiers are ordered so the narrowest sufficient one is found.
func TestTiers_CoverMagnitudeCap(t *testing.T) {
	t.Parallel()

	num := newNumbers()
	if len(num.tiers) == 0 {
		t.Fatal("newNumbers built no fixed-point tiers")
	}
	for i := 1; i < len(num.tiers); i++ {
		if num.tiers[i-1].digits >= num.tiers[i].digits {
			t.Errorf("tiers[%d].digits = %d is not below tiers[%d].digits = %d", i-1, num.tiers[i-1].digits, i, num.tiers[i].digits)
		}
	}
	if widest := num.tiers[len(num.tiers)-1]; widest.digits != MaxIntegerDigits {
		t.Errorf("widest tier serves %d digits, want %d", widest.digits, MaxIntegerDigits)
	}
}

// productionFixedPoint returns the widest fixed-point tier numbers uses, the one serving
// bases with up to MaxIntegerDigits integer digits.
func productionFixedPoint() *fixedPoint {
	return newFixedPoint(MaxIntegerDigits)
}

func TestFixedPoint_Ln2(t *testing.T) {
	t.Parallel()

	// ln 2 = 0.693147180559945309417232121458176568075500134360255254120680009493393621969694715605863326996418687542...
	const ln2 = "0.6931471805599453094172321214581765680755001343602552541206800094933936219696947156058633269964186875"
	const knownPlaces = int32(len(ln2) - 2)
	fp := productionFixedPoint()
	if fp.places <= knownPlaces {
		t.Fatalf("fixed point works with %d places, want more than the %d known digits of ln 2", fp.places, knownPlaces)
	}
	got := fp.toDecimal(fp.ln2).Truncate(knownPlaces).String()
	if got != ln2 {
		t.Errorf("ln2 = %s, want %s", got, ln2)
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

// TestFixedPoint_Exp checks exp against values computed independently (Python decimal at
// 400 digits), rounded to 32 places: both signs of the 2^k split, a small argument that
// needs no split, and the largest argument pow can produce (ln 10^100 = 230.26).
func TestFixedPoint_Exp(t *testing.T) {
	t.Parallel()

	fp := productionFixedPoint()
	cases := []struct{ arg, want string }{
		{"1", "2.71828182845904523536028747135266"},
		{"-1", "0.36787944117144232159552377016146"},
		{"10", "22026.46579480671651695790064528424437"},
		{"0.3", "1.34985880757600310398374431332801"},
		{"230.25", "9915268022112193113739658471051846694218112679960420565817010409757919871841610196484460056223539119.81457846279824367426611945368732"},
	}
	for _, tc := range cases {
		t.Run(tc.arg, func(t *testing.T) {
			t.Parallel()
			got := fp.toDecimal(fp.exp(fp.fromDecimal(mustDecimal(t, tc.arg)))).Round(IntermediatePlaces).String()
			if got != tc.want {
				t.Errorf("exp(%s) = %s, want %s", tc.arg, got, tc.want)
			}
		})
	}
	// e^-74 = 7.28e-33 sits below the intermediate scale (k = -107 bits): the fixed point
	// still holds its digits, and the 32-place rounding lifts it to one unit.
	small := fp.toDecimal(fp.exp(fp.fromDecimal(mustDecimal(t, "-74"))))
	if got, want := small.Truncate(45).String(), "0."+strings.Repeat("0", 32)+"7281290178321"; got != want {
		t.Errorf("exp(-74) = %s, want %s", got, want)
	}
	if got, want := small.Round(IntermediatePlaces).String(), "0."+strings.Repeat("0", 31)+"1"; got != want {
		t.Errorf("exp(-74) rounded to 32 places = %s, want %s", got, want)
	}
}

func TestFixedPoint_FromDecimal(t *testing.T) {
	t.Parallel()

	fp := productionFixedPoint()
	cases := []struct{ value, want string }{
		{"-1.5", "-1.5"},
		{"0.25", "0.25"},
		{"0", "0"},
		{"-0." + strings.Repeat("0", int(fp.places)) + "1", "0"}, // beyond the working scale: truncated
		{"0." + strings.Repeat("0", int(fp.places)-1) + "1", "0." + strings.Repeat("0", int(fp.places)-1) + "1"},
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
