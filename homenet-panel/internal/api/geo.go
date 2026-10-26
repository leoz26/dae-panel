package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"homenet-panel/internal/geodata"
)

// geoDir 返回 dae 的 geo 数据目录（由 GeoPath 的目录推导）。
func (s *Server) geoDir() string {
	if s.GeoDir != "" {
		return s.GeoDir
	}
	if s.GeoPath == "" {
		return "/usr/share/dae"
	}
	i := strings.LastIndexByte(s.GeoPath, '/')
	if i <= 0 {
		return "/usr/share/dae"
	}
	return s.GeoPath[:i]
}

// geoTags 读取并缓存 geosite.dat 的 tag 集合（按 mtime 失效）。
func (s *Server) geoTags() map[string]bool {
	path := s.GeoPath
	if path == "" {
		return nil
	}
	s.geoMu.Lock()
	defer s.geoMu.Unlock()
	var mod time.Time
	var size int64
	if st, err := os.Stat(path); err == nil {
		mod, size = st.ModTime(), st.Size()
	} else {
		s.geoTagsCache = nil
		return nil
	}
	if s.geoTagsCache != nil && s.geoMod.Equal(mod) && s.geoSize == size {
		return s.geoTagsCache
	}
	tags, err := geodata.ReadTags(path)
	if err != nil {
		s.geoTagsCache = nil
		return nil
	}
	s.geoTagsCache = tags
	s.geoMod = mod
	s.geoSize = size
	return tags
}

// geoStatePath 是「上次更新记录」的落盘位置。
func (s *Server) geoStatePath() string {
	if s.BackupDir == "" {
		return ""
	}
	return filepath.Join(s.BackupDir, "geo-version.json")
}

func (s *Server) handleGeo(w http.ResponseWriter, r *http.Request) {
	st := geodata.Inspect(s.geoDir(), s.BackupDir)
	writeJSON(w, 200, map[string]any{
		"data":  st,
		"keep":  s.GeoKeep,
		"state": geodata.LoadState(s.geoStatePath()),
	})
}

// handleGeoVersions 对比本机 dat 与上游最新构建：只做 HEAD 与 GitHub API 查询，不下载正文、不写盘。
func (s *Server) handleGeoVersions(w http.ResponseWriter, r *http.Request) {
	urls := map[string]string{}
	q := r.URL.Query()
	if v := q.Get("geoip"); v != "" {
		urls["geoip"] = v
	}
	if v := q.Get("geosite"); v != "" {
		urls["geosite"] = v
	}
	rep := geodata.DetectVersions(geodata.VersionOptions{
		Dir:       s.geoDir(),
		URLs:      urls,
		StatePath: s.geoStatePath(),
	})
	s.audit(r, "geo.versions", "check", len(rep.Errors) == 0, rep.RemoteTag)
	writeJSON(w, 200, map[string]any{"report": rep})
}

func (s *Server) handleGeoUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Targets []string          `json:"targets"`
		URLs    map[string]string `json:"urls"`
		DryRun  bool              `json:"dryRun"`
	}
	if r.Method == http.MethodPost {
		if err := readJSON(r, &req); err != nil {
			writeJSON(w, 400, map[string]any{"error": "请求体格式错误: " + err.Error()})
			return
		}
	}
	res, err := geodata.Update(geodata.UpdateOptions{
		Dir:       s.geoDir(),
		BackupDir: s.BackupDir,
		Targets:   req.Targets,
		URLs:      req.URLs,
		Keep:      s.GeoKeep,
		DryRun:    req.DryRun,
		StatePath: s.geoStatePath(),
	})
	if err != nil {
		s.audit(r, "geo.update", strings.Join(req.Targets, ","), false, err.Error())
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	out := map[string]any{"result": res, "dryRun": req.DryRun}
	if req.DryRun {
		s.audit(r, "geo.update", "dry-run", len(res.Errors) == 0, describeGeoTargets(res))
		writeJSON(w, 200, out)
		return
	}
	if len(res.Errors) > 0 {
		s.audit(r, "geo.update", strings.Join(req.Targets, ","), false, describeGeoTargets(res))
		writeJSON(w, 422, map[string]any{"error": strings.Join(res.Errors, "；"), "result": res})
		return
	}
	if pid := s.Dae.RunningPID(); pid > 0 && len(res.Updated) > 0 {
		code, msg := s.Dae.Reload(pid)
		prog, _ := s.Dae.Progress()
		out["reloadCode"] = code
		out["reloadOut"] = msg
		out["progress"] = prog
		if code != 0 {
			s.audit(r, "geo.update", strings.Join(req.Targets, ","), false, "已替换数据但 dae reload 失败: "+msg)
			writeJSON(w, 200, out)
			return
		}
	}
	s.audit(r, "geo.update", strings.Join(req.Targets, ","), true, describeGeoTargets(res))
	writeJSON(w, 200, out)
}

func describeGeoTargets(res geodata.Result) string {
	var parts []string
	for _, u := range res.Updated {
		p := u.Name
		if u.Tags > 0 {
			p += " tags=" + strconv.Itoa(u.Tags)
		}
		parts = append(parts, p)
	}
	if len(res.Errors) > 0 {
		parts = append(parts, "失败: "+strings.Join(res.Errors, "；"))
	}
	return strings.Join(parts, " / ")
}
