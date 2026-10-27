package api

import (
	"net/http"
	"strings"

	"homenet-panel/internal/mesh"
)

// handleMesh 返回组网（EasyTier）状态：节点表、路由表、服务状态。
func (s *Server) handleMesh(w http.ResponseWriter, r *http.Request) {
	st := mesh.Detect(mesh.Options{})
	writeJSON(w, 200, map[string]any{"status": st})
}

// handleMeshService 启停组网服务。
func (s *Server) handleMeshService(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	switch action {
	case "start", "stop", "restart":
	default:
		writeJSON(w, 400, map[string]any{"error": "action 只能是 start / stop / restart"})
		return
	}
	msg, err := mesh.Control(mesh.Options{}, action)
	s.audit(r, "mesh.service", action, err == nil, msg)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "message": msg, "status": mesh.Detect(mesh.Options{})})
}
