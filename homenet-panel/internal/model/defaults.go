package model

import (
	"strings"
)

// 本文件放「新装机就该有」的默认值。与 EssentialRules 的区别：
// EssentialRules 是安全底线（缺了会出事，必须前置注入）；这里只是调优默认值，
// 只在用户/线上配置里缺该键时才补，绝不覆盖已有值。

// DefaultGlobalLines 是 global 块的调优默认值（按 key 判断缺失）。
// 起因（192.168.1.85 实测，见 architecture.md §14.50）：dae v2.1.1 的 check_tolerance 默认是 0，
// 探测抖动一次就换节点；tcp_check_url 默认打 cp.cloudflare.com，机场环境下不可靠。
var DefaultGlobalLines = []string{
	"check_interval: 60s",
	"check_tolerance: 500ms",
	"tcp_check_url: 'http://www.gstatic.com/generate_204'",
}

// globalKey 从一行 global 配置里取 key（冒号前部分），取不到返回空。
func globalKey(line string) string {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "#") {
		return ""
	}
	i := strings.Index(t, ":")
	if i <= 0 {
		return ""
	}
	return strings.TrimSpace(t[:i])
}

// EnsureGlobalDefaults 补齐 global 块里缺失的调优默认项，返回新正文与被补入的键。
// 已存在的键（哪怕值与默认不同）一律保留 —— 现场调优优先。
// bootstrap_resolver 不写死：按 dns 段里第一个 ip 形式的上游推导（见 DeriveBootstrapResolver）。
func EnsureGlobalDefaults(globalBody, dnsBody string) (string, []string) {
	present := map[string]bool{}
	for _, ln := range strings.Split(globalBody, "\n") {
		if k := globalKey(ln); k != "" {
			present[k] = true
		}
	}
	var added []string
	lines := append([]string{}, DefaultGlobalLines...)
	if ip := DeriveBootstrapResolver(dnsBody); ip != "" {
		lines = append(lines, "bootstrap_resolver: '"+ip+"'")
	}
	out := globalBody
	for _, ln := range lines {
		k := globalKey(ln)
		if k == "" || present[k] {
			continue
		}
		if out != "" && !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		out += "    " + ln + "\n"
		present[k] = true
		added = append(added, k)
	}
	return out, added
}

// DeriveBootstrapResolver 从 dns 段的 upstream 里取第一个 ip 形式的上游，返回 "ip:port"。
// dae 要求 bootstrap_resolver 必须是 ip:port（写成 udp://x:53 会报 each colon-separated field must have at least one digit）。
func DeriveBootstrapResolver(dnsBody string) string {
	inUpstream := false
	for _, ln := range strings.Split(dnsBody, "\n") {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if strings.HasSuffix(t, "{") {
			inUpstream = strings.HasPrefix(t, "upstream")
			continue
		}
		if t == "}" {
			inUpstream = false
			continue
		}
		if !inUpstream {
			continue
		}
		i := strings.Index(t, ":")
		if i <= 0 {
			continue
		}
		val := unquote(strings.TrimSpace(t[i+1:]))
		if ip, port, ok := splitNetAddr(val); ok {
			return ip + ":" + port
		}
	}
	return ""
}

// splitNetAddr 从 'udp://1.2.3.4:53' / 'tls://223.5.5.5:853' / '1.2.3.4:53' 里取出 ip 与端口。
// 只认 IP 形式（域名会让 bootstrap 变成鸡生蛋问题）。
func splitNetAddr(v string) (ip, port string, ok bool) {
	s := v
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	j := strings.LastIndex(s, ":")
	if j <= 0 {
		return "", "", false
	}
	host, p := s[:j], s[j+1:]
	if !isIPv4ish(host) || p == "" {
		return "", "", false
	}
	for _, c := range p {
		if c < '0' || c > '9' {
			return "", "", false
		}
	}
	return host, p, true
}

func isIPv4ish(h string) bool {
	parts := strings.Split(h, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" || len(p) > 3 {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

// DNSGroupName 是 DNS 专用组的约定名（把「境外域名查询走哪条线」从业务分组里独立出来）。
const DNSGroupName = "dnsnode"

// DNSGroupTemplate 生成一个 DNS 专用组：地区取境内常见的就近出口，先排除隧道节点。
// subtag 为空时不加 subtag 约束；keywords 为空时用默认的四地。
func DNSGroupTemplate(subtag string, keywords []string) Group {
	if len(keywords) == 0 {
		keywords = []string{"香港", "日本", "新加坡", "台湾"}
	}
	g := Group{Name: DNSGroupName, Policy: "min_avg10", Extra: []string{"check_interval: 30s", "check_tolerance: 1s"}}
	for _, kw := range keywords {
		f := "name(keyword: '" + kw + "')"
		if subtag != "" {
			f = "subtag(" + subtag + ") && " + f
		}
		g.Filters = append(g.Filters, f)
	}
	return g
}

// HasGroup 报告组表里是否已有该名字的组。
func HasGroup(groups []Group, name string) bool {
	for _, g := range groups {
		if g.Name == name {
			return true
		}
	}
	return false
}

// EnsureDNSGroupRule 在存在 DNS 专用组时，保证「8.8.8.8/8.8.4.4 的 53」指向它（避免落到业务组）。
// 组不存在时只补「公共 DNS 直连」这一条更安全的规则。
func EnsureDNSGroupRule(rules []Rule, groups []Group) ([]Rule, []string) {
	have := map[string]bool{}
	for _, r := range rules {
		have[strings.TrimSpace(r.Matcher)] = true
	}
	var head []Rule
	var added []string
	direct := Rule{Matcher: "dip(223.5.5.5, 119.29.29.29) && dport(53)", Out: "direct"}
	if !have[direct.Matcher] {
		head = append(head, direct)
		added = append(added, direct.Matcher)
	}
	if HasGroup(groups, DNSGroupName) {
		g := Rule{Matcher: "dip(8.8.8.8, 8.8.4.4) && dport(53)", Out: DNSGroupName}
		if !have[g.Matcher] {
			head = append(head, g)
			added = append(added, g.Matcher)
		}
	}
	if len(head) == 0 {
		return rules, nil
	}
	out := make([]Rule, 0, len(head)+len(rules))
	out = append(out, head...)
	out = append(out, rules...)
	return out, added
}
