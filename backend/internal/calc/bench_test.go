package calc

import (
	"errors"
	"math"
	"regexp"
	"strings"
	"testing"
	"time"
)

// canonicalValue is the result pattern published in the API contract, kept byte-for-byte.
var canonicalValue = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]*[1-9])?$`) //nolint:gocritic // same text as api/openapi.yaml

// maxValueLength is the longest canonical result: a sign, 100 integer digits, a point and
// 16 decimal places.
const maxValueLength = 1 + MaxIntegerDigits + 1 + ResultPlaces

// benchmarkCase is one entry of the NFR-4 worst-case corpus. Either value (a successful
// result) or code (an arithmetic error) is expected; value "" with code "" only requires
// success.
type benchmarkCase struct {
	name  string
	input string
	value string
	code  string
}

func benchmarkCorpus() []benchmarkCase {
	return []benchmarkCase{
		{name: "literal_1024_digits", input: strings.Repeat("1", MaxLength), code: CodeResultTooLarge},
		{name: "fraction_1022_digits", input: "0." + strings.Repeat("1", MaxLength-2), value: "0.1111111111111111"},
		{name: "pow_1.0001^1000", input: "1.0001^1000", value: "1.1051653926032327"},
		{name: "pow_nested_(1.0001^1000)^1000", input: "(1.0001^1000)^1000"},
		{name: "pow_precheck_reject_(1.1^1000)^1000", input: "(1.1^1000)^1000", code: CodeResultTooLarge},
		{name: "pow_chain_2^0.5_x32", input: strings.Repeat("2^0.5*", 31) + "2^0.5", value: "65536"},
		{name: "sqrt_chain_x32", input: strings.Repeat("sqrt(", 32) + "16" + strings.Repeat(")", 32), value: "1.0000000006455436"},
		{name: "nesting_depth_32", input: strings.Repeat("(1+", 32) + "1" + strings.Repeat(")", 32), value: "33"},
		{name: "largest_integer_power_2^332", input: "2^332", value: bigPow(2, 332)},
		{name: "pow_99^50", input: "99^50", value: bigPow(99, 50)},
		{name: "pow_9^999.5", input: "9^999.5", code: CodeResultTooLarge},
		{name: "pow_large_fractional_(10^99)^0.999", input: "(10^99)^0.999", value: "796159350417318744185341970687172307579007314201602764575008496413321708599304314072090503021624071.6058243083988372"},
		{name: "pow_fractional_chain_0.5^0.5_x255", input: fractionalPowerChain("0.5"), value: "0.641185744504986"},
		{name: "pow_fractional_chain_2^0.5_x255", input: fractionalPowerChain("2"), value: "1.5596104694623693"},
		{name: "pow_fractional_chain_0.7^0.7_x255", input: "0.7" + strings.Repeat("^0.7", fractionalPowerChainLength), value: "0.7620134308107167"},
	}
}

// fractionalPowerChainLength is how many "^0.5" steps follow the base in the longest
// fractional-power chain the length limit admits: "0.5" + 255 × "^0.5" is 1,023 code
// points. "^" is right-associative, so every one of the 255 steps is a fractional power
// whose base is 0.5 (or the leading base for the outermost step). The 0.7 variant has
// the costliest mantissa for the logarithm (0.5 is a power of two, whose mantissa is
// exactly 1) and the expected values come from Python's decimal module at 120 digits,
// rounding every step to 32 places half up exactly as the evaluator does.
const fractionalPowerChainLength = 255

// fractionalPowerChain returns base followed by fractionalPowerChainLength "^0.5" steps.
func fractionalPowerChain(base string) string {
	return base + strings.Repeat("^0.5", fractionalPowerChainLength)
}

// fractionalPowerChainBudget is the NFR-4 bound for one evaluation of a corpus case.
const fractionalPowerChainBudget = 5 * time.Millisecond

// TestFractionalPowerChainBudget guards NFR-4 for the true worst case of "^" chains: the
// best of five evaluations of the 255-step fractional-power chain must finish within the
// budget. Timing is meaningless under the race detector and slow in the -short gate, so
// both skip it; the benchmark quotes the exact numbers.
func TestFractionalPowerChainBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("timing guard: skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("timing guard: skipped under the race detector")
	}
	calc := New()
	input := fractionalPowerChain("0.5")
	best := time.Duration(math.MaxInt64)
	for range 5 {
		start := time.Now()
		if _, err := calc.Evaluate(t.Context(), input); err != nil {
			t.Fatalf("Evaluate(0.5^0.5 chain) returned error %v", err)
		}
		best = min(best, time.Since(start))
	}
	if best >= fractionalPowerChainBudget {
		t.Errorf("Evaluate(0.5^0.5 x%d) best of 5 = %v, want under %v", fractionalPowerChainLength, best, fractionalPowerChainBudget)
	}
}

// TestBenchmarkCorpus verifies every benchmark input in the normal test run, so the
// benchmark never measures an expression that fails unexpectedly.
func TestBenchmarkCorpus(t *testing.T) {
	t.Parallel()

	for _, tc := range benchmarkCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if len(tc.input) > MaxLength {
				t.Fatalf("corpus input %q is %d characters, above MaxLength", tc.name, len(tc.input))
			}
			got, err := evaluate(t, tc.input)
			if tc.code != "" {
				var aerr *ArithmeticError
				if !errors.As(err, &aerr) || aerr.Code != tc.code {
					t.Fatalf("Evaluate(%s) = %+v, %v, want %s", tc.name, got, err, tc.code)
				}
				return
			}
			if err != nil {
				t.Fatalf("Evaluate(%s) returned error %v", tc.name, err)
			}
			if tc.value != "" && got.Value != tc.value {
				t.Errorf("Evaluate(%s).Value = %q, want %q", tc.name, got.Value, tc.value)
			}
			if !canonicalValue.MatchString(got.Value) {
				t.Errorf("Evaluate(%s).Value = %q is not canonical", tc.name, got.Value)
			}
		})
	}
}

func BenchmarkEvaluate(b *testing.B) {
	calc := New()
	for _, bc := range benchmarkCorpus() {
		b.Run(bc.name, func(b *testing.B) {
			for b.Loop() {
				_, _ = calc.Evaluate(b.Context(), bc.input) // outcomes are verified by TestBenchmarkCorpus
			}
		})
	}
}
