package calc

import (
	"math/big"

	"github.com/shopspring/decimal"
)

// fixedPoint computes natural logarithms and exponentials on integers scaled by
// 10^places, so fractional powers (x^f = e^(f ln x)) never touch binary floating point
// and never share mutable state between goroutines.
//
// Error analysis, with u = 10^-places one unit of the working scale, every fixed-point
// operation truncating at most once (an error of at most u), and N the number of series
// terms, which is below `places` for every series used here:
//
//   - ln(x) writes x = m 2^k with m in [1, 2), takes lnRootReductions square roots of m
//     (each contracts the error, so it stays within 2u), sums the atanh series (N u) and
//     scales the sum back by 2^lnRootReductions, then adds k ln 2, whose own error is |k|
//     times the N u of the ln 2 series. For the calculator's domain (10^-32 <= x <
//     10^100, so |k| <= 333) the result is within about 2×10^5 u of ln x.
//   - t = f ln x with |f| < 1 inherits that error.
//   - exp(t) splits t = k ln 2 + r with |r| <= ln 2 / 2 (|k| <= 333 again), sums the
//     Taylor series of e^r (N u plus e^r times the error of r, which is that of t plus
//     |k| N u) and shifts the sum by k bits, which is exact for k >= 0 and a floor
//     otherwise. Because no squaring is involved, the relative error of e^r is not
//     amplified: the absolute error of the result stays around |e^t| × 4×10^5 u.
//
// So the results carry all their significant digits except about six, and a caller that
// rounds to IntermediatePlaces gets a correctly rounded value as long as the working
// precision covers the significant digits of the largest reachable result plus those six
// plus a margin; see fixedGuardPlaces.
type fixedPoint struct {
	places  int32
	one     *big.Int // 10^places
	ln2     *big.Int // ln 2, scaled
	halfLn2 *big.Int // ln 2 / 2, scaled
}

// fixedGuardPlaces is the working margin beyond the significant digits callers need. The
// argument reductions of ln and exp consume about six of them (see fixedPoint); the rest
// keep the callers' rounding correct.
const fixedGuardPlaces = 40

// lnRootReductions is how many square roots bring a mantissa in [1, 2) close enough to 1
// for the logarithm series to converge in a few terms.
const lnRootReductions = 8

// newFixedPoint prepares a fixed-point context whose results are correct in their first
// `digits` significant decimal digits; it works with fixedGuardPlaces more places.
func newFixedPoint(digits int32) *fixedPoint {
	working := digits + fixedGuardPlaces
	f := &fixedPoint{places: working, one: pow10(int64(working))}
	// ln 2 = 2 atanh(1/3); the argument is the fixed-point value one/3.
	f.ln2 = f.atanhTwice(new(big.Int).Quo(f.one, big.NewInt(3)))
	f.halfLn2 = new(big.Int).Rsh(f.ln2, 1)
	return f
}

// pow returns x^exponent for x > 0 as a decimal with `places` decimal places.
func (f *fixedPoint) pow(x, exponent decimal.Decimal) decimal.Decimal {
	t := f.mul(f.ln(f.fromDecimal(x)), f.fromDecimal(exponent))
	return f.toDecimal(f.exp(t))
}

// fromDecimal converts a decimal to the fixed-point scale, truncating extra places.
func (f *fixedPoint) fromDecimal(d decimal.Decimal) *big.Int {
	v := scaleCoefficient(d, f.places)
	if d.Sign() < 0 {
		v.Neg(v)
	}
	return v
}

// toDecimal converts a fixed-point value back to a decimal.
func (f *fixedPoint) toDecimal(v *big.Int) decimal.Decimal {
	return decimal.NewFromBigInt(v, -f.places)
}

func (f *fixedPoint) mul(a, b *big.Int) *big.Int {
	r := new(big.Int).Mul(a, b)
	return r.Quo(r, f.one)
}

func (f *fixedPoint) div(a, b *big.Int) *big.Int {
	r := new(big.Int).Mul(a, f.one)
	return r.Quo(r, b)
}

func (f *fixedPoint) sqrt(a *big.Int) *big.Int {
	r := new(big.Int).Mul(a, f.one)
	return r.Sqrt(r)
}

// ln returns the natural logarithm of x > 0. The argument is written as m * 2^k with m in
// [1, 2), m is reduced further by repeated square roots, and the series for
// ln((1+y)/(1-y)) with y = (m-1)/(m+1) converges in a handful of terms.
func (f *fixedPoint) ln(x *big.Int) *big.Int {
	m := new(big.Int).Set(x)
	k := int64(0)
	twoOne := new(big.Int).Lsh(f.one, 1)
	for m.Cmp(twoOne) >= 0 {
		m.Rsh(m, 1)
		k++
	}
	for m.Cmp(f.one) < 0 {
		m.Lsh(m, 1)
		k--
	}
	for range lnRootReductions {
		m = f.sqrt(m)
	}
	y := f.div(new(big.Int).Sub(m, f.one), new(big.Int).Add(m, f.one))
	lnM := f.atanhTwice(y)
	lnM.Lsh(lnM, lnRootReductions)
	return lnM.Add(lnM, new(big.Int).Mul(big.NewInt(k), f.ln2))
}

// atanhTwice returns 2 atanh(y) = 2 (y + y^3/3 + y^5/5 + ...) for 0 <= y < 1.
func (f *fixedPoint) atanhTwice(y *big.Int) *big.Int {
	y2 := f.mul(y, y)
	term := new(big.Int).Set(y)
	sum := new(big.Int).Set(y)
	for n := int64(3); ; n += 2 {
		term = f.mul(term, y2)
		if term.Sign() == 0 {
			break
		}
		sum.Add(sum, new(big.Int).Quo(term, big.NewInt(n)))
	}
	return sum.Lsh(sum, 1)
}

// exp returns e^t for |t| small enough that t / ln 2 fits an int64, which holds for every
// argument pow can produce (|t| < ln 10^100). The argument is split as t = k ln 2 + r with
// k = floor((t + ln 2 / 2) / ln 2), so |r| <= ln 2 / 2; e^r is summed from its Taylor
// series and the result is e^r shifted by k bits, an exact operation on the integer (a
// floor for negative k) that, unlike squaring, does not amplify the error of the sum.
func (f *fixedPoint) exp(t *big.Int) *big.Int {
	k := new(big.Int).Add(t, f.halfLn2)
	k.Div(k, f.ln2)
	r := new(big.Int).Mul(k, f.ln2)
	r.Sub(t, r)
	sum := new(big.Int).Set(f.one)
	term := new(big.Int).Set(f.one)
	for n := int64(1); ; n++ {
		term = f.mul(term, r)
		term.Quo(term, big.NewInt(n))
		if term.Sign() == 0 {
			break
		}
		sum.Add(sum, term)
	}
	shift := k.Int64()
	if shift < 0 {
		return sum.Rsh(sum, uint(-shift))
	}
	return sum.Lsh(sum, uint(shift))
}
