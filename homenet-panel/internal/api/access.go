package api

import (
	"net/http"
	"strconv"
	"strings"

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

// handleAccessPlan：生成接入计划（纯 dry-run：只渲染步骤与片段，不执行任何命令）。
func (s *Server) handleAccessPlan(w http.ResponseWriter, r *http.Request) {
	opt := access.PlanOptions{}
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		opt.Mode = access.AccessMode(q.Get("mode"))
		opt.DNSMode = q.Get("dnsMode")
		opt.SNAT = q.Get("snat")
		opt.ProxyIP = q.Get("proxyIp")
		opt.TestDevice = q.Get("testDevice")
		if v := q.Get("tproxyPort"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				opt.TProxyPort = n
			}
		}
		if v := q.Get("lanIfaces"); v != "" {
			for _, n := range strings.Split(v, ",") {
				if n = strings.TrimSpace(n); n != "" {
					opt.LanIfaces = append(opt.LanIfaces, n)
				}
			}
		}
	case http.MethodPost:
		if err := readJSON(r, &opt); err != nil {
			writeJSON(w, 400, map[string]any{"error": "无效的请求体"})
			return
		}
	default:
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	probe := access.Collect(nil, "/", s.EntryPath)
	plan := access.PlanFor(probe, opt)
	writeJSON(w, 200, map[string]any{
		"plan":   plan,
		"dryRun": true,
		"note":   "这只是计划：面板不会自己改网络。apply 阶段会先校验指纹，再备份并只写自有片段/自有 nft 表",
	})
}

// handleAccessVerify：只读自检（不改系统、不重启服务），可反复执行。
func (s *Server) handleAccessVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	probe := access.Collect(nil, "/", s.EntryPath)
	in := access.VerifyInput{Probe: probe}
	if s.Bytes != nil {
		snap := s.Bytes.Snapshot()
		in.Clients = len(snap.Clients)
		in.IfaceRx = snap.IfaceRx
		in.IfaceTx = snap.IfaceTx
	}
	if s.Traffic != nil {
		in.DNSQueries = s.Traffic.Snapshot(1).Queries
	}
	checks := access.Verify(in)
	okCount, warnCount := 0, 0
	for _, c := range checks {
		switch c.Level {
		case "ok":
			okCount++
		case "warn", "fail":
			warnCount++
		}
	}
	writeJSON(w, 200, map[string]any{
		"checks":  checks,
		"ok":      okCount,
		"warns":   warnCount,
		"summary": strconv.Itoa(okCount) + " 项通过 / " + strconv.Itoa(warnCount) + " 项待确认",
	})
}
