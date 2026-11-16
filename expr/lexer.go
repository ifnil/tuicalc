// Package expr implements the math language used by the calculator: a
// lexer, a parser that understands implicit multiplication, and a compiler
// that turns a list of entries into evaluable values and functions.
package expr

import (
	"fmt"
	"strconv"
	"unicode"
)

type tokKind uint8

const (
	tokEOF tokKind = iota
	tokNum
	tokIdent
	tokOp // one of + - * / ^ % !
	tokLParen
	tokRParen
	tokComma
	tokEq
	tokBar
)

type token struct {
	kind tokKind
	op   rune
	text string
	num  float64
	pos  int
}

func (t token) String() string {
	switch t.kind {
	case tokEOF:
		return "end of input"
	case tokNum, tokIdent:
		return fmt.Sprintf("'%s'", t.text)
	case tokOp:
		return fmt.Sprintf("'%c'", t.op)
	case tokLParen:
		return "'('"
	case tokRParen:
		return "')'"
	case tokComma:
		return "','"
	case tokEq:
		return "'='"
	case tokBar:
		return "'|'"
	}
	return "token"
}

func isDigit(r rune) bool      { return r >= '0' && r <= '9' }
func isIdentStart(r rune) bool { return r == '_' || unicode.IsLetter(r) }
func isIdentChar(r rune) bool  { return isIdentStart(r) || isDigit(r) }

func lex(src string) ([]token, error) {
	rs := []rune(src)
	var toks []token
	op := func(r rune, pos int) { toks = append(toks, token{kind: tokOp, op: r, pos: pos}) }

	for i := 0; i < len(rs); {
		c := rs[i]
		switch {
		case unicode.IsSpace(c):
			i++

		case isDigit(c) || (c == '.' && i+1 < len(rs) && isDigit(rs[i+1])):
			start := i
			for i < len(rs) && isDigit(rs[i]) {
				i++
			}
			if i < len(rs) && rs[i] == '.' {
				i++
				for i < len(rs) && isDigit(rs[i]) {
					i++
				}
			}
			// Scientific notation only when an exponent actually follows,
			// so "2e" and "2ex" still mean 2·e and 2·e·x.
			if i < len(rs) && (rs[i] == 'e' || rs[i] == 'E') {
				j := i + 1
				if j < len(rs) && (rs[j] == '+' || rs[j] == '-') {
					j++
				}
				if j < len(rs) && isDigit(rs[j]) {
					i = j
					for i < len(rs) && isDigit(rs[i]) {
						i++
					}
				}
			}
			text := string(rs[start:i])
			v, err := strconv.ParseFloat(text, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid number '%s'", text)
			}
			toks = append(toks, token{kind: tokNum, num: v, text: text, pos: start})

		case isIdentStart(c):
			start := i
			for i < len(rs) && isIdentChar(rs[i]) {
				i++
			}
			toks = append(toks, token{kind: tokIdent, text: string(rs[start:i]), pos: start})

		default:
			pos := i
			i++
			switch c {
			case '+', '/', '^', '%', '!':
				op(c, pos)
			case '-', '−':
				op('-', pos)
			case '*':
				if i < len(rs) && rs[i] == '*' { // ** is power
					i++
					op('^', pos)
				} else {
					op('*', pos)
				}
			case '×', '·', '⋅', '∙':
				op('*', pos)
			case '÷':
				op('/', pos)
			case '²', '³':
				op('^', pos)
				n := 2.0
				if c == '³' {
					n = 3
				}
				toks = append(toks, token{kind: tokNum, num: n, text: strconv.Itoa(int(n)), pos: pos})
			case '(':
				toks = append(toks, token{kind: tokLParen, pos: pos})
			case ')':
				toks = append(toks, token{kind: tokRParen, pos: pos})
			case ',':
				toks = append(toks, token{kind: tokComma, pos: pos})
			case '=':
				toks = append(toks, token{kind: tokEq, pos: pos})
			case '|':
				toks = append(toks, token{kind: tokBar, pos: pos})
			default:
				return nil, fmt.Errorf("unexpected character '%c'", c)
			}
		}
	}
	toks = append(toks, token{kind: tokEOF, pos: len(rs)})
	return toks, nil
}
