package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"homenet-panel/internal/audit"
	"homenet-panel/internal/subs"
	"homenet-panel/internal/subsrc"
)

// ---- 审计 ----

func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// audit 记录一条审计日志（失败不阻断主流程）。
func (s *Server) audit(r *http.Request, action, target string, ok bool, detail string) {
	actor := ""
	if r != nil {
		actor = s.currentUser(r)
		if actor == "" && s.Token != "" && r.Header.Get("Authorization") == "Bearer "+s.Token {
			actor = "token"
		}
	}
	if actor == "" {
		actor = "anonymous"
	}
	_ = audit.Append(s.AuditPath, audit.Entry{
		Actor:  actor,
		IP:     clientIP(r),
		Action: action,
		Target: target,
		OK:     ok,
		Detail: detail,
	})
}

// handleAuditExport 导出审计日志（csv 默认，也可 jsonl）；按时间正序，包含历史轮转文件。
func (s *Server) handleAuditExport(w http.ResponseWriter, r *http.Request) {
	items, err := audit.ReadAll(s.AuditPath)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	stamp := time.Now().Format("20060102-150405")
	if r.URL.Query().Get("format") == "jsonl" {
		w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=\"homenet-audit-"+stamp+".jsonl\"")
		enc := json.NewEncoder(w)
		for _, e := range items {
			_ = enc.Encode(e)
		}
		s.audit(r, "audit.export", "jsonl", true, fmt.Sprintf("%d 条", len(items)))
		return
	}
	b, cerr := audit.CSV(items)
	if cerr != nil {
		writeJSON(w, 500, map[string]any{"error": cerr.Error()})
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"homenet-audit-"+stamp+".csv\"")
	w.Write(b)
	s.audit(r, "audit.export", "csv", true, fmt.Sprintf("%d 条", len(items)))
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	n := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if x, err := strconv.Atoi(v); err == nil && x > 0 {
			n = x
		}
	}
	if n > 2000 {
		n = 2000
	}
	items, err := audit.Tail(s.AuditPath, n)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	files, bytes := audit.Files(s.AuditPath)
	maxBytes, keep := audit.Retention()
	writeJSON(w, 200, map[string]any{
		"items": items, "path": s.AuditPath, "count": len(items),
		"retention": map[string]any{"maxMB": maxBytes >> 20, "keep": keep, "desc": audit.Describe()},
		"files":     files, "bytes": bytes,
	})
}

// ---- 订阅源：上游 URL ↔ 本地镜像 ----

func (s *Server) baseDir() string { return filepath.Dir(s.EntryPath) }

// mirrorPath 把 dae 订阅源（file://subs/x.sub）换算为本地绝对路径。
// 返回空字符串表示不是本地文件源（例如直接指向 http(s)）。
func (s *Server) mirrorPath(source string) string {
	src := strings.TrimSpace(source)
	if !strings.HasPrefix(src, "file://") {
		return ""
	}
	rel := strings.TrimPrefix(src, "file://")
	rel = strings.TrimPrefix(rel, "./")
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(s.baseDir(), filepath.FromSlash(rel))
}

// syncEntries 用模型里的订阅补齐登记表（返回合并后的列表，缺 URL 的项 url 为空）。
func (s *Server) syncEntries() (*subsrc.Store, []subsrc.Entry) {
	st := subsrc.Load(s.SubsrcPath)
	m, _, _ := s.loadModel()
	var out []subsrc.Entry
	seen := map[string]bool{}
	if m != nil {
		for _, sub := range m.Subscriptions {
			seen[sub.Name] = true
			mp := s.mirrorPath(sub.Source)
			if e := st.Find(sub.Name); e != nil {
				e.Source = sub.Source
				if mp != "" {
					e.File = mp
				}
				out = append(out, *e)
				continue
			}
			out = append(out, subsrc.Entry{Name: sub.Name, Source: sub.Source, File: mp})
		}
	}
	for _, e := range st.Items {
		if !seen[e.Name] {
			out = append(out, e)
		}
	}
	return st, out
}

// handleSubsrcList 返回所有订阅的「上游 URL + 本地镜像 + 最近检查结果」。
func (s *Server) handleSubsrcList(w http.ResponseWriter, r *http.Request) {
	st, items := s.syncEntries()
	writeJSON(w, 200, map[string]any{
		"items":      items,
		"checkHours": st.CheckHours,
		"lastRun":    st.LastRun,
		"path":       s.SubsrcPath,
	})
}

// handleSubsrcSetURL 保存某个订阅的上游 URL。
func (s *Server) handleSubsrcSetURL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string  `json:"name"`
		URL  string  `json:"url"`
		UA   *string `json:"ua"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad json"})
		return
	}
	name := strings.TrimSpace(req.Name)
	url := strings.TrimSpace(req.URL)
	if name == "" {
		writeJSON(w, 400, map[string]any{"error": "缺少订阅名"})
		return
	}
	if url != "" && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "file://") {
		writeJSON(w, 400, map[string]any{"error": "URL 必须是 http(s):// 或 file://"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st, items := s.syncEntries()
	cur := subsrc.Entry{Name: name}
	for _, it := range items {
		if it.Name == name {
			cur = it
			break
		}
	}
	cur.URL = url
	if req.UA != nil {
		cur.UA = strings.TrimSpace(*req.UA)
	}
	e := st.Upsert(cur)
	if err := st.Save(); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	s.audit(r, "subsrc.seturl", name, true, "url 已更新")
	writeJSON(w, 200, map[string]any{"ok": true, "item": e})
}

// handleSubsrcCheck 只拉取上游做差异比对，不写任何文件（可全量或指定 name）。
func (s *Server) handleSubsrcCheck(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	_ = readJSON(r, &req)
	name := strings.TrimSpace(req.Name)
	s.mu.Lock()
	defer s.mu.Unlock()
	st, items := s.syncEntries()
	base := s.baseDir()
	checked := 0
	for _, it := range items {
		if name != "" && it.Name != name {
			continue
		}
		if strings.TrimSpace(it.URL) == "" {
			continue
		}
		e := st.Upsert(it)
		subsrc.Check(e, base)
		checked++
	}
	st.LastRun = time.Now().UTC().Format(time.RFC3339)
	if err := st.Save(); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	s.audit(r, "subsrc.check", name, true, "已检查")
	_, after := s.syncEntries()
	writeJSON(w, 200, map[string]any{"ok": true, "checked": checked, "items": after})
}

// handleSubsrcApply 拉取上游并覆盖本地镜像，随后 dae reload；失败自动恢复旧文件并再次 reload。
// 未带 confirm 时只做 dryRun（返回差异）。
func (s *Server) handleSubsrcApply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Confirm bool   `json:"confirm"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad json"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeJSON(w, 400, map[string]any{"error": "缺少订阅名"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st, items := s.syncEntries()
	var entry subsrc.Entry
	found := false
	for _, it := range items {
		if it.Name == name {
			entry, found = it, true
			break
		}
	}
	if !found {
		writeJSON(w, 404, map[string]any{"error": "未找到该订阅: " + name})
		return
	}
	if strings.TrimSpace(entry.URL) == "" {
		writeJSON(w, 422, map[string]any{"error": "该订阅未配置上游 URL"})
		return
	}
	if entry.File == "" {
		writeJSON(w, 422, map[string]any{"error": "该订阅引用的不是本地镜像文件（" + entry.Source + "），无法由面板更新"})
		return
	}
	base := s.baseDir()
	data, format, skipped, err := subsrc.FetchPayload(&entry, base)
	if err != nil {
		e := st.Upsert(entry)
		e.LastCheck = time.Now().UTC().Format(time.RFC3339)
		e.LastOK = false
		e.Err = "拉取上游失败: " + subsrc.FriendlyErr(err)
		_ = st.Save()
		s.audit(r, "subsrc.apply", name, false, e.Err)
		writeJSON(w, 502, map[string]any{"error": e.Err})
		return
	}
	nodes, perr := subs.Parse(data)
	if perr != nil || len(nodes) == 0 {
		msg := "上游内容解析出 0 节点，拒绝覆盖（避免 dae 拒绝重载）"
		if perr != nil {
			msg = "上游内容解析失败: " + perr.Error()
		}
		e := st.Upsert(entry)
		e.LastCheck = time.Now().UTC().Format(time.RFC3339)
		e.LastOK = false
		e.Err = msg
		_ = st.Save()
		s.audit(r, "subsrc.apply", name, false, msg)
		writeJSON(w, 422, map[string]any{"error": msg})
		return
	}
	oldRaw, _ := os.ReadFile(entry.File)
	oldNodes, _ := subs.Parse(oldRaw)
	added, removed := subsrc.Diff(subsrc.Keys(oldNodes), subsrc.Keys(nodes))
	out := map[string]any{
		"name":        name,
		"file":        entry.File,
		"url":         entry.URL,
		"remoteNodes": len(nodes),
		"localNodes":  len(oldNodes),
		"format":      format,
		"skipped":     skipped,
		"added":       len(added),
		"removed":     len(removed),
		"addedKeys":   added,
		"removedKeys": removed,
	}
	if !req.Confirm {
		out["dryRun"] = true
		writeJSON(w, 200, out)
		return
	}
	// 备份旧镜像（便于人工回退）
	backupPath := ""
	if len(oldRaw) > 0 {
		if err := os.MkdirAll(filepath.Join(s.BackupDir, "subs"), 0o700); err == nil {
			backupPath = filepath.Join(s.BackupDir, "subs", name+"."+time.Now().Format("20060102-150405")+".sub")
			if werr := os.WriteFile(backupPath, oldRaw, 0o600); werr != nil {
				backupPath = ""
			}
		}
	}
	out["backupPath"] = backupPath
	if err := subsrc.WriteFileAtomic(entry.File, data, 0o600); err != nil {
		s.audit(r, "subsrc.apply", name, false, "写入失败: "+err.Error())
		writeJSON(w, 500, map[string]any{"error": "写入镜像失败: " + err.Error()})
		return
	}
	pid := s.Dae.RunningPID()
	if pid <= 0 {
		out["reloaded"] = false
		out["message"] = "镜像已更新，但 dae 未运行，未执行 reload"
		e := st.Upsert(entry)
		e.LastCheck = time.Now().UTC().Format(time.RFC3339)
		e.LastOK = true
		e.Changed = false
		e.LocalCount = len(nodes)
		e.RemoteCount = len(nodes)
		e.Added, e.Removed = 0, 0
		e.AddedKeys, e.RemovedKeys = nil, nil
		e.LastApplied = time.Now().UTC().Format(time.RFC3339)
		_ = st.Save()
		s.audit(r, "subsrc.apply", name, true, "镜像已更新(dae 未运行)")
		writeJSON(w, 200, out)
		return
	}
	code, reloadOut := s.Dae.Reload(pid)
	prog, _ := s.Dae.Progress()
	out["reloadCode"] = code
	out["reloadOut"] = reloadOut
	out["progress"] = prog
	if code != 0 || prog.IsError() {
		restored := false
		if len(oldRaw) > 0 {
			if werr := subsrc.WriteFileAtomic(entry.File, oldRaw, 0o600); werr == nil {
				restored = true
				if pid2 := s.Dae.RunningPID(); pid2 > 0 {
					_, _ = s.Dae.Reload(pid2)
				}
			}
		}
		msg := prog.Message
		if msg == "" {
			msg = reloadOut
		}
		e := st.Upsert(entry)
		e.LastCheck = time.Now().UTC().Format(time.RFC3339)
		e.LastOK = false
		e.Err = "reload 失败: " + msg
		_ = st.Save()
		s.audit(r, "subsrc.apply", name, false, "reload 失败: "+msg)
		out["restored"] = restored
		out["error"] = "dae reload 失败，已恢复旧镜像: " + msg
		writeJSON(w, 422, out)
		return
	}
	e := st.Upsert(entry)
	e.LastCheck = time.Now().UTC().Format(time.RFC3339)
	e.LastOK = true
	e.Err = ""
	e.Format = format
	e.Skipped = len(skipped)
	e.RemoteCount = len(nodes)
	e.LocalCount = len(nodes)
	e.Changed = false
	e.Added, e.Removed = 0, 0
	e.AddedKeys, e.RemovedKeys = nil, nil
	e.LastApplied = time.Now().UTC().Format(time.RFC3339)
	_ = st.Save()
	out["reloaded"] = true
	out["message"] = "已更新镜像并 reload 成功"
	s.audit(r, "subsrc.apply", name, true, "节点 "+strconv.Itoa(len(oldNodes))+"→"+strconv.Itoa(len(nodes)))
	writeJSON(w, 200, out)
}

// SetCheckHours 设置巡检周期（小时）。
func (s *Server) SetCheckHours(h int) {
	st := subsrc.Load(s.SubsrcPath)
	if h > 0 {
		st.CheckHours = h
	}
	_ = st.Save()
}

// StartBackground 启动订阅新鲜度巡检（只检查、不自动写盘；有变化由界面提示）。
func (s *Server) StartBackground() {
	go func() {
		time.Sleep(90 * time.Second)
		for {
			s.runSubsrcCheckOnce()
			hours := 6
			if st := subsrc.Load(s.SubsrcPath); st.CheckHours > 0 {
				hours = st.CheckHours
			}
			time.Sleep(time.Duration(hours) * time.Hour)
		}
	}()
}

func (s *Server) runSubsrcCheckOnce() {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, items := s.syncEntries()
	base := s.baseDir()
	n := 0
	for _, it := range items {
		if strings.TrimSpace(it.URL) == "" {
			continue
		}
		e := st.Upsert(it)
		subsrc.Check(e, base)
		n++
	}
	if n == 0 {
		return
	}
	st.LastRun = time.Now().UTC().Format(time.RFC3339)
	_ = st.Save()
	_ = audit.Append(s.AuditPath, audit.Entry{Action: "subsrc.auto-check", OK: true, Detail: "巡检 " + strconv.Itoa(n) + " 个订阅"})
}

func errText(err error, msg string) string {
	if err != nil {
		return err.Error()
	}
	return msg
}
