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

// handleMeshLan 返回「局域网互通」状态（只读）。
func (s *Server) handleMeshLan(w http.ResponseWriter, r *http.Request) {
	st := mesh.DetectLan(mesh.Options{})
	writeJSON(w, 200, map[string]any{"status": st})
}

// handleMeshLanApply 启用/停用局域网互通（NAT）。
func (s *Server) handleMeshLanApply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action    string   `json:"action"`
		LanCIDRs  []string `json:"lanCidrs"`
		MeshIface string   `json:"meshIface"`
		DryRun    bool     `json:"dryRun"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	switch action {
	case "enable":
		c := mesh.LanConfig{LanCIDRs: req.LanCIDRs, MeshIface: req.MeshIface}
		if len(c.LanCIDRs) == 0 {
			c.LanCIDRs = mesh.DetectLan(mesh.Options{}).LanCIDRs
		}
		res := mesh.ApplyLan(mesh.Options{}, c, req.DryRun)
		s.audit(r, "mesh.lan.enable", "hnp_mesh", res.OK, strings.Join(c.LanCIDRs, ","))
		if !res.OK {
			writeJSON(w, 422, map[string]any{"error": res.Error, "result": res})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "result": res, "status": mesh.DetectLan(mesh.Options{})})
	case "disable":
		res := mesh.RemoveLan(mesh.Options{}, req.DryRun)
		s.audit(r, "mesh.lan.disable", "hnp_mesh", res.OK, "")
		if !res.OK {
			writeJSON(w, 422, map[string]any{"error": res.Error, "result": res})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "result": res, "status": mesh.DetectLan(mesh.Options{})})
	default:
		writeJSON(w, 400, map[string]any{"error": "action 只能是 enable / disable"})
	}
}
