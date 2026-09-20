package calc

import (
	"math/big"

	"github.com/shopspring/decimal"
)

// fixedPoint computes natural logarithms and exponentials on math/big integers scaled by
// 2^bits, a binary fixed point fine enough to resolve `places` decimal places
// (2^-bits < 10^-places), so fractional powers (x^f = e^(f ln x)) never touch binary
// floating point and never share mutable state between goroutines. The scale is binary
// rather than decimal because dropping the low bits of a product is a shift, whereas
// dividing by 10^places costs several times more and would dominate the series below.
// An instance serves bases with up to `digits` integer digits; numbers keeps one
// instance per tier of digits and picks the narrowest that covers the base, so the cost
// of a fractional power follows the size of its base rather than the magnitude cap.
//
// Error analysis, with u = 2^-bits one unit of the working scale, every fixed-point
// operation rounding down at most once (an error below u), and N the number of terms of
// a series, which the stop condition (a term below u) bounds by about places / -log10 of
// the series ratio: 1.05 places for the ln 2 series (ratio 1/9), 0.66 places for the
// mantissa series (ratio below 0.03) and 0.6 places for the exponential series:
//
//   - ln(x) writes x = m 2^k with m in [1/sqrt 2, sqrt 2), an exact shift (up to the
//     floor of the last bit), then sums the series for ln((1+y)/(1-y)) with
//     y = (m-1)/(m+1), |y| < 0.172, whose derivative 2/(1-y^2) keeps the error of y
//     within 5u in the sum, and adds k ln 2, whose own error is |k| times the N u of the
//     ln 2 series. For the calculator's domain (10^-32 <= x < 10^100, so |k| <= 333) the
//     result is within about 10^5 u of ln x.
//   - t = f ln x with |f| < 1 inherits that error.
//   - exp(t) splits t = k ln 2 + r with |r| <= ln 2 / 2 (|k| <= 333 again), sums the
//     Taylor series of e^r (N u plus e^r times the error of r, which is that of t plus
//     |k| N u) and shifts the sum by k bits, which is exact for k >= 0 and a floor
//     otherwise. Because no squaring is involved, the relative error of e^r is not
//     amplified: the absolute error of the result stays around |e^t| × 4×10^5 u.
//   - converting the result to a decimal rounds it to `places` places, at most
//     10^-places / 2 more.
//
// So the results carry all their significant digits except about six, and a caller that
// rounds to IntermediatePlaces gets a correctly rounded value as long as the working
// precision covers the significant digits of the largest reachable result plus those six
// plus a margin; see fixedGuardPlaces.
//
// Per tier, with d = digits and a base 10^-32 <= x < 10^d (every non-zero intermediate
// value the tier serves): |t| = |f ln x| < ln 10^max(d, 32), so |k| <= 3.33 max(d, 32)
// and N shrinks with places, and the bounds above, derived for d = 100, only shrink.
// For x >= 1, x^f <= x < 10^d, so the result has at most d integer digits and its
// absolute error is below 10^d × 4×10^5 × 10^-(d + IntermediatePlaces + fixedGuardPlaces)
// = 4×10^-67; for x < 1 the result is at most 1 and the error smaller still. Both are far
// below the 10^-32 the caller rounds to, whichever tier is used.
type fixedPoint struct {
	digits     int32    // integer digits of the largest base served, hence of the largest result
	places     int32    // decimal places resolved: digits + IntermediatePlaces + fixedGuardPlaces
	bits       uint     // the scale is 2^bits, the smallest power of two above 10^places
	one        *big.Int // 2^bits
	half       *big.Int // 2^(bits-1), the rounding term of toDecimal
	decimalOne *big.Int // 10^places, the scale of the decimals exchanged with callers
	sqrt2      *big.Int // sqrt 2, scaled: the upper bound of a centred mantissa
	ln2        *big.Int // ln 2, scaled
	halfLn2    *big.Int // ln 2 / 2, scaled
}

// fixedGuardPlaces is the working margin beyond the significant digits callers need. The
// argument reductions of ln and exp consume about six of them (see fixedPoint); the rest
// keep the callers' rounding correct.
const fixedGuardPlaces = 40

// newFixedPoint prepares a fixed-point context for bases with up to `digits` integer
// digits: its results are correct in their first digits + IntermediatePlaces significant
// decimal digits, and it works with fixedGuardPlaces more places.
func newFixedPoint(digits int32) *fixedPoint {
	places := digits + IntermediatePlaces + fixedGuardPlaces
	decimalOne := pow10(int64(places))
	bits := uint(decimalOne.BitLen()) // 2^(bits-1) <= 10^places < 2^bits
	f := &fixedPoint{
		digits:     digits,
		places:     places,
		bits:       bits,
		one:        new(big.Int).Lsh(big.NewInt(1), bits),
		half:       new(big.Int).Lsh(big.NewInt(1), bits-1),
		decimalOne: decimalOne,
	}
	// sqrt 2 scaled by 2^bits is the integer square root of 2 × 2^(2 bits).
	f.sqrt2 = new(big.Int).Lsh(big.NewInt(1), 2*bits+1)
	f.sqrt2.Sqrt(f.sqrt2)
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

// fromDecimal converts a decimal to the fixed-point scale, truncating toward zero what
// the scale cannot resolve.
func (f *fixedPoint) fromDecimal(d decimal.Decimal) *big.Int {
	// d = c 10^e, so |d| 2^bits = (c 2^bits) 10^e: exact for e >= 0, floored otherwise.
	v := new(big.Int).Lsh(d.Abs().Coefficient(), f.bits)
	if e := int64(d.Exponent()); e >= 0 {
		v.Mul(v, pow10(e))
	} else {
		v.Quo(v, pow10(-e))
	}
	if d.Sign() < 0 {
		v.Neg(v)
	}
	return v
}

// toDecimal converts a fixed-point value to a decimal with `places` decimal places,
// rounding half away from zero.
func (f *fixedPoint) toDecimal(v *big.Int) decimal.Decimal {
	c := new(big.Int).Abs(v)
	c.Mul(c, f.decimalOne)
	c.Add(c, f.half)
	c.Rsh(c, f.bits)
	if v.Sign() < 0 {
		c.Neg(c)
	}
	return decimal.NewFromBigInt(c, -f.places)
}

func (f *fixedPoint) mul(a, b *big.Int) *big.Int {
	r := new(big.Int).Mul(a, b)
	return r.Rsh(r, f.bits)
}

func (f *fixedPoint) div(a, b *big.Int) *big.Int {
	r := new(big.Int).Lsh(a, f.bits)
	return r.Quo(r, b)
}

// ln returns the natural logarithm of x > 0. The argument is written as m * 2^k with m
// in [1/sqrt 2, sqrt 2), so the series for ln((1+y)/(1-y)) with y = (m-1)/(m+1) has
// |y| < 0.172 and gains more than five bits per term.
func (f *fixedPoint) ln(x *big.Int) *big.Int {
	// m 2^bits has the bit length of one (2^bits) exactly when m is in [1, 2).
	k := int64(x.BitLen()) - int64(f.one.BitLen())
	m := new(big.Int)
	if k >= 0 {
		m.Rsh(x, uint(k))
	} else {
		m.Lsh(x, uint(-k))
	}
	if m.Cmp(f.sqrt2) >= 0 {
		m.Rsh(m, 1)
		k++
	}
	// The series is odd in y and needs y >= 0 (a negative power would floor to -1 and
	// never vanish), so a mantissa below 1 is handled by symmetry.
	y := f.div(new(big.Int).Sub(m, f.one), new(big.Int).Add(m, f.one))
	negative := y.Sign() < 0
	lnM := f.atanhTwice(y.Abs(y))
	if negative {
		lnM.Neg(lnM)
	}
	return lnM.Add(lnM, new(big.Int).Mul(big.NewInt(k), f.ln2))
}

// atanhTwice returns 2 atanh(y) = 2 (y + y^3/3 + y^5/5 + ...) for 0 <= y < 1.
func (f *fixedPoint) atanhTwice(y *big.Int) *big.Int {
	y2 := f.mul(y, y)
	term := new(big.Int).Set(y)
	sum := new(big.Int).Set(y)
	product, quotient, remainder := new(big.Int), new(big.Int), new(big.Int)
	n, step := big.NewInt(1), big.NewInt(2)
	for {
		n.Add(n, step)
		// Every value is kept in a reusable integer (QuoRem so the discarded remainder
		// does not allocate either); a product never aliases its operands, so term and
		// product trade places every iteration.
		product.Mul(term, y2)
		product.Rsh(product, f.bits)
		term, product = product, term
		if term.Sign() == 0 {
			break
		}
		quotient.QuoRem(term, n, remainder)
		sum.Add(sum, quotient)
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
	product, remainder := new(big.Int), new(big.Int)
	n, step := big.NewInt(0), big.NewInt(1)
	for {
		n.Add(n, step)
		// term_n = term_(n-1) r / n, computed in reusable integers as in atanhTwice.
		product.Mul(term, r)
		product.Rsh(product, f.bits)
		term, product = product, term
		term.QuoRem(term, n, remainder)
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
