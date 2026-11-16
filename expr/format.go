package expr

import (
	"math"
	"strconv"
	"strings"
)

// FormatNumber renders a value for display, rounding away floating point
// noise (0.1+0.2 shows as 0.3) and using exponents only for very large or
// very small magnitudes.
func FormatNumber(v float64) string {
	return FormatPrec(v, 12)
}

// FormatPrec is FormatNumber with a given number of significant digits.
func FormatPrec(v float64, digits int) string {
	switch {
	case math.IsNaN(v):
		return "undefined"
	case math.IsInf(v, 1):
		return "∞"
	case math.IsInf(v, -1):
		return "-∞"
	case v == 0:
		return "0"
	}
	g := strconv.FormatFloat(v, 'g', digits, 64)
	r, _ := strconv.ParseFloat(g, 64)
	if a := math.Abs(r); a >= 1e-6 && a < 1e15 {
		return strconv.FormatFloat(r, 'f', -1, 64)
	}
	return cleanExponent(g)
}

// FormatTick renders an axis tick value that is a multiple of step.
func FormatTick(v, step float64) string {
	if math.Abs(v) < step/2 {
		return "0"
	}
	exp := int(math.Floor(math.Log10(step) + 1e-9))
	a := math.Abs(v)
	if a >= 1e6 || exp < -4 {
		// Enough significant digits to tell neighbouring ticks apart.
		digits := int(math.Floor(math.Log10(a)+1e-9)) - exp + 1
		digits = max(1, min(digits, 12))
		return cleanExponent(strconv.FormatFloat(v, 'g', digits, 64))
	}
	return strconv.FormatFloat(v, 'f', max(0, -exp), 64)
}

func cleanExponent(s string) string {
	mant, exp, ok := strings.Cut(s, "e")
	if !ok {
		return s
	}
	if strings.Contains(mant, ".") {
		mant = strings.TrimRight(strings.TrimRight(mant, "0"), ".")
	}
	exp = strings.TrimPrefix(exp, "+")
	neg := strings.HasPrefix(exp, "-")
	exp = strings.TrimLeft(strings.TrimPrefix(exp, "-"), "0")
	if neg {
		exp = "-" + exp
	}
	return mant + "e" + exp
}

// NiceStep returns the smallest 1, 2 or 5 × 10^k that is at least raw.
func NiceStep(raw float64) float64 {
	if raw <= 0 || math.IsNaN(raw) || math.IsInf(raw, 0) {
		return 1
	}
	p := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 5, 10} {
		if m*p >= raw*(1-1e-9) {
			return m * p
		}
	}
	return 10 * p
}
