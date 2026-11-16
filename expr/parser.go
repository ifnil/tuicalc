package expr

import "fmt"

// AST nodes. Names in varNode and callNode are already resolved by the
// parser: every varNode refers to a parameter, user variable or constant,
// and every callNode to a builtin or user function.
type node interface{}

type (
	numNode  struct{ v float64 }
	varNode  struct{ name string }
	negNode  struct{ x node }
	factNode struct{ x node }
	binNode  struct {
		op   rune
		l, r node
	}
	callNode struct {
		name string
		args []node
	}
)

type nameKind uint8

const (
	nameUnknown nameKind = iota
	nameVar
	nameFunc
)

// scope answers name lookups for the parser and records which names the
// parsed body depends on.
type scope struct {
	params    []string
	userVars  map[string]bool
	userFuncs map[string]int // name -> arity

	usedParams map[string]bool
	deps       map[string]bool // user definitions referenced
}

func newScope(params []string, userVars map[string]bool, userFuncs map[string]int) *scope {
	return &scope{
		params:     params,
		userVars:   userVars,
		userFuncs:  userFuncs,
		usedParams: map[string]bool{},
		deps:       map[string]bool{},
	}
}

func (s *scope) isParam(name string) bool {
	for _, p := range s.params {
		if p == name {
			return true
		}
	}
	return false
}

func (s *scope) lookup(name string) nameKind {
	switch {
	case s.isParam(name):
		return nameVar
	case s.userVars[name]:
		return nameVar
	case s.userFuncs[name] > 0:
		return nameFunc
	}
	if _, ok := constants[name]; ok {
		return nameVar
	}
	if _, ok := builtins[name]; ok {
		return nameFunc
	}
	return nameUnknown
}

func (s *scope) use(name string) {
	if s.isParam(name) {
		s.usedParams[name] = true
	} else if s.userVars[name] || s.userFuncs[name] > 0 {
		s.deps[name] = true
	}
}

type parser struct {
	toks     []token
	i        int
	sc       *scope
	absDepth int
}

func parseBody(toks []token, sc *scope) (node, error) {
	p := &parser{toks: toks, sc: sc}
	if p.peek().kind == tokEOF {
		return nil, fmt.Errorf("empty expression")
	}
	n, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tokEOF {
		if t.kind == tokRParen {
			return nil, fmt.Errorf("unmatched ')'")
		}
		return nil, fmt.Errorf("unexpected %s", t)
	}
	return n, nil
}

func (p *parser) peek() token { return p.toks[p.i] }

func (p *parser) next() token {
	t := p.toks[p.i]
	if t.kind != tokEOF {
		p.i++
	}
	return t
}

func (p *parser) isOp(ops ...rune) (rune, bool) {
	t := p.peek()
	if t.kind != tokOp {
		return 0, false
	}
	for _, o := range ops {
		if t.op == o {
			return o, true
		}
	}
	return 0, false
}

// startsImplicit reports whether t can begin the right-hand factor of an
// implicit multiplication such as 2x, x(x+1) or 3|x|.
func (p *parser) startsImplicit(t token) bool {
	switch t.kind {
	case tokIdent, tokLParen:
		return true
	case tokBar:
		return p.absDepth == 0
	}
	return false
}

func (p *parser) isFuncIdent(t token) bool {
	return t.kind == tokIdent && p.sc.lookup(t.text) == nameFunc
}

func (p *parser) parseExpr() (node, error) {
	l, err := p.parseMul()
	if err != nil {
		return nil, err
	}
	for {
		op, ok := p.isOp('+', '-')
		if !ok {
			return l, nil
		}
		p.next()
		r, err := p.parseMul()
		if err != nil {
			return nil, err
		}
		l = binNode{op, l, r}
	}
}

func (p *parser) parseMul() (node, error) {
	l, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		if op, ok := p.isOp('*', '/', '%'); ok {
			p.next()
			r, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			l = binNode{op, l, r}
			continue
		}
		if p.startsImplicit(p.peek()) {
			r, err := p.parsePower()
			if err != nil {
				return nil, err
			}
			l = binNode{'*', l, r}
			continue
		}
		return l, nil
	}
}

func (p *parser) parseUnary() (node, error) {
	if op, ok := p.isOp('-', '+'); ok {
		p.next()
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		if op == '-' {
			return negNode{x}, nil
		}
		return x, nil
	}
	return p.parsePower()
}

func (p *parser) parsePower() (node, error) {
	lead, base, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		if _, ok := p.isOp('!'); !ok {
			break
		}
		p.next()
		base = factNode{base}
	}
	if _, ok := p.isOp('^'); ok {
		p.next()
		exp, err := p.parseUnary() // right associative, allows 2^-x
		if err != nil {
			return nil, err
		}
		base = binNode{'^', base, exp}
	}
	// A split identifier such as "xe" in xe^x only raises its last factor.
	for i := len(lead) - 1; i >= 0; i-- {
		base = binNode{'*', lead[i], base}
	}
	return base, nil
}

// parsePrimary returns the primary expression. When an unknown identifier
// was split into several known names (e.g. "pix" → pi·x), the leading
// factors are returned separately so that postfix operators and powers
// bind only to the last one.
func (p *parser) parsePrimary() ([]node, node, error) {
	t := p.peek()
	switch t.kind {
	case tokNum:
		p.next()
		return nil, numNode{t.num}, nil

	case tokLParen:
		p.next()
		saved := p.absDepth
		p.absDepth = 0
		n, err := p.parseExpr()
		p.absDepth = saved
		if err != nil {
			return nil, nil, err
		}
		if p.peek().kind != tokRParen {
			return nil, nil, fmt.Errorf("missing ')'")
		}
		p.next()
		return nil, n, nil

	case tokBar:
		p.next()
		p.absDepth++
		n, err := p.parseExpr()
		p.absDepth--
		if err != nil {
			return nil, nil, err
		}
		if p.peek().kind != tokBar {
			return nil, nil, fmt.Errorf("missing closing '|'")
		}
		p.next()
		return nil, callNode{"abs", []node{n}}, nil

	case tokIdent:
		return p.parseIdent()

	case tokEOF:
		return nil, nil, fmt.Errorf("unexpected end of expression")
	}
	return nil, nil, fmt.Errorf("unexpected %s", t)
}

func (p *parser) parseIdent() ([]node, node, error) {
	t := p.next()
	name := t.text
	switch p.sc.lookup(name) {
	case nameVar:
		p.sc.use(name)
		return nil, varNode{name}, nil
	case nameFunc:
		n, err := p.parseCall(name)
		return nil, n, err
	}

	parts := splitName([]rune(name), p.sc.lookup)
	if len(parts) < 2 {
		if p.peek().kind == tokLParen {
			return nil, nil, fmt.Errorf("unknown function '%s'", name)
		}
		return nil, nil, fmt.Errorf("unknown variable '%s'", name)
	}
	var lead []node
	for _, part := range parts[:len(parts)-1] {
		p.sc.use(part)
		lead = append(lead, varNode{part})
	}
	last := parts[len(parts)-1]
	if p.sc.lookup(last) == nameFunc {
		n, err := p.parseCall(last)
		return lead, n, err
	}
	p.sc.use(last)
	return lead, varNode{last}, nil
}

// splitName splits an unknown identifier into known names, trying longer
// names first: "pix" → [pi x], "xsin" → [x sin]. Only the final part may be
// a function. It returns nil if no split exists.
func splitName(rs []rune, lookup func(string) nameKind) []string {
	if len(rs) == 0 {
		return []string{}
	}
	if len(rs) > 24 {
		return nil
	}
	for l := len(rs); l >= 1; l-- {
		part := string(rs[:l])
		k := lookup(part)
		if k == nameVar || (k == nameFunc && l == len(rs)) {
			if rest := splitName(rs[l:], lookup); rest != nil {
				return append([]string{part}, rest...)
			}
		}
	}
	return nil
}

func (p *parser) parseCall(name string) (node, error) {
	p.sc.use(name)
	minArgs, maxArgs := p.arity(name)

	var args []node
	if p.peek().kind == tokLParen {
		p.next()
		saved := p.absDepth
		p.absDepth = 0
		if p.peek().kind != tokRParen {
			for {
				a, err := p.parseExpr()
				if err != nil {
					p.absDepth = saved
					return nil, err
				}
				args = append(args, a)
				if p.peek().kind != tokComma {
					break
				}
				p.next()
			}
		}
		p.absDepth = saved
		if p.peek().kind != tokRParen {
			return nil, fmt.Errorf("missing ')' after arguments to %s", name)
		}
		p.next()
	} else {
		// Function application without parentheses: "sin x", "ln 2x".
		// The argument is a run of implicitly multiplied factors that stops
		// at the next function name, so "sin x cos x" is sin(x)·cos(x).
		t := p.peek()
		if t.kind != tokNum && !p.startsImplicit(t) {
			return nil, fmt.Errorf("%s needs an argument, like %s(x)", name, name)
		}
		arg, err := p.parsePower()
		if err != nil {
			return nil, err
		}
		for p.startsImplicit(p.peek()) && !p.isFuncIdent(p.peek()) {
			r, err := p.parsePower()
			if err != nil {
				return nil, err
			}
			arg = binNode{'*', arg, r}
		}
		args = []node{arg}
	}

	if len(args) < minArgs || (maxArgs >= 0 && len(args) > maxArgs) {
		return nil, arityError(name, minArgs, maxArgs, len(args))
	}
	return callNode{name, args}, nil
}

func (p *parser) arity(name string) (int, int) {
	if !p.sc.isParam(name) {
		if n := p.sc.userFuncs[name]; n > 0 {
			return n, n
		}
	}
	b := builtins[name]
	return b.minArgs, b.maxArgs
}

func arityError(name string, minArgs, maxArgs, got int) error {
	plural := func(n int) string {
		if n == 1 {
			return "argument"
		}
		return "arguments"
	}
	switch {
	case maxArgs < 0:
		return fmt.Errorf("%s takes at least %d %s", name, minArgs, plural(minArgs))
	case minArgs == maxArgs:
		return fmt.Errorf("%s takes %d %s, got %d", name, minArgs, plural(minArgs), got)
	default:
		return fmt.Errorf("%s takes %d to %d arguments, got %d", name, minArgs, maxArgs, got)
	}
}
