package subs

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Node 是一个订阅里解析出的单个节点（只提取展示所需的非敏感字段）。
type Node struct {
	Scheme string `json:"scheme"`
	Host   string `json:"host"`
	Port   string `json:"port"`
	Tag    string `json:"tag"`
	Subtag string `json:"subtag"` // 所属订阅名（对应 dae 的 subtag）
	Raw    string `json:"-"`      // 完整原始链接（含凭据，绝不外传）
}

// Source 描述一个订阅来源解析结果。
type Source struct {
	Name  string `json:"name"`
	URL   string `json:"source"`
	Count int    `json:"count"`
	Nodes []Node `json:"nodes"`
	Error string `json:"error,omitempty"`
}

// 超时 30s：机场订阅偶尔较慢（实测 dual 上游约 1s，但个别时段会明显变慢）。
var httpClient = &http.Client{Timeout: 30 * time.Second}

// UserAgent 是拉取 http(s) 订阅时使用的默认 User-Agent（空=Go 默认）。
// 实测部分机场按 UA 决定返回内容甚至直接 403（如 s3.dlpublicsub.com 只认 clash 客户端 UA）。
var UserAgent = ""

// SetUserAgent 设置默认 UA（进程级，供命令行参数注入）。
func SetUserAgent(ua string) { UserAgent = strings.TrimSpace(ua) }

// Resolve 解析单个订阅来源为节点列表。
// baseDir 用于解析 file:// 相对路径（dae 约定：相对入口配置所在目录）。
// 只读，不修改任何文件。
func Resolve(baseDir, name, source string) Source {
	return ResolveUA(baseDir, name, source, "")
}

// ResolveUA 同 Resolve，但可为该来源指定独立的 User-Agent（空=用默认值）。
func ResolveUA(baseDir, name, source, ua string) Source {
	out := Source{Name: name, URL: source}
	raw, err := fetchWithUA(baseDir, source, ua)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	nodes, err := decodeAndParse(raw)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	for i := range nodes {
		nodes[i].Subtag = name
	}
	out.Nodes = nodes
	out.Count = len(nodes)
	return out
}

// FetchRaw 拉取订阅原始内容（http(s) 或 file://，后者相对 baseDir），不做解析。
// 供订阅源更新模块复用同一套超时/大小限制逻辑。
func FetchRaw(baseDir, source string) ([]byte, error) { return fetchWithUA(baseDir, source, "") }

// FetchRawUA 同 FetchRaw，但可指定 User-Agent（空=用默认值）。
func FetchRawUA(baseDir, source, ua string) ([]byte, error) { return fetchWithUA(baseDir, source, ua) }

// Parse 把订阅原始内容（base64 或明文链接列表）解析为节点。
func Parse(raw []byte) ([]Node, error) { return decodeAndParse(raw) }

func fetch(baseDir, source string) ([]byte, error) { return fetchWithUA(baseDir, source, "") }

func fetchWithUA(baseDir, source, ua string) ([]byte, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		req, err := http.NewRequest("GET", source, nil)
		if err != nil {
			return nil, err
		}
		agent := strings.TrimSpace(ua)
		if agent == "" {
			agent = UserAgent
		}
		if agent != "" {
			req.Header.Set("User-Agent", agent)
		}
		req.Header.Set("Accept", "*/*")
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, errors.New("订阅返回 HTTP " + resp.Status)
		}
		buf := make([]byte, 0, 4096)
		tmp := make([]byte, 32*1024)
		for {
			n, e := resp.Body.Read(tmp)
			buf = append(buf, tmp[:n]...)
			if e != nil {
				break
			}
			if len(buf) > 8*1024*1024 {
				return nil, errors.New("订阅内容过大(>8MB)")
			}
		}
		return buf, nil
	}
	path := source
	if strings.HasPrefix(source, "file://") {
		path = strings.TrimPrefix(source, "file://")
		if filepath.IsAbs(path) {
			return nil, errors.New("file:// 仅支持相对入口配置目录的相对路径(R2)")
		}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	return os.ReadFile(path)
}

// decodeAndParse 处理明文或 base64 包装的订阅内容，切分为节点行并逐个解析。
func decodeAndParse(content []byte) ([]Node, error) {
	s := strings.TrimSpace(string(content))
	if s == "" {
		return nil, errors.New("订阅内容为空")
	}
	if !strings.Contains(s, "://") {
		if dec, ok := tryBase64(s); ok && strings.Contains(dec, "://") {
			s = dec
		} else {
			return nil, errors.New("无法识别订阅格式(既非节点链接也非 base64)")
		}
	}
	var nodes []Node
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "://") {
			continue
		}
		n := parseLink(line)
		n.Raw = line
		nodes = append(nodes, n)
	}
	if len(nodes) == 0 {
		return nil, errors.New("订阅未解析出任何节点")
	}
	return nodes, nil
}

func tryBase64(s string) (string, bool) {
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	encodings := []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	}
	for _, enc := range encodings {
		if b, err := enc.DecodeString(s); err == nil {
			return string(b), true
		}
	}
	return "", false
}

// parseLink 尽力从一条节点链接中提取 scheme/host/port/tag，绝不回传凭据。
func parseLink(line string) Node {
	scheme := line
	rest := ""
	if i := strings.Index(line, "://"); i >= 0 {
		scheme = strings.ToLower(line[:i])
		rest = line[i+3:]
	}
	if scheme == "vmess" {
		return parseVmess(rest)
	}
	// 去掉 fragment(#tag) 与 query(?...)
	tag := ""
	if i := strings.Index(rest, "#"); i >= 0 {
		tag = rest[i+1:]
		rest = rest[:i]
	}
	if i := strings.Index(rest, "?"); i >= 0 {
		rest = rest[:i]
	}
	hostport := rest
	if i := strings.LastIndex(rest, "@"); i >= 0 {
		hostport = rest[i+1:]
	}
	host, port := splitHostPort(hostport)
	if tag != "" {
		if u, err := url.QueryUnescape(tag); err == nil {
			tag = u
		}
	}
	if tag == "" {
		tag = host
	}
	return Node{Scheme: scheme, Host: host, Port: port, Tag: tag}
}

func splitHostPort(hp string) (host, port string) {
	hp = strings.TrimSpace(hp)
	if hp == "" {
		return "", ""
	}
	// 去掉可能的路径
	if i := strings.Index(hp, "/"); i >= 0 {
		hp = hp[:i]
	}
	// IPv6 [::1]:443
	if strings.HasPrefix(hp, "[") {
		if j := strings.Index(hp, "]"); j >= 0 {
			host = hp[1:j]
			rest := hp[j+1:]
			port = strings.TrimPrefix(rest, ":")
			return host, port
		}
	}
	if i := strings.LastIndex(hp, ":"); i >= 0 {
		return hp[:i], hp[i+1:]
	}
	return hp, ""
}

func parseVmess(rest string) Node {
	if i := strings.Index(rest, "#"); i >= 0 {
		rest = rest[:i]
	}
	dec, ok := tryBase64(rest)
	if !ok {
		return Node{Scheme: "vmess"}
	}
	var m struct {
		Add  string `json:"add"`
		Port any    `json:"port"`
		PS   string `json:"ps"`
	}
	if err := json.Unmarshal([]byte(dec), &m); err != nil {
		return Node{Scheme: "vmess"}
	}
	port := ""
	switch v := m.Port.(type) {
	case float64:
		port = trimNum(v)
	case string:
		port = v
	}
	tag := m.PS
	if tag == "" {
		tag = m.Add
	}
	return Node{Scheme: "vmess", Host: m.Add, Port: port, Tag: tag}
}

func trimNum(f float64) string {
	i := int64(f)
	if f == float64(i) {
		return itoa(i)
	}
	return ""
}

func itoa(i int64) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		p--
		b[p] = '-'
	}
	return string(b[p:])
}
