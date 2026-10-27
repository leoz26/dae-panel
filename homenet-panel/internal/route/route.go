package route

import (
	"regexp"
	"strings"

	"homenet-panel/internal/geo"
)

// NormalizeHost 把用户输入归一成可匹配的主机名：
// 去掉协议、路径、查询串、片段、端口、结尾点与通配前缀，全小写。
// 支持 "https://www.example.com/a?b#c"、"example.com:443"、"[::1]:443"、"*.example.com"。
func NormalizeHost(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	if strings.HasPrefix(s, "[") {
		if j := strings.Index(s, "]"); j > 0 {
			s = s[1:j]
		}
	} else if strings.Count(s, ":") == 1 {
		if j := strings.LastIndex(s, ":"); j >= 0 {
			s = s[:j]
		}
	}
	s = strings.TrimPrefix(s, "*.")
	s = strings.TrimSuffix(s, ".")
	return strings.ToLower(s)
}

// val 是三态求值结果。
type val int

const (
	vNo  val = iota // 不命中
	vYes            // 命中
	vUnk            // 未知（依赖地址/解析，如 geoip）
	vCtx            // 未知但仅为连接上下文谓词（pname/mac/l4proto 等），与域名维度无关
)

// Eval 对分流匹配表达式求值。host 为待测域名；db 可为 nil。
func Eval(expr, host string, db *geo.DB) val {
	p := &parser{s: expr, host: strings.ToLower(strings.TrimSuffix(host, ".")), db: db}
	v := p.parseOr()
	return v
}

type parser struct {
	s    string
	i    int
	host string
	db   *geo.DB
}

func (p *parser) skip() {
	for p.i < len(p.s) && (p.s[p.i] == ' ' || p.s[p.i] == '\t' || p.s[p.i] == '\n') {
		p.i++
	}
}

func (p *parser) parseOr() val {
	v := p.parseAnd()
	for {
		p.skip()
		if strings.HasPrefix(p.s[p.i:], "||") {
			p.i += 2
			r := p.parseAnd()
			v = orVal(v, r)
		} else {
			return v
		}
	}
}

func (p *parser) parseAnd() val {
	v := p.parseUnary()
	for {
		p.skip()
		if strings.HasPrefix(p.s[p.i:], "&&") {
			p.i += 2
			r := p.parseUnary()
			v = andVal(v, r)
		} else {
			return v
		}
	}
}

func (p *parser) parseUnary() val {
	p.skip()
	if p.i < len(p.s) && p.s[p.i] == '!' {
		p.i++
		return notVal(p.parseUnary())
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() val {
	p.skip()
	if p.i < len(p.s) && p.s[p.i] == '(' {
		p.i++
		v := p.parseOr()
		p.skip()
		if p.i < len(p.s) && p.s[p.i] == ')' {
			p.i++
		}
		return v
	}
	// 读取标识符
	start := p.i
	for p.i < len(p.s) && (isIdent(p.s[p.i])) {
		p.i++
	}
	name := p.s[start:p.i]
	p.skip()
	if p.i < len(p.s) && p.s[p.i] == '(' {
		// 读取到配对的右括号
		depth := 0
		j := p.i
		var sb strings.Builder
		for j < len(p.s) {
			c := p.s[j]
			if c == '(' {
				depth++
				if depth == 1 {
					j++
					continue
				}
			} else if c == ')' {
				depth--
				if depth == 0 {
					j++
					break
				}
			}
			sb.WriteByte(c)
			j++
		}
		p.i = j
		return p.predicate(name, strings.TrimSpace(sb.String()))
	}
	return vUnk
}

func isIdent(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func (p *parser) predicate(name, inner string) val {
	switch name {
	case "domain":
		return p.evalDomain(inner)
	case "pname", "process_name", "pid", "uid", "gid", "cgroup", "mac", "l4proto", "proto":
		return vCtx // 连接上下文谓词，与“域名走哪条”无关
	default:
		return vUnk // ip/sip/dip/ipset 等地址谓词，需解析/运行时数据
	}
}

func (p *parser) evalDomain(inner string) val {
	// domain(...) 支持逗号分隔的多个参数，任一命中即命中（OR）。
	terms := splitTopComma(inner)
	if len(terms) > 1 {
		var acc val = vNo
		anyKnown := false
		for _, t := range terms {
			r := p.evalDomainTerm(t)
			if r == vNo || r == vYes {
				anyKnown = true
			}
			acc = orVal(acc, r)
		}
		if acc == vNo && !anyKnown {
			return vUnk
		}
		return acc
	}
	return p.evalDomainTerm(inner)
}

func splitTopComma(s string) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	out = append(out, strings.TrimSpace(s[start:]))
	return out
}

func (p *parser) evalDomainTerm(inner string) val {
	prefix, val := "suffix", strings.TrimSpace(inner)
	if idx := strings.Index(inner, ":"); idx >= 0 {
		prefix = strings.ToLower(strings.TrimSpace(inner[:idx]))
		val = strings.TrimSpace(inner[idx+1:])
	}
	val = strings.ToLower(strings.Trim(val, "'\""))
	switch prefix {
	case "geosite":
		m, known := p.db.Match(val, p.host)
		if !known {
			return vUnk
		}
		if m {
			return vYes
		}
		return vNo
	case "full":
		if p.host == val {
			return vYes
		}
		return vNo
	case "keyword":
		if strings.Contains(p.host, val) {
			return vYes
		}
		return vNo
	case "regex":
		if re, err := regexp.Compile(val); err == nil && re.MatchString(p.host) {
			return vYes
		}
		return vNo
	default: // suffix / domain / 无前缀
		if p.host == val || strings.HasSuffix(p.host, "."+val) {
			return vYes
		}
		return vNo
	}
}

func orVal(a, b val) val {
	if a == vYes || b == vYes {
		return vYes
	}
	if a == vUnk || b == vUnk {
		return vUnk
	}
	if a == vCtx || b == vCtx {
		return vCtx
	}
	return vNo
}

func andVal(a, b val) val {
	if a == vNo || b == vNo {
		return vNo
	}
	if a == vUnk || b == vUnk {
		return vUnk
	}
	if a == vCtx || b == vCtx {
		return vCtx
	}
	return vYes
}

func notVal(a val) val {
	switch a {
	case vYes:
		return vNo
	case vNo:
		return vYes
	case vCtx:
		return vCtx
	default:
		return vUnk
	}
}

// RuleResult 是单条规则的求值结果。
type RuleResult struct {
	Index   int    `json:"index"`
	Matcher string `json:"matcher"`
	Out     string `json:"out"`
	Result  string `json:"result"` // match | nomatch | unknown
}

// TestResult 是一次分流命中测试的结果。
type TestResult struct {
	Domain   string       `json:"domain"`
	Rules    []RuleResult `json:"rules"`
	Fallback string       `json:"fallback"`
	Chosen   string       `json:"chosen"`
	Decided  bool         `json:"decided"`
	Reason   string       `json:"reason"`
}

// Rule 是一条待测的分流规则。
type Rule struct {
	Matcher string
	Out     string
}

// Test 对给定域名按规则顺序求值，返回命中结果。
func Test(domain string, rules []Rule, fallback string, db *geo.DB) TestResult {
	res := TestResult{Domain: NormalizeHost(domain), Fallback: fallback}
	firstKnownMatch := -1
	sawUnknownBefore := false
	sawUnknown := false
	for i, r := range rules {
		v := Eval(r.Matcher, res.Domain, db)
		rr := RuleResult{Index: i, Matcher: r.Matcher, Out: r.Out}
		switch v {
		case vYes:
			rr.Result = "match"
			if firstKnownMatch < 0 {
				firstKnownMatch = i
			}
		case vNo:
			rr.Result = "nomatch"
		case vCtx:
			rr.Result = "ctx" // 上下文谓词（进程/端口/协议），域名维度不参与判定
		default:
			rr.Result = "unknown" // 地址/解析谓词，需运行时才能定
			sawUnknown = true
			if firstKnownMatch < 0 {
				sawUnknownBefore = true
			}
		}
		res.Rules = append(res.Rules, rr)
	}
	switch {
	case firstKnownMatch >= 0 && !sawUnknownBefore:
		res.Chosen = rules[firstKnownMatch].Out
		res.Decided = true
		res.Reason = "确定命中第 " + itoa(firstKnownMatch+1) + " 条规则"
	case firstKnownMatch >= 0:
		res.Chosen = rules[firstKnownMatch].Out
		res.Decided = false
		res.Reason = "可能命中更早的未知规则；若无，则命中第 " + itoa(firstKnownMatch+1) + " 条"
	case sawUnknown:
		res.Chosen = fallback
		res.Decided = false
		res.Reason = "存在依赖运行时数据（geoip/pname 等）的规则，无法本地判定；若无命中则走 fallback=" + fallback
	default:
		res.Chosen = fallback
		res.Decided = true
		res.Reason = "无规则命中，走 fallback"
	}
	return res
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
