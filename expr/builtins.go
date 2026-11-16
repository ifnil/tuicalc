package expr

import (
	"math"
	"sort"
)

type builtin struct {
	minArgs, maxArgs int // maxArgs < 0 means variadic
	f1               func(float64) float64
	f2               func(float64, float64) float64
	fn               func([]float64) float64
	angleIn          bool // argument is an angle (affected by degree mode)
	angleOut         bool // result is an angle (affected by degree mode)
}

var constants = map[string]float64{
	"pi":  math.Pi,
	"π":   math.Pi,
	"tau": 2 * math.Pi,
	"τ":   2 * math.Pi,
	"e":   math.E,
	"phi": math.Phi,
	"φ":   math.Phi,
	"inf": math.Inf(1),
	"∞":   math.Inf(1),
}

func one(f func(float64) float64) builtin {
	return builtin{minArgs: 1, maxArgs: 1, f1: f}
}

func two(f func(float64, float64) float64) builtin {
	return builtin{minArgs: 2, maxArgs: 2, f2: f}
}

func trig(f func(float64) float64) builtin {
	b := one(f)
	b.angleIn = true
	return b
}

func arc(f func(float64) float64) builtin {
	b := one(f)
	b.angleOut = true
	return b
}

var builtins = map[string]builtin{
	"sin": trig(math.Sin),
	"cos": trig(math.Cos),
	"tan": trig(math.Tan),
	"sec": trig(func(x float64) float64 { return 1 / math.Cos(x) }),
	"csc": trig(func(x float64) float64 { return 1 / math.Sin(x) }),
	"cot": trig(func(x float64) float64 { return 1 / math.Tan(x) }),

	"asin":   arc(math.Asin),
	"acos":   arc(math.Acos),
	"atan":   arc(math.Atan),
	"arcsin": arc(math.Asin),
	"arccos": arc(math.Acos),
	"arctan": arc(math.Atan),
	"atan2":  {minArgs: 2, maxArgs: 2, f2: math.Atan2, angleOut: true},

	"sinh":  one(math.Sinh),
	"cosh":  one(math.Cosh),
	"tanh":  one(math.Tanh),
	"asinh": one(math.Asinh),
	"acosh": one(math.Acosh),
	"atanh": one(math.Atanh),

	"sqrt":  one(math.Sqrt),
	"cbrt":  one(math.Cbrt),
	"exp":   one(math.Exp),
	"ln":    one(math.Log),
	"log":   one(math.Log10),
	"log10": one(math.Log10),
	"log2":  one(math.Log2),
	"abs":   one(math.Abs),
	"floor": one(math.Floor),
	"ceil":  one(math.Ceil),
	"round": one(math.Round),
	"trunc": one(math.Trunc),
	"sign":  one(sign),
	"sgn":   one(sign),
	"gamma": one(math.Gamma),
	"fact":  one(factorial),
	"erf":   one(math.Erf),

	"pow":   two(pow),
	"mod":   two(mod),
	"hypot": two(math.Hypot),
	"root":  two(func(x, n float64) float64 { return pow(x, 1/n) }),
	"logb":  two(func(b, x float64) float64 { return math.Log(x) / math.Log(b) }),
	"nCr":   two(choose),
	"nPr":   two(perm),

	"min":    {minArgs: 1, maxArgs: -1, fn: minOf},
	"max":    {minArgs: 1, maxArgs: -1, fn: maxOf},
	"mean":   {minArgs: 1, maxArgs: -1, fn: mean},
	"median": {minArgs: 1, maxArgs: -1, fn: median},
}

// BuiltinNames returns the names of all builtin functions, sorted.
func BuiltinNames() []string {
	names := make([]string, 0, len(builtins))
	for n := range builtins {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func sign(x float64) float64 {
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	}
	return x // keeps 0 and NaN
}

func mod(a, b float64) float64 {
	r := math.Mod(a, b)
	if r != 0 && (r < 0) != (b < 0) {
		r += b
	}
	return r
}

// pow is math.Pow, except that negative bases raised to rational powers
// with odd denominators stay real, so x^(1/3) is defined for negative x.
func pow(x, y float64) float64 {
	r := math.Pow(x, y)
	if !math.IsNaN(r) || x >= 0 || math.IsNaN(x) || math.IsInf(y, 0) || math.IsNaN(y) {
		return r
	}
	num, den, ok := rationalize(y)
	if !ok || den%2 == 0 {
		return math.NaN()
	}
	r = math.Pow(-x, y)
	if num%2 != 0 {
		r = -r
	}
	return r
}

// rationalize finds p/q == y for a small denominator q.
func rationalize(y float64) (int64, int64, bool) {
	for q := int64(1); q <= 64; q++ {
		p := math.Round(y * float64(q))
		if math.Abs(p/float64(q)-y) < 1e-12 {
			return int64(p), q, true
		}
	}
	return 0, 0, false
}

func factorial(x float64) float64 {
	if x == math.Trunc(x) && x >= 0 && x <= 170 {
		r := 1.0
		for i := 2.0; i <= x; i++ {
			r *= i
		}
		return r
	}
	if x < 0 && x == math.Trunc(x) {
		return math.NaN()
	}
	return math.Gamma(x + 1)
}

func choose(n, k float64) float64 {
	if n != math.Trunc(n) || k != math.Trunc(k) || k < 0 || n < 0 {
		return math.NaN()
	}
	if k > n {
		return 0
	}
	k = math.Min(k, n-k)
	r := 1.0
	for i := 1.0; i <= k; i++ {
		r = r * (n - k + i) / i
	}
	return math.Round(r)
}

func perm(n, k float64) float64 {
	if n != math.Trunc(n) || k != math.Trunc(k) || k < 0 || n < 0 {
		return math.NaN()
	}
	if k > n {
		return 0
	}
	r := 1.0
	for i := n - k + 1; i <= n; i++ {
		r *= i
	}
	return r
}

func minOf(xs []float64) float64 {
	r := xs[0]
	for _, x := range xs[1:] {
		r = math.Min(r, x)
	}
	return r
}

func maxOf(xs []float64) float64 {
	r := xs[0]
	for _, x := range xs[1:] {
		r = math.Max(r, x)
	}
	return r
}

func mean(xs []float64) float64 {
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

func median(xs []float64) float64 {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}
