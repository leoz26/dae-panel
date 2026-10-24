package model

import (
	"fmt"
	"strings"
)

// Header 是面板生成配置的托管头注释。
const Header = "# 由 HomeNet Panel 生成并托管（panel-managed），请勿手改"

// Subscription 对应 dae subscription 块中的一项。
type Subscription struct {
	Name   string `json:"name"`
	Source string `json:"source"` // 如 file://subs/CF.sub 或 https://...
}

// Group 对应 group 块中的一个代理组。
type Group struct {
	Name    string   `json:"name"`
	Policy  string   `json:"policy"`
	Filters []string `json:"filters"`
}

// Rule 对应 routing 块中的一条分流规则。
type Rule struct {
	Matcher string `json:"matcher"` // 如 domain(geosite:youtube)
	Out     string `json:"out"`     // 如 hk / direct
}

// EssentialRules 是面板无条件保证存在的「安全直连」规则。
//
// 起因（192.168.1.85 实测）：dae 的 fallback 是 proxy，而 live 规则里没有内网直连，
// 结果本机 dhcpcd 的 DHCP(68→67) 与面板自身的出站请求也会被丢进代理组经海外节点出去，
// journal 可见 "UdpEndpoint read loop exited ... lAddr=192.168.1.85:68 ... proxy_addr=192.168.1.1:67"。
// 因此这些规则由面板强制前置注入，不依赖用户在界面上维护（界面上即使删掉也会被补回）。
var EssentialRules = []Rule{
	{Matcher: "dip(192.168.0.0/16)", Out: "direct"},
	{Matcher: "dip(127.0.0.0/8, 169.254.0.0/16, 224.0.0.0/4)", Out: "direct"},
	// DHCP DISCOVER/REQUEST 的源地址是 0.0.0.0、目的地址是 255.255.255.255，
	// 不在内网网段里；实测漏掉它时广播仍被塞进 proxy 组（lAddr=0.0.0.0:68）。
	{Matcher: "dip(255.255.255.255/32)", Out: "direct"},
	{Matcher: "pname(homenet-panel, dae, dhcpcd, systemd-resolved, sshd, systemd-timesyncd)", Out: "direct"},
}

// EnsureEssentials 把缺失的安全直连规则前置插入，返回新规则表与被补入的匹配器列表。
// 已有的同 matcher 规则不重复添加，但位置保持在最前（保证优先级最高）。
func EnsureEssentials(rules []Rule) ([]Rule, []string) {
	have := map[string]bool{}
	for _, r := range rules {
		have[strings.TrimSpace(r.Matcher)] = true
	}
	var added []string
	head := make([]Rule, 0, len(EssentialRules))
	for _, e := range EssentialRules {
		if have[e.Matcher] {
			continue
		}
		head = append(head, e)
		added = append(added, e.Matcher)
	}
	if len(head) == 0 {
		return rules, nil
	}
	out := make([]Rule, 0, len(head)+len(rules))
	out = append(out, head...)
	out = append(out, rules...)
	return out, added
}

// Model 是面板托管的配置结构化模型。
type Model struct {
	Global        string         `json:"global"` // global 块正文（逐行保留）
	DNS           string         `json:"dns"`    // dns 块正文（逐行保留）
	Subscriptions []Subscription `json:"subscriptions"`
	Groups        []Group        `json:"groups"`
	Rules         []Rule         `json:"rules"`
	Fallback      string         `json:"fallback"`
}

// ---- 解析 ----

func matchBrace(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// findBlock 在 text[from:] 中寻找以 keyword 开头、后接 '{' 的块，返回其正文与右花括号绝对位置。
func findBlock(text, keyword string, from int) (body string, coreEnd int, ok bool) {
	li := 0
	for li < len(text) {
		nl := strings.IndexByte(text[li:], '\n')
		var line string
		var next int
		if nl < 0 {
			line = text[li:]
			next = len(text)
		} else {
			line = text[li : li+nl+1]
			next = li + nl + 1
		}
		lineStart := li
		li = next
		if lineStart < from {
			continue
		}
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, keyword) {
			continue
		}
		rest := strings.TrimLeft(t[len(keyword):], " \t")
		if !strings.HasPrefix(rest, "{") {
			continue
		}
		braceAbs := lineStart + strings.Index(line, "{")
		end := matchBrace(text, braceAbs)
		if end < 0 {
			return "", 0, false
		}
		return text[braceAbs+1 : end], end, true
	}
	return "", 0, false
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func parseSubscriptions(body string) []Subscription {
	var subs []Subscription
	for _, ln := range strings.Split(body, "\n") {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		idx := strings.Index(t, ":")
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(t[:idx])
		src := unquote(t[idx+1:])
		if name == "" || src == "" {
			continue
		}
		subs = append(subs, Subscription{Name: name, Source: src})
	}
	return subs
}

func parseGroups(body string) []Group {
	var groups []Group
	lines := strings.Split(body, "\n")
	i := 0
	for i < len(lines) {
		t := strings.TrimSpace(lines[i])
		i++
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if strings.HasSuffix(t, "{") {
			name := strings.TrimSpace(strings.TrimSuffix(t, "{"))
			g := Group{Name: name}
			for i < len(lines) {
				b := strings.TrimSpace(lines[i])
				i++
				if b == "}" {
					break
				}
				if strings.HasPrefix(b, "policy:") {
					g.Policy = strings.TrimSpace(strings.TrimPrefix(b, "policy:"))
				} else if strings.HasPrefix(b, "filter:") {
					g.Filters = append(g.Filters, strings.TrimSpace(strings.TrimPrefix(b, "filter:")))
				}
			}
			groups = append(groups, g)
		}
	}
	return groups
}

func parseRouting(body string) ([]Rule, string) {
	var rules []Rule
	fallback := ""
	for _, ln := range strings.Split(body, "\n") {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if strings.HasPrefix(t, "fallback:") {
			fallback = strings.TrimSpace(strings.TrimPrefix(t, "fallback:"))
			continue
		}
		if idx := strings.Index(t, "->"); idx >= 0 {
			m := strings.TrimSpace(t[:idx])
			o := strings.TrimSpace(t[idx+2:])
			if m != "" && o != "" {
				rules = append(rules, Rule{Matcher: m, Out: o})
			}
		}
	}
	return rules, fallback
}

// Parse 从入口配置与主体配置文本解析出模型。
func Parse(entryText, coreText string) (*Model, error) {
	m := &Model{}
	if body, _, ok := findBlock(entryText, "subscription", 0); ok {
		m.Subscriptions = parseSubscriptions(body)
	}
	// core: 顺序解析 global -> group -> dns -> routing，避免 dns 内部的 routing 被误取。
	pos := 0
	if body, end, ok := findBlock(coreText, "global", pos); ok {
		m.Global = strings.Trim(body, "\n")
		pos = end
	}
	if body, end, ok := findBlock(coreText, "group", pos); ok {
		m.Groups = parseGroups(body)
		pos = end
	}
	if body, end, ok := findBlock(coreText, "dns", pos); ok {
		m.DNS = strings.Trim(body, "\n")
		pos = end
	}
	if body, _, ok := findBlock(coreText, "routing", pos); ok {
		m.Rules, m.Fallback = parseRouting(body)
	}
	if m.Global == "" && len(m.Groups) == 0 && len(m.Rules) == 0 {
		return m, fmt.Errorf("未解析出任何配置块（可能格式不匹配）")
	}
	return m, nil
}

// ---- 渲染 ----

func ensureNL(s string) string {
	s = strings.Trim(s, "\n")
	if s == "" {
		return ""
	}
	return s + "\n"
}

// Render 把模型渲染为入口配置与主体配置两段文本。
func Render(m *Model) (entry, core string) {
	var e strings.Builder
	e.WriteString(Header + "\n")
	e.WriteString("include {\n    config.d/*.dae\n}\n\n")
	e.WriteString("subscription {\n")
	for _, s := range m.Subscriptions {
		fmt.Fprintf(&e, "    %s: '%s'\n", s.Name, s.Source)
	}
	e.WriteString("}\n")

	var c strings.Builder
	c.WriteString(Header + "\n")
	c.WriteString("global {\n")
	c.WriteString(ensureNL(m.Global))
	c.WriteString("}\n\n")

	c.WriteString("group {\n")
	for _, g := range m.Groups {
		fmt.Fprintf(&c, "    %s {\n", g.Name)
		if g.Policy != "" {
			fmt.Fprintf(&c, "        policy: %s\n", g.Policy)
		}
		for _, f := range g.Filters {
			fmt.Fprintf(&c, "        filter: %s\n", f)
		}
		c.WriteString("    }\n")
	}
	c.WriteString("}\n\n")

	c.WriteString("dns {\n")
	c.WriteString(ensureNL(m.DNS))
	c.WriteString("}\n\n")

	c.WriteString("routing {\n")
	rules, _ := EnsureEssentials(m.Rules)
	for _, r := range rules {
		fmt.Fprintf(&c, "    %s -> %s\n", r.Matcher, r.Out)
	}
	if m.Fallback != "" {
		fmt.Fprintf(&c, "    fallback: %s\n", m.Fallback)
	}
	c.WriteString("}\n")

	return e.String(), c.String()
}

// ---- 订阅合并（模式B）辅助：subtag 改写 ----

// RewriteSubtags 把 group filter 中形如 subtag(x) 且 x 属于 oldNames 的谓词改写为 subtag(newName)，
// 并对改写后重复的 filter 行去重（保留首次出现）。
// 用途：多订阅合并为单一订阅后，原先按来源订阅名区分节点的 subtag(CF)/subtag(dual) 需统一为 subtag(merged)。
func RewriteSubtags(filters []string, oldNames []string, newName string) []string {
	old := map[string]bool{}
	for _, n := range oldNames {
		old[strings.TrimSpace(n)] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, f := range filters {
		nf := rewriteSubtagLine(f, old, newName)
		if !seen[nf] {
			seen[nf] = true
			out = append(out, nf)
		}
	}
	return out
}

// rewriteSubtagLine 替换单行 filter 中的 subtag(x) 谓词（仅当 x 命中 old）。
func rewriteSubtagLine(line string, old map[string]bool, newName string) string {
	var b strings.Builder
	i := 0
	for i < len(line) {
		idx := strings.Index(line[i:], "subtag(")
		if idx < 0 {
			b.WriteString(line[i:])
			break
		}
		start := i + idx
		b.WriteString(line[i:start])
		open := start + len("subtag(")
		// 找配对的右括号
		depth := 1
		j := open
		for j < len(line) {
			if line[j] == '(' {
				depth++
			} else if line[j] == ')' {
				depth--
				if depth == 0 {
					break
				}
			}
			j++
		}
		if j >= len(line) { // 括号不配对，原样保留剩余
			b.WriteString(line[start:])
			break
		}
		arg := strings.Trim(strings.TrimSpace(line[open:j]), "'\"")
		if old[arg] {
			b.WriteString("subtag(" + newName + ")")
		} else {
			b.WriteString(line[start : j+1])
		}
		i = j + 1
	}
	return b.String()
}
