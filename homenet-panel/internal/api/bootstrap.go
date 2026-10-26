package api

import (
	"net/http"
	"sync"

	"homenet-panel/internal/bootstrap"
)

var setupMu sync.Mutex

func (s *Server) setupOptions() bootstrap.Options {
	return bootstrap.Options{EntryPath: s.EntryPath, CorePath: s.CorePath}
}

// handleSetupStatus 返回「安装与初始化」页需要的现场状态（只读）。
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	st := bootstrap.Detect(s.setupOptions())
	writeJSON(w, 200, map[string]any{"status": st})
}

// handleSetupInstallDae 通过官方 APT 源安装 dae。
func (s *Server) handleSetupInstallDae(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DryRun bool `json:"dryRun"`
	}
	_ = readJSON(r, &req)
	setupMu.Lock()
	defer setupMu.Unlock()
	opts := s.setupOptions()
	opts.DryRun = req.DryRun
	res := bootstrap.InstallDae(opts)
	s.audit(r, "setup.installDae", "dae", res.OK, res.Error)
	code := 200
	if !res.OK {
		code = 422
	}
	writeJSON(w, code, map[string]any{"result": res, "status": bootstrap.Detect(s.setupOptions())})
}

// handleSetupInitConfig 写入最小可用配置（入口 + panel-core.dae），validate 失败自动回滚。
func (s *Server) handleSetupInitConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Force  bool `json:"force"`
		DryRun bool `json:"dryRun"`
	}
	_ = readJSON(r, &req)
	setupMu.Lock()
	defer setupMu.Unlock()
	opts := s.setupOptions()
	opts.DryRun = req.DryRun
	res := bootstrap.WriteInitConfig(opts, req.Force)
	s.audit(r, "setup.initConfig", s.EntryPath, res.OK, res.Error)
	code := 200
	if !res.OK {
		code = 422
	}
	writeJSON(w, code, map[string]any{"result": res, "status": bootstrap.Detect(s.setupOptions())})
}

// handleSetupStartDae 启动 dae 并设置开机自启。
func (s *Server) handleSetupStartDae(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DryRun bool `json:"dryRun"`
	}
	_ = readJSON(r, &req)
	setupMu.Lock()
	defer setupMu.Unlock()
	opts := s.setupOptions()
	opts.DryRun = req.DryRun
	res := bootstrap.StartDae(opts)
	s.audit(r, "setup.startDae", "dae", res.OK, res.Error)
	code := 200
	if !res.OK {
		code = 422
	}
	writeJSON(w, code, map[string]any{"result": res, "status": bootstrap.Detect(s.setupOptions())})
}
