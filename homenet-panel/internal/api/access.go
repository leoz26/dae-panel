package api

import (
	"net/http"

	"homenet-panel/internal/access"
)

// handleAccessProbe：只读探测本机接入能力并给出默认档位推荐（P-A 阶段，零副作用）。
func (s *Server) handleAccessProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	p := access.Collect(nil, "/", s.EntryPath)
	writeJSON(w, 200, map[string]any{
		"probe": p,
		"guide": access.Guide(p.Rec.Mode),
	})
}

// handleAccessRecommend：把向导问答并入推荐，返回档位 + 理由 + 操作要点。
func (s *Server) handleAccessRecommend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	var ans access.Answers
	if err := readJSON(r, &ans); err != nil {
		writeJSON(w, 400, map[string]any{"error": "无效的请求体"})
		return
	}
	p := access.Collect(nil, "/", s.EntryPath)
	rec := access.Recommend(p, ans)
	writeJSON(w, 200, map[string]any{
		"rec":      rec,
		"guide":    access.Guide(rec.Mode),
		"label":    rec.Mode.Label(),
		"question": access.Questions(),
	})
}
