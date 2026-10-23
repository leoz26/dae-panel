package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSetVerify(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "auth.json")
	if err := Set(p, "admin", "s3cret"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := Verify(p, "admin", "s3cret"); !ok {
		t.Fatal("正确密码应通过")
	}
	if ok, _ := Verify(p, "admin", "wrong"); ok {
		t.Fatal("错误密码不应通过")
	}
	if ok, _ := Verify(p, "other", "s3cret"); ok {
		t.Fatal("错误用户名不应通过")
	}
	if u, _ := Username(p); u != "admin" {
		t.Fatalf("username want admin got %s", u)
	}
}

func TestSessions(t *testing.T) {
	s := NewSessions(time.Hour)
	tok := s.Create("admin")
	if u, ok := s.User(tok); !ok || u != "admin" {
		t.Fatal("会话应有效")
	}
	if _, ok := s.User("bogus"); ok {
		t.Fatal("伪造 token 不应有效")
	}
	s.Delete(tok)
	if _, ok := s.User(tok); ok {
		t.Fatal("删除后不应有效")
	}
}

func TestSessionsPersistAcrossReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")
	s := NewSessions(time.Hour)
	s.SetPath(path)
	tok := s.CreateTTL("admin", 2*time.Hour)
	if _, ok := s.User(tok); !ok {
		t.Fatal("会话应有效")
	}
	// 文件存在且 0600，且不包含明文 token（只存摘要）
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("会话文件应落盘: %v", err)
	}
	if strings.Contains(string(b), tok) {
		t.Fatal("会话文件不应包含明文 token")
	}
	if st, err := os.Stat(path); err == nil && st.Mode().Perm() != 0o600 {
		t.Fatalf("会话文件权限应为 0600，实际 %o", st.Mode().Perm())
	}
	// 模拟进程重启：新实例 Load 后会话仍有效
	s2 := NewSessions(time.Hour)
	s2.SetPath(path)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	if u, ok := s2.User(tok); !ok || u != "admin" {
		t.Fatal("重启后会话应仍有效")
	}
	// 删除也要落盘
	s2.Delete(tok)
	s3 := NewSessions(time.Hour)
	s3.SetPath(path)
	_ = s3.Load()
	if _, ok := s3.User(tok); ok {
		t.Fatal("删除后重启不应再有效")
	}
}

func TestSessionsExpireNotLoaded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")
	s := NewSessions(time.Hour)
	s.SetPath(path)
	tok := s.CreateTTL("admin", 10*time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	if _, ok := s.User(tok); ok {
		t.Fatal("过期会话不应有效")
	}
}

func TestSessionsRememberTTLConstants(t *testing.T) {
	if RememberSessionTTL <= DefaultSessionTTL {
		t.Fatal("记住我的有效期应长于默认")
	}
	if RememberSessionTTL != 30*24*time.Hour {
		t.Fatalf("记住我应为 30 天，实际 %v", RememberSessionTTL)
	}
}
