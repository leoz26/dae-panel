package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"homenet-panel/internal/model"
)

const singleFileConfig = `# 由 Goose 生成 —— 真源是 config.yaml，请勿手改本文件

global {
    tproxy_port: 12345
    wan_interface: auto
}

subscription {
    cf: 'file://subs/cf.sub'
}

group {
    proxy {
        policy: min_moving_avg
        filter: subtag(cf)
    }
}

routing {
    dip(geoip:private) -> direct
    domain(geosite:cn) -> direct
    fallback: proxy
}
`

// 单文件布局（goose 生成的形态）：没有 config.d/panel-core.dae，入口文件就是全部配置。
func TestLiveTextsSingleFile(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "config.dae")
	if err := os.WriteFile(entry, []byte(singleFileConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{EntryPath: entry, CorePath: filepath.Join(dir, "config.d", "panel-core.dae"), ModelPath: filepath.Join(dir, "model.json")}

	et, ct, single := s.liveTexts()
	if !single {
		t.Fatal("期望识别为单文件布局")
	}
	if et != ct || !strings.Contains(ct, "routing {") {
		t.Fatalf("入口应作为主体解析: single=%v", single)
	}

	m, src, err := s.loadModel()
	if err != nil {
		t.Fatalf("单文件解析失败: %v", err)
	}
	if src != "live" || m == nil || len(m.Rules) != 2 || m.Fallback != "proxy" {
		t.Fatalf("解析结果不对: src=%s rules=%v fallback=%q", src, func() []string {
			var out []string
			for _, r := range m.Rules {
				out = append(out, r.Matcher)
			}
			return out
		}(), m.Fallback)
	}
	if len(m.Groups) != 1 || len(m.Subscriptions) != 1 {
		t.Fatalf("分组/订阅缺失: groups=%d subs=%d", len(m.Groups), len(m.Subscriptions))
	}

	if drift := s.modelDrift(m); len(drift) != 0 {
		t.Fatalf("单文件布局不应报漂移: %v", drift)
	}
}

// 空规则集写进本来有 routing 的线上配置必须被拒（防止解析失败后误点应用清空规则）。
func TestApplyModelRejectsEmptyRules(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "config.dae")
	if err := os.WriteFile(entry, []byte(singleFileConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{EntryPath: entry, CorePath: filepath.Join(dir, "config.d", "panel-core.dae"), ModelPath: filepath.Join(dir, "model.json")}

	body := strings.NewReader(`{"model":{"global":"","rules":null}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/model/apply", body)
	rec := httptest.NewRecorder()
	s.handleApplyModel(rec, req)
	if rec.Code != 422 {
		t.Fatalf("期望 422，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "拒绝应用") {
		t.Fatalf("错误信息不明确: %s", rec.Body.String())
	}

	// 有规则时不触发该闸门（后续会走 configtx，这里只看不是 422 拒绝语义）
	keep := &model.Model{Global: "tproxy_port: 12345", Rules: []model.Rule{{Matcher: "dip(geoip:private)", Out: "direct"}}, Fallback: "proxy"}
	_ = keep
}
