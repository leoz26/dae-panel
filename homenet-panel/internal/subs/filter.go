package subs

import (
	"errors"
	"regexp"
	"strings"
)

// MatchGroup 判断节点是否命中分组过滤条件。
// dae 语义：多行 filter 之间是 OR；同一行内支持 && / ||，括号分组。
func MatchGroup(filters []string, n Node) bool {
	for _, f := range filters {
		if strings.TrimSpace(f) == "" {
			continue
		}
		if evalExpr(f, n) {
			return true
		}
	}
	return false
}

func evalExpr(s string, n Node) bool {
	toks, err := tokenize(s)
	if err != nil || len(toks) == 0 {
		return false
	}
	p := &parser{toks: toks}
	v := p.orExpr(n)
	return v
}

type tok struct {
	kind string
	val  string
}

var errUnterminated = errors.New("未闭合的引号")

func tokenize(s string) ([]tok, error) {
	var out []tok
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(':
			out = append(out, tok{"lp", "("})
			i++
		case c == ')':
			out = append(out, tok{"rp", ")"})
			i++
		case c == ',':
			out = append(out, tok{"comma", ","})
			i++
		case c == ':':
			out = append(out, tok{"colon", ":"})
			i++
		case c == '\'' || c == '"':
			q := c
			j := i + 1
			var b strings.Builder
			for j < len(s) && s[j] != q {
				if s[j] == '\\' && j+1 < len(s) {
					j++
					b.WriteByte(s[j])
					j++
				} else {
					b.WriteByte(s[j])
					j++
				}
			}
			if j >= len(s) {
				return nil, errUnterminated
			}
			out = append(out, tok{"str", b.String()})
			i = j + 1
		case c == '&' && i+1 < len(s) && s[i+1] == '&':
			out = append(out, tok{"op", "&&"})
			i += 2
		case c == '|' && i+1 < len(s) && s[i+1] == '|':
			out = append(out, tok{"op", "||"})
			i += 2
		default:
			j := i
			for j < len(s) && !isDelim(s, j) {
				j++
			}
			if j == i {
				j++
			}
			out = append(out, tok{"word", s[i:j]})
			i = j
		}
	}
	return out, nil
}

func isDelim(s string, j int) bool {
	c := s[j]
	if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '(' || c == ')' || c == ',' || c == ':' || c == '\'' || c == '"' {
		return true
	}
	if c == '&' && j+1 < len(s) && s[j+1] == '&' {
		return true
	}
	if c == '|' && j+1 < len(s) && s[j+1] == '|' {
		return true
	}
	return false
}

type parser struct {
	toks []tok
	pos  int
}

func (p *parser) eof() bool { return p.pos >= len(p.toks) }
func (p *parser) peek() tok {
	if p.eof() {
		return tok{}
	}
	return p.toks[p.pos]
}
func (p *parser) next() tok { t := p.peek(); p.pos++; return t }

func (p *parser) matchLP() bool {
	if p.peek().kind == "lp" {
		p.pos++
		return true
	}
	return false
}
func (p *parser) matchComma() bool {
	if p.peek().kind == "comma" {
		p.pos++
		return true
	}
	return false
}
func (p *parser) matchOp(op string) bool {
	if p.peek().kind == "op" && p.peek().val == op {
		p.pos++
		return true
	}
	return false
}

func (p *parser) orExpr(n Node) bool {
	v := p.andExpr(n)
	for p.matchOp("||") {
		r := p.andExpr(n)
		v = v || r
	}
	return v
}

func (p *parser) andExpr(n Node) bool {
	v := p.unary(n)
	for p.matchOp("&&") {
		r := p.unary(n)
		v = v && r
	}
	return v
}

func (p *parser) unary(n Node) bool {
	if p.matchLP() {
		v := p.orExpr(n)
		if p.peek().kind == "rp" {
			p.pos++
		}
		return v
	}
	return p.predicate(n)
}

type arg struct {
	key string
	val string
}

func (p *parser) predicate(n Node) bool {
	fnTok := p.next()
	if fnTok.kind != "word" {
		return false
	}
	if p.peek().kind != "lp" {
		return false
	}
	p.pos++ // consume '('
	var args []arg
	for !p.eof() && p.peek().kind != "rp" {
		var a arg
		// key: value ?
		if p.peek().kind == "word" && p.pos+1 < len(p.toks) && p.toks[p.pos+1].kind == "colon" {
			a.key = p.next().val
			p.pos++ // colon
			a.val = p.nextValue()
		} else {
			a.val = p.nextValue()
		}
		args = append(args, a)
		if !p.matchComma() {
			break
		}
	}
	if p.peek().kind == "rp" {
		p.pos++
	}
	return evalPredicate(fnTok.val, args, n)
}

func (p *parser) nextValue() string {
	t := p.peek()
	if t.kind == "str" || t.kind == "word" {
		p.pos++
		return t.val
	}
	return ""
}

func evalPredicate(fn string, args []arg, n Node) bool {
	switch strings.ToLower(fn) {
	case "subtag":
		if len(args) == 0 {
			return false
		}
		return strings.EqualFold(n.Subtag, args[0].val)
	case "name":
		if len(args) == 0 {
			return false
		}
		mode := strings.ToLower(args[0].key)
		val := args[0].val
		switch mode {
		case "regex":
			re, err := regexp.Compile(val)
			if err != nil {
				return false
			}
			return re.MatchString(n.Tag)
		case "full":
			return n.Tag == val
		case "prefix":
			return strings.HasPrefix(n.Tag, val)
		case "suffix":
			return strings.HasSuffix(n.Tag, val)
		default: // keyword
			return strings.Contains(n.Tag, val)
		}
	}
	return false
}
