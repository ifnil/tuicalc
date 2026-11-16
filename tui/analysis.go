package tui

import (
	"math"
	"sort"
)

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// nextFeature scans from x0 in direction dir (+1 or -1) in steps of step
// and returns the first root, minimum or maximum of fn it finds.
func nextFeature(fn func(float64) float64, x0, step float64, dir, maxSteps int) (float64, string, bool) {
	s := step * float64(dir)
	px := x0 + s*0.01 // skip the point we're on
	py := fn(px)
	ppx, ppy := math.NaN(), math.NaN()
	for range maxSteps {
		x := px + s
		y := fn(x)

		// Root: a sign change between neighbouring samples.
		if finite(py) && finite(y) {
			if y == 0 {
				return x, "root", true
			}
			if (py < 0) != (y < 0) {
				if r, ok := bisectRoot(fn, px, x, py); ok {
					return r, "root", true
				}
			}
		}

		// Extremum: the slope changes sign at the middle sample.
		if finite(ppy) && finite(py) && finite(y) {
			d1, d2 := py-ppy, y-py
			if d1 != 0 && d2 != 0 && (d1 > 0) != (d2 > 0) {
				isMax := d1 > 0
				xe := goldenSection(fn, math.Min(ppx, x), math.Max(ppx, x), isMax)
				spread := math.Max(math.Max(ppy, py), y) - math.Min(math.Min(ppy, py), y)
				// A pole also looks like a peak; reject values far beyond
				// what the samples suggest.
				if ye := fn(xe); finite(ye) && math.Abs(ye-py) <= 4*spread+1e-12 {
					if isMax {
						return xe, "maximum", true
					}
					return xe, "minimum", true
				}
			}
		}
		ppx, ppy = px, py
		px, py = x, y
	}
	return 0, "", false
}

// bisectRoot narrows a sign change down to a root. It rejects poles such
// as the one in tan(x), where the function blows up instead of crossing 0.
func bisectRoot(fn func(float64) float64, a, b, ya float64) (float64, bool) {
	yb := fn(b)
	scale := math.Abs(ya) + math.Abs(yb)
	for range 100 {
		m := (a + b) / 2
		if m == a || m == b {
			break
		}
		ym := fn(m)
		if !finite(ym) {
			return 0, false
		}
		if ym == 0 {
			return m, true
		}
		if (ym < 0) == (ya < 0) {
			a, ya = m, ym
		} else {
			b = m
		}
	}
	r := (a + b) / 2
	if y := fn(r); !finite(y) || math.Abs(y) > 1e-6*scale+1e-9 {
		return 0, false
	}
	return r, true
}

func goldenSection(fn func(float64) float64, a, b float64, isMax bool) float64 {
	g := (math.Sqrt(5) - 1) / 2
	f := fn
	if isMax {
		f = func(x float64) float64 { return -fn(x) }
	}
	c, d := b-g*(b-a), a+g*(b-a)
	fc, fd := f(c), f(d)
	for range 100 {
		if fc < fd {
			b, d, fd = d, c, fc
			c = b - g*(b-a)
			fc = f(c)
		} else {
			a, c, fc = c, d, fd
			d = a + g*(b-a)
			fd = f(d)
		}
		if b-a < 1e-15*math.Max(1, math.Abs(a)) {
			break
		}
	}
	return (a + b) / 2
}

// fitRange picks a y range that shows the sampled values, ignoring a few
// extreme outliers such as values near an asymptote.
func fitRange(ys []float64) (float64, float64, bool) {
	if len(ys) == 0 {
		return 0, 0, false
	}
	sort.Float64s(ys)
	lo, hi := ys[0], ys[len(ys)-1]
	q := func(p float64) float64 { return ys[int(p*float64(len(ys)-1))] }
	if q5, q95 := q(0.05), q(0.95); q95 > q5 && hi-lo > 20*(q95-q5) {
		lo, hi = q5, q95
	}
	if hi-lo < 1e-9*math.Max(1, math.Abs(hi)) {
		lo, hi = lo-1, hi+1
	}
	pad := (hi - lo) * 0.08
	return lo - pad, hi + pad, true
}
