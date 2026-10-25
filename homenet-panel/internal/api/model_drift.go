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
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		lines = append(lines, t)
	}
	// 空块（subscription {} / group {}）与纯空白差异不算改过
	var kept []string
	for i := 0; i < len(lines); i++ {
		if i+1 < len(lines) && lines[i+1] == "}" {
			if head := strings.TrimSuffix(lines[i], "{"); head == "subscription " || head == "group " || head == "subscription" || head == "group" {
				i++
				continue
			}
		}
		kept = append(kept, lines[i])
	}
	j := strings.Join(kept, "\n")
	// 折叠连续空白，并去掉标点两侧的空白：语义不变，避免手工排版的假告警
	j = strings.Join(strings.Fields(j), " ")
	j = strings.NewReplacer(" {", "{", " }", "}", "( ", "(", " )", ")", " ,", ",", " -> ", "->", " :", ":", ": ", ":").Replace(j)
	return j
}

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
