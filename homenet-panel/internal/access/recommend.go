package access

import "strconv"

// AccessMode 是接入档位（见 architecture.md §4.4.3）。
type AccessMode string

const (
	ModeGateway          AccessMode = "gateway"            // A：本机就是网关（同机 OpenWrt/软路由）
	ModeBypassManual     AccessMode = "bypass-manual"      // B1：单台设备手改网关/DNS
	ModeBypassRouterDHCP AccessMode = "bypass-router-dhcp" // B2：主路由 DHCP 下发网关/DNS
	ModeBypassSelfDHCP   AccessMode = "bypass-self-dhcp"   // B3：面板托管 DHCP（默认关闭，需显式授权）
	ModeBypassPBR        AccessMode = "bypass-pbr"         // C ：主路由策略路由（只生成片段）
	ModeObserve          AccessMode = "observe"            // 只观测，不接管
)

// Label 返回中文名称，供 UI 展示。
func (m AccessMode) Label() string {
	switch m {
	case ModeGateway:
		return "A 网关（同机）"
	case ModeBypassManual:
		return "B1 旁路由 · 单机手改"
	case ModeBypassRouterDHCP:
		return "B2 旁路由 · 主路由 DHCP 下发网关"
	case ModeBypassSelfDHCP:
		return "B3 旁路由 · 面板托管 DHCP（高风险）"
	case ModeBypassPBR:
		return "C 旁路由 · 主路由策略路由"
	case ModeObserve:
		return "观察模式（不接管）"
	}
	return string(m)
}

// Valid 判断是否为已知档位。
func (m AccessMode) Valid() bool {
	switch m {
	case ModeGateway, ModeBypassManual, ModeBypassRouterDHCP, ModeBypassSelfDHCP, ModeBypassPBR, ModeObserve:
		return true
	}
	return false
}

// Answers 是向导三问（及各档附加问）的用户回答；nil 表示"不知道"。
type Answers struct {
	DHCPGateway    *bool `json:"dhcpGateway"`    // 主路由 DHCP 能否自定义下发的网关
	DHCPDNS        *bool `json:"dhcpDns"`        // 能否自定义下发的 DNS
	CanDisableDHCP *bool `json:"canDisableDhcp"` // 能否关闭主路由 DHCP
	PBR            *bool `json:"pbr"`            // 主路由能否做策略路由（ip rule/nftset）
}

// Questions 是与厂商无关的固定三问 + 一条可选问。
func Questions() []Question {
	return []Question{
		{Key: "dhcpGateway", Text: "主路由后台的 DHCP 设置里，能修改「下发的网关地址」吗？",
			Why: "能改 → 全屋设备自动走旁路由（B2）；不能改 → 只能单台手改验证（B1）"},
		{Key: "dhcpDns", Text: "能修改「下发的 DNS 地址」吗？",
			Why: "DNS 必须经过本机，否则 dae 拿不到域名，geosite/域名规则会失效"},
		{Key: "canDisableDhcp", Text: "能关闭主路由的 DHCP 服务吗？",
			Why: "只有能关，才允许选 B3（面板自己发 DHCP）：风险最高，默认关闭"},
		{Key: "pbr", Text: "主路由能做策略路由吗（ip rule / nftset / 指定设备下一跳）？",
			Why: "能 → 可以只让指定设备/域名走本机，影响面最小（C 档，面板只生成片段）"},
	}
}

// Recommend 依据探测结果与用户回答给出档位建议，永远附带理由。
func Recommend(p *Probe, a Answers) Rec {
	r := Rec{Mode: ModeObserve, Questions: Questions()}
	if p == nil {
		r.Blockers = append(r.Blockers, "没有探测结果")
		r.Reasons = append(r.Reasons, "无法判断本机能力，先按观察模式运行")
		return r
	}

	// 1) 硬性阻塞项
	if p.Platform != "linux" && p.Platform != "openwrt" {
		r.Blockers = append(r.Blockers, "当前平台 "+p.Platform+" 不是 Linux/OpenWrt：接入层必须运行在旁路由或网关上")
	}
	if !p.HasBPF || !p.HasBTF {
		r.Blockers = append(r.Blockers, "内核缺少 BPF/BTF 支持：dae 无法在数据面工作，只能观察")
	}
	if len(r.Blockers) > 0 {
		r.Reasons = append(r.Reasons, "存在阻塞项，先不要接管流量；修好后重新探测")
		return r
	}

	// 2) 基线档位：同机 OpenWrt → A；普通 Linux 主机 → B1
	switch {
	case p.Platform == "openwrt":
		r.Mode = ModeGateway
		r.Reasons = append(r.Reasons, "检测到 OpenWrt 且内核支持 BPF/BTF：dae 与本机同体，可走网关档（A）")
		if p.ServiceManager == "procd" {
			r.Reasons = append(r.Reasons, "服务管理器为 procd：面板按 procd 适配，不改 /etc/config/*")
		}
	default:
		r.Mode = ModeBypassManual
		r.Reasons = append(r.Reasons,
			"普通 Linux 主机（非 OpenWrt）：dae 只能做旁路由，默认按最安全的 B1（单机手改网关）",
			"决定能否全屋生效的唯一开关是「主路由 DHCP 能否自定义下发网关」")
	}

	// 3) 用户回答驱动的升降档
	if a.DHCPGateway != nil && *a.DHCPGateway {
		r.Mode = ModeBypassRouterDHCP
		r.Reasons = append(r.Reasons, "你确认主路由能改 DHCP 下发的网关 → 升到 B2，全屋生效，回滚=改回设置")
	} else if a.DHCPGateway != nil && !*a.DHCPGateway {
		r.Reasons = append(r.Reasons, "主路由不能改 DHCP 网关 → 保持 B1（单台设备手改验证）")
	}
	if a.PBR != nil && *a.PBR {
		r.Reasons = append(r.Reasons, "主路由能做策略路由 → 可另选 C 档：面板只生成 nftset/ip rule 片段，由你执行（不远程登录主路由）")
	}
	if a.CanDisableDHCP != nil && *a.CanDisableDHCP {
		r.Reasons = append(r.Reasons, "能关闭主路由 DHCP → 可选 B3（面板托管 DHCP）：默认关闭、需二次确认、且 30 分钟后自动失效")
	}
	if a.DHCPDNS != nil && !*a.DHCPDNS {
		r.Reasons = append(r.Reasons, "主路由不能改 DNS → 需要额外手段让 DNS 经过本机，否则域名分流失效（自检矩阵会报出来）")
	}

	// 4) 环境提示（不改变档位）
	if p.DefaultGw == "" {
		r.Reasons = append(r.Reasons, "未检测到默认路由：请确认本机已联网，否则无法验证出口")
	}
	if p.MemlockKB > 0 && p.MemlockKB < 65536 {
		r.Reasons = append(r.Reasons, "memlock 上限偏小（"+strconv.Itoa(p.MemlockKB)+" KB）：daemon 侧建议设置 LimitMEMLOCK=infinity，避免 BPF map 分配失败")
	}
	if !p.IPForward {
		r.Reasons = append(r.Reasons, "ip_forward=0：dae 的 auto_config_kernel_parameter: true 会自动打开；若关闭该选项则面板不做修改")
	}
	if len(p.StaleTables) > 0 {
		r.Reasons = append(r.Reasons, "发现上次遗留的面板 nft 表（panel_*）：先执行回滚清理，避免规则叠加")
	}
	if !p.DaeInstalled {
		r.Reasons = append(r.Reasons, "未检测到 dae：请先安装 dae，再进入接入阶段")
	} else if !p.DaeRunning {
		r.Reasons = append(r.Reasons, "dae 已安装但未运行：接入前先让 dae 正常启动")
	}
	if len(p.DaeLanIfaces) == 0 {
		r.Reasons = append(r.Reasons, "dae 当前未声明 lan_interface：正在被代理的只有本机流量，LAN 设备还没送流量过来")
	}
	return r
}

// Guide 返回该档位的人话操作要点（只读说明，不执行）。
func Guide(m AccessMode) []string {
	switch m {
	case ModeGateway:
		return []string{
			"前提：dae 与面板跑在同一台 OpenWrt/软路由上，内核支持 BPF/BTF",
			"步骤：确认 lan_interface 指向 LAN 侧网桥；DHCP 仍由路由器自己负责，面板只读不写",
			"自检：局域网设备访问外网时，字节统计的 clients 表出现内网地址",
			"回滚：停面板、删 panel_* nft 表即可（不改 /etc/config/*）",
		}
	case ModeBypassManual:
		return []string{
			"前提：无（任何路由器都能用），影响面最小",
			"步骤 1：面板确认 dae 的 lan_interface 指向内网网卡",
			"步骤 2：只在一台设备上把网关与 DNS 改成这台旁路由的 IP",
			"步骤 3：用该设备访问外网，检查字节统计里 clients 出现它的内网 IP、dae0 计数增长",
			"回滚：把该设备的网关/DNS 改回主路由或设为自动",
		}
	case ModeBypassRouterDHCP:
		return []string{
			"前提：主路由 DHCP 能自定义「下发的网关」（多数光猫/家用路由不提供，需先确认）",
			"步骤 1：进入主路由 DHCP 设置，把网关与 DNS 指向这台旁路由的 IP",
			"步骤 2：设备重新连 WiFi 或续租，确认拿到新网关",
			"步骤 3：抽样自检设备的出口 IP、DNS 命中、dae0 计数",
			"回滚：把主路由 DHCP 的网关/DNS 改回默认（务必先截图记录原值）",
		}
	case ModeBypassSelfDHCP:
		return []string{
			"前提：能关闭主路由 DHCP；这是风险最高的一档，默认关闭",
			"步骤 1：面板启动自己的 DHCP（只下发网关与 DNS 两项）",
			"步骤 2：关闭主路由 DHCP，观察全屋设备能否正常拿到 IP",
			"步骤 3：面板在 30 分钟内可一键回滚；超时未确认则自动停止自己的 DHCP",
			"回滚：先恢复主路由 DHCP，再停面板 DHCP（顺序不能反）",
		}
	case ModeBypassPBR:
		return []string{
			"前提：主路由能配 ip rule / nftset / 按设备指定下一跳（基本只有 OpenWrt/软路由）",
			"步骤：面板生成主路由侧片段（nftset + ip rule + 需要的 uci），你复制到主路由执行",
			"注意：面板不远程登录主路由；片段只影响被命中的设备/域名",
			"回滚：删除对应 nftset/ip rule 即可",
		}
	}
	return []string{
		"观察模式：不改变任何流量路径",
		"可用能力：dae 运行状态、订阅与规则编辑、字节统计、DNS 命中",
		"升级路径：先确认内核 BPF/BTF，再选接入档位",
	}
}
