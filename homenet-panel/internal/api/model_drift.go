package api

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"homenet-panel/internal/model"
)

// normalizeConfig 去掉行尾空白与首尾空行，避免纯格式差异造成误报。
func normalizeConfig(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		out = append(out, strings.TrimRight(ln, " \t"))
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// modelDrift 比较面板模型渲染结果与线上配置文件，返回差异说明（长度为 0 表示一致）。
// 面板模型（model.json）是 UI 的真源，线上文件是 dae 的真源，两者可能被手工改动拉开。
func (s *Server) modelDrift(m *model.Model) []string {
	if m == nil {
		return nil
	}
	var drift []string
	if !s.coreExists() {
		// 单文件布局：线上没有主体文件，无从比较（渲染结果天然不同），只提示不告警。
		return nil
	}
	entry, core := model.Render(m)
	if live, err := os.ReadFile(s.CorePath); err != nil {
		drift = append(drift, fmt.Sprintf("读不到线上主体配置 %s：%v", s.CorePath, err))
	} else if normalizeConfig(string(live)) != normalizeConfig(core) {
		drift = append(drift, "线上主体配置 "+s.CorePath+" 与面板模型渲染结果不同（可能被手工改过，或面板模型较旧）")
	}
	if live, err := os.ReadFile(s.EntryPath); err == nil {
		if normalizeConfig(string(live)) != normalizeConfig(entry) {
			drift = append(drift, "线上入口配置 "+s.EntryPath+" 与面板模型渲染结果不同")
		}
	}
	return drift
}

// handleResyncModel 用线上配置反向重建面板模型（覆盖 model.json），不改动线上文件。
func (s *Server) handleResyncModel(w http.ResponseWriter, r *http.Request) {
	entry, core, _ := s.liveTexts()
	m, perr := model.Parse(entry, core)
	if perr != nil {
		writeJSON(w, 422, map[string]any{"error": "解析线上配置失败: " + perr.Error()})
		return
	}
	if err := s.saveModel(m); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	s.audit(r, "model.resync", s.EntryPath+"+"+s.CorePath, true,
		fmt.Sprintf("规则 %d 条 / 分组 %d 个 / 订阅 %d 个", len(m.Rules), len(m.Groups), len(m.Subscriptions)))
	writeJSON(w, 200, map[string]any{"model": m, "ok": true})
}
