package api

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"homenet-panel/internal/audit"
	"homenet-panel/internal/auth"
	"homenet-panel/internal/configtx"
	"homenet-panel/internal/dae"
	"homenet-panel/internal/geo"
	"homenet-panel/internal/model"
	"homenet-panel/internal/preflight"
	"homenet-panel/internal/route"
	"homenet-panel/internal/runtimecheck"
	"homenet-panel/internal/subs"
	"homenet-panel/internal/subsrc"
	"homenet-panel/internal/traffic"
)

//go:embed index.html vue.global.prod.js
var webFS embed.FS

const cookieName = "hnp_session"

type Config struct {
	Dae          *dae.Adapter
	BackupDir    string
	Token        string // 可选静态 Bearer（自动化脚本用）
	AuthPath     string // 账号密码凭证文件
	EntryPath    string // 入口配置（含 include 与 subscription）
	CorePath     string // 主体配置（global/group/dns/routing）
	ModelPath    string // 结构化模型存储
	TLS          bool   // 启用 HTTPS 时 cookie 置 Secure
	GeoPath      string // geosite.dat 路径（分流命中调试）
	TrafficIface string // DNS 旁路统计抓包网卡（空=关闭流量统计）
	AuditPath    string // 审计日志（JSONL）路径
	SubsrcPath   string // 订阅源登记表路径
}

type Server struct {
	Dae          *dae.Adapter
	BackupDir    string
	Token        string
	AuthPath     string
	EntryPath    string
	CorePath     string
	ModelPath    string
	TLS          bool
	GeoPath      string
	TrafficIface string
	AuditPath    string
	SubsrcPath   string

	Sessions *auth.Sessions
	mu       sync.Mutex // 串行化变更类操作

	geoOnce sync.Once
	geoDB   *geo.DB
	geoErr  error

	Traffic    traffic.Source // 可插拔统计采集源（DNS 旁路；eBPF 预留）
	TrafficErr error

	Bytes      *traffic.ConnCollector // 字节级流量统计（socket 计数器轮询）
	bytesMu    sync.Mutex
	bytesIdx   map[string]string // ip:port -> 节点名
	bytesIdxAt time.Time
}

func New(c Config) *Server {
	entry := c.EntryPath
	if entry == "" {
		entry = dae.DefaultConfig
	}
	core := c.CorePath
	if core == "" {
		core = filepath.Join(filepath.Dir(entry), "config.d", "panel-core.dae")
	}
	ap := c.AuthPath
	if ap == "" {
		ap = auth.DefaultPath
	}
	mp := c.ModelPath
	if mp == "" {
		mp = "/var/lib/homenet-panel/model.json"
	}
	gp := c.GeoPath
	if gp == "" {
		gp = "/usr/share/dae/geosite.dat"
	}
	apAudit := c.AuditPath
	if apAudit == "" {
		apAudit = "/var/lib/homenet-panel/audit.log"
	}
	ssPath := c.SubsrcPath
	if ssPath == "" {
		ssPath = "/var/lib/homenet-panel/subsrc.json"
	}
	s := &Server{
		Dae:          c.Dae,
		BackupDir:    c.BackupDir,
		Token:        c.Token,
		AuthPath:     ap,
		EntryPath:    entry,
		CorePath:     core,
		ModelPath:    mp,
		TLS:          c.TLS,
		GeoPath:      gp,
		TrafficIface: c.TrafficIface,
		AuditPath:    apAudit,
		SubsrcPath:   ssPath,
		Sessions:     auth.NewSessions(12 * time.Hour),
	}
	if c.TrafficIface != "" {
		if src, err := traffic.NewDNSSource(c.TrafficIface); err != nil {
			s.TrafficErr = err
		} else {
			s.Traffic = src
		}
	}
	return s
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	b, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (s *Server) currentUser(r *http.Request) string {
	if c, err := r.Cookie(cookieName); err == nil {
		if u, ok := s.Sessions.User(c.Value); ok {
			return u
		}
	}
	return ""
}

// auth 中间件：允许会话 Cookie，或配置了静态 Token 时允许 Bearer（自动化）。
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Token != "" && r.Header.Get("Authorization") == "Bearer "+s.Token {
			next(w, r)
			return
		}
		if s.currentUser(r) != "" {
			next(w, r)
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
	}
}

// preflightHook 与 CLI 一致：--allow-goose 仅豁免 goose 头。
func preflightHook(allowGoose bool) func(path string, content []byte) error {
	return func(path string, content []byte) error {
		r := preflight.Detect(path, content)
		if !r.Blocking() {
			return nil
		}
		if allowGoose && len(r.GooseRunning) == 0 && len(r.OtherWriters) == 0 && len(r.SubErrors) == 0 {
			return nil
		}
		return &blockErr{reasons: r.Reasons()}
	}
}

type blockErr struct{ reasons []string }

func (e *blockErr) Error() string { return "预检阻断: " + strings.Join(e.reasons, "; ") }

// ---- 登录 ----

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad json"})
		return
	}
	ok, err := auth.Verify(s.AuthPath, req.Username, req.Password)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "凭证文件不可用: " + err.Error()})
		return
	}
	if !ok {
		_ = audit.Append(s.AuditPath, audit.Entry{Actor: req.Username, IP: clientIP(r), Action: "login", OK: false, Detail: "用户名或密码错误"})
		writeJSON(w, 401, map[string]any{"error": "用户名或密码错误"})
		return
	}
	tok := s.Sessions.Create(req.Username)
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.TLS,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.Sessions.TTL().Seconds()),
	})
	_ = audit.Append(s.AuditPath, audit.Entry{Actor: req.Username, IP: clientIP(r), Action: "login", OK: true})
	writeJSON(w, 200, map[string]any{"ok": true, "user": req.Username})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.Sessions.Delete(c.Value)
	}
	s.audit(r, "logout", "", true, "")
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == "" {
		writeJSON(w, 401, map[string]any{"error": "unauthorized"})
		return
	}
	writeJSON(w, 200, map[string]any{"user": u})
}

// ---- 模型（内容层） ----

func (s *Server) loadModel() (*model.Model, string, error) {
	if b, err := os.ReadFile(s.ModelPath); err == nil {
		var m model.Model
		if json.Unmarshal(b, &m) == nil {
			return &m, "file", nil
		}
	}
	entry, _ := os.ReadFile(s.EntryPath)
	core, _ := os.ReadFile(s.CorePath)
	m, err := model.Parse(string(entry), string(core))
	return m, "live", err
}

func (s *Server) saveModel(m *model.Model) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.ModelPath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.ModelPath, b, 0o600)
}

func (s *Server) handleGetModel(w http.ResponseWriter, r *http.Request) {
	m, src, err := s.loadModel()
	out := map[string]any{
		"model":     m,
		"source":    src,
		"entryPath": s.EntryPath,
		"corePath":  s.CorePath,
		// 面板强制注入的安全直连规则（界面上不可删，仅展示）
		"essentialRules": model.EssentialRules,
	}
	if err != nil {
		out["parseError"] = err.Error()
	}
	writeJSON(w, 200, out)
}

func (s *Server) handlePreviewModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model model.Model `json:"model"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad json"})
		return
	}
	entry, core := model.Render(&req.Model)
	writeJSON(w, 200, map[string]any{"entry": entry, "core": core})
}

// allSources 解析当前模型所有订阅的节点来源。
func (s *Server) allSources() ([]subs.Source, string) {
	m, _, _ := s.loadModel()
	baseDir := filepath.Dir(s.EntryPath)
	var out []subs.Source
	if m != nil {
		for _, sub := range m.Subscriptions {
			out = append(out, subs.Resolve(baseDir, sub.Name, sub.Source))
		}
	}
	return out, baseDir
}

func flatten(sources []subs.Source) []subs.Node {
	var all []subs.Node
	for _, s := range sources {
		all = append(all, s.Nodes...)
	}
	return all
}

// handleSubsGroups 返回每个分组命中的节点（按 dae filter 语义本地求值）。
func (s *Server) handleSubsGroups(w http.ResponseWriter, r *http.Request) {
	m, _, err := s.loadModel()
	if err != nil && m == nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	sources, _ := s.allSources()
	all := flatten(sources)
	type grpOut struct {
		Name   string      `json:"name"`
		Policy string      `json:"policy"`
		Count  int         `json:"count"`
		Nodes  []subs.Node `json:"nodes"`
	}
	var groups []grpOut
	if m != nil {
		for _, g := range m.Groups {
			matched := []subs.Node{}
			for _, n := range all {
				if subs.MatchGroup(g.Filters, n) {
					matched = append(matched, n)
				}
			}
			groups = append(groups, grpOut{g.Name, g.Policy, len(matched), matched})
		}
	}
	writeJSON(w, 200, map[string]any{"groups": groups, "total": len(all)})
}

// handleSubsSpeedtest 并发测量节点 TCP 建连延迟。
func (s *Server) handleSubsSpeedtest(w http.ResponseWriter, r *http.Request) {
	type target struct {
		Host string `json:"host"`
		Port string `json:"port"`
	}
	var req struct {
		Nodes     []target `json:"nodes"`
		TimeoutMs int      `json:"timeoutMs"`
	}
	_ = readJSON(r, &req)
	targets := req.Nodes
	if len(targets) == 0 {
		sources, _ := s.allSources()
		for _, n := range flatten(sources) {
			targets = append(targets, target{n.Host, n.Port})
		}
	}
	timeout := 3000 * time.Millisecond
	if req.TimeoutMs > 0 {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}
	type lat struct {
		Host string `json:"host"`
		Port string `json:"port"`
		Ms   int64  `json:"ms"`
		Err  string `json:"error,omitempty"`
	}
	results := make([]lat, len(targets))
	sem := make(chan struct{}, 30)
	var wg sync.WaitGroup
	for i := range targets {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ms, err := subs.DialLatency(targets[i].Host, targets[i].Port, timeout)
			results[i] = lat{targets[i].Host, targets[i].Port, ms, errString(err)}
		}(i)
	}
	wg.Wait()
	writeJSON(w, 200, map[string]any{"results": results})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// handleAggregatePreview 预览多订阅合并去重结果。
func (s *Server) handleAggregatePreview(w http.ResponseWriter, r *http.Request) {
	sources, _ := s.allSources()
	perSource := []map[string]any{}
	for _, src := range sources {
		perSource = append(perSource, map[string]any{"name": src.Name, "count": src.Count, "error": src.Error})
	}
	_, stats := subs.Aggregate(sources)
	writeJSON(w, 200, map[string]any{"perSource": perSource, "stats": stats})
}

// handleAggregateBuild 合并去重后生成 panel-merged.sub（base64，0600），供 dae 以 file:// 订阅。
func (s *Server) handleAggregateBuild(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	_ = readJSON(r, &req)
	sources, baseDir := s.allSources()
	merged, stats := subs.Aggregate(sources)
	if len(merged) == 0 {
		writeJSON(w, 422, map[string]any{"error": "合并后 0 节点，无法生成", "stats": stats})
		return
	}
	var lines []string
	for _, n := range merged {
		if n.Raw != "" {
			lines = append(lines, n.Raw)
		}
	}
	content := base64.StdEncoding.EncodeToString([]byte(strings.Join(lines, "\n")))
	path := req.Path
	if path == "" {
		path = filepath.Join(baseDir, "subs", "panel-merged.sub")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"path": path, "count": len(lines), "stats": stats})
}

func (s *Server) geo() *geo.DB {
	s.geoOnce.Do(func() {
		s.geoDB, s.geoErr = geo.Load(s.GeoPath)
	})
	return s.geoDB
}

// handleRouteTest 对给定域名按当前分流规则顺序求值，报告命中的规则与出口。
func (s *Server) handleRouteTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	var req struct {
		Domain string       `json:"domain"`
		Model  *model.Model `json:"model"` // 可选：测试“正在编辑但未保存”的模型，而非 live
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad json"})
		return
	}
	domain := strings.TrimSpace(req.Domain)
	if domain == "" {
		writeJSON(w, 400, map[string]any{"error": "缺少 domain"})
		return
	}
	var src *model.Model
	if req.Model != nil {
		src = req.Model
	} else {
		m, _, err := s.loadModel()
		if err != nil && m == nil {
			writeJSON(w, 500, map[string]any{"error": "无法加载模型: " + err.Error()})
			return
		}
		src = m
	}
	rules := make([]route.Rule, 0, len(src.Rules))
	for _, ru := range src.Rules {
		rules = append(rules, route.Rule{Matcher: ru.Matcher, Out: ru.Out})
	}
	res := route.Test(domain, rules, src.Fallback, s.geo())
	out := map[string]any{"result": res, "geoLoaded": s.geoDB != nil, "geoPath": s.GeoPath}
	if s.geoErr != nil {
		out["geoError"] = s.geoErr.Error()
	}
	writeJSON(w, 200, out)
}

// handleTrafficHits 返回按域名统计的查询量，并用当前分流规则标注归属出口。
func (s *Server) handleTrafficHits(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	if s.Traffic == nil {
		detail := ""
		if s.TrafficErr != nil {
			detail = s.TrafficErr.Error()
		}
		writeJSON(w, 200, map[string]any{
			"available": false,
			"error":     "流量统计未启用",
			"detail":    detail,
			"sources":   traffic.Sources(false, detail),
		})
		return
	}
	snap := s.Traffic.Snapshot(limit)

	m, _, err := s.loadModel()
	var rules []route.Rule
	fallback := ""
	if m != nil {
		for _, ru := range m.Rules {
			rules = append(rules, route.Rule{Matcher: ru.Matcher, Out: ru.Out})
		}
		fallback = m.Fallback
	}
	db := s.geo()

	type hitOut struct {
		Domain   string    `json:"domain"`
		Count    int64     `json:"count"`
		LastSeen time.Time `json:"lastSeen"`
		Out      string    `json:"out"`
		Decided  bool      `json:"decided"`
	}
	groups := map[string]int64{}
	hits := make([]hitOut, 0, len(snap.Hits))
	for _, h := range snap.Hits {
		out := ""
		decided := false
		if m != nil {
			res := route.Test(h.Domain, rules, fallback, db)
			out = res.Chosen
			decided = res.Decided
		}
		hits = append(hits, hitOut{Domain: h.Domain, Count: h.Count, LastSeen: h.LastSeen, Out: out, Decided: decided})
		key := out
		if key == "" {
			key = "(未知)"
		} else if !decided {
			key = key + "(含未决)"
		}
		groups[key] += h.Count
	}
	out := map[string]any{
		"available": true,
		"source":    snap.Source,
		"iface":     snap.Iface,
		"startedAt": snap.StartedAt,
		"updatedAt": snap.UpdatedAt,
		"queries":   snap.Queries,
		"domains":   snap.Domains,
		"hits":      hits,
		"groups":    groups,
		"geoLoaded": s.geoDB != nil,
		"sources":   traffic.Sources(true, ""),
	}
	if snap.Error != "" {
		out["error"] = snap.Error
	}
	if err != nil {
		out["modelError"] = err.Error()
	}
	writeJSON(w, 200, out)
}

// handleTrafficSources 返回可插拔采集源清单（含预留的 eBPF 源）。
func (s *Server) handleTrafficSources(w http.ResponseWriter, r *http.Request) {
	detail := ""
	ready := s.Traffic != nil
	if s.TrafficErr != nil {
		detail = s.TrafficErr.Error()
	}
	writeJSON(w, 200, map[string]any{
		"iface":   s.TrafficIface,
		"active":  ready,
		"detail":  detail,
		"sources": traffic.Sources(ready, detail),
	})
}

func (s *Server) handleSubsNodes(w http.ResponseWriter, r *http.Request) {
	m, _, err := s.loadModel()
	if err != nil && m == nil {
		writeJSON(w, 500, map[string]any{"error": "无法加载模型: " + err.Error()})
		return
	}
	baseDir := filepath.Dir(s.EntryPath)
	var out []subs.Source
	for _, sub := range m.Subscriptions {
		out = append(out, subs.Resolve(baseDir, sub.Name, sub.Source))
	}
	total := 0
	for _, s := range out {
		total += s.Count
	}
	writeJSON(w, 200, map[string]any{
		"baseDir":       baseDir,
		"total":         total,
		"subscriptions": out,
	})
}

func (s *Server) handleApplyModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model model.Model `json:"model"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad json"})
		return
	}
	entry, core := model.Render(&req.Model)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.CorePath), 0o700); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	m := configtx.New(s.Dae, s.BackupDir)
	m.Preflight = preflightHook(false)
	res, err := m.ApplyMulti(s.EntryPath, []configtx.Change{
		{Path: s.EntryPath, NewContent: []byte(entry)},
		{Path: s.CorePath, NewContent: []byte(core)},
	})
	s.audit(r, "model.apply", s.EntryPath+"+"+s.CorePath, err == nil, errText(err, res.Message))
	code := 200
	out := map[string]any{"result": res, "entry": entry, "core": core}
	if err != nil {
		code = 422
		out["error"] = err.Error()
	} else {
		if serr := s.saveModel(&req.Model); serr != nil {
			out["modelSaveError"] = serr.Error()
		}
	}
	writeJSON(w, code, out)
}

// handleAggregateSwitch 模式B 一键安全切换：合并多订阅为一个文件，改订阅引用并把分组
// filter 中的 subtag(旧订阅名) 统一改写为 subtag(新名)，然后以事务方式应用（校验+reload+失败回滚）。
// 默认 dryRun（isDryRun 未显式 confirm 时只返回预览，不落盘、不改配置）。
func (s *Server) handleAggregateSwitch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`    // 合并订阅名，默认 merged
		Path    string `json:"path"`    // 合并文件路径，默认 <配置目录>/subs/panel-merged.sub
		DryRun  bool   `json:"dryRun"`  // true 或未 confirm 时仅预览
		Confirm bool   `json:"confirm"` // 真正切换需为 true
	}
	_ = readJSON(r, &req)
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "merged"
	}
	sources, baseDir := s.allSources()
	merged, stats := subs.Aggregate(sources)
	if len(merged) == 0 {
		writeJSON(w, 422, map[string]any{"error": "合并后 0 节点，无法切换", "stats": stats})
		return
	}
	m, _, err := s.loadModel()
	if err != nil && m == nil {
		writeJSON(w, 500, map[string]any{"error": "无法加载模型: " + err.Error()})
		return
	}
	oldNames := make([]string, 0, len(m.Subscriptions))
	for _, sub := range m.Subscriptions {
		oldNames = append(oldNames, sub.Name)
	}
	path := req.Path
	if path == "" {
		path = filepath.Join(baseDir, "subs", "panel-merged.sub")
	}
	// 计算可移植的订阅源：dae 的 file:// 仅支持相对入口配置目录（R2），故必须落在 baseDir 下。
	rel, rerr := filepath.Rel(baseDir, path)
	var source string
	if rerr == nil && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
		source = "file://" + filepath.ToSlash(rel)
	} else {
		writeJSON(w, 422, map[string]any{"error": "合并文件必须位于配置目录内（dae file:// 仅支持相对路径）", "path": path, "baseDir": baseDir})
		return
	}
	nm := *m
	nm.Subscriptions = []model.Subscription{{Name: name, Source: source}}
	nm.Groups = make([]model.Group, len(m.Groups))
	groupChanges := make([]map[string]any, 0, len(m.Groups))
	for i, g := range m.Groups {
		ng := g
		ng.Filters = model.RewriteSubtags(g.Filters, oldNames, name)
		nm.Groups[i] = ng
		if !reflect.DeepEqual(g.Filters, ng.Filters) {
			groupChanges = append(groupChanges, map[string]any{"name": g.Name, "before": g.Filters, "after": ng.Filters})
		}
	}
	entry, core := model.Render(&nm)
	out := map[string]any{
		"mergedPath":       path,
		"count":            len(merged),
		"stats":            stats,
		"oldSubscriptions": oldNames,
		"newSource":        source,
		"groupChanges":     groupChanges,
		"model":            nm,
		"entry":            entry,
		"core":             core,
	}
	if req.DryRun || !req.Confirm {
		out["dryRun"] = true
		writeJSON(w, 200, out)
		return
	}
	// 落盘合并文件
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	var lines []string
	for _, n := range merged {
		if n.Raw != "" {
			lines = append(lines, n.Raw)
		}
	}
	contentB64 := base64.StdEncoding.EncodeToString([]byte(strings.Join(lines, "\n")))
	if err := os.WriteFile(path, []byte(contentB64), 0o600); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	// 事务应用
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.CorePath), 0o700); err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	tx := configtx.New(s.Dae, s.BackupDir)
	tx.Preflight = preflightHook(false)
	res, aerr := tx.ApplyMulti(s.EntryPath, []configtx.Change{
		{Path: s.EntryPath, NewContent: []byte(entry)},
		{Path: s.CorePath, NewContent: []byte(core)},
	})
	out["result"] = res
	code := 200
	if aerr != nil {
		code = 422
		out["error"] = aerr.Error()
	}
	s.audit(r, "aggregate.switch", source, aerr == nil, errText(aerr, res.Message))
	if aerr == nil {
		if serr := s.saveModel(&nm); serr != nil {
			out["modelSaveError"] = serr.Error()
		}
		out["applied"] = true
	}
	writeJSON(w, code, out)
}

func (s *Server) Register(mux *http.ServeMux) {
	// 公共端点
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/logout", s.handleLogout)
	mux.HandleFunc("/api/me", s.handleMe)

	// 受保护端点
	mux.HandleFunc("/api/health", s.auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/api/status", s.auth(func(w http.ResponseWriter, r *http.Request) {
		pid := s.Dae.RunningPID()
		prog, perr := s.Dae.Progress()
		out := map[string]any{"pid": pid, "service": s.Dae.ServiceActive(), "configPath": s.Dae.ConfigPath}
		// 订阅变化提醒：只读登记表，供顶部状态栏显示「N 条订阅上游有变化」
		if st := subsrc.Load(s.SubsrcPath); st != nil {
			changed, names := subsrc.Summary(st)
			out["subsrcChanged"] = changed
			out["subsrcNames"] = names
			out["subsrcLastRun"] = st.LastRun
		}
		if perr == nil {
			out["progressCode"] = prog.Describe()
			out["progressMsg"] = prog.Message
		} else {
			out["progressErr"] = perr.Error()
		}
		writeJSON(w, 200, out)
	}))
	mux.HandleFunc("/api/detect", s.auth(func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Dae.Detect()
		if err != nil {
			writeJSON(w, 200, map[string]any{"ok": false, "output": v, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "version": v})
	}))
	mux.HandleFunc("/api/runtimecheck", s.auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, runtimecheck.Run())
	}))
	mux.HandleFunc("/api/outline", s.auth(func(w http.ResponseWriter, r *http.Request) {
		out, err := s.Dae.Outline()
		if err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		io.WriteString(w, out)
	}))
	mux.HandleFunc("/api/preflight", s.auth(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			path = s.Dae.ConfigPath
		}
		content, err := os.ReadFile(path)
		if err != nil {
			writeJSON(w, 400, map[string]any{"error": err.Error()})
			return
		}
		rep := preflight.Detect(path, content)
		writeJSON(w, 200, map[string]any{"report": rep, "blocking": rep.Blocking(), "reasons": rep.Reasons()})
	}))
	mux.HandleFunc("/api/validate", s.auth(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path string `json:"path"`
		}
		_ = readJSON(r, &req)
		if req.Path == "" {
			req.Path = s.Dae.ConfigPath
		}
		vr := s.Dae.Validate(req.Path)
		writeJSON(w, 200, map[string]any{"ok": vr.OK, "exitCode": vr.ExitCode, "output": vr.Output})
	}))
	mux.HandleFunc("/api/apply", s.auth(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Target     string `json:"target"`
			Content    string `json:"content"`
			AllowGoose bool   `json:"allowGoose"`
		}
		if err := readJSON(r, &req); err != nil {
			writeJSON(w, 400, map[string]any{"error": "bad json: " + err.Error()})
			return
		}
		if req.Target == "" {
			req.Target = s.Dae.ConfigPath
		}
		if req.Content == "" {
			writeJSON(w, 400, map[string]any{"error": "content 为空"})
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		m := configtx.New(s.Dae, s.BackupDir)
		m.Preflight = preflightHook(req.AllowGoose)
		res, err := m.Apply(configtx.Change{Path: req.Target, NewContent: []byte(req.Content)})
		code := 200
		out := map[string]any{"result": res}
		if err != nil {
			code = 422
			out["error"] = err.Error()
		}
		s.audit(r, "apply", req.Target, err == nil, errText(err, res.Message))
		writeJSON(w, code, out)
	}))
	mux.HandleFunc("/api/rollback", s.auth(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Target string `json:"target"`
		}
		_ = readJSON(r, &req)
		if req.Target == "" {
			req.Target = s.Dae.ConfigPath
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		m := configtx.New(s.Dae, s.BackupDir)
		res, err := m.Rollback(req.Target)
		code := 200
		out := map[string]any{"result": res}
		if err != nil {
			code = 422
			out["error"] = err.Error()
		}
		s.audit(r, "rollback", req.Target, err == nil, errText(err, res.Message))
		writeJSON(w, code, out)
	}))
	mux.HandleFunc("/api/suspend", s.auth(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		code, out := s.Dae.Suspend(s.Dae.RunningPID())
		s.audit(r, "suspend", "", code == 0, out)
		writeJSON(w, 200, map[string]any{"exitCode": code, "output": out})
	}))

	// 内容层
	mux.HandleFunc("/api/model", s.auth(s.handleGetModel))
	mux.HandleFunc("/api/model/preview", s.auth(s.handlePreviewModel))
	mux.HandleFunc("/api/model/apply", s.auth(s.handleApplyModel))
	mux.HandleFunc("/api/subs/nodes", s.auth(s.handleSubsNodes))
	mux.HandleFunc("/api/subs/groups", s.auth(s.handleSubsGroups))
	mux.HandleFunc("/api/subs/speedtest", s.auth(s.handleSubsSpeedtest))
	mux.HandleFunc("/api/aggregate/preview", s.auth(s.handleAggregatePreview))
	mux.HandleFunc("/api/aggregate/build", s.auth(s.handleAggregateBuild))
	mux.HandleFunc("/api/aggregate/switch", s.auth(s.handleAggregateSwitch))
	mux.HandleFunc("/api/route/test", s.auth(s.handleRouteTest))
	mux.HandleFunc("/api/traffic/hits", s.auth(s.handleTrafficHits))
	mux.HandleFunc("/api/traffic/sources", s.auth(s.handleTrafficSources))
	mux.HandleFunc("/api/traffic/bytes", s.auth(s.handleTrafficBytes))

	// 订阅源更新 + 审计
	mux.HandleFunc("/api/subsrc", s.auth(s.handleSubsrcList))
	mux.HandleFunc("/api/subsrc/url", s.auth(s.handleSubsrcSetURL))
	mux.HandleFunc("/api/subsrc/check", s.auth(s.handleSubsrcCheck))
	mux.HandleFunc("/api/subsrc/apply", s.auth(s.handleSubsrcApply))
	// 网络接入向导（P-A 只读探测）
	mux.HandleFunc("/api/access/probe", s.auth(s.handleAccessProbe))
	mux.HandleFunc("/api/access/recommend", s.auth(s.handleAccessRecommend))

	mux.HandleFunc("/api/audit", s.auth(s.handleAudit))
	mux.HandleFunc("/api/audit/export", s.auth(s.handleAuditExport))

	// 本地 Vue（去 CDN 依赖）
	mux.HandleFunc("/assets/vue.js", func(w http.ResponseWriter, r *http.Request) {
		b, err := webFS.ReadFile("vue.global.prod.js")
		if err != nil {
			http.Error(w, "asset not found", 500)
			return
		}
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Write(b)
	})

	// 前端静态页（公共）
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		b, err := webFS.ReadFile("index.html")
		if err != nil {
			http.Error(w, "ui not found", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(b)
	})
}
