package clashconv

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

const sampleYAML = `
allow-lan: true
proxies:
    - alterId: 0
      cipher: auto
      name: "\\U0001F1ED\\U0001F1F0 香港 VM"
      network: ws
      port: "36031"
      server: node.example.com
      type: vmess
      udp: true
      uuid: 11111111-2222-3333-4444-555555555555
      ws-headers:
        Host: cdn.example.com
      ws-opts:
        Path: /abcd
        headers:
            Host: cdn.example.com
      ws-path: /abcd
    - client-fingerprint: chrome
      flow: xtls-rprx-vision
      name: "\\U0001F1ED\\U0001F1F0 香港 DP"
      port: "36443"
      server: node.example.com
      servername: sni.example.com
      tls: true
      type: vless
      udp: true
      uuid: 11111111-2222-3333-4444-555555555555
    - client-fingerprint: chrome
      flow: xtls-rprx-vision
      name: "香港 REALITY"
      port: 8443
      reality-opts:
        public-key: pbkvalue
        short-id: 6d0ce9d7
      server: node2.example.com
      servername: sdk.example.com
      tls: true
      type: vless
      udp: true
      uuid: 11111111-2222-3333-4444-555555555555
    - cipher: aes-128-gcm
      name: "ss 节点"
      password: "pa: ss"
      port: 8388
      server: ss.example.com
      type: ss
      udp: true
    - name: "trojan 节点"
      password: tpass
      port: 443
      server: tj.example.com
      sni: tj-sni.example.com
      type: trojan
      udp: true
    - name: "hy2 节点"
      password: hpass
      port: 443
      server: hy.example.com
      skip-cert-verify: true
      sni: hy-sni.example.com
      type: hysteria2
    - name: "不支持的协议"
      server: x.example.com
      port: 1
      type: snell
proxy-groups:
    - name: PROXY
      type: select
`

func TestLooksLikeClash(t *testing.T) {
	if !LooksLikeClash([]byte(sampleYAML)) {
		t.Fatal("应识别为 Clash YAML")
	}
	links := "vless://uuid@a.example.com:443?security=tls#x\nvmess://xx"
	if LooksLikeClash([]byte(links)) {
		t.Fatal("链接列表不应识别为 Clash YAML")
	}
	if LooksLikeClash([]byte(base64.StdEncoding.EncodeToString([]byte(links)))) {
		t.Fatal("base64 内容不应识别为 Clash YAML")
	}
}

func TestParseProxies(t *testing.T) {
	ps, err := ParseProxies([]byte(sampleYAML))
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 7 {
		t.Fatalf("代理数应为 7，实际 %d", len(ps))
	}
	if ps[0].Type != "vmess" || ps[0].Get("ws-opts.Path") != "/abcd" || ps[0].Get("ws-opts.headers.Host") != "cdn.example.com" {
		t.Fatalf("vmess 嵌套字段解析错误: %+v", ps[0].F)
	}
	if !strings.Contains(ps[0].Name, "香港 VM") {
		t.Fatalf("name 转义未还原: %q", ps[0].Name)
	}
	if ps[2].Get("reality-opts.public-key") != "pbkvalue" {
		t.Fatalf("reality 字段解析错误: %+v", ps[2].F)
	}
	if ps[3].Get("port") != "8388" {
		t.Fatalf("裸数字 port 解析错误: %q", ps[3].Get("port"))
	}
}

func TestConvert(t *testing.T) {
	res, err := Convert([]byte(sampleYAML))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Links) != 6 {
		t.Fatalf("应转换出 6 条链接，实际 %d（skipped=%v）", len(res.Links), res.Skipped)
	}
	if len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0], "snell") {
		t.Fatalf("不支持的协议应记录到 Skipped: %v", res.Skipped)
	}
	// vmess：base64(JSON) 且关键字段正确
	if !strings.HasPrefix(res.Links[0], "vmess://") {
		t.Fatalf("首条应为 vmess: %q", res.Links[0])
	}
	raw := strings.TrimPrefix(res.Links[0], "vmess://")
	dec, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(dec, &m); err != nil {
		t.Fatal(err)
	}
	if m["add"] != "node.example.com" || m["port"] != "36031" || m["net"] != "ws" || m["path"] != "/abcd" || m["host"] != "cdn.example.com" {
		t.Fatalf("vmess 字段错误: %v", m)
	}
	// vless tls
	if !strings.Contains(res.Links[1], "security=tls") || !strings.Contains(res.Links[1], "sni=sni.example.com") {
		t.Fatalf("vless tls 参数错误: %q", res.Links[1])
	}
	// vless reality
	if !strings.Contains(res.Links[2], "security=reality") || !strings.Contains(res.Links[2], "pbk=pbkvalue") || !strings.Contains(res.Links[2], "sid=6d0ce9d7") {
		t.Fatalf("vless reality 参数错误: %q", res.Links[2])
	}
	// ss：userinfo 为 base64url(method:password)
	ssLink := res.Links[3]
	if !strings.HasPrefix(ssLink, "ss://") || !strings.Contains(ssLink, "@ss.example.com:8388") {
		t.Fatalf("ss 链接格式错误: %q", ssLink)
	}
	ui := strings.TrimPrefix(strings.Split(ssLink, "@")[0], "ss://")
	if d, err := base64.RawURLEncoding.DecodeString(ui); err != nil || string(d) != "aes-128-gcm:pa: ss" {
		t.Fatalf("ss userinfo 错误: %q -> %q (%v)", ui, string(d), err)
	}
	if !strings.HasPrefix(res.Links[4], "trojan://tpass@tj.example.com:443") {
		t.Fatalf("trojan 链接错误: %q", res.Links[4])
	}
	if !strings.HasPrefix(res.Links[5], "hysteria2://hpass@hy.example.com:443") || !strings.Contains(res.Links[5], "insecure=1") {
		t.Fatalf("hysteria2 链接错误: %q", res.Links[5])
	}
}

// TestRealSample 用真实上游样本做回归（样本含凭据，不入库）：
//
//	CLASH_SAMPLE=/tmp/clash-raw.yaml LINKS_SAMPLE=/tmp/dual-links.txt go test ./internal/clashconv -run RealSample -v
func TestRealSample(t *testing.T) {
	cy := os.Getenv("CLASH_SAMPLE")
	lf := os.Getenv("LINKS_SAMPLE")
	if cy == "" || lf == "" {
		t.Skip("未设置 CLASH_SAMPLE/LINKS_SAMPLE，跳过真实样本回归")
	}
	raw, err := os.ReadFile(cy)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(lf)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Convert(raw)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, l := range res.Links {
		k, v := linkKey(l)
		got[k] = v
	}
	wantSet := map[string]string{}
	for _, l := range strings.Split(string(want), "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		k, v := linkKey(l)
		wantSet[k] = v
	}
	var missing, extra []string
	for k := range wantSet {
		if _, ok := got[k]; !ok {
			missing = append(missing, k)
		}
	}
	for k := range got {
		if _, ok := wantSet[k]; !ok {
			extra = append(extra, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("节点集合不一致：缺少 %v，多出 %v", missing, extra)
	}
	for k, wantParams := range wantSet {
		if got[k] != wantParams {
			t.Errorf("参数不一致 %s\n  期望 %s\n  实际 %s", k, wantParams, got[k])
		}
	}
	t.Logf("真实样本回归通过：%d 节点，跳过 %d 个", len(got), len(res.Skipped))
}

// linkKey 把链接归一化为 (目标) → 排序后的参数集合，便于集合比对。
// vmess 走 base64(JSON)，按语义字段归一（各家客户端的键序/冗余键不同）。
func linkKey(link string) (string, string) {
	body := link
	scheme := ""
	if i := strings.Index(body, "://"); i >= 0 {
		scheme, body = body[:i], body[i+3:]
	}
	if scheme == "vmess" {
		if i := strings.Index(body, "#"); i >= 0 {
			body = body[:i]
		}
		if dec, err := base64.StdEncoding.DecodeString(body); err == nil {
			var m map[string]any
			if json.Unmarshal(dec, &m) == nil {
				get := func(k string) string {
					if v, ok := m[k].(string); ok {
						return v
					}
					return ""
				}
				key := get("add") + ":" + get("port") + "/" + get("id")
				// path 与 ws-path 是同一信息的两种写法；scy 缺省即 auto，都不参与比较。
				if get("path") == "" {
					if wp := get("ws-path"); wp != "" {
						m["path"] = wp
					}
				}
				var ps []string
				for _, k := range []string{"aid", "net", "type", "host", "path", "tls", "sni", "ps", "v"} {
					if v := get(k); v != "" {
						ps = append(ps, k+"="+v)
					}
				}
				sort.Strings(ps)
				return key, strings.Join(ps, "&")
			}
		}
	}
	if i := strings.Index(body, "#"); i >= 0 {
		body = body[:i]
	}
	q := ""
	if i := strings.Index(body, "?"); i >= 0 {
		q, body = body[i+1:], body[:i]
	}
	ps := strings.Split(q, "&")
	sort.Strings(ps)
	return body, strings.Join(ps, "&")
}

const ssrYAML = `
proxies:
    - {name: 🇨🇳 【隧道】台湾 P1, server: so260520-cn-27v3.example.com, port: 42345, type: ssr, cipher: aes-256-cfb, password: H6JmjL, protocol: auth_aes128_sha1, obfs: tls1.2_ticket_auth, protocol-param: "25164:hvF4PIxq", obfs-param: 2fc6925164.microsoft.com, udp: true}
    - {name: 无密码, server: n2.example.com, port: 443, type: ssr, cipher: aes-128-cfb}
proxy-groups:
    - {name: auto, type: url, proxies: [}}
`

func TestConvertSSR(t *testing.T) {
	res, err := Convert([]byte(ssrYAML))
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if len(res.Links) != 1 {
		t.Fatalf("应转出 1 条 ssr 链接，实际 %d（跳过 %v）", len(res.Links), res.Skipped)
	}
	if len(res.Skipped) != 1 {
		t.Fatalf("缺密码的节点应被跳过并报告，实际 %v", res.Skipped)
	}
	if !strings.HasPrefix(res.Links[0], "ssr://") {
		t.Fatalf("前缀不对: %s", res.Links[0])
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(res.Links[0], "ssr://"))
	if err != nil {
		t.Fatalf("链接主体应是 base64url: %v", err)
	}
	body := string(raw)
	for _, want := range []string{"so260520-cn-27v3.example.com:42345:auth_aes128_sha1:aes-256-cfb:tls1.2_ticket_auth:", "obfsparam=", "protoparam=", "remarks="} {
		if !strings.Contains(body, want) {
			t.Fatalf("主体缺少 %q: %s", want, body)
		}
	}
	// 密码应再做一次 base64url 编码
	if !strings.Contains(body, base64.RawURLEncoding.EncodeToString([]byte("H6JmjL"))) {
		t.Fatalf("密码未编码: %s", body)
	}
}

// TestRealSummerSample 用一台真实机场的 Clash YAML 回归（默认跳过，靠环境变量启用）。
func TestRealSummerSample(t *testing.T) {
	path := os.Getenv("SUMMER_SAMPLE")
	if path == "" {
		t.Skip("设置 SUMMER_SAMPLE 指向真实 Clash YAML 才跑")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Convert(data)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	t.Logf("真实样本：共 %d 个代理，转出 %d 条，跳过 %d 个，类型 %v", res.Total, len(res.Links), len(res.Skipped), res.ByType)
	if len(res.Links) == 0 {
		t.Fatal("应转出链接")
	}
	for _, l := range res.Links {
		if !strings.HasPrefix(l, "ssr://") {
			t.Fatalf("该样本应全是 ssr 链接: %s", l)
		}
	}
}
