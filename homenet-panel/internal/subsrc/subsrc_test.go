package subsrc

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"homenet-panel/internal/subs"
)

func b64(s string) []byte { return []byte(base64.StdEncoding.EncodeToString([]byte(s))) }

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "subsrc.json")
	st := Load(p)
	if st.CheckHours != 6 {
		t.Fatalf("默认周期应为 6，got %d", st.CheckHours)
	}
	st.Upsert(Entry{Name: "CF", URL: "https://example.com/sub", Source: "file://subs/CF.sub", File: filepath.Join(dir, "CF.sub")})
	st.CheckHours = 3
	if err := st.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("登记表权限应为 0600，got %o", fi.Mode().Perm())
	}
	got := Load(p)
	if got.CheckHours != 3 || len(got.Items) != 1 || got.Items[0].URL != "https://example.com/sub" {
		t.Fatalf("reload 不一致: %+v", got)
	}
	// Upsert 同名校验：应更新而非新增
	got.Upsert(Entry{Name: "CF", URL: "https://example.com/sub2"})
	if len(got.Items) != 1 || got.Items[0].URL != "https://example.com/sub2" {
		t.Fatalf("upsert 未更新: %+v", got.Items)
	}
	// UA 必须随 Upsert 一起落库（否则设置页保存后仍会用默认 UA）
	got.Upsert(Entry{Name: "CF", URL: "https://example.com/sub2", UA: "clash-verge/v1.7.7"})
	if got.Items[0].UA != "clash-verge/v1.7.7" {
		t.Fatalf("upsert 未保留 UA: %+v", got.Items[0])
	}
	if err := got.Save(); err != nil {
		t.Fatal(err)
	}
	if reloaded := Load(p); reloaded.Items[0].UA != "clash-verge/v1.7.7" {
		t.Fatalf("UA 未持久化: %+v", reloaded.Items[0])
	}
}

func TestDiff(t *testing.T) {
	old := map[string]bool{"a": true, "b": true}
	neu := map[string]bool{"b": true, "c": true}
	added, removed := Diff(old, neu)
	if len(added) != 1 || added[0] != "c" {
		t.Errorf("added=%v", added)
	}
	if len(removed) != 1 || removed[0] != "a" {
		t.Errorf("removed=%v", removed)
	}
}

func TestCheckDetectsChange(t *testing.T) {
	dir := t.TempDir()
	// 本地镜像：2 个节点
	local := filepath.Join(dir, "CF.sub")
	if err := os.WriteFile(local, b64("vless://u@1.1.1.1:443#A\ntrojan://p@2.2.2.2:8443#B"), 0o600); err != nil {
		t.Fatal(err)
	}
	// 上游：B（保留）+ C（新增）
	upstream := filepath.Join(dir, "up.sub")
	if err := os.WriteFile(upstream, b64("trojan://p@2.2.2.2:8443#B\nss://x@3.3.3.3:8388#C"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := Entry{Name: "CF", URL: "file://up.sub", Source: "file://CF.sub", File: local}
	Check(&e, dir)
	if !e.LastOK || e.Err != "" {
		t.Fatalf("check 失败: %+v", e)
	}
	if e.LocalCount != 2 || e.RemoteCount != 2 {
		t.Fatalf("节点数 local=%d remote=%d", e.LocalCount, e.RemoteCount)
	}
	if !e.Changed || e.Added != 1 || e.Removed != 1 {
		t.Fatalf("差异识别错误: %+v", e)
	}
	// 相同内容不应报变化
	if err := os.WriteFile(upstream, b64("vless://u@1.1.1.1:443#A\ntrojan://p@2.2.2.2:8443#B"), 0o600); err != nil {
		t.Fatal(err)
	}
	e2 := Entry{Name: "CF", URL: "file://up.sub", Source: "file://CF.sub", File: local}
	Check(&e2, dir)
	if e2.Changed {
		t.Errorf("内容一致却报变化: %+v", e2)
	}
	// 未配置 URL
	e3 := Entry{Name: "X", Source: "file://CF.sub"}
	Check(&e3, dir)
	if e3.LastOK || e3.Err == "" {
		t.Errorf("空 URL 应报错: %+v", e3)
	}
}

func TestWriteFileAtomicPerm(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "subs", "x.sub")
	if err := WriteFileAtomic(p, []byte("data"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("权限应为 0600，got %o", fi.Mode().Perm())
	}
	b, _ := os.ReadFile(p)
	if string(b) != "data" {
		t.Errorf("内容=%q", b)
	}
	// 覆盖写
	if err := WriteFileAtomic(p, []byte("data2"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(p)
	if string(b) != "data2" {
		t.Errorf("覆盖后内容=%q", b)
	}
}

func TestKeysIgnoresCredentials(t *testing.T) {
	nodes := []subs.Node{{Scheme: "vless", Host: "1.1.1.1", Port: "443", Tag: "A"}}
	k := Keys(nodes)
	if _, ok := k["vless|1.1.1.1|443|A"]; !ok {
		t.Errorf("keys=%v", k)
	}
}

const clashFixture = `
proxies:
    - name: "香港 DP"
      port: "36443"
      server: node.example.com
      servername: sni.example.com
      tls: true
      type: vless
      uuid: 11111111-2222-3333-4444-555555555555
      flow: xtls-rprx-vision
      client-fingerprint: chrome
    - cipher: aes-128-gcm
      name: "ss 节点"
      password: pw
      port: 8388
      server: ss.example.com
      type: ss
    - name: "不支持的协议"
      port: 1
      server: x.example.com
      type: snell
`

func TestFetchPayloadConvertsClash(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "up.yaml"), []byte(clashFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	e := Entry{Name: "dual", URL: "file://up.yaml", Source: "file://dual.sub"}
	data, format, skipped, err := FetchPayload(&e, dir)
	if err != nil {
		t.Fatalf("FetchPayload: %v", err)
	}
	if format != "clash" {
		t.Errorf("format=%q，应为 clash", format)
	}
	if len(skipped) != 1 {
		t.Errorf("应记录 1 个跳过项（snell），got %v", skipped)
	}
	if strings.Contains(string(data), "proxies:") {
		t.Fatalf("输出不应再是 YAML: %s", data)
	}
	nodes, err := subs.Parse(data)
	if err != nil {
		t.Fatalf("转换结果应可被 subs 解析: %v", err)
	}
	if len(nodes) != 2 || nodes[0].Scheme != "vless" || nodes[1].Scheme != "ss" {
		t.Fatalf("节点解析异常: %+v", nodes)
	}
}

func TestCheckRecordsClashFormat(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "up.yaml"), []byte(clashFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	// 本地镜像先放转换后的链接（模拟上一次成功 apply 的结果）
	e := Entry{Name: "dual", URL: "file://up.yaml", Source: "file://dual.sub", File: filepath.Join(dir, "dual.sub")}
	data, _, _, err := FetchPayload(&e, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.File, data, 0o600); err != nil {
		t.Fatal(err)
	}
	Check(&e, dir)
	if !e.LastOK || e.Err != "" {
		t.Fatalf("check 失败: %+v", e)
	}
	if e.Format != "clash" || e.Skipped != 1 {
		t.Errorf("format=%q skipped=%d", e.Format, e.Skipped)
	}
	if e.Changed {
		t.Errorf("镜像与上游一致，不应报变化: %+v", e)
	}
}

// 机场上游对短时间内重复请求会挂起，所以「检查 → 应用」必须复用同一次拉取结果。
func TestFetchPayloadCache(t *testing.T) {
	ClearCache()
	defer ClearCache()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write([]byte("vless://u@h:443?security=tls#n1"))
	}))
	defer srv.Close()
	e := &Entry{Name: "x", URL: srv.URL}
	if _, _, _, err := FetchPayload(e, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := FetchPayload(e, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("缓存未生效：上游被请求 %d 次", got)
	}
	if _, _, _, err := FetchPayloadForce(e, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("Force 应强制拉取，实际 %d 次", got)
	}
}

func TestFriendlyErrRateLimit(t *testing.T) {
	if got := FriendlyErr(errors.New("boom")); got != "boom" {
		t.Fatalf("不应改写普通错误: %q", got)
	}
	msg := FriendlyErr(errors.New(`Get "https://h/api?token=abc123&flag=clash": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`))
	if !strings.Contains(msg, "限流") {
		t.Fatalf("超时错误应带限流提示: %q", msg)
	}
	if strings.Contains(msg, "abc123") || !strings.Contains(msg, "token=***") {
		t.Fatalf("错误信息里的 token 应打码: %q", msg)
	}
}

// 状态栏提醒依赖 Summary；变化明细必须有上限，避免登记表被撑大。
func TestSummaryAndCapKeys(t *testing.T) {
	st := &Store{Items: []Entry{{Name: "A", Changed: true}, {Name: "B"}, {Name: "C", Changed: true}}}
	n, names := Summary(st)
	if n != 2 || len(names) != 2 || names[0] != "A" {
		t.Fatalf("Summary 结果异常: %d %v", n, names)
	}
	long := make([]string, 40)
	for i := range long {
		long[i] = "k" + string(rune('a'+i%26))
	}
	got := capKeys(long)
	if len(got) != maxKeys+1 || got[len(got)-1] != "…" {
		t.Fatalf("capKeys 截断异常: len=%d", len(got))
	}
}

func TestFetchPayloadRejectsHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><head><title>404 Not Found</title></head><body>nope</body></html>"))
	}))
	defer srv.Close()
	e := &Entry{Name: "x", URL: srv.URL}
	_, format, _, err := FetchPayloadForce(e, t.TempDir())
	if err == nil {
		t.Fatal("网页响应应报错")
	}
	if format != "html" {
		t.Fatalf("format 应为 html，得到 %q", format)
	}
	if !strings.Contains(err.Error(), "网页") {
		t.Fatalf("错误信息应提示返回网页: %v", err)
	}
}

func TestLooksLikeHTML(t *testing.T) {
	if lookLikeHTML([]byte("vless://a@b:443#n")) {
		t.Fatal("链接不应判为 HTML")
	}
	if !lookLikeHTML([]byte("  <!DOCTYPE html><html></html>")) {
		t.Fatal("HTML 应被识别")
	}
	if !lookLikeHTML([]byte("<html><body>x</body></html>")) {
		t.Fatal("HTML 应被识别")
	}
}

func TestStoreDelete(t *testing.T) {
	dir := t.TempDir()
	st := &Store{Path: filepath.Join(dir, "subsrc.json"), CheckHours: 6}
	st.Upsert(Entry{Name: "a", URL: "https://x/a"})
	st.Upsert(Entry{Name: "b", URL: "https://x/b"})
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	if !st.Delete("a") {
		t.Fatal("删除已存在项应返回 true")
	}
	if st.Delete("a") {
		t.Fatal("重复删除应返回 false")
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	back := Load(filepath.Join(dir, "subsrc.json"))
	if back.Find("a") != nil || back.Find("b") == nil {
		t.Fatalf("删除后应有 b、无 a，实际 %+v", back.Items)
	}
}
