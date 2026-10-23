package auth

import (
	"path/filepath"
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
