package expr

import (
	"math"
	"strings"
	"testing"
)

func eval1(t *testing.T, src string) Result {
	t.Helper()
	return Compile([]string{src}, Options{})[0]
}

func TestValues(t *testing.T) {
	cases := map[string]float64{
		"2+2":           4,
		"2+3*4":         14,
		"(2+3)*4":       20,
		"2^3^2":         512,
		"-2^2":          -4,
		"2^-1":          0.5,
		"10 % 3":        1,
		"-1 % 3":        2,
		"5!":            120,
		"3!^2":          36,
		"2pi":           2 * math.Pi,
		"2π":            2 * math.Pi,
		"sqrt(16)":      4,
		"sin(pi/2)":     1,
		"|-3|":          3,
		"2|-3|":         6,
		"||-2|-5|":      3,
		"max(1, 5, 3)":  5,
		"min(4, 2)":     2,
		"ln(e)":         1,
		"log(1000)":     3,
		"logb(2, 8)":    3,
		"nCr(5, 2)":     10,
		"(1+2)(3+4)":    21,
		"2(3)":          6,
		"1e3":           1000,
		"2e":            2 * math.E,
		".5 + 1.5":      2,
		"3²":            9,
		"2**10":         1024,
		"8^(1/3)":       2,
		"(-8)^(1/3)":    -2,
		"sin pi":        0,
		"sqrt 16 + 1":   5,
		"cos(0)cos(0)":  1,
		"atan2(1, 1)*4": math.Pi,
	}
	for src, want := range cases {
		r := eval1(t, src)
		if r.Err != nil {
			t.Errorf("%q: unexpected error %v", src, r.Err)
			continue
		}
		if r.Kind != Value {
			t.Errorf("%q: kind %v, want Value", src, r.Kind)
			continue
		}
		if math.Abs(r.Value-want) > 1e-9 {
			t.Errorf("%q = %v, want %v", src, r.Value, want)
		}
	}
}

func TestGraphs(t *testing.T) {
	cases := []struct {
		src  string
		x    float64
		want float64
	}{
		{"x^2", 3, 9},
		{"y = 2x + 1", 2, 5},
		{"xsin(x)", math.Pi / 2, math.Pi / 2},
		{"xe^x", 1, math.E},
		{"2pix", 1, 2 * math.Pi},
		{"sin x cos x", 1, math.Sin(1) * math.Cos(1)},
		{"sin 2x", 1, math.Sin(2)},
		{"x^(1/3)", -27, -3},
		{"x^(2/3)", -8, 4},
		{"|x|", -4, 4},
		{"y = 3", 100, 3},
		{"x!", 4, 24},
	}
	for _, c := range cases {
		r := eval1(t, c.src)
		if r.Err != nil {
			t.Errorf("%q: unexpected error %v", c.src, r.Err)
			continue
		}
		if r.Fn == nil {
			t.Errorf("%q: not plottable (kind %v)", c.src, r.Kind)
			continue
		}
		if got := r.Fn(c.x); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%q at x=%v = %v, want %v", c.src, c.x, got, c.want)
		}
	}
}

func TestDefinitions(t *testing.T) {
	rs := Compile([]string{
		"f(x) = x^2 + a",
		"a = 3",
		"g(t) = f(t) * 2",
		"y = g(x) + b",
		"b = a + 1",
		"g(2)",
		"h(u, v) = u - v",
		"h(5, 2)",
		"a(x+1)", // a is a variable, so this is a·(x+1)
		"af(2)",  // a·f(2)
	}, Options{})
	for i, r := range rs {
		if r.Err != nil {
			t.Fatalf("entry %d: unexpected error %v", i, r.Err)
		}
	}
	if rs[0].Kind != Function || rs[0].Fn == nil {
		t.Errorf("f should be a plottable function, got %+v", rs[0])
	}
	if got := rs[3].Fn(1); got != (1+3)*2+4 {
		t.Errorf("y at 1 = %v", got)
	}
	if rs[4].Value != 4 || rs[4].Literal {
		t.Errorf("b = %v (literal %v)", rs[4].Value, rs[4].Literal)
	}
	if !rs[1].Literal {
		t.Errorf("a = 3 should be literal")
	}
	if rs[5].Value != 14 {
		t.Errorf("g(2) = %v", rs[5].Value)
	}
	if rs[6].Fn != nil {
		t.Errorf("two-parameter functions are not plottable")
	}
	if rs[7].Value != 3 {
		t.Errorf("h(5,2) = %v", rs[7].Value)
	}
	if got := rs[8].Fn(1); got != 6 {
		t.Errorf("a(x+1) at 1 = %v", got)
	}
	if rs[9].Value != 21 {
		t.Errorf("af(2) = %v", rs[9].Value)
	}
}

func TestVLine(t *testing.T) {
	r := eval1(t, "x = 2pi")
	if r.Kind != VLine || math.Abs(r.Value-2*math.Pi) > 1e-12 {
		t.Errorf("got %+v", r)
	}
}

func TestDegrees(t *testing.T) {
	r := Compile([]string{"sin(90)", "asin(1)"}, Options{Degrees: true})
	if math.Abs(r[0].Value-1) > 1e-12 || math.Abs(r[1].Value-90) > 1e-12 {
		t.Errorf("got %v, %v", r[0].Value, r[1].Value)
	}
}

func TestErrors(t *testing.T) {
	cases := map[string]string{
		"2+":          "unexpected end",
		"(2":          "missing ')'",
		"2)":          "unmatched ')'",
		"foo":         "unknown variable 'foo'",
		"foo(2)":      "unknown function 'foo'",
		"sin(1, 2)":   "sin takes 1 argument",
		"a = x":       "can't depend on x",
		"x = x":       "must be a constant",
		"pi = 3":      "built-in constant",
		"2 = 3":       "left side",
		"y = ":        "missing expression",
		"a = 1 = 2":   "only one '='",
		"2 $ 3":       "unexpected character",
		"f(x, x) = x": "repeated",
		"y = q":       "unknown variable 'q'",
		"sin":         "needs an argument",
	}
	for src, want := range cases {
		r := eval1(t, src)
		if r.Err == nil {
			t.Errorf("%q: expected error containing %q, got %+v", src, want, r)
			continue
		}
		if !strings.Contains(r.Err.Error(), want) {
			t.Errorf("%q: error %q does not contain %q", src, r.Err, want)
		}
	}
}

func TestCrossEntryErrors(t *testing.T) {
	rs := Compile([]string{"a = b + 1", "b = a", "c = a", "d = 1", "d = 2", "f(x) = f(x)"}, Options{})
	for i := 0; i < 2; i++ {
		if rs[i].Err == nil || !strings.Contains(rs[i].Err.Error(), "circular") {
			t.Errorf("entry %d: want circular error, got %v", i, rs[i].Err)
		}
	}
	if rs[2].Err == nil || !strings.Contains(rs[2].Err.Error(), "'a' has an error") {
		t.Errorf("entry 2: got %v", rs[2].Err)
	}
	for i := 3; i < 5; i++ {
		if rs[i].Err == nil || !strings.Contains(rs[i].Err.Error(), "more than once") {
			t.Errorf("entry %d: want duplicate error, got %v", i, rs[i].Err)
		}
	}
	if rs[5].Err == nil || !strings.Contains(rs[5].Err.Error(), "circular") {
		t.Errorf("recursive function: got %v", rs[5].Err)
	}
}

func TestEmpty(t *testing.T) {
	if r := eval1(t, "   "); r.Kind != Empty {
		t.Errorf("got %+v", r)
	}
}

func TestFormatNumber(t *testing.T) {
	cases := map[float64]string{
		0.1 + 0.2:            "0.3",
		1.0 / 3:              "0.333333333333",
		4:                    "4",
		-2.5:                 "-2.5",
		1e20:                 "1e20",
		1.5e-9:               "1.5e-9",
		123456789012:         "123456789012",
		math.NaN():           "undefined",
		math.Inf(-1):         "-∞",
		math.Copysign(0, -1): "0",
	}
	for v, want := range cases {
		if got := FormatNumber(v); got != want {
			t.Errorf("FormatNumber(%v) = %q, want %q", v, got, want)
		}
	}
}

func TestFormatTick(t *testing.T) {
	cases := []struct {
		v, step float64
		want    string
	}{
		{5, 1, "5"},
		{-0.5, 0.1, "-0.5"},
		{0.25, 0.05, "0.25"},
		{1e-12, 0.2, "0"},
		{2e6, 1e6, "2e6"},
		{2.5e7, 5e6, "2.5e7"},
		{3e-5, 1e-5, "3e-5"},
	}
	for _, c := range cases {
		if got := FormatTick(c.v, c.step); got != c.want {
			t.Errorf("FormatTick(%v, %v) = %q, want %q", c.v, c.step, got, c.want)
		}
	}
}

func TestNiceStep(t *testing.T) {
	cases := map[float64]float64{0.7: 1, 1: 1, 1.1: 2, 3: 5, 7: 10, 0.013: 0.02}
	for raw, want := range cases {
		if got := NiceStep(raw); math.Abs(got-want) > 1e-12 {
			t.Errorf("NiceStep(%v) = %v, want %v", raw, got, want)
		}
	}
}
