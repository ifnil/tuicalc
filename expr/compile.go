package expr

import (
	"errors"
	"fmt"
	"math"
)

// Kind classifies a compiled entry.
type Kind int

const (
	Empty    Kind = iota
	Invalid       // the entry has an error
	Value         // a constant expression such as 2+2
	Variable      // a = 3
	Function      // f(x) = x^2
	Graph         // y = sin(x), or a bare expression in x
	VLine         // x = 3
)

// Options controls evaluation.
type Options struct {
	Degrees bool // trig functions take and return degrees
}

// Result is one compiled entry.
type Result struct {
	Kind   Kind
	Name   string   // defined name for Variable and Function
	Params []string // parameters for Function
	Value  float64  // for Value, Variable and VLine
	Err    error    // for Invalid

	// Fn evaluates the curve for Graph entries and single-parameter
	// Functions. It is nil for everything else.
	Fn func(x float64) float64

	// Literal is true when a Variable is defined by a plain number, so
	// showing its value again would be redundant.
	Literal bool
}

// Plottable reports whether the result draws something on a graph.
func (r Result) Plottable() bool { return r.Fn != nil || r.Kind == VLine }

type stmtKind int

const (
	stEmpty stmtKind = iota
	stExpr
	stGraph
	stVLine
	stVar
	stFunc
)

type stmt struct {
	kind   stmtKind
	name   string
	params []string
	body   []token
	err    error

	ast   node
	deps  map[string]bool
	usesX bool
	eval  evalFn
}

type evalFn func(args []float64) float64

var errCycle = errors.New("circular definition")

// Compile analyzes a list of entries together, so that entries can refer to
// variables and functions defined by other entries.
func Compile(srcs []string, opts Options) []Result {
	stmts := make([]*stmt, len(srcs))
	for i, src := range srcs {
		stmts[i] = splitStatement(src)
	}

	// Collect definitions so every body can see every name.
	defs := map[string]*stmt{}
	userVars := map[string]bool{}
	userFuncs := map[string]int{}
	for _, s := range stmts {
		if s.err != nil || (s.kind != stVar && s.kind != stFunc) {
			continue
		}
		if prev, dup := defs[s.name]; dup {
			err := fmt.Errorf("'%s' is defined more than once", s.name)
			prev.err, s.err = err, err
			continue
		}
		defs[s.name] = s
		if s.kind == stVar {
			userVars[s.name] = true
		} else {
			userFuncs[s.name] = len(s.params)
		}
	}

	// Parse bodies.
	for _, s := range stmts {
		if s.err != nil || s.kind == stEmpty {
			continue
		}
		params := s.params
		if s.kind != stFunc {
			params = []string{"x"}
		}
		sc := newScope(params, userVars, userFuncs)
		ast, err := parseBody(s.body, sc)
		if err != nil {
			if s.err == nil {
				s.err = err
			}
			continue
		}
		s.ast, s.deps, s.usesX = ast, sc.deps, sc.usedParams["x"]
		switch {
		case s.kind == stVar && s.usesX:
			s.err = fmt.Errorf("'%s' can't depend on x; did you mean %s(x) = …?", s.name, s.name)
		case s.kind == stVLine && s.usesX:
			s.err = fmt.Errorf("x = … must be a constant")
		case s.kind == stFunc && s.deps[s.name]:
			s.err = errCycle
		}
	}

	// Definitions that can reach themselves are circular.
	for name, s := range defs {
		if s.err == nil && reaches(defs, s, name, map[string]bool{}) {
			s.err = errCycle
		}
	}

	c := &compiler{defs: defs, opts: opts, state: map[string]int{}}
	results := make([]Result, len(stmts))
	for i, s := range stmts {
		results[i] = c.result(s)
	}
	return results
}

func reaches(defs map[string]*stmt, from *stmt, target string, seen map[string]bool) bool {
	for d := range from.deps {
		if d == target {
			return true
		}
		if seen[d] {
			continue
		}
		seen[d] = true
		if next, ok := defs[d]; ok && reaches(defs, next, target, seen) {
			return true
		}
	}
	return false
}

// splitStatement lexes an entry and classifies it by its left-hand side.
func splitStatement(src string) *stmt {
	toks, err := lex(src)
	if err != nil {
		return &stmt{kind: stExpr, err: err}
	}
	if len(toks) == 1 {
		return &stmt{kind: stEmpty}
	}

	eq := -1
	for i, t := range toks {
		if t.kind == tokEq {
			if eq >= 0 {
				return &stmt{kind: stExpr, err: fmt.Errorf("only one '=' is allowed")}
			}
			eq = i
		}
	}
	if eq < 0 {
		return &stmt{kind: stExpr, body: toks}
	}

	lhs, body := toks[:eq], toks[eq+1:]
	s := &stmt{body: body}
	if len(body) == 1 {
		s.kind = stExpr
		s.err = fmt.Errorf("missing expression after '='")
		return s
	}

	switch {
	case len(lhs) == 1 && lhs[0].kind == tokIdent:
		name := lhs[0].text
		switch name {
		case "y":
			s.kind = stGraph
		case "x":
			s.kind = stVLine
		default:
			s.kind = stVar
			s.name = name
			s.err = checkDefName(name)
		}
		return s

	case len(lhs) >= 3 && lhs[0].kind == tokIdent && lhs[1].kind == tokLParen && lhs[len(lhs)-1].kind == tokRParen:
		s.kind = stFunc
		s.name = lhs[0].text
		params, err := parseParams(lhs[2 : len(lhs)-1])
		if err != nil {
			s.err = err
			return s
		}
		s.params = params
		if s.name == "x" || s.name == "y" {
			s.err = fmt.Errorf("'%s' can't be used as a function name", s.name)
		} else {
			s.err = checkDefName(s.name)
		}
		return s
	}

	s.kind = stExpr
	s.err = fmt.Errorf("left side must be y, x, a name, or a function like f(x)")
	return s
}

func parseParams(toks []token) ([]string, error) {
	var params []string
	seen := map[string]bool{}
	for i, t := range toks {
		if i%2 == 1 {
			if t.kind != tokComma {
				return nil, fmt.Errorf("parameters must be names separated by commas")
			}
			continue
		}
		if t.kind != tokIdent {
			return nil, fmt.Errorf("parameters must be names separated by commas")
		}
		if seen[t.text] {
			return nil, fmt.Errorf("parameter '%s' is repeated", t.text)
		}
		if _, ok := builtins[t.text]; ok {
			return nil, fmt.Errorf("'%s' is a built-in function", t.text)
		}
		seen[t.text] = true
		params = append(params, t.text)
	}
	if len(params) == 0 || len(toks)%2 == 0 {
		return nil, fmt.Errorf("parameters must be names separated by commas")
	}
	return params, nil
}

func checkDefName(name string) error {
	if _, ok := constants[name]; ok {
		return fmt.Errorf("'%s' is a built-in constant", name)
	}
	if _, ok := builtins[name]; ok {
		return fmt.Errorf("'%s' is a built-in function", name)
	}
	return nil
}

type compiler struct {
	defs  map[string]*stmt
	opts  Options
	state map[string]int // 1 = compiling, 2 = done
}

func (c *compiler) result(s *stmt) Result {
	if s.kind == stEmpty {
		return Result{Kind: Empty}
	}
	if err := c.compileStmt(s); err != nil {
		return Result{Kind: Invalid, Name: s.name, Err: err}
	}
	switch s.kind {
	case stExpr:
		if s.usesX {
			return Result{Kind: Graph, Fn: unary(s.eval)}
		}
		return Result{Kind: Value, Value: s.eval(nil)}
	case stGraph:
		return Result{Kind: Graph, Fn: unary(s.eval)}
	case stVLine:
		return Result{Kind: VLine, Value: s.eval(nil)}
	case stVar:
		_, literal := s.ast.(numNode)
		if neg, ok := s.ast.(negNode); ok {
			_, literal = neg.x.(numNode)
		}
		return Result{Kind: Variable, Name: s.name, Value: s.eval(nil), Literal: literal}
	case stFunc:
		r := Result{Kind: Function, Name: s.name, Params: s.params}
		if len(s.params) == 1 {
			r.Fn = unary(s.eval)
		}
		return r
	}
	return Result{Kind: Invalid, Err: fmt.Errorf("unsupported entry")}
}

func unary(f evalFn) func(float64) float64 {
	buf := make([]float64, 1)
	return func(x float64) float64 {
		buf[0] = x
		return f(buf)
	}
}

// compileStmt compiles s after compiling the definitions it depends on.
func (c *compiler) compileStmt(s *stmt) error {
	if s.err != nil {
		return s.err
	}
	if s.eval != nil {
		return nil
	}
	if s.name != "" {
		if c.state[s.name] == 1 {
			return errCycle
		}
		c.state[s.name] = 1
		defer func() { c.state[s.name] = 2 }()
	}
	for dep := range s.deps {
		d, ok := c.defs[dep]
		if !ok {
			continue
		}
		if err := c.compileStmt(d); err != nil {
			s.err = fmt.Errorf("'%s' has an error", dep)
			return s.err
		}
	}
	params := s.params
	if s.kind != stFunc {
		params = []string{"x"}
	}
	f, err := c.compile(s.ast, params)
	if err != nil {
		s.err = err
		return err
	}
	if s.kind == stVar {
		// Variables are constants: evaluate once.
		v := f(nil)
		f = func([]float64) float64 { return v }
	}
	s.eval = f
	return nil
}

func (c *compiler) compile(n node, params []string) (evalFn, error) {
	switch n := n.(type) {
	case numNode:
		v := n.v
		return func([]float64) float64 { return v }, nil

	case varNode:
		for i, p := range params {
			if p == n.name {
				return func(a []float64) float64 { return a[i] }, nil
			}
		}
		if d, ok := c.defs[n.name]; ok && d.kind == stVar {
			v := d.eval(nil)
			return func([]float64) float64 { return v }, nil
		}
		if v, ok := constants[n.name]; ok {
			return func([]float64) float64 { return v }, nil
		}
		return nil, fmt.Errorf("unknown variable '%s'", n.name)

	case negNode:
		x, err := c.compile(n.x, params)
		if err != nil {
			return nil, err
		}
		return func(a []float64) float64 { return -x(a) }, nil

	case factNode:
		x, err := c.compile(n.x, params)
		if err != nil {
			return nil, err
		}
		return func(a []float64) float64 { return factorial(x(a)) }, nil

	case binNode:
		l, err := c.compile(n.l, params)
		if err != nil {
			return nil, err
		}
		r, err := c.compile(n.r, params)
		if err != nil {
			return nil, err
		}
		switch n.op {
		case '+':
			return func(a []float64) float64 { return l(a) + r(a) }, nil
		case '-':
			return func(a []float64) float64 { return l(a) - r(a) }, nil
		case '*':
			return func(a []float64) float64 { return l(a) * r(a) }, nil
		case '/':
			return func(a []float64) float64 { return l(a) / r(a) }, nil
		case '%':
			return func(a []float64) float64 { return mod(l(a), r(a)) }, nil
		case '^':
			if rn, ok := n.r.(numNode); ok && rn.v == 2 {
				return func(a []float64) float64 { v := l(a); return v * v }, nil
			}
			return func(a []float64) float64 { return pow(l(a), r(a)) }, nil
		}
		return nil, fmt.Errorf("unknown operator '%c'", n.op)

	case callNode:
		args := make([]evalFn, len(n.args))
		for i, an := range n.args {
			f, err := c.compile(an, params)
			if err != nil {
				return nil, err
			}
			args[i] = f
		}
		if d, ok := c.defs[n.name]; ok && d.kind == stFunc {
			return callUser(d.eval, args), nil
		}
		b, ok := builtins[n.name]
		if !ok {
			return nil, fmt.Errorf("unknown function '%s'", n.name)
		}
		return c.callBuiltin(b, args), nil
	}
	return nil, fmt.Errorf("internal error: unexpected node %T", n)
}

func callUser(body evalFn, args []evalFn) evalFn {
	return func(a []float64) float64 {
		vals := make([]float64, len(args))
		for i, f := range args {
			vals[i] = f(a)
		}
		return body(vals)
	}
}

func (c *compiler) callBuiltin(b builtin, args []evalFn) evalFn {
	in, out := 1.0, 1.0
	if c.opts.Degrees {
		if b.angleIn {
			in = math.Pi / 180
		}
		if b.angleOut {
			out = 180 / math.Pi
		}
	}
	switch {
	case b.f1 != nil:
		f, x := b.f1, args[0]
		if in == 1 && out == 1 {
			return func(a []float64) float64 { return f(x(a)) }
		}
		return func(a []float64) float64 { return f(x(a)*in) * out }
	case b.f2 != nil:
		f, x, y := b.f2, args[0], args[1]
		return func(a []float64) float64 { return f(x(a), y(a)) * out }
	default:
		f := b.fn
		return func(a []float64) float64 {
			vals := make([]float64, len(args))
			for i, g := range args {
				vals[i] = g(a)
			}
			return f(vals)
		}
	}
}
