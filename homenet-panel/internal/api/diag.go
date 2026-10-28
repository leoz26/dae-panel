package api

import (
	"net/http"
	"strconv"
	"time"

	"homenet-panel/internal/diag"
)

// handleDiagHealth：只读聚合 dae 运行日志里的健康事件（判死/超时/DNS/选点）。
// 为什么读日志而不读 dae 内部状态：dae 没有暴露指标接口，journal 是唯一权威来源。
func (s *Server) handleDiagHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]any{"error": "method not allowed"})
		return
	}
	minutes := 60
	if v := r.URL.Query().Get("minutes"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1440 {
			minutes = n
		}
	}
	unit := "dae"
	if v := r.URL.Query().Get("unit"); v != "" && len(v) <= 32 {
		unit = v
	}
	rep := diag.FromCommand(nil, unit, minutes, time.Now())
	if len(rep.Nodes) > 30 {
		rep.Nodes = rep.Nodes[:30]
	}
	if len(rep.Groups) > 12 {
		rep.Groups = rep.Groups[:12]
	}
	if len(rep.Series) > 240 {
		rep.Series = rep.Series[len(rep.Series)-240:]
	}
	kinds := []map[string]string{}
	for _, k := range []string{diag.KindNoDialer, diag.KindBootstrap, diag.KindIOTimeout, diag.KindServfail, diag.KindNotAlive, diag.KindProxyIPFailure, diag.KindReselect, diag.KindSelect} {
		kinds = append(kinds, map[string]string{"key": k, "name": diag.KindName(k)})
	}
	writeJSON(w, 200, map[string]any{
		"code":   200,
		"report": rep,
		"kinds":  kinds,
	})
}
