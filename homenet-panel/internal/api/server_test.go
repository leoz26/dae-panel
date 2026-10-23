package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"homenet-panel/internal/auth"
	"homenet-panel/internal/dae"
)

func newTestServer(t *testing.T, token, user, pass string) http.Handler {
	t.Helper()
	dir := t.TempDir()
	ap := filepath.Join(dir, "auth.json")
	if user != "" {
		if err := auth.Set(ap, user, pass); err != nil {
			t.Fatal(err)
		}
	}
	s := New(Config{
		Dae:         dae.New(),
		BackupDir:   filepath.Join(dir, "bk"),
		Token:       token,
		AuthPath:    ap,
		EntryPath:   filepath.Join(dir, "config.dae"),
		CorePath:    filepath.Join(dir, "config.d", "panel-core.dae"),
		ModelPath:   filepath.Join(dir, "model.json"),
		SessionPath: filepath.Join(dir, "sessions.json"),
	})
	mux := http.NewServeMux()
	s.Register(mux)
	return mux
}

func TestHealthRequiresAuth(t *testing.T) {
	h := newTestServer(t, "", "", "")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/health", nil))
	if rr.Code != 401 {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

func TestStaticToken(t *testing.T) {
	h := newTestServer(t, "secret", "", "")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/health", nil))
	if rr.Code != 401 {
		t.Fatalf("want 401 without token, got %d", rr.Code)
	}
	req := httptest.NewRequest("GET", "/api/health", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	if rr2.Code != 200 {
		t.Fatalf("want 200 with bearer, got %d", rr2.Code)
	}
}

func TestLoginFlow(t *testing.T) {
	h := newTestServer(t, "", "admin", "pw123")

	// 错误密码
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"admin","password":"bad"}`)))
	if rr.Code != 401 {
		t.Fatalf("wrong pass want 401 got %d", rr.Code)
	}

	// 正确密码
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"admin","password":"pw123"}`)))
	if rr.Code != 200 {
		t.Fatalf("login want 200 got %d: %s", rr.Code, rr.Body.String())
	}
	cookies := rr.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no cookie set")
	}
	var sess *http.Cookie
	for _, c := range cookies {
		if c.Name == cookieName {
			sess = c
		}
	}
	if sess == nil {
		t.Fatal("session cookie not found")
	}

	// 带 cookie 访问受保护端点
	req := httptest.NewRequest("GET", "/api/health", nil)
	req.AddCookie(sess)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	if rr2.Code != 200 {
		t.Fatalf("with cookie want 200 got %d", rr2.Code)
	}
}

func TestIndexServed(t *testing.T) {
	h := newTestServer(t, "", "", "")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "HomeNet Panel") {
		t.Fatalf("index not served: %d", rr.Code)
	}
}

func TestApplyBadJSON(t *testing.T) {
	h := newTestServer(t, "secret", "", "")
	req := httptest.NewRequest("POST", "/api/apply", strings.NewReader("{not json"))
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 400 {
		t.Fatalf("want 400 for bad json, got %d", rr.Code)
	}
}

func TestApplyEmptyContent(t *testing.T) {
	h := newTestServer(t, "secret", "", "")
	req := httptest.NewRequest("POST", "/api/apply", strings.NewReader(`{"target":"/tmp/x","content":""}`))
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 400 {
		t.Fatalf("want 400 for empty content, got %d", rr.Code)
	}
}

// TestIndexHTMLScriptHasNoHTMLEntities 防止再犯：把 HTML 实体（如 &#39;）写进 <script> 里的 JS，
// 会让整段脚本语法错误、Vue 无法挂载，页面表现为原样显示 {{ 变量 }}。
func TestIndexHTMLScriptHasNoHTMLEntities(t *testing.T) {
	b, err := webFS.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(b)
	// 逐个 <script> 块检查（跳过 src 引用的外链）
	rest := html
	for {
		i := strings.Index(rest, "<script")
		if i < 0 {
			break
		}
		tagEnd := strings.Index(rest[i:], ">")
		if tagEnd < 0 {
			break
		}
		tag := rest[i : i+tagEnd+1]
		body := rest[i+tagEnd+1:]
		j := strings.Index(body, "</script>")
		if j < 0 {
			break
		}
		block := body[:j]
		if !strings.Contains(tag, "src=") {
			for _, ent := range []string{"&#39;", "&quot;", "&amp;", "&lt;", "&gt;"} {
				if strings.Contains(block, ent) {
					t.Fatalf("内联 <script> 里出现 HTML 实体 %q（会导致 JS 语法错误）", ent)
				}
			}
		}
		rest = body[j+len("</script>"):]
	}
	// 页面必须有可挂载的根节点
	if !strings.Contains(html, "id=\"app\"") {
		t.Fatal("index.html 缺少 #app 挂载点")
	}
}

func loginCookie(t *testing.T, h http.Handler, body string) *http.Cookie {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST", "/api/login", strings.NewReader(body)))
	if rr.Code != 200 {
		t.Fatalf("login want 200 got %d: %s", rr.Code, rr.Body.String())
	}
	for _, c := range rr.Result().Cookies() {
		if c.Name == cookieName {
			return c
		}
	}
	t.Fatal("session cookie not found")
	return nil
}

func TestLoginRememberExtendsCookie(t *testing.T) {
	h := newTestServer(t, "", "admin", "pw123")
	short := loginCookie(t, h, `{"username":"admin","password":"pw123"}`)
	long := loginCookie(t, h, `{"username":"admin","password":"pw123","remember":true}`)
	if short.MaxAge <= 0 || long.MaxAge <= 0 {
		t.Fatal("cookie 必须是持久 cookie（MaxAge>0），否则关浏览器就掉登录")
	}
	wantShort := int(auth.DefaultSessionTTL.Seconds())
	wantLong := int(auth.RememberSessionTTL.Seconds())
	if short.MaxAge != wantShort {
		t.Fatalf("未勾选记住我应为 %d 秒，实际 %d", wantShort, short.MaxAge)
	}
	if long.MaxAge != wantLong {
		t.Fatalf("勾选记住我应为 %d 秒，实际 %d", wantLong, long.MaxAge)
	}
	// 明文 HTTP 访问时不能置 Secure，否则浏览器不保存这个 cookie
	if short.Secure || long.Secure {
		t.Fatal("明文请求下 cookie 不应置 Secure")
	}
	if !short.HttpOnly {
		t.Fatal("cookie 必须是 HttpOnly")
	}
}

func TestSessionSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	ap := filepath.Join(dir, "auth.json")
	if err := auth.Set(ap, "admin", "pw123"); err != nil {
		t.Fatal(err)
	}
	mk := func() http.Handler {
		s := New(Config{
			Dae:         dae.New(),
			BackupDir:   filepath.Join(dir, "bk"),
			AuthPath:    ap,
			EntryPath:   filepath.Join(dir, "config.dae"),
			CorePath:    filepath.Join(dir, "config.d", "panel-core.dae"),
			ModelPath:   filepath.Join(dir, "model.json"),
			SessionPath: filepath.Join(dir, "sessions.json"),
		})
		mux := http.NewServeMux()
		s.Register(mux)
		return mux
	}
	h1 := mk()
	ck := loginCookie(t, h1, `{"username":"admin","password":"pw123","remember":true}`)

	// 模拟面板重启：新 Server 实例（同一 sessions.json）
	h2 := mk()
	req := httptest.NewRequest("GET", "/api/health", nil)
	req.AddCookie(ck)
	rr := httptest.NewRecorder()
	h2.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("重启后旧 cookie 应仍有效，实际 %d", rr.Code)
	}
}

func TestCertEndpointWithoutTLS(t *testing.T) {
	h := newTestServer(t, "", "", "")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/panel.crt", nil))
	if rr.Code != 404 {
		t.Fatalf("未启用 TLS 时应 404，实际 %d", rr.Code)
	}
}

func TestMustChangePasswordGate(t *testing.T) {
	dir := t.TempDir()
	ap := filepath.Join(dir, "auth.json")
	if err := auth.SetWithFlag(ap, "admin", "initpw12", true); err != nil {
		t.Fatal(err)
	}
	mk := func() http.Handler {
		s := New(Config{
			Dae:         dae.New(),
			BackupDir:   filepath.Join(dir, "bk"),
			AuthPath:    ap,
			EntryPath:   filepath.Join(dir, "config.dae"),
			CorePath:    filepath.Join(dir, "config.d", "panel-core.dae"),
			ModelPath:   filepath.Join(dir, "model.json"),
			SessionPath: filepath.Join(dir, "sessions.json"),
		})
		mux := http.NewServeMux()
		s.Register(mux)
		return mux
	}
	h := mk()
	ck := loginCookie(t, h, `{"username":"admin","password":"initpw12"}`)

	// /api/me 应标记 mustChange
	req := httptest.NewRequest("GET", "/api/me", nil)
	req.AddCookie(ck)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"mustChange":true`) {
		t.Fatalf("/api/me 应返回 mustChange:true，实际 %d %s", rr.Code, rr.Body.String())
	}

	// 其他端点应被 403 拦住
	req = httptest.NewRequest("GET", "/api/status", nil)
	req.AddCookie(ck)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 403 || !strings.Contains(rr.Body.String(), "must_change_password") {
		t.Fatalf("初始密码状态下 /api/status 应 403，实际 %d %s", rr.Code, rr.Body.String())
	}

	// 弱密码应被拒
	req = httptest.NewRequest("POST", "/api/passwd", strings.NewReader(`{"oldPassword":"initpw12","newPassword":"abcdefgh"}`))
	req.AddCookie(ck)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 400 {
		t.Fatalf("纯字母密码应被拒，实际 %d %s", rr.Code, rr.Body.String())
	}
	// 旧密码错误应被拒
	req = httptest.NewRequest("POST", "/api/passwd", strings.NewReader(`{"oldPassword":"wrong","newPassword":"goodpass12"}`))
	req.AddCookie(ck)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("旧密码错误应 401，实际 %d", rr.Code)
	}
	// 正常改密
	req = httptest.NewRequest("POST", "/api/passwd", strings.NewReader(`{"oldPassword":"initpw12","newPassword":"goodpass12"}`))
	req.AddCookie(ck)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("改密应成功，实际 %d %s", rr.Code, rr.Body.String())
	}
	// 改完标记清除：受保护端点通了，且旧密码失效
	req = httptest.NewRequest("GET", "/api/health", nil)
	req.AddCookie(ck)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("改密后 /api/health 应 200，实际 %d", rr.Code)
	}
	if auth.NeedsChange(ap, "admin") {
		t.Fatal("改密后 mustChange 标记应被清除")
	}
	if ok, _ := auth.Verify(ap, "admin", "goodpass12"); !ok {
		t.Fatal("新密码应可用")
	}
	if ok, _ := auth.Verify(ap, "admin", "initpw12"); ok {
		t.Fatal("旧密码应失效")
	}
}

func TestPasswordWeakness(t *testing.T) {
	cases := map[string]bool{
		"short1":      true, // 太短
		"onlyletters": true, // 无数字/符号
		"12345678":    true, // 纯数字且常见
		"admin":       true, // 常见
		"goodpass12":  false,
		"P@ssw0rd!":   false,
	}
	for pw, weak := range cases {
		if got := passwordWeakness(pw) != ""; got != weak {
			t.Fatalf("%q 期望 weak=%v 实际 %v", pw, weak, got)
		}
	}
}
