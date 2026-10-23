package clashconv

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Proxy 是 Clash/Mihomo YAML 里 proxies 段的一个代理项。
// F 用「扁平化键」保存字段：嵌套映射用点号连接（如 ws-opts.Path、reality-opts.public-key）。
type Proxy struct {
	Type string
	Name string
	F    map[string]string
}

// Get 按顺序返回第一个非空的扁平字段值。
func (p Proxy) Get(keys ...string) string {
	for _, k := range keys {
		if v, ok := p.F[k]; ok {
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		}
	}
	return ""
}

// Result 是一次 Clash YAML → 节点链接转换的结果。
type Result struct {
	Links   []string       // 转换出的节点链接（一行一个）
	Total   int            // proxies 段里的代理总数
	ByType  map[string]int // 各类型代理数量
	Skipped []string       // 未转换的代理及原因
}

// LooksLikeClash 判断内容是否为 Clash/Mihomo 的 YAML 配置（而不是节点链接列表）。
// 判据：存在独立的 proxies: 行，且整体不是 node:// 链接列表。
func LooksLikeClash(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	s := string(b)
	t := strings.TrimSpace(s)
	if strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[") {
		return false // JSON/数组，交给链接解析
	}
	hasProxies := false
	for _, ln := range strings.Split(s, "\n") {
		if strings.TrimSpace(ln) == "proxies:" {
			hasProxies = true
			break
		}
	}
	if !hasProxies {
		return false
	}
	// 链接列表里不会出现 YAML 的 proxies: 段；但极端情况下两者混排，仍按 YAML 处理。
	return true
}

// ParseProxies 解析 Clash YAML 的 proxies 段。
// 只支持本项目需要的 YAML 子集：块状列表 + 两层以内的嵌套映射，不做通用 YAML 解析。
func ParseProxies(b []byte) ([]Proxy, error) {
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	start, baseIndent := -1, 0
	for i, ln := range lines {
		if strings.TrimSpace(stripComment(ln)) == "proxies:" {
			start, baseIndent = i, indentWidth(ln)
			break
		}
	}
	if start < 0 {
		return nil, fmt.Errorf("未找到 proxies 段")
	}
	var out []Proxy
	type frame struct {
		indent int
		prefix string
	}
	var stack []frame
	for i := start + 1; i < len(lines); i++ {
		raw := lines[i]
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		ind := indentWidth(raw)
		if ind <= baseIndent {
			break // 离开 proxies 段
		}
		if strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
			out = append(out, Proxy{F: map[string]string{}})
			stack = stack[:0]
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
			if trimmed == "" {
				continue
			}
		}
		if len(out) == 0 {
			continue // proxies 段里第一个元素之前的内容，忽略
		}
		for len(stack) > 0 && stack[len(stack)-1].indent >= ind {
			stack = stack[:len(stack)-1]
		}
		prefix := ""
		if len(stack) > 0 {
			prefix = stack[len(stack)-1].prefix
		}
		k, v, ok := splitKV(trimmed)
		if !ok {
			continue
		}
		if v == "" {
			stack = append(stack, frame{indent: ind, prefix: prefix + k + "."})
			continue
		}
		cur := &out[len(out)-1]
		cur.F[prefix+k] = v
	}
	for i := range out {
		out[i].Type = strings.ToLower(out[i].Get("type"))
		out[i].Name = out[i].Get("name")
	}
	return out, nil
}

// Convert 把 Clash YAML 转换为 dae 可消费的节点链接（ss/vmess/trojan/vless/hysteria2/tuic）。
func Convert(b []byte) (Result, error) {
	proxies, err := ParseProxies(b)
	if err != nil {
		return Result{}, err
	}
	res := Result{Total: len(proxies), ByType: map[string]int{}}
	for _, p := range proxies {
		res.ByType[p.Type]++
		link, reason := toLink(p)
		if reason != "" {
			res.Skipped = append(res.Skipped, p.Type+"「"+p.Name+"」: "+reason)
			continue
		}
		res.Links = append(res.Links, link)
	}
	if len(res.Links) == 0 {
		types := make([]string, 0, len(res.ByType))
		for t := range res.ByType {
			types = append(types, t)
		}
		sort.Strings(types)
		return res, fmt.Errorf("Clash 配置里没有可转换的节点（共 %d 个代理，类型 %s）", res.Total, strings.Join(types, ","))
	}
	return res, nil
}

func toLink(p Proxy) (string, string) {
	switch p.Type {
	case "vmess":
		return vmessLink(p)
	case "vless":
		return vlessLink(p)
	case "ss":
		return ssLink(p)
	case "trojan":
		return trojanLink(p)
	case "hysteria2", "hy2":
		return hysteria2Link(p)
	case "tuic":
		return tuicLink(p)
	case "":
		return "", "缺少 type 字段"
	default:
		return "", "暂不支持该协议，需人工处理"
	}
}

func hostPort(p Proxy) (string, string, string) {
	server := p.Get("server")
	port := p.Get("port")
	if server == "" {
		return "", "", "缺少 server"
	}
	if port == "" {
		return "", "", "缺少 port"
	}
	return server, port, ""
}

func vmessLink(p Proxy) (string, string) {
	server, port, reason := hostPort(p)
	if reason != "" {
		return "", reason
	}
	id := p.Get("uuid", "id")
	if id == "" {
		return "", "缺少 uuid"
	}
	net := p.Get("network", "net")
	if net == "" {
		net = "tcp"
	}
	path := p.Get("ws-opts.Path", "ws-path")
	host := p.Get("ws-opts.headers.Host", "ws-headers.Host", "host")
	sni := p.Get("servername", "sni")
	tls := ""
	if p.Get("tls") == "true" {
		tls = "tls"
	}
	aid := p.Get("alterId", "aid")
	if aid == "" {
		aid = "0"
	}
	scy := p.Get("cipher")
	if scy == "" {
		scy = "auto"
	}
	typ := p.Get("ws-opts.v2ray-http-upgrade") // 绝大多数场景为 none
	if typ == "" {
		typ = "none"
	}
	// v2rayN 风格的 vmess 链接：base64(JSON)。字段顺序固定，便于对比与测试。
	fields := [][2]string{
		{"v", "2"},
		{"ps", p.Name},
		{"add", server},
		{"port", port},
		{"id", id},
		{"aid", aid},
		{"scy", scy},
		{"net", net},
		{"type", typ},
		{"host", host},
		{"path", path},
		{"tls", tls},
		{"sni", sni},
	}
	var sb strings.Builder
	sb.WriteString("{")
	for i, kv := range fields {
		if i > 0 {
			sb.WriteString(",")
		}
		kb, _ := json.Marshal(kv[0])
		vb, _ := json.Marshal(kv[1])
		sb.Write(kb)
		sb.WriteString(":")
		sb.Write(vb)
	}
	sb.WriteString("}")
	return "vmess://" + base64.StdEncoding.EncodeToString([]byte(sb.String())), ""
}

func vlessLink(p Proxy) (string, string) {
	server, port, reason := hostPort(p)
	if reason != "" {
		return "", reason
	}
	id := p.Get("uuid", "id")
	if id == "" {
		return "", "缺少 uuid"
	}
	q := map[string]string{
		"encryption": "none",
		"type":       firstNonEmpty(p.Get("network", "net"), "tcp"),
	}
	if v := p.Get("flow"); v != "" {
		q["flow"] = v
	}
	if v := p.Get("client-fingerprint", "fingerprint"); v != "" {
		q["fp"] = v
	}
	realityPk := p.Get("reality-opts.public-key")
	if realityPk != "" {
		q["security"] = "reality"
		q["pbk"] = realityPk
		if v := p.Get("reality-opts.short-id"); v != "" {
			q["sid"] = v
		}
	} else if p.Get("tls") == "true" {
		q["security"] = "tls"
	} else {
		q["security"] = "none"
	}
	if v := p.Get("servername", "sni"); v != "" {
		q["sni"] = v
	}
	if v := p.Get("ws-opts.Path", "ws-path"); v != "" {
		q["path"] = v
	}
	if v := p.Get("ws-opts.headers.Host", "ws-headers.Host", "host"); v != "" {
		q["host"] = v
	}
	if v := p.Get("grpc-opts.grpc-service-name", "serviceName"); v != "" {
		q["serviceName"] = v
	}
	if p.Get("skip-cert-verify") == "true" {
		q["allowInsecure"] = "1"
	}
	return "vless://" + id + "@" + server + ":" + port + "?" + encodeQuery(q) + fragment(p.Name), ""
}

func ssLink(p Proxy) (string, string) {
	server, port, reason := hostPort(p)
	if reason != "" {
		return "", reason
	}
	if p.Get("plugin") != "" {
		return "", "带 plugin 的 ss 需人工确认"
	}
	method := p.Get("cipher", "method")
	pass := p.Get("password")
	if method == "" || pass == "" {
		return "", "缺少 cipher/password"
	}
	userinfo := base64.RawURLEncoding.EncodeToString([]byte(method + ":" + pass))
	return "ss://" + userinfo + "@" + server + ":" + port + fragment(p.Name), ""
}

func trojanLink(p Proxy) (string, string) {
	server, port, reason := hostPort(p)
	if reason != "" {
		return "", reason
	}
	pass := p.Get("password")
	if pass == "" {
		return "", "缺少 password"
	}
	q := map[string]string{}
	if v := p.Get("sni", "servername"); v != "" {
		q["sni"] = v
	}
	if v := p.Get("network", "net"); v != "" {
		q["type"] = v
	}
	if v := p.Get("ws-opts.Path"); v != "" {
		q["path"] = v
	}
	if v := p.Get("ws-opts.headers.Host"); v != "" {
		q["host"] = v
	}
	if p.Get("skip-cert-verify") == "true" {
		q["allowInsecure"] = "1"
	}
	qs := ""
	if len(q) > 0 {
		qs = "?" + encodeQuery(q)
	}
	return "trojan://" + url.QueryEscape(pass) + "@" + server + ":" + port + qs + fragment(p.Name), ""
}

func hysteria2Link(p Proxy) (string, string) {
	server, port, reason := hostPort(p)
	if reason != "" {
		return "", reason
	}
	pass := p.Get("password", "auth")
	if pass == "" {
		return "", "缺少 password"
	}
	q := map[string]string{}
	if v := p.Get("sni", "servername"); v != "" {
		q["sni"] = v
	}
	if p.Get("skip-cert-verify") == "true" {
		q["insecure"] = "1"
	}
	if v := p.Get("obfs"); v != "" {
		q["obfs"] = v
	}
	if v := p.Get("obfs-password"); v != "" {
		q["obfs-password"] = v
	}
	qs := ""
	if len(q) > 0 {
		qs = "?" + encodeQuery(q)
	}
	return "hysteria2://" + url.QueryEscape(pass) + "@" + server + ":" + port + qs + fragment(p.Name), ""
}

func tuicLink(p Proxy) (string, string) {
	server, port, reason := hostPort(p)
	if reason != "" {
		return "", reason
	}
	id := p.Get("uuid")
	pass := p.Get("password")
	if id == "" || pass == "" {
		return "", "缺少 uuid/password"
	}
	q := map[string]string{}
	if v := p.Get("sni", "servername"); v != "" {
		q["sni"] = v
	}
	if v := p.Get("alpn"); v != "" {
		q["alpn"] = v
	}
	if v := p.Get("congestion-controller"); v != "" {
		q["congestion_control"] = v
	}
	if p.Get("skip-cert-verify") == "true" {
		q["allow_insecure"] = "1"
	}
	qs := ""
	if len(q) > 0 {
		qs = "?" + encodeQuery(q)
	}
	return "tuic://" + id + ":" + url.QueryEscape(pass) + "@" + server + ":" + port + qs + fragment(p.Name), ""
}

func fragment(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return "#" + name
}

// encodeQuery 按键名排序后拼 query（与常见客户端输出一致，便于人工比对）。
func encodeQuery(q map[string]string) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteString("&")
		}
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(url.QueryEscape(q[k]))
	}
	return sb.String()
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func indentWidth(line string) int {
	n := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case ' ':
			n++
		case '\t':
			n += 2
		default:
			return n
		}
	}
	return n
}

func stripComment(line string) string {
	if i := strings.Index(line, " #"); i >= 0 {
		return line[:i]
	}
	return line
}

// splitKV 把 "key: value" 拆开，并处理引号包裹/带转义的标量值。
func splitKV(s string) (key, val string, ok bool) {
	i := strings.Index(s, ":")
	if i < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(s[:i])
	if key == "" {
		return "", "", false
	}
	val = unquote(strings.TrimSpace(s[i+1:]))
	return key, val, true
}

// unquote 处理 YAML 标量：双引号（支持 \\U0001F1ED 等转义）、单引号、裸值。
func unquote(v string) string {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		inner := v[1 : len(v)-1]
		inner = strings.ReplaceAll(inner, "\\/", "/")
		if s, err := strconv.Unquote("\"" + inner + "\""); err == nil {
			return s
		}
		return inner
	}
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		return strings.ReplaceAll(v[1:len(v)-1], "\x27\x27", "\x27")
	}
	return v
}
