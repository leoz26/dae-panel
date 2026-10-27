package subs

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeAndParseBase64(t *testing.T) {
	plain := "vless://uuid-1@1.2.3.4:443?type=ws#%E9%A6%99%E6%B8%AF%20DP\n" +
		"trojan://pw@5.6.7.8:8443#SG-node"
	b64 := base64.StdEncoding.EncodeToString([]byte(plain))
	nodes, err := decodeAndParse([]byte(b64))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("want 2 nodes, got %d", len(nodes))
	}
	if nodes[0].Scheme != "vless" || nodes[0].Host != "1.2.3.4" || nodes[0].Port != "443" {
		t.Errorf("node0 = %+v", nodes[0])
	}
	if nodes[0].Tag != "香港 DP" {
		t.Errorf("node0 tag = %q", nodes[0].Tag)
	}
	if nodes[1].Scheme != "trojan" || nodes[1].Host != "5.6.7.8" || nodes[1].Tag != "SG-node" {
		t.Errorf("node1 = %+v", nodes[1])
	}
}

func TestDecodePlain(t *testing.T) {
	nodes, err := decodeAndParse([]byte("ss://abc@9.9.9.9:8388#tag\n"))
	if err != nil || len(nodes) != 1 {
		t.Fatalf("err=%v nodes=%d", err, len(nodes))
	}
	if nodes[0].Host != "9.9.9.9" || nodes[0].Port != "8388" {
		t.Errorf("node=%+v", nodes[0])
	}
}

func TestVmess(t *testing.T) {
	inner := `{"add":"v.example.com","port":443,"ps":"VM 节点"}`
	link := "vmess://" + base64.StdEncoding.EncodeToString([]byte(inner))
	n := parseLink(link)
	if n.Scheme != "vmess" || n.Host != "v.example.com" || n.Port != "443" || n.Tag != "VM 节点" {
		t.Errorf("vmess node=%+v", n)
	}
}

func TestEmpty(t *testing.T) {
	if _, err := decodeAndParse([]byte("   ")); err == nil {
		t.Errorf("want error for empty")
	}
}

func TestResolveFileRelative(t *testing.T) {
	dir := t.TempDir()
	sub := "vless://u@host1:443#A"
	if err := os.WriteFile(filepath.Join(dir, "x.sub"), []byte(base64.StdEncoding.EncodeToString([]byte(sub))), 0600); err != nil {
		t.Fatal(err)
	}
	s := Resolve(dir, "test", "file://x.sub")
	if s.Error != "" || s.Count != 1 {
		t.Fatalf("resolve: err=%q count=%d", s.Error, s.Count)
	}
	if s.Nodes[0].Host != "host1" {
		t.Errorf("node=%+v", s.Nodes[0])
	}
}

func TestResolveFileAbsoluteRejected(t *testing.T) {
	s := Resolve("/tmp", "test", "file:///etc/dae/subs/x.sub")
	if s.Error == "" {
		t.Errorf("want error for absolute file:// path")
	}
}

func TestFetchRawUA(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		if gotUA != "clash-verge/v1.7.7" {
			w.WriteHeader(403)
			return
		}
		_, _ = w.Write([]byte("vless://u@1.1.1.1:443#A"))
	}))
	defer srv.Close()

	// 留空时用兜底 UA（DefaultUA），不再把 Go 默认 UA 暴露给机场
	raw, err := FetchRaw(t.TempDir(), srv.URL)
	if err != nil {
		t.Fatalf("留空 UA 时应使用 DefaultUA 并成功: %v", err)
	}
	if !strings.Contains(string(raw), "vless://") {
		t.Fatalf("内容异常: %q", raw)
	}
	if gotUA != DefaultUA {
		t.Errorf("未发送 DefaultUA: %q", gotUA)
	}

	// 显式指定 UA 优先
	if _, err := FetchRawUA(t.TempDir(), srv.URL, "clash-verge/v1.7.7"); err != nil {
		t.Errorf("指定 UA 应成功: %v", err)
	}

	// 进程级默认 UA 优先于 DefaultUA
	SetUserAgent("mihomo/1.18.0")
	defer SetUserAgent("")
	if _, err := FetchRaw(t.TempDir(), srv.URL); err == nil {
		t.Errorf("SetUserAgent 后应使用 mihomo UA 并被 403 拒绝")
	}
	if gotUA != "mihomo/1.18.0" {
		t.Errorf("SetUserAgent 未生效: %q", gotUA)
	}
}

func TestFetchRawNoUAFallsBackToGoDefault(t *testing.T) {
	if DefaultUA == "" {
		t.Skip("DefaultUA 为空，无需回退")
	}
	old := DefaultUA
	DefaultUA = ""
	defer func() { DefaultUA = old }()

	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	if _, err := FetchRaw(t.TempDir(), srv.URL); err != nil {
		t.Fatalf("应成功: %v", err)
	}
	if !strings.HasPrefix(gotUA, "Go-http-client") {
		t.Errorf("DefaultUA 置空后应回退到 Go 默认 UA，实际 %q", gotUA)
	}
}
func TestParseSSR(t *testing.T) {
	// 真实样本（summer 机场）：名字在 base64 载荷的 remarks 参数里。
	line := "ssr://c28yNjA1MjAtY24tMjd2My5zb25sb2RkMzY1LmNvbTo0MjM0NTphdXRoX2FlczEyOF9zaGExOmFlcy0yNTYtY2ZiOnRsczEuMl90aWNrZXRfYXV0aDpTRFpLYldwTS8_b2Jmc3BhcmFtPU1tWmpOamt5TlRFMk5DNXRhV055YjNOdlpuUXVZMjl0JnByb3RvcGFyYW09TWpVeE5qUTZhSFpHTkZCSmVIRSZyZW1hcmtzPTQ0Q1E2WnFuNllHVDQ0Q1I1WS13NXJtLUlGQXg"
	n := parseLink(line)
	if n.Scheme != "ssr" {
		t.Fatalf("scheme=%q", n.Scheme)
	}
	if n.Host != "so260520-cn-27v3.sonlodd365.com" {
		t.Fatalf("host=%q", n.Host)
	}
	if n.Port != "42345" {
		t.Fatalf("port=%q", n.Port)
	}
	if n.Tag != "【隧道】台湾 P1" {
		t.Fatalf("tag 应取自 remarks，得到 %q", n.Tag)
	}
	if strings.Contains(n.Raw, "SDZKbWpM") {
		t.Fatalf("Raw 未赋值时不该带凭据")
	}
	if bad := parseLink("ssr://not-base64!!"); bad.Scheme != "ssr" || bad.Host != "" {
		t.Fatalf("坏链接应退化为只有 scheme：%+v", bad)
	}
}
