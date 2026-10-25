package api

import (
	"os"
	"path/filepath"
	"testing"

	"homenet-panel/internal/model"
)

func TestModelDriftDetectsDifference(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "config.dae")
	core := filepath.Join(dir, "config.d", "panel-core.dae")
	if err := os.MkdirAll(filepath.Dir(core), 0o755); err != nil {
		t.Fatal(err)
	}
	s := &Server{EntryPath: entry, CorePath: core}
	m := &model.Model{
		Global:   "    dial_mode: domain",
		DNS:      "    upstream {\n        cn: 'udp://223.5.5.5:53'\n    }",
		Fallback: "proxy",
		Rules:    []model.Rule{{Matcher: "domain(geosite:youtube)", Out: "hk"}},
	}
	e, c := model.Render(m)
	if err := os.WriteFile(entry, []byte(e), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(core, []byte(c), 0o600); err != nil {
		t.Fatal(err)
	}
	if d := s.modelDrift(m); len(d) != 0 {
		t.Fatalf("同样内容应判一致，got %v", d)
	}
	if err := os.WriteFile(core, []byte(c+"\n\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if d := s.modelDrift(m); len(d) != 0 {
		t.Fatalf("行尾空行不应算漂移，got %v", d)
	}
	if err := os.WriteFile(core, []byte(c+"    ip(geoip:cn) -> direct\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if d := s.modelDrift(m); len(d) == 0 {
		t.Fatal("线上被手工加规则应判漂移")
	}
	if d := s.modelDrift(nil); len(d) != 0 {
		t.Fatalf("nil 模型不应报漂移，got %v", d)
	}
}

func TestNormalizeIgnoresFormattingOnly(t *testing.T) {
	a := "global {\n  tproxy_port: 12345\n}\n\nsubscription {\n}\n"
	b := "global {\ntproxy_port:12345\n}\n"
	if normalizeConfig(a) != normalizeConfig(b) {
		t.Fatalf("排版差异不应算作不一致：%q vs %q", normalizeConfig(a), normalizeConfig(b))
	}
	c := "global {\n  tproxy_port: 23456\n}\n"
	if normalizeConfig(a) == normalizeConfig(c) {
		t.Fatal("真实差异必须被检出")
	}
}
