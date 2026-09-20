package calc

import (
	"errors"
	"regexp"
	"strings"
	"testing"
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
