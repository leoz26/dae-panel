package model

import (
	"fmt"
	"strings"
)

// sq 是单引号字符，用于渲染 dae 配置里的字符串字面量。
const sq = "\u0027"

// DNSUpstream 是 dns 块 upstream 里的一项（名字 → 地址）。
type DNSUpstream struct {
	Name string `json:"name"`
	Addr string `json:"addr"` // 如 udp://223.5.5.5:53 / tcp://8.8.8.8:53 / https://1.1.1.1/dns-query
}

// DNSRule 是 dns routing 里的一条规则（qname/qtype … -> 上游名 / reject / accept）。
type DNSRule struct {
	Matcher string `json:"matcher"`
	Out     string `json:"out"`
}

// DNSSpec 是 dns 块的结构化视图。
type DNSSpec struct {
	Upstreams        []DNSUpstream `json:"upstreams"`
	Request          []DNSRule     `json:"request"`
	RequestFallback  string        `json:"requestFallback"`
	Response         []DNSRule     `json:"response"`
	ResponseFallback string        `json:"responseFallback"`
}

// DefaultDNSSpec 是面板新建/空配置时使用的默认 DNS（与历史生成的配置一致）。
func DefaultDNSSpec() DNSSpec {
	return DNSSpec{
		Upstreams:        []DNSUpstream{{Name: "cn", Addr: "udp://223.5.5.5:53"}},
		RequestFallback:  "cn",
		ResponseFallback: "accept",
	}
}

// ParseDNS 把 dns 块正文解析成结构化视图。解析是宽松的：看不懂的行忽略，不报错。
func ParseDNS(body string) DNSSpec {
	var s DNSSpec
	var stack []string // 当前所在块：upstream / routing / request / response
	for _, raw := range strings.Split(body, "\n") {
		line := stripLineComment(raw)
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if t == "}" {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			continue
		}
		if name, ok := blockOpen(t); ok {
			stack = append(stack, name)
			continue
		}
		ctx := ""
		if len(stack) > 0 {
			ctx = stack[len(stack)-1]
		}
		switch ctx {
		case "upstream":
			if k, v, ok := splitKV(t); ok {
				s.Upstreams = append(s.Upstreams, DNSUpstream{Name: k, Addr: v})
			}
		case "request", "response":
			if strings.HasPrefix(t, "fallback") {
				if _, v, ok := splitKV(t); ok {
					if ctx == "request" {
						s.RequestFallback = v
					} else {
						s.ResponseFallback = v
					}
				}
				continue
			}
			if i := strings.Index(t, "->"); i > 0 {
				r := DNSRule{Matcher: strings.TrimSpace(t[:i]), Out: strings.TrimSpace(t[i+2:])}
				if r.Matcher == "" || r.Out == "" {
					continue
				}
				if ctx == "request" {
					s.Request = append(s.Request, r)
				} else {
					s.Response = append(s.Response, r)
				}
			}
		}
	}
	return s
}

// RenderDNS 把结构化视图渲染成 dns 块正文（4 空格基准缩进，可直接放回 Model.DNS）。
func RenderDNS(s DNSSpec) string {
	var b strings.Builder
	if len(s.Upstreams) > 0 {
		b.WriteString("    upstream {\n")
		for _, u := range s.Upstreams {
			if strings.TrimSpace(u.Name) == "" {
				continue
			}
			fmt.Fprintf(&b, "        %s: %s\n", strings.TrimSpace(u.Name), quoteAddr(u.Addr))
		}
		b.WriteString("    }\n")
	}
	if len(s.Request) > 0 || strings.TrimSpace(s.RequestFallback) != "" || len(s.Response) > 0 || strings.TrimSpace(s.ResponseFallback) != "" {
		b.WriteString("    routing {\n")
		if len(s.Request) > 0 || strings.TrimSpace(s.RequestFallback) != "" {
			b.WriteString("        request {\n")
			for _, r := range s.Request {
				if strings.TrimSpace(r.Matcher) == "" || strings.TrimSpace(r.Out) == "" {
					continue
				}
				fmt.Fprintf(&b, "            %s -> %s\n", strings.TrimSpace(r.Matcher), strings.TrimSpace(r.Out))
			}
			if f := strings.TrimSpace(s.RequestFallback); f != "" {
				fmt.Fprintf(&b, "            fallback: %s\n", f)
			}
			b.WriteString("        }\n")
		}
		if len(s.Response) > 0 || strings.TrimSpace(s.ResponseFallback) != "" {
			b.WriteString("        response {\n")
			for _, r := range s.Response {
				if strings.TrimSpace(r.Matcher) == "" || strings.TrimSpace(r.Out) == "" {
					continue
				}
				fmt.Fprintf(&b, "            %s -> %s\n", strings.TrimSpace(r.Matcher), strings.TrimSpace(r.Out))
			}
			if f := strings.TrimSpace(s.ResponseFallback); f != "" {
				fmt.Fprintf(&b, "            fallback: %s\n", f)
			}
			b.WriteString("        }\n")
		}
		b.WriteString("    }\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// LintDNS 检查 DNS 结构化视图的常见错误：上游重名/空地址、规则引用了不存在上游、协议前缀不认识。
func LintDNS(s DNSSpec) []string {
	var out []string
	names := map[string]bool{}
	for _, u := range s.Upstreams {
		n := strings.TrimSpace(u.Name)
		if n == "" {
			out = append(out, "上游服务器缺少名字")
			continue
		}
		if names[n] {
			out = append(out, fmt.Sprintf("上游服务器名字重复：%s", n))
		}
		names[n] = true
		a := strings.TrimSpace(u.Addr)
		if a == "" {
			out = append(out, fmt.Sprintf("上游 %s 没有填地址", n))
			continue
		}
		if !hasUpstreamScheme(a) {
			out = append(out, fmt.Sprintf("上游 %s 的地址 %q 缺少协议前缀（udp:// / tcp:// / tls:// / https:// / h3://）", n, a))
		}
	}
	checkOut := func(rules []DNSRule, where, fallback string, allowAccept bool) {
		for _, r := range rules {
			o := strings.TrimSpace(r.Out)
			if o == "" || names[o] || o == "reject" {
				continue
			}
			if o == "drop" {
				out = append(out, fmt.Sprintf("%s规则 %q 的动作用了 drop，dae 不认（实测报 upstream \"drop\" not found），请改成 reject", where, r.Matcher))
				continue
			}
			if o == "accept" {
				if !allowAccept {
					out = append(out, fmt.Sprintf("%s规则 %q 用了 accept，请求规则里只能填上游名或 reject（实测报 upstream \"accept\" not found）", where, r.Matcher))
				}
				continue
			}
			out = append(out, fmt.Sprintf("%s规则 %q 指向的上游 %q 不存在", where, r.Matcher, o))
		}
		if f := strings.TrimSpace(fallback); f != "" && !names[f] && f != "reject" && f != "drop" && !(allowAccept && f == "accept") {
			out = append(out, fmt.Sprintf("%s兜底指向的上游 %q 不存在", where, f))
		}
	}
	checkOut(s.Request, "请求", s.RequestFallback, false)
	checkOut(s.Response, "响应", s.ResponseFallback, true)
	for _, r := range s.Request {
		if strings.Contains(strings.TrimSpace(r.Matcher), "geolocation") {
			out = append(out, fmt.Sprintf("%q 里的 geolocation: 在 dns 规则里不支持（实测报 addQName: unsupported key），请用 qname(geosite:cn)", r.Matcher))
		}
	}
	return out
}

func hasUpstreamScheme(a string) bool {
	for _, p := range []string{"udp://", "tcp://", "tcp+udp://", "udp+tcp://", "tls://", "https://", "h3://", "quic://"} {
		if strings.HasPrefix(a, p) {
			return true
		}
	}
	return false
}

// quoteAddr 给地址加单引号（已经带引号则原样返回）。
func quoteAddr(a string) string {
	a = strings.TrimSpace(a)
	if a == "" {
		return sq + sq
	}
	if strings.HasPrefix(a, sq) || strings.HasPrefix(a, "\u0022") {
		return a
	}
	return sq + a + sq
}

// blockOpen 判断一行是否是块开头（name {），返回块名。
func blockOpen(t string) (string, bool) {
	i := strings.Index(t, "{")
	if i < 0 {
		return "", false
	}
	name := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(t[:i]), ":"))
	if name == "" {
		return "", false
	}
	if strings.ContainsAny(name, " ()") {
		return "", false
	}
	return name, true
}

// splitKV 把 key: value 拆开并去掉值的引号。
func splitKV(t string) (string, string, bool) {
	i := strings.Index(t, ":")
	if i <= 0 {
		return "", "", false
	}
	k := strings.TrimSpace(t[:i])
	v := strings.TrimSpace(t[i+1:])
	if k == "" || v == "" {
		return "", "", false
	}
	v = strings.Trim(v, sq+"\u0022")
	return k, v, true
}

// stripLineComment 去掉行内 # 注释（引号内的 # 保留）。
func stripLineComment(line string) string {
	var q byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		if q != 0 {
			if c == q {
				q = 0
			}
			continue
		}
		if c == 0x27 || c == 0x22 {
			q = c
			continue
		}
		if c == 0x23 {
			return line[:i]
		}
	}
	return line
}
