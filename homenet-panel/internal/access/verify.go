package access

import (
	"fmt"
)

// VerifyInput 是自检需要的外部事实（全部只读，由调用方注入）。
type VerifyInput struct {
	Probe      *Probe `json:"-"`
	Clients    int    `json:"clients"`    // 字节统计里对端为内网的连接数（有 LAN 流量的硬证据）
	DNSQueries int64  `json:"dnsQueries"` // 面板采集到的 DNS 查询总数
	IfaceRx    int64  `json:"ifaceRx"`
	IfaceTx    int64  `json:"ifaceTx"`
}

// CheckResult 是一项自检结果。Level: ok | warn | fail | info。
type CheckResult struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Level  string `json:"level"`
	Detail string `json:"detail"`
}

// Verify 执行只读自检：不修改系统、不重启 dae，可反复调用。
func Verify(in VerifyInput) []CheckResult {
	out := []CheckResult{}
	p := in.Probe

	// 1) dae 是否在跑
	switch {
	case p == nil:
		out = append(out, CheckResult{Key: "dae_running", Name: "dae 正在运行", Level: "fail", Detail: "没有探测结果，无法判断"})
	case !p.DaeInstalled:
		out = append(out, CheckResult{Key: "dae_running", Name: "dae 正在运行", Level: "fail", Detail: "未检测到 dae 可执行文件"})
	case !p.DaeRunning:
		out = append(out, CheckResult{Key: "dae_running", Name: "dae 正在运行", Level: "fail", Detail: "dae 已安装但未运行（" + p.DaeVersion + "）"})
	default:
		out = append(out, CheckResult{Key: "dae_running", Name: "dae 正在运行", Level: "ok", Detail: p.DaeVersion})
	}

	// 2) 是否有内网设备经本机上网（LAN 接管的硬证据）
	if in.Clients > 0 {
		out = append(out, CheckResult{Key: "lan_clients", Name: "字节统计里出现内网客户端", Level: "ok",
			Detail: fmt.Sprintf("当前有 %d 条连接的对端是内网地址：LAN 流量确实经过本机", in.Clients)})
	} else {
		out = append(out, CheckResult{Key: "lan_clients", Name: "字节统计里出现内网客户端", Level: "warn",
			Detail: "还没有内网设备走本机：旁路由接线/网关指向可能没生效，或还没有设备访问外网"})
	}

	// 3) 拦截侧网卡计数
	if in.IfaceRx+in.IfaceTx > 0 {
		out = append(out, CheckResult{Key: "dae0_counters", Name: "拦截侧网卡计数在增长", Level: "ok",
			Detail: fmt.Sprintf("累计收 %d B / 发 %d B：有流量被接管", in.IfaceRx, in.IfaceTx)})
	} else {
		out = append(out, CheckResult{Key: "dae0_counters", Name: "拦截侧网卡计数在增长", Level: "warn",
			Detail: "拦截侧网卡计数为 0：暂时没有任何流量被接管（或字节统计未启用）"})
	}

	// 4) DNS 是否经过本机
	if in.DNSQueries > 0 {
		out = append(out, CheckResult{Key: "dns_hits", Name: "DNS 命中记录在增长", Level: "ok",
			Detail: fmt.Sprintf("已采集 %d 次 DNS 查询：域名规则（geosite/关键词）可生效", in.DNSQueries)})
	} else {
		out = append(out, CheckResult{Key: "dns_hits", Name: "DNS 命中记录在增长", Level: "warn",
			Detail: "还没有 DNS 记录：确认 LAN 设备的 DNS 指向本机，否则域名分流失效"})
	}

	// 5) 系统原有配置是否被动过
	out = append(out, CheckResult{Key: "config_untouched", Name: "系统原有配置未被修改", Level: "info",
		Detail: "面板只写自有片段（config.d/panel-access.dae）与自有 nft 表（panel_dns/panel_snat），从不修改 DHCP/PPPoE/NAT 与 /etc/config/*"})

	// 6) 是否有遗留的面板对象
	if p != nil && len(p.StaleTables) > 0 {
		out = append(out, CheckResult{Key: "stale_tables", Name: "没有遗留的 panel_* 表/片段", Level: "warn",
			Detail: "发现遗留表：" + joinComma(p.StaleTables) + "；先执行回滚清理再重新接管，避免规则叠加"})
	} else {
		out = append(out, CheckResult{Key: "stale_tables", Name: "没有遗留的 panel_* 表/片段", Level: "ok", Detail: "环境干净"})
	}

	return out
}

func joinComma(in []string) string {
	out := ""
	for i, s := range in {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
