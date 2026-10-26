package api

import (
	"net/http"
	"strings"

	"homenet-panel/internal/model"
)

// handleDNSParse 把 dns 块正文解析成结构化视图（只读，不写盘）。
// POST /api/dns/parse {"text": "..."} -> {"spec": {...}, "warnings": [...]}
func (s *Server) handleDNSParse(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &in); err != nil {
		writeJSON(w, 400, map[string]any{"error": "请求体不是合法 JSON: " + err.Error()})
		return
	}
	spec := model.ParseDNS(in.Text).Normalize()
	if len(spec.Upstreams) == 0 && len(spec.Request) == 0 && len(spec.Response) == 0 {
		// 线上没有 dns 块或解析不出内容时，给一份可直接用的默认值。
		spec = model.DefaultDNSSpec().Normalize()
		writeJSON(w, 200, map[string]any{"spec": spec, "warnings": model.LintDNS(spec), "defaulted": true})
		return
	}
	writeJSON(w, 200, map[string]any{"spec": spec, "warnings": model.LintDNS(spec), "defaulted": false})
}

// handleDNSRender 把结构化视图渲染回 dns 块正文（只读，不写盘）。
// POST /api/dns/render {"spec": {...}} -> {"text": "...", "warnings": [...]}
func (s *Server) handleDNSRender(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Spec model.DNSSpec `json:"spec"`
	}
	if err := readJSON(r, &in); err != nil {
		writeJSON(w, 400, map[string]any{"error": "请求体不是合法 JSON: " + err.Error()})
		return
	}
	spec := in.Spec
	for i := range spec.Upstreams {
		spec.Upstreams[i].Name = strings.TrimSpace(spec.Upstreams[i].Name)
		spec.Upstreams[i].Addr = strings.TrimSpace(spec.Upstreams[i].Addr)
	}
	text := model.RenderDNS(spec)
	writeJSON(w, 200, map[string]any{"text": text, "warnings": model.LintDNS(spec)})
}
