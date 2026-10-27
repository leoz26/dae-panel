// Package access 负责"把流量送到 dae 面前"这一层的探测、选档与计划生成。
//
// 设计约束（见 architecture.md §4.4.9）：
//   - 不写死网络参数、服务管理器、厂商：一律探测；
//   - 探测失败 fail-open 到 observe 并说明原因，绝不因探测失败而中断；
//   - 只读：本包不修改任何系统状态。
package access

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Runner 抽象命令执行，便于单元测试注入假实现。
type Runner interface {
	Run(name string, args ...string) (string, error)
}

type execRunner struct{}

func (execRunner) Run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}

// Iface 是探测到的网卡快照。
type Iface struct {
	Name     string   `json:"name"`
	Addrs    []string `json:"addrs,omitempty"`
	MAC      string   `json:"mac,omitempty"`
	Up       bool     `json:"up"`
	Loopback bool     `json:"loopback"`
	Bridge   bool     `json:"bridge"`
	Virtual  bool     `json:"virtual"`
}

// Question 是向导要向用户确认的问题（与厂商无关）。
type Question struct {
	Key  string `json:"key"`
	Text string `json:"text"`
	Why  string `json:"why"`
}

// Rec 是选档推荐结果，永远附带理由。
type Rec struct {
	Mode      AccessMode `json:"mode"`
	Reasons   []string   `json:"reasons"`
	Blockers  []string   `json:"blockers,omitempty"`
	Questions []Question `json:"questions"`
}

// Probe 是本机接入能力的只读快照。
type Probe struct {
	Platform       string            `json:"platform"` // openwrt | linux | darwin | other
	OSName         string            `json:"osName,omitempty"`
	Arch           string            `json:"arch"`
	Kernel         string            `json:"kernel,omitempty"`
	ServiceManager string            `json:"serviceManager"` // systemd | procd | other | none
	HasBPF         bool              `json:"hasBpf"`
	HasBTF         bool              `json:"hasBtf"`
	MemlockKB      int               `json:"memlockKB,omitempty"`
	Nft            bool              `json:"nft"`
	DefaultGw      string            `json:"defaultGw,omitempty"`
	DefaultIface   string            `json:"defaultIface,omitempty"`
	IPForward      bool              `json:"ipForward"`
	Ifaces         []Iface           `json:"ifaces"`
	BusyPorts      map[string]string `json:"busyPorts,omitempty"`
	DaeInstalled   bool              `json:"daeInstalled"`
	DaeVersion     string            `json:"daeVersion,omitempty"`
	DaeRunning     bool              `json:"daeRunning"`
	DaeLanIfaces   []string          `json:"daeLanIfaces,omitempty"`
	StaleTables    []string          `json:"staleTables,omitempty"` // 面板遗留的 panel_* 表
	Notes          []string          `json:"notes,omitempty"`
	Rec            Rec               `json:"rec"`
}

// Collect 采集只读探测快照并给出默认推荐。rootDir 一般传 "/"，便于测试。
func Collect(r Runner, rootDir, daeConfigPath string) *Probe {
	if r == nil {
		r = execRunner{}
	}
	if rootDir == "" {
		rootDir = "/"
	}
	p := &Probe{
		Platform:  detectPlatform(rootDir),
		Arch:      runtime.GOARCH,
		Ifaces:    []Iface{},
		BusyPorts: map[string]string{},
	}
	p.OSName = osName(rootDir)
	if v, err := r.Run("uname", "-r"); err == nil {
		p.Kernel = strings.TrimSpace(v)
	}
	p.ServiceManager = detectServiceManager(rootDir)

	if _, err := os.Stat(filepath.Join(rootDir, "sys/kernel/btf/vmlinux")); err == nil {
		p.HasBTF = true
	}
	// BPF 可用性的启发式判断：有 bpffs 挂载点或内核暴露 unprivileged_bpf_disabled
	if _, err := os.Stat(filepath.Join(rootDir, "sys/fs/bpf")); err == nil {
		p.HasBPF = true
	}
	if _, err := os.Stat(filepath.Join(rootDir, "proc/sys/kernel/unprivileged_bpf_disabled")); err == nil {
		p.HasBPF = true
	}
	p.MemlockKB = memlockKB(filepath.Join(rootDir, "proc/self/limits"))

	// 片段存在时，自有 nft 表是「接入已生效」的产物，不算遗留。
	accessFile := filepath.Join(filepath.Dir(daeConfigPath), "config.d", "panel-access.dae")
	_, accessErr := os.Stat(accessFile)
	accessEnabled := accessErr == nil

	if _, err := r.Run("nft", "list", "tables"); err == nil {
		p.Nft = true
		if out, err := r.Run("nft", "list", "tables"); err == nil {
			for _, ln := range strings.Split(out, "\n") {
				f := strings.Fields(ln)
				if len(f) >= 3 && f[0] == "table" {
					name := f[2]
					if strings.HasPrefix(name, "panel_") && !accessEnabled {
						p.StaleTables = append(p.StaleTables, ln)
					}
				}
			}
		}
	}

	if out, err := r.Run("ip", "-4", "route", "show", "default"); err == nil {
		p.DefaultGw, p.DefaultIface = parseDefaultRoute(out)
	}
	if out, err := r.Run("ip", "-o", "addr"); err == nil {
		p.Ifaces = collectIfaces(rootDir, out)
	}
	if p.DefaultIface != "" {
		for i := range p.Ifaces {
			if p.Ifaces[i].Name == p.DefaultIface {
				p.Notes = append(p.Notes, "默认出口网卡："+p.DefaultIface)
			}
		}
	}

	if b, err := os.ReadFile(filepath.Join(rootDir, "proc/sys/net/ipv4/ip_forward")); err == nil {
		p.IPForward = strings.TrimSpace(string(b)) == "1"
	}

	p.BusyPorts = busyPorts(rootDir, []string{"53", "67", "68", "12345"})

	// dae 是否存在/在跑/版本（不同版本子命令不同：v2.1.1 只支持 --version）
	p.DaeInstalled, p.DaeVersion = detectDae(r)
	if p.DaeInstalled && daeProcessRunning(r) {
		p.DaeRunning = true
	}
	p.DaeLanIfaces = daeLanIfaces(daeConfigPath)

	if p.Platform != "linux" && p.Platform != "openwrt" {
		p.Notes = append(p.Notes, "当前平台不是 Linux/OpenWrt，只能用 observe 档；接入层需在旁路由/网关上运行")
	}

	p.Rec = Recommend(p, Answers{})
	return p
}

func detectPlatform(root string) string {
	if _, err := os.Stat(filepath.Join(root, "etc/openwrt_release")); err == nil {
		return "openwrt"
	}
	// 有 procfs 即视为 Linux（与宿主 GOOS 无关，便于在 macOS 上对假根目录做测试）
	if _, err := os.Stat(filepath.Join(root, "proc/sys")); err == nil {
		return "linux"
	}
	if runtime.GOOS == "linux" {
		return "linux"
	}
	if runtime.GOOS == "darwin" {
		return "darwin"
	}
	return "other"
}

func osName(root string) string {
	for _, f := range []string{"etc/os-release", "usr/lib/os-release"} {
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			continue
		}
		for _, ln := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(ln, "PRETTY_NAME=") {
				return strings.Trim(strings.TrimPrefix(ln, "PRETTY_NAME="), "\"")
			}
		}
	}
	return ""
}

func detectServiceManager(root string) string {
	if _, err := os.Stat(filepath.Join(root, "etc/openwrt_release")); err == nil {
		if _, err := os.Stat(filepath.Join(root, "sbin/procd")); err == nil {
			return "procd"
		}
		return "procd"
	}
	if _, err := os.Stat(filepath.Join(root, "run/systemd/system")); err == nil {
		return "systemd"
	}
	return "other"
}

func memlockKB(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		ln := sc.Text()
		if !strings.HasPrefix(ln, "Max locked memory") {
			continue
		}
		f := strings.Fields(ln)
		if len(f) < 5 {
			return 0
		}
		if f[4] == "unlimited" {
			return -1
		}
		n, err := strconv.Atoi(f[4])
		if err != nil {
			return 0
		}
		return n / 1024
	}
	return 0
}

// parseDefaultRoute 解析 "ip -4 route show default" 输出。
func parseDefaultRoute(out string) (gw, iface string) {
	for _, ln := range strings.Split(out, "\n") {
		f := strings.Fields(ln)
		for i := 0; i < len(f)-1; i++ {
			switch f[i] {
			case "via":
				if gw == "" {
					gw = f[i+1]
				}
			case "dev":
				if iface == "" {
					iface = f[i+1]
				}
			}
		}
		if gw != "" || iface != "" {
			return gw, iface
		}
	}
	return gw, iface
}

// collectIfaces 解析 "ip -o addr" 输出并补充 /sys/class/net 元信息。
func collectIfaces(root, out string) []Iface {
	byName := map[string]*Iface{}
	order := []string{}
	for _, ln := range strings.Split(out, "\n") {
		f := strings.Fields(ln)
		if len(f) < 4 {
			continue
		}
		// 形如: 2: ens2    inet 192.168.1.85/24 brd ... scope global ens2
		idx := strings.Index(ln, ": ")
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(ln[idx+2:])
		if sp := strings.IndexAny(name, " 	"); sp >= 0 {
			name = name[:sp]
		}
		if name == "" {
			continue
		}
		it, ok := byName[name]
		if !ok {
			it = &Iface{Name: name}
			byName[name] = it
			order = append(order, name)
		}
		for i := 0; i < len(f)-1; i++ {
			if f[i] == "inet" || f[i] == "inet6" {
				it.Addrs = append(it.Addrs, f[i+1])
				break
			}
		}
	}
	res := make([]Iface, 0, len(order))
	for _, n := range order {
		it := byName[n]
		sys := filepath.Join(root, "sys/class/net", n)
		if b, err := os.ReadFile(filepath.Join(sys, "address")); err == nil {
			it.MAC = strings.TrimSpace(string(b))
		}
		if b, err := os.ReadFile(filepath.Join(sys, "operstate")); err == nil {
			it.Up = strings.TrimSpace(string(b)) == "up"
		}
		if _, err := os.Stat(filepath.Join(sys, "bridge")); err == nil {
			it.Bridge = true
		}
		if _, err := os.Lstat(filepath.Join(sys, "device")); err != nil {
			it.Virtual = true
		}
		it.Loopback = n == "lo"
		res = append(res, *it)
	}
	return res
}

// busyPorts 读 /proc/net/{udp,tcp} 判断关注端口是否已被占用。
func busyPorts(root string, ports []string) map[string]string {
	want := map[string]bool{}
	for _, p := range ports {
		want[p] = true
	}
	res := map[string]string{}
	for _, f := range []string{"proc/net/udp", "proc/net/tcp", "proc/net/udp6", "proc/net/tcp6"} {
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			continue
		}
		for _, ln := range strings.Split(string(b), "\n")[1:] {
			cols := strings.Fields(ln)
			if len(cols) < 2 {
				continue
			}
			local := cols[1]
			i := strings.LastIndex(local, ":")
			if i < 0 {
				continue
			}
			n, err := strconv.ParseInt(local[i+1:], 16, 32)
			if err != nil {
				continue
			}
			port := strconv.Itoa(int(n))
			if want[port] && res[port] == "" {
				res[port] = strings.TrimPrefix(f, "proc/net/")
			}
		}
	}
	return res
}

// detectDae 探测 dae 是否安装及版本，兼容不同版本命令行差异。
func detectDae(r Runner) (bool, string) {
	for _, args := range [][]string{{"--version"}, {"version"}, {"-v"}} {
		if out, err := r.Run("dae", args...); err == nil && strings.TrimSpace(out) != "" {
			return true, firstLine(out)
		}
	}
	if _, err := exec.LookPath("dae"); err == nil {
		return true, ""
	}
	return false, ""
}

// daeConfigFiles 返回入口配置 + 其 include 展开后的文件清单（只读，最多两跳）。
func daeConfigFiles(entry string) []string {
	if entry == "" {
		return nil
	}
	if _, err := os.Stat(entry); err != nil {
		return nil
	}
	files := []string{entry}
	dir := filepath.Dir(entry)
	b, err := os.ReadFile(entry)
	if err != nil {
		return files
	}
	inInclude := false
	for _, ln := range strings.Split(string(b), "\n") {
		s := strings.TrimSpace(ln)
		if strings.HasPrefix(s, "include") && strings.HasSuffix(s, "{") {
			inInclude = true
			continue
		}
		if inInclude {
			if s == "}" {
				inInclude = false
				continue
			}
			pat := strings.Trim(strings.TrimSpace(s), "'\"")
			if pat == "" {
				continue
			}
			if !filepath.IsAbs(pat) {
				pat = filepath.Join(dir, pat)
			}
			matches, err := filepath.Glob(pat)
			if err != nil || len(matches) == 0 {
				if _, err := os.Stat(pat); err == nil {
					matches = []string{pat}
				}
			}
			for _, m := range matches {
				if m != entry {
					files = append(files, m)
				}
			}
		}
	}
	return files
}

// daeLanIfaces 从 dae 配置（含 include 展开）中读取 lan_interface 声明。
// daeProcessRunning 判断 dae 是否在跑：pgrep -x 在 busybox/OpenWrt 上不可靠，
// 退化到 dae 的 pid 文件 + kill -0（与 bootstrap 中的同名判断口径一致）。
func daeProcessRunning(r Runner) bool {
	if out, err := r.Run("pgrep", "-x", "dae"); err == nil && strings.TrimSpace(out) != "" {
		return true
	}
	if out, err := r.Run("cat", "/var/run/dae.pid"); err == nil {
		if pid := strings.TrimSpace(out); pid != "" {
			if _, e := r.Run("kill", "-0", pid); e == nil {
				return true
			}
		}
	}
	return false
}

func daeLanIfaces(configPath string) []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range daeConfigFiles(configPath) {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		out = append(out, scanLanInterface(string(b), seen)...)
	}
	return out
}

func scanLanInterface(content string, seen map[string]bool) []string {
	var out []string
	for _, ln := range strings.Split(content, "\n") {
		s := strings.TrimSpace(ln)
		if !strings.HasPrefix(s, "lan_interface") {
			continue
		}
		if i := strings.Index(s, ":"); i >= 0 {
			s = s[i+1:]
		}
		for _, part := range strings.Split(s, ",") {
			part = strings.Trim(strings.TrimSpace(part), "'\"")
			if part == "" || seen[part] {
				continue
			}
			seen[part] = true
			out = append(out, part)
		}
	}
	return out
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
