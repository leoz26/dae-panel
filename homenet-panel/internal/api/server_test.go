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
		Dae:       dae.New(),
		BackupDir: filepath.Join(dir, "bk"),
		Token:     token,
		AuthPath:  ap,
		EntryPath: filepath.Join(dir, "config.dae"),
		CorePath:  filepath.Join(dir, "config.d", "panel-core.dae"),
		ModelPath: filepath.Join(dir, "model.json"),
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
