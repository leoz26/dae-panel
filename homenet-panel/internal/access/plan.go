package access

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// DefaultTProxyPort 是渲染 dae global 时使用的默认 tproxy 端口（与 dae 官方示例一致）。
const DefaultTProxyPort = 12345

// PlanOptions 是生成接入计划的输入。面板只"出计划"，不擅自改网络。
type PlanOptions struct {
	Mode       AccessMode `json:"mode"`
	LanIfaces  []string   `json:"lanIfaces"`
	TProxyPort int        `json:"tproxyPort"`
	DNSMode    string     `json:"dnsMode"`    // hijack53 | forward-from-router | none
	SNAT       string     `json:"snat"`       // auto | on | off
	ProxyIP    string     `json:"proxyIp"`    // 旁路由自身内网 IP（渲染 DNS 劫持/SNAT 用）
	TestDevice string     `json:"testDevice"` // B1 手动步骤里点名的测试设备（可空）
}

// Step 是计划里的一步：Snippet 为可直接复制的片段（nft/dae 配置/路由器操作说明）。
type Step struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	// Note 是给人看的白话补充（界面里显示在详情下方的提示框内），不影响执行。
	Note    string `json:"note,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

// Check 是自检项（全部只读）。
type Check struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	How  string `json:"how"`
}

// Plan 是一次接入变更的完整计划：面板侧步骤 + 需人工/路由器侧的步骤 + 回滚 + 自检。
type Plan struct {
	Mode         AccessMode `json:"mode"`
	Label        string     `json:"label"`
	ModeExplicit bool       `json:"modeExplicit"`
	DryRun       bool       `json:"dryRun"`
	LanIfaces    []string   `json:"lanIfaces"`
	ProxyIP      string     `json:"proxyIp,omitempty"`
	TProxyPort   int        `json:"tproxyPort"`
	DNSMode      string     `json:"dnsMode"`
	SNAT         string     `json:"snat"`
	Steps        []Step     `json:"steps"`
	ManualSteps  []Step     `json:"manualSteps"`
	Rollback     []Step     `json:"rollback"`
	Checks       []Check    `json:"checks"`
	Warnings     []string   `json:"warnings,omitempty"`
	Fingerprint  string     `json:"fingerprint"`
}

// PlanFor 依据探测结果与选项生成接入计划（纯函数，无副作用）。
//
// 默认档位遵循产品口径：未显式指定时用探测推荐（普通 Linux 主机 = B1 单机手改），
// 其余档位只保留"可生成计划"的路径，不默认启用。
func PlanFor(p *Probe, opt PlanOptions) Plan {
	pl := Plan{DryRun: true, ModeExplicit: opt.Mode.Valid()}
	pl.Mode = opt.Mode
	if !pl.Mode.Valid() {
		if p != nil {
			pl.Mode = p.Rec.Mode
		} else {
			pl.Mode = ModeObserve
		}
	}
	if !pl.Mode.Valid() {
		pl.Mode = ModeObserve
	}
	pl.Label = pl.Mode.Label()

	pl.TProxyPort = opt.TProxyPort
	if pl.TProxyPort <= 0 {
		pl.TProxyPort = DefaultTProxyPort
	}
	pl.LanIfaces = resolveLanIfaces(p, opt.LanIfaces)
	pl.ProxyIP = strings.TrimSpace(opt.ProxyIP)
	if pl.ProxyIP == "" {
		pl.ProxyIP = privateIPv4(p)
	}
	pl.DNSMode = opt.DNSMode
	if pl.DNSMode == "" {
		pl.DNSMode = "hijack53"
	}
	pl.SNAT = opt.SNAT
	if pl.SNAT == "" {
		pl.SNAT = "auto"
	}

	// 1) 面板侧：dae global 片段（决定"哪些流量进入 dae"）
	if pl.Mode == ModeObserve {
		pl.Warnings = append(pl.Warnings, "观察模式不改变任何流量路径：本计划只做只读校验")
	} else {
		pl.Steps = append(pl.Steps, Step{
			Title:  "确认 dae 的 LAN 绑定（lan_interface）",
			Detail: "dae 靠 lan_interface 决定处理哪些网卡的流量；不写这一项时只有本机流量被代理。地道的做法是由面板写入自有片段 config.d/panel-access.dae，而不是改入口配置。",
			Note: "打个比方：lan_interface 就是「dae 在小区哪个门岗值班」。不填 = 只管自己家（本机）；填 ens2/br-lan = 从这个门进来的所有住户（LAN 设备）都归它管。\n" +
				"注意它决定的是「谁来处理」，不是「怎么处理」——怎么分流仍然看下面的路由规则。",
			Snippet: RenderDaeGlobal(pl.LanIfaces, pl.TProxyPort),
		})
	}

	// 2) 面板侧：自有 nft 表（DNS 重定向 / 可选 SNAT）
	if pl.Mode != ModeObserve {
		pl.Steps = append(pl.Steps, Step{
			Title:  "自有 nft 表：DNS 重定向（panel_dns）",
			Detail: "DNS 必须经过本机，否则 dae 拿不到域名，geosite/关键词规则会失效。表按名字创建，随时可整表删除（完全卸载）。",
			Note: "为什么「看不到域名」：网络包里只有目标 IP（门牌号），没有域名（收件人姓名）。「youtube.com = 142.250.72.14」这个对照关系，只在查 DNS 的那一瞬间出现过；设备若把 DNS 发给主路由，dae 就没听见，只能看到一个发往 142.250.72.14 的连接，认不出是 YouTube，所有按域名写的规则全部失效（只剩 ip/geoip 规则有效）。\n" +
				"这一块就是把「查号码的过程」也交给 dae 听一遍：内网设备发出的 DNS 查询（UDP/TCP 53）在到达主路由前被改送到旁路由，dae 于是知道谁是谁。\n" +
				"（dae 也会从 HTTPS 握手的 SNI 里嗅探域名作补漏，但加密/纯 IP 连接看不到，不能依赖。）",
			Snippet: RenderNftDNS(pl.ProxyIP, pl.LanIfaces, pl.DNSMode),
		})
		pl.Steps = append(pl.Steps, Step{
			Title:  "自有 nft 表：条件性 SNAT（panel_snat）",
			Detail: "只有「未被劫持却被转发」的流量（ICMP、内网互访、部分 IPv6）在跨网段或主路由开 rp_filter 时才需要 SNAT；被 tproxy 劫持的 TCP/UDP 在旁路由本地终结，回程天然对称，不需要 SNAT。",
			Note: "为什么「通常不需要」：被 dae 接管的 TCP/UDP 连接，旁路由是「本地收下再自己发出去」，回包自然回到旁路由，路是对称的。\n" +
				"只有没被接管、只是被「转发」过去的流量（ping/ICMP、内网互访、部分 IPv6）才可能回程绕道；症状是「能连上但网页打不开」或「ping 不通」。真遇到了再把这里改成 on。",
			Snippet: RenderNftSNAT(pl.SNAT, pl.LanIfaces),
		})
		pl.Steps = append(pl.Steps, Step{
			Title:  "说明：拦截规则由 dae 自己安装（panel_tproxy 默认不建）",
			Detail: "dae 会为 lan_interface 自动安装 netkit/nft 拦截，面板不再叠一层 tproxy 规则以免冲突；panel_tproxy 仅在你需要自定义放行例外时才创建。",
			Note: "tproxy 是「把连接半路改道到本地代理端口」的手艺。dae 已经自己会做了（它会建 netkit 设备对 + 独立 netns 来拦流量），面板再建一层只会互相打架。\n" +
				"所以这里默认只留一段注释：等哪天你要「某些目标明确不走代理」的白名单例外，才有必要建这张表。",
			Snippet: RenderNftTProxyPlaceholder(),
		})
	}

	// 3) 人工/路由器侧
	pl.ManualSteps = append(pl.ManualSteps, manualStepsFor(pl, opt)...)

	// 4) 回滚
	pl.Rollback = rollbackStepsFor(pl)

	// 5) 自检
	pl.Checks = ChecksFor(pl.Mode)

	// 6) 风险提示（与档位相关）
	pl.Warnings = append(pl.Warnings, modeWarnings(pl.Mode)...)

	pl.Fingerprint = fingerprintOf(&pl)
	return pl
}

// resolveLanIfaces 依次取：用户显式指定 → dae 已声明的 lan_interface → 探测到的内网网卡。
func resolveLanIfaces(p *Probe, want []string) []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	for _, n := range want {
		add(n)
	}
	if len(out) > 0 {
		return out
	}
	if p != nil {
		for _, n := range p.DaeLanIfaces {
			add(n)
		}
		if len(out) > 0 {
			return out
		}
		for _, it := range p.Ifaces {
			if it.Loopback || it.Virtual || !it.Up {
				continue
			}
			if hasPrivateAddr(it.Addrs) {
				add(it.Name)
			}
		}
	}
	return out
}

func hasPrivateAddr(addrs []string) bool {
	for _, a := range addrs {
		ip := a
		if i := strings.LastIndex(a, "/"); i > 0 {
			ip = a[:i]
		}
		if isPrivateIP(ip) {
			return true
		}
	}
	return false
}

// privateIPv4 返回本机第一个内网 IPv4（渲染 DNS 重定向目标用）。
func privateIPv4(p *Probe) string {
	if p == nil {
		return ""
	}
	for _, it := range p.Ifaces {
		if it.Loopback || it.Virtual {
			continue
		}
		for _, a := range it.Addrs {
			ip := a
			if i := strings.LastIndex(a, "/"); i > 0 {
				ip = a[:i]
			}
			if isPrivateIP(ip) && !strings.Contains(ip, ":") {
				return ip
			}
		}
	}
	return ""
}

func isPrivateIP(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	// 10/8、172.16/12、192.168/16、127/8、169.254/16
	switch {
	case strings.HasPrefix(s, "10."):
		return true
	case strings.HasPrefix(s, "192.168."):
		return true
	case strings.HasPrefix(s, "127."):
		return true
	case strings.HasPrefix(s, "169.254."):
		return true
	}
	if strings.HasPrefix(s, "172.") {
		var second int
		if _, err := fmt.Sscanf(s, "172.%d.", &second); err == nil && second >= 16 && second <= 31 {
			return true
		}
	}
	return false
}

// RenderDaeGlobal 渲染写入 config.d/panel-access.dae 的 global 片段。
func RenderDaeGlobal(lanIfaces []string, tproxyPort int) string {
	if tproxyPort <= 0 {
		tproxyPort = DefaultTProxyPort
	}
	lines := []string{
		"# 由 HomeNet Panel 生成（panel-access）——请勿手改",
		"global {",
		"    tproxy_port: " + fmt.Sprintf("%d", tproxyPort),
		"    wan_interface: auto",
		"    auto_config_kernel_parameter: true",
	}
	if len(lanIfaces) > 0 {
		lines = append(lines, "    lan_interface: "+strings.Join(lanIfaces, ","))
	} else {
		lines = append(lines, "    # lan_interface 未确定：先只代理本机；确认 LAN 网卡后再填")
	}
	lines = append(lines, "}")
	return strings.Join(lines, "\n")
}

// RenderNftDNS 渲染 DNS 重定向表（只写自有表 panel_dns）。
func RenderNftDNS(proxyIP string, lanIfaces []string, dnsMode string) string {
	if dnsMode == "none" {
		return "# DNS 重定向已关闭：需要你自己保证 LAN 的 DNS 指向本机（否则域名规则失效）"
	}
	target := proxyIP
	if target == "" {
		target = "<旁路由IP>"
	}
	iif := strings.Join(lanIfaces, ", ")
	if iif == "" {
		iif = "<LAN网卡>"
	}
	lines := []string{
		"# 只是片段，不自动执行；按名删除即可完全卸载",
		"nft add table inet panel_dns",
		"nft 'add chain inet panel_dns prerouting { type nat hook prerouting priority dstnat; policy accept; }'",
		"nft add rule inet panel_dns prerouting iifname {" + iif + "} ip daddr " + target + " return",
		"nft add rule inet panel_dns prerouting iifname {" + iif + "} udp dport 53 dnat to " + target + ":53",
		"nft add rule inet panel_dns prerouting iifname {" + iif + "} tcp dport 53 dnat to " + target + ":53",
	}
	if dnsMode == "forward-from-router" {
		lines = append(lines, "# 说明：你选择由主路由转发 DNS 到本机，可只保留上面的 return 规则或整表不建")
	}
	return strings.Join(lines, "\n")
}

// RenderNftSNAT 渲染条件性 SNAT 表；auto 时默认不生成，仅给判断依据。
func RenderNftSNAT(snat string, lanIfaces []string) string {
	switch snat {
	case "on":
		lines := []string{
			"nft add table inet panel_snat",
			"nft 'add chain inet panel_snat postrouting { type nat hook postrouting priority srcnat; policy accept; }'",
			"nft add rule inet panel_snat postrouting ip saddr 192.168.0.0/16 oifname != \"lo\" masquerade",
		}
		return strings.Join(lines, "\n")
	case "off":
		return "# SNAT 已关闭：只在「未被劫持却被转发」的流量跨网段、且主路由开启 rp_filter/严格 zone 时才需要它"
	default:
		return "# 默认不生成（auto）：先不加 SNAT 跑自检；只有内网设备「能连上但打不开网页」（回程绕道）时才打开"
	}
}

// RenderNftTProxyPlaceholder 说明面板不重复安装拦截规则。
func RenderNftTProxyPlaceholder() string {
	lines := []string{
		"# panel_tproxy 默认不创建：dae 会为 lan_interface 自行安装拦截规则。",
		"# 如确需自定义放行例外，再手工建表，例如：",
		"# nft add table inet panel_tproxy",
		"# nft 'add chain inet panel_tproxy prerouting { type filter hook prerouting priority mangle; policy accept; }'",
		"# nft add rule inet panel_tproxy prerouting ip daddr 192.168.0.0/16 return",
	}
	return strings.Join(lines, "\n")
}

// manualStepsFor 生成"必须人工/路由器上做"的步骤。
func manualStepsFor(pl Plan, opt PlanOptions) []Step {
	dev := opt.TestDevice
	if dev == "" {
		dev = "一台测试设备（手机/笔记本）"
	}
	ip := pl.ProxyIP
	if ip == "" {
		ip = "<旁路由IP>"
	}
	switch pl.Mode {
	case ModeBypassManual:
		return []Step{
			{Title: "在" + dev + "上把网关改成旁路由", Detail: "让这台设备的默认路由指向旁路由；其他设备不受影响，随时可改回。", Snippet: "网关: " + ip},
			{Title: "把该设备的 DNS 也改成旁路由", Detail: "DNS 不过本机 → dae 拿不到域名 → 域名规则失效（自检第 5 项会报出来）。", Snippet: "DNS: " + ip},
			{Title: "验证并记录", Detail: "在该设备上访问一个境外站点与一个境内站点，确认都能通、延迟符合预期；记录改前的网关/DNS 便于回滚。"},
		}
	case ModeBypassRouterDHCP:
		return []Step{
			{Title: "先记录主路由 DHCP 的原值", Detail: "截图或抄下「网关 / DNS」两行原值，改错会整段断网，必须能一键改回。"},
			{Title: "把 DHCP 下发的网关改成旁路由", Detail: "在路由器后台的 DHCP/地址管理页，把「自定义网关地址/默认网关」填成旁路由 IP。", Snippet: "网关: " + ip},
			{Title: "把 DHCP 下发的 DNS 改成旁路由", Detail: "同页把 DNS 服务器改成旁路由 IP；备用 DNS 建议留空或填主路由。", Snippet: "DNS: " + ip},
			{Title: "让设备重新拿地址", Detail: "重连 WiFi 或等待续租，用 ipconfig/ifconfig 确认网关已是旁路由。"},
		}
	case ModeGateway:
		return []Step{
			{Title: "确认 DHCP 仍由路由器自己负责", Detail: "面板不接管 DHCP；网关档下 dae 与面板跑在同一台设备上。"},
			{Title: "把 LAN 侧网桥填入 lan_interface", Detail: "OpenWrt 通常是 br-lan；软路由可能是 br0/eth1。"},
		}
	case ModeBypassPBR:
		return []Step{
			{Title: "在主路由上配置策略路由", Detail: "面板只生成片段，不远程登录主路由；只影响被命中的设备/域名。", Snippet: RenderPBR(ip, opt)},
		}
	case ModeBypassSelfDHCP:
		return []Step{
			{Title: "（高风险，默认关闭）由面板托管 DHCP", Detail: "需要显式授权、二次确认，并在 30 分钟内未确认自动停止；本版本不提供该档的自动执行。"},
			{Title: "关闭主路由 DHCP 前，先确认面板 DHCP 已就绪", Detail: "顺序反了会导致全屋断网。"},
		}
	}
	return []Step{{Title: "观察模式不需要任何网络改动", Detail: "只读查看状态、订阅、规则与字节统计即可。"}}
}

// RenderPBR 渲染主路由侧片段（离线文本，面板不执行）。
func RenderPBR(proxyIP string, opt PlanOptions) string {
	if proxyIP == "" {
		proxyIP = "<旁路由IP>"
	}
	lines := []string{
		"# 在主路由（OpenWrt 等）上执行：把命中的设备/域名下一跳指向旁路由",
		"nft add set inet fw4 pbr_hop { type ipv4_addr; }",
		"nft add element inet fw4 pbr_hop { " + proxyIP + " }",
		"ip rule add fwmark 0x100 table 100",
		"ip route add default via " + proxyIP + " table 100",
		"# 具体匹配（按设备 / 按域名集合）由你在主路由侧决定；面板不远程执行任何命令",
	}
	return strings.Join(lines, "\n")
}

// rollbackStepsFor 生成回滚步骤。
func rollbackStepsFor(pl Plan) []Step {
	out := []Step{
		{Title: "删除面板创建的自有 nft 表", Detail: "按名删表即完全卸载，不改系统原有防火墙配置。", Snippet: "nft delete table inet panel_dns 2>/dev/null; nft delete table inet panel_snat 2>/dev/null; nft delete table inet panel_tproxy 2>/dev/null"},
	}
	switch pl.Mode {
	case ModeBypassManual:
		out = append(out, Step{Title: "把测试设备的网关/DNS 改回主路由（或设为自动获取）", Detail: "单台设备回滚，秒级生效。"})
	case ModeBypassRouterDHCP:
		out = append(out, Step{Title: "把主路由 DHCP 的网关/DNS 改回原值", Detail: "用之前记录的原值恢复；设备续租后恢复原路径。"})
		out = append(out, Step{Title: "若整段断网：直接在路由器后台恢复原值", Detail: "不要先停面板，先把 DHCP 恢复回原样。"})
	case ModeGateway:
		out = append(out, Step{Title: "停 dae / 面板，并删除 panel-access.dae 片段", Detail: "路由器自身的 DHCP/PPPoE/NAT 未被动过，因此恢复即回到原状。"})
	case ModeBypassPBR:
		out = append(out, Step{Title: "在主路由上删除 ip rule 与 nftset", Detail: "只影响被命中的设备/域名。"})
	}
	out = append(out, Step{Title: "面板侧：删除 config.d/panel-access.dae 并 dae reload", Detail: "面板会在 apply 阶段自动备份旧文件；这一步等同于「还原到接入前」。"})
	return out
}

// ChecksFor 返回该档位的自检清单（全部只读，可反复执行）。
func ChecksFor(m AccessMode) []Check {
	base := []Check{
		{Key: "dae_running", Name: "dae 正在运行", How: "读取服务状态与进程；失败则先修 dae，不要继续接管"},
		{Key: "lan_clients", Name: "字节统计里出现内网客户端", How: "有内网设备走本机 → clients 表非空（这是「LAN 流量真的来了」的硬证据）"},
		{Key: "dae0_counters", Name: "拦截侧网卡计数在增长", How: "读 /sys/class/net/<拦截网卡>/statistics；有流量即有东西被接管"},
		{Key: "dns_hits", Name: "DNS 命中记录在增长", How: "分流命中页能看到域名 → 说明 DNS 经过本机、域名规则可生效"},
		{Key: "config_untouched", Name: "系统原有配置未被修改", How: "OpenWrt 下比对 /etc/config 的校验和；面板只写自有片段与自有 nft 表"},
		{Key: "stale_tables", Name: "没有遗留的 panel_* 表/片段", How: "避免重复叠加规则；有遗留先执行回滚"},
	}
	if m == ModeObserve {
		return base[:1]
	}
	return base
}

// modeWarnings 返回与档位相关的风险提示。
func modeWarnings(m AccessMode) []string {
	switch m {
	case ModeBypassManual:
		return []string{"B1 只影响你手动改过的那台设备；其余设备仍走主路由"}
	case ModeBypassRouterDHCP:
		return []string{
			"B2 全屋生效：改 DHCP 网关/DNS 前必须先记录原值，改错会导致整段断网",
			"部分固件的「自定义网关」也会影响路由器自身出口，改完立刻验证",
		}
	case ModeBypassSelfDHCP:
		return []string{"B3 风险最高：本版本不提供自动执行；关闭主路由 DHCP 的顺序错了会全屋断网"}
	case ModeBypassPBR:
		return []string{"C 档需要主路由支持策略路由；面板只出片段，执行与回滚都在主路由侧"}
	case ModeGateway:
		return []string{"A 档要求 dae 与面板同机（OpenWrt/软路由）且内核支持 BPF/BTF"}
	}
	return nil
}

// fingerprintOf 计算计划指纹：apply 时用它校验"用户看到的就是要执行的"。
func fingerprintOf(pl *Plan) string {
	var b strings.Builder
	b.WriteString(string(pl.Mode))
	b.WriteString("|")
	b.WriteString(strings.Join(pl.LanIfaces, ","))
	b.WriteString("|")
	b.WriteString(fmt.Sprintf("%d", pl.TProxyPort))
	b.WriteString("|")
	b.WriteString(pl.DNSMode)
	b.WriteString("|")
	b.WriteString(pl.SNAT)
	b.WriteString("|")
	for _, s := range pl.Steps {
		b.WriteString(s.Title)
		b.WriteString(s.Snippet)
	}
	for _, s := range pl.ManualSteps {
		b.WriteString(s.Title)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:8])
}
