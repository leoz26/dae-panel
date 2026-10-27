package mesh

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

const nlChar = string(rune(10))

// LanStatus 描述「局域网互通」的现状。
type LanStatus struct {
	MeshIface      string   `json:"meshIface"`
	MeshAddrs      []string `json:"meshAddrs"`
	LanIface       string   `json:"lanIface"`
	LanCIDRs       []string `json:"lanCidrs"`
	PeerCIDRs      []string `json:"peerCidrs"`
	Enabled        bool     `json:"enabled"`
	NftTable       bool     `json:"nftTable"`
	Rules          []string `json:"rules"`
	ServiceName    string   `json:"serviceName"`
	ServiceEnabled bool     `json:"serviceEnabled"`
	ScriptPath     string   `json:"scriptPath"`
	ScriptManaged  bool     `json:"scriptManaged"`
	Note           string   `json:"note,omitempty"`
	Error          string   `json:"error,omitempty"`
}

// LanConfig 是启用互通所需的参数。
type LanConfig struct {
	LanCIDRs  []string `json:"lanCidrs"`
	MeshIface string   `json:"meshIface"`
}

// Step 是执行过程中的一步。
type Step struct {
	Title   string `json:"title"`
	Command string `json:"command"`
	Output  string `json:"output,omitempty"`
	OK      bool   `json:"ok"`
	DryRun  bool   `json:"dryRun"`
}

// LanResult 是启用/停用的结果。
type LanResult struct {
	Steps  []Step `json:"steps"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	DryRun bool   `json:"dryRun"`
}

const (
	meshNatTable   = "hnp_mesh"
	meshNatService = "hnp-mesh-nat"
	meshNatScript  = "/usr/local/sbin/hnp-mesh-nat.sh"
	meshNatUnit    = "/etc/systemd/system/hnp-mesh-nat.service"
)

func (o Options) lanRunner() Runner {
	if o.Runner != nil {
		return o.Runner
	}
	return execRunner{}
}

func splitLines(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == 10 })
}

// meshAddrs 返回 tun/wg 类接口上的 IPv4 地址。
func meshAddrs(r Runner) (string, []string) {
	out, err := r.Run("ip", "-4", "-o", "addr", "show")
	if err != nil {
		return "", nil
	}
	var iface string
	var addrs []string
	for _, line := range splitLines(out) {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		name := strings.TrimSuffix(f[1], ":")
		if !strings.HasPrefix(name, "tun") && !strings.HasPrefix(name, "wt") && !strings.HasPrefix(name, "wg") {
			continue
		}
		cidr := f[3]
		if !strings.Contains(cidr, ".") {
			continue
		}
		if iface == "" {
			iface = name
		}
		addrs = append(addrs, cidr)
	}
	return iface, addrs
}

func skipIface(name string) bool {
	if name == "lo" {
		return true
	}
	for _, p := range []string{"tun", "wt", "wg", "dae", "docker", "br-", "veth", "virbr"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// lanIfaces 返回对外网卡与它的私有网段。
func lanIfaces(r Runner) (string, []string) {
	out, err := r.Run("ip", "-4", "-o", "addr", "show")
	if err != nil {
		return "", nil
	}
	def := defaultIface(r)
	primary := ""
	var cidrs []string
	for _, line := range splitLines(out) {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		name := strings.TrimSuffix(f[1], ":")
		if skipIface(name) {
			continue
		}
		cidr := f[3]
		if !isPrivateV4(strings.SplitN(cidr, "/", 2)[0]) {
			continue
		}
		if primary == "" {
			primary = name
		}
		if def != "" && name == def {
			primary = name
			cidrs = nil
		}
		if name == primary {
			cidrs = append(cidrs, normalizeCIDR(cidr))
		}
	}
	return primary, cidrs
}

func defaultIface(r Runner) string {
	out, err := r.Run("ip", "-4", "route", "show", "default")
	if err != nil {
		return ""
	}
	f := strings.Fields(out)
	for i := 0; i < len(f)-1; i++ {
		if f[i] == "dev" {
			return f[i+1]
		}
	}
	return ""
}

func isPrivateV4(ip string) bool {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return false
	}
	a := atoi(parts[0])
	b := atoi(parts[1])
	switch {
	case a == 10:
		return true
	case a == 172 && b >= 16 && b <= 31:
		return true
	case a == 192 && b == 168:
		return true
	case a == 100 && b >= 64 && b <= 127:
		return true
	}
	return false
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func normalizeCIDR(cidr string) string {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return cidr
	}
	return ipnet.String()
}

// PeerCIDRs 收集对端发布的网段（来自 easytier-cli route 的 proxy_cidrs 列）。
func PeerCIDRs(o Options) []string {
	r := o.lanRunner()
	out, err := r.Run(o.CLIPath, "route")
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var res []string
	for _, row := range ParseRoutes(out) {
		if row.ProxyCIDRs == "" || row.ProxyCIDRs == "-" {
			continue
		}
		for _, c := range strings.Split(row.ProxyCIDRs, ",") {
			c = strings.TrimSpace(c)
			if c == "" || seen[c] {
				continue
			}
			seen[c] = true
			res = append(res, c)
		}
	}
	return res
}

// DetectLan 读取当前互通状态，只读不写。
func DetectLan(o Options) LanStatus {
	o = o.normalized()
	r := o.lanRunner()
	st := LanStatus{ServiceName: meshNatService, ScriptPath: meshNatScript}
	st.MeshIface, st.MeshAddrs = meshAddrs(r)
	st.LanIface, st.LanCIDRs = lanIfaces(r)
	st.PeerCIDRs = PeerCIDRs(o)
	if out, err := r.Run("nft", "list", "table", "ip", meshNatTable); err == nil {
		st.NftTable = true
		for _, line := range splitLines(out) {
			t := strings.TrimSpace(line)
			if strings.Contains(t, "masquerade") {
				st.Rules = append(st.Rules, t)
				st.Enabled = true
			}
		}
	}
	if _, err := os.Stat(filepath.Join(o.RootDir, strings.TrimPrefix(meshNatScript, "/"))); err == nil {
		st.ScriptManaged = true
	}
	if out, err := r.Run("systemctl", "is-enabled", meshNatService); err == nil {
		v := firstLine(out, err)
		if v == "enabled" || v == "enabled-runtime" {
			st.ServiceEnabled = true
		}
	}
	switch {
	case st.MeshIface == "":
		st.Note = "没有发现 tun/wg 类接口：EasyTier 可能没在运行，先确认上方组网状态"
	case len(st.PeerCIDRs) == 0:
		st.Note = "对端还没有发布网段：需要在对端 EasyTier 上加 -n <它的局域网网段>"
	case !st.NftTable:
		st.Note = "未启用局域网互通：启用后只对发往隧道（" + st.MeshIface + "）的流量做 NAT，不影响上网"
	case !st.ServiceEnabled:
		st.Note = "规则已在运行，但没有开机重建：重启后需重新启用"
	}
	return st
}

// RenderNatScript 生成幂等的 NAT 脚本。
func RenderNatScript(cfg LanConfig) string {
	iface := cfg.MeshIface
	if iface == "" {
		iface = "tun0"
	}
	ls := []string{
		"#!/bin/sh",
		"# 由 HomeNet Panel 生成：让本网段设备经 EasyTier 访问对端局域网",
		"# 只维护自有 nft 表 " + meshNatTable + "，不碰其他表；删表即完全卸载",
		"set -e",
		"nft list table ip " + meshNatTable + " >/dev/null 2>&1 || nft add table ip " + meshNatTable,
		"nft list chain ip " + meshNatTable + " post >/dev/null 2>&1 || nft add chain ip " + meshNatTable + ` post '{ type nat hook postrouting priority srcnat; policy accept; }'`,
	}
	for _, c := range cfg.LanCIDRs {
		ls = append(ls, fmt.Sprintf(`nft list chain ip %s post | grep -q 'ip saddr %s masquerade' || nft add rule ip %s post oifname "%s" ip saddr %s masquerade`, meshNatTable, c, meshNatTable, iface, c))
	}
	ls = append(ls, "exit 0")
	return strings.Join(ls, nlChar) + nlChar
}

// RenderNatUnit 生成 systemd oneshot 单元。
func RenderNatUnit() string {
	ls := []string{
		"[Unit]",
		"Description=HomeNet Panel - NAT for EasyTier mesh LAN access",
		"After=network-online.target easytier.service",
		"Wants=network-online.target",
		"",
		"[Service]",
		"Type=oneshot",
		"RemainAfterExit=yes",
		"ExecStart=" + meshNatScript,
		"",
		"[Install]",
		"WantedBy=multi-user.target",
	}
	return strings.Join(ls, nlChar) + nlChar
}

// ApplyLan 写脚本与单元、立即建规则并启用开机重建。
func ApplyLan(o Options, cfg LanConfig, dryRun bool) LanResult {
	o = o.normalized()
	r := o.lanRunner()
	res := LanResult{DryRun: dryRun}
	add := func(title, cmd, out string, err error) {
		st := Step{Title: title, Command: cmd, Output: strings.TrimSpace(out), OK: err == nil, DryRun: dryRun}
		if err != nil && st.Output == "" {
			st.Output = err.Error()
		}
		res.Steps = append(res.Steps, st)
		if err != nil && res.Error == "" {
			res.Error = title + "失败: " + err.Error()
		}
	}
	if len(cfg.LanCIDRs) == 0 {
		res.Error = "没有可用的局域网网段（探测不到带私网地址的网卡）"
		return res
	}
	if cfg.MeshIface == "" {
		if iface, _ := meshAddrs(r); iface != "" {
			cfg.MeshIface = iface
		} else {
			res.Error = "没有发现隧道接口（EasyTier 未运行）"
			return res
		}
	}
	script := RenderNatScript(cfg)
	unit := RenderNatUnit()
	if dryRun {
		add("将写入 NAT 脚本", meshNatScript, "", nil)
		for _, line := range splitLines(script) {
			if strings.Contains(line, "masquerade") {
				add("将执行的规则", line, "", nil)
			}
		}
		add("将写入 systemd 单元", meshNatUnit, "", nil)
		add("将启用开机重建", "systemctl enable --now "+meshNatService, "", nil)
		res.OK = true
		return res
	}
	if err := writeHostFile(o, meshNatScript, script, 0o755); err != nil {
		res.Error = "写脚本失败: " + err.Error()
		return res
	}
	add("写入 NAT 脚本", meshNatScript, "", nil)
	out, err := r.Run("sh", meshNatScript)
	add("立即建立 NAT 规则", "sh "+meshNatScript, out, err)
	if err != nil {
		return res
	}
	if err := writeHostFile(o, meshNatUnit, unit, 0o644); err != nil {
		res.Error = "写 systemd 单元失败: " + err.Error()
		return res
	}
	add("写入 systemd 单元", meshNatUnit, "", nil)
	out, err = r.Run("systemctl", "daemon-reload")
	add("systemctl daemon-reload", "systemctl daemon-reload", out, err)
	out, err = r.Run("systemctl", "enable", "--now", meshNatService)
	add("启用开机重建", "systemctl enable --now "+meshNatService, out, err)
	res.OK = res.Error == ""
	return res
}

// RemoveLan 停用服务并删除自有 nft 表。
func RemoveLan(o Options, dryRun bool) LanResult {
	o = o.normalized()
	r := o.lanRunner()
	res := LanResult{DryRun: dryRun}
	add := func(title, cmd, out string, err error) {
		st := Step{Title: title, Command: cmd, Output: strings.TrimSpace(out), OK: err == nil, DryRun: dryRun}
		if err != nil && st.Output == "" {
			st.Output = err.Error()
		}
		res.Steps = append(res.Steps, st)
		if err != nil && res.Error == "" {
			res.Error = title + "失败: " + err.Error()
		}
	}
	if dryRun {
		add("将停用服务", "systemctl disable --now "+meshNatService, "", nil)
		add("将删除 nft 表", "nft delete table ip "+meshNatTable, "", nil)
		res.OK = true
		return res
	}
	out, err := r.Run("systemctl", "disable", "--now", meshNatService)
	add("停用服务", "systemctl disable --now "+meshNatService, out, err)
	out, err = r.Run("nft", "delete", "table", "ip", meshNatTable)
	add("删除 nft 表", "nft delete table ip "+meshNatTable, out, err)
	res.OK = res.Error == ""
	return res
}

func writeHostFile(o Options, path, content string, mode os.FileMode) error {
	full := filepath.Join(o.RootDir, strings.TrimPrefix(path, "/"))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	tmp := full + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), mode); err != nil {
		return err
	}
	return os.Rename(tmp, full)
}
