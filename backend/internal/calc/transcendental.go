package calc

import (
	"math/big"

	"github.com/shopspring/decimal"
)

// fixedPoint computes natural logarithms and exponentials on integers scaled by
// 10^places, so fractional powers (x^f = e^(f ln x)) never touch binary floating point
// and never share mutable state between goroutines. Results carry about `places` correct
// decimal places minus the guard margin consumed by argument reduction; callers round them
// to IntermediatePlaces.
type fixedPoint struct {
	places int32
	one    *big.Int // 10^places
	ln2    *big.Int // ln 2, scaled
}

// fixedGuardPlaces absorbs the error amplified by the argument reductions of ln and exp.
const fixedGuardPlaces = 16

// lnRootReductions is how many square roots bring a mantissa in [1, 2) close enough to 1
// for the logarithm series to converge in a few terms.
const lnRootReductions = 8

// newFixedPoint prepares a fixed-point context with the requested number of correct
// decimal places.
func newFixedPoint(places int32) *fixedPoint {
	working := places + fixedGuardPlaces
	f := &fixedPoint{places: working, one: pow10(int64(working))}
	// ln 2 = 2 atanh(1/3); the argument is the fixed-point value one/3.
	f.ln2 = f.atanhTwice(new(big.Int).Quo(f.one, big.NewInt(3)))
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

// exp returns e^t. The argument is halved until it is below 10^-3, the Taylor series is
// summed, and the result is squared back the same number of times; a negative argument
// takes the reciprocal.
func (f *fixedPoint) exp(t *big.Int) *big.Int {
	if t.Sign() == 0 {
		return new(big.Int).Set(f.one)
	}
	u := new(big.Int).Abs(t)
	threshold := new(big.Int).Quo(f.one, big.NewInt(1000))
	halvings := 0
	for u.Cmp(threshold) > 0 {
		u.Rsh(u, 1)
		halvings++
	}
	sum := new(big.Int).Set(f.one)
	term := new(big.Int).Set(f.one)
	for n := int64(1); ; n++ {
		term = f.mul(term, u)
		term.Quo(term, big.NewInt(n))
		if term.Sign() == 0 {
			break
		}
		sum.Add(sum, term)
	}
	for range halvings {
		sum = f.mul(sum, sum)
	}
	if t.Sign() < 0 {
		return f.div(f.one, sum)
	}
	return sum
}
