package mesh

import (
	"os/exec"
	"strings"
)

// Runner 抽象外部命令执行，便于测试注入。
type Runner interface {
	Run(name string, args ...string) (string, error)
}

type execRunner struct{}

func (execRunner) Run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

// Peer 是 easytier-cli peer 的一行。
type Peer struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
	Cost     string `json:"cost"`
	Latency  string `json:"latency"`
	Loss     string `json:"loss"`
	Rx       string `json:"rx"`
	Tx       string `json:"tx"`
	Tunnel   string `json:"tunnel"`
	NAT      string `json:"nat"`
	Version  string `json:"version"`
}

// Route 是 easytier-cli route 的一行。
type Route struct {
	IP          string `json:"ip"`
	Hostname    string `json:"hostname"`
	ProxyCIDRs  string `json:"proxyCidrs"`
	NextHop     string `json:"nextHop"`
	NextHopHost string `json:"nextHopHost"`
	Latency     string `json:"latency"`
	PathLen     string `json:"pathLen"`
	PathLatency string `json:"pathLatency"`
	Version     string `json:"version"`
}

// Status 是组网模块的整体状态。
type Status struct {
	Installed      bool    `json:"installed"`
	Running        bool    `json:"running"`
	Version        string  `json:"version"`
	BinPath        string  `json:"binPath"`
	ServiceManager string  `json:"serviceManager"`
	ServiceName    string  `json:"serviceName"`
	ConfigHint     string  `json:"configHint"`
	Peers          []Peer  `json:"peers"`
	Routes         []Route `json:"routes"`
	Error          string  `json:"error,omitempty"`
	Note           string  `json:"note,omitempty"`
}

// Options 控制探测行为。
type Options struct {
	Runner      Runner
	RootDir     string
	CLIPath     string
	ServiceName string
}

func (o Options) normalized() Options {
	if o.Runner == nil {
		o.Runner = execRunner{}
	}
	if o.RootDir == "" {
		o.RootDir = "/"
	}
	if o.CLIPath == "" {
		o.CLIPath = "easytier-cli"
	}
	if o.ServiceName == "" {
		o.ServiceName = "easytier"
	}
	return o
}

const (
	svcSystemd = "systemd"
	svcProcd   = "procd"
	svcUnknown = "unknown"
)

// ParseTable 解析 easytier-cli 的 Markdown 风格表格。
func ParseTable(out string) [][]string {
	var rows [][]string
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "|") {
			continue
		}
		cells := strings.Split(line, "|")
		var row []string
		empty := true
		dashes := true
		for _, c := range cells {
			c = strings.TrimSpace(c)
			if c != "" {
				empty = false
			}
			if c != "" && strings.Trim(c, "-") != "" {
				dashes = false
			}
			row = append(row, c)
		}
		if empty || dashes {
			continue
		}
		// 去掉首尾因 "|" 切分产生的空列
		if len(row) > 0 && row[0] == "" {
			row = row[1:]
		}
		if len(row) > 0 && row[len(row)-1] == "" {
			row = row[:len(row)-1]
		}
		rows = append(rows, row)
	}
	return rows
}

func cell(row []string, i int) string {
	if i < len(row) {
		return row[i]
	}
	return ""
}

// ParsePeers 解析 easytier-cli peer 输出（第一行是表头）。
func ParsePeers(out string) []Peer {
	rows := ParseTable(out)
	if len(rows) < 2 {
		return nil
	}
	peers := make([]Peer, 0, len(rows)-1)
	for _, r := range rows[1:] {
		peers = append(peers, Peer{
			IP:       cell(r, 0),
			Hostname: cell(r, 1),
			Cost:     cell(r, 2),
			Latency:  cell(r, 3),
			Loss:     cell(r, 4),
			Rx:       cell(r, 5),
			Tx:       cell(r, 6),
			Tunnel:   cell(r, 7),
			NAT:      cell(r, 8),
			Version:  cell(r, 9),
		})
	}
	return peers
}

// ParseRoutes 解析 easytier-cli route 输出。
func ParseRoutes(out string) []Route {
	rows := ParseTable(out)
	if len(rows) < 2 {
		return nil
	}
	routes := make([]Route, 0, len(rows)-1)
	for _, r := range rows[1:] {
		routes = append(routes, Route{
			IP:          cell(r, 0),
			Hostname:    cell(r, 1),
			ProxyCIDRs:  cell(r, 2),
			NextHop:     cell(r, 3),
			NextHopHost: cell(r, 4),
			Latency:     cell(r, 5),
			PathLen:     cell(r, 6),
			PathLatency: cell(r, 7),
			Version:     cell(r, len(r)-1),
		})
	}
	return routes
}

func detectServiceManager(o Options) string {
	if _, err := o.Runner.Run("test", "-f", o.RootDir+"etc/openwrt_release"); err == nil {
		return svcProcd
	}
	if _, err := o.Runner.Run("test", "-d", o.RootDir+"run/systemd/system"); err == nil {
		return svcSystemd
	}
	return svcUnknown
}

func serviceState(o Options, mgr string) bool {
	switch mgr {
	case svcSystemd:
		out, err := o.Runner.Run("systemctl", "is-active", o.ServiceName)
		return err == nil && strings.TrimSpace(out) == "active"
	case svcProcd:
		out, err := o.Runner.Run("/etc/init.d/"+o.ServiceName, "status")
		return err == nil && strings.TrimSpace(out) != ""
	}
	return false
}

func guessBin(o Options) string {
	if p, err := exec.LookPath("easytier-core"); err == nil {
		return p
	}
	return ""
}

// Detect 采集组网状态；任何命令失败都降级成提示而不返回错误。
func Detect(o Options) Status {
	o = o.normalized()
	st := Status{ServiceName: o.ServiceName, Peers: []Peer{}, Routes: []Route{}}
	st.ServiceManager = detectServiceManager(o)

	if out, err := o.Runner.Run("easytier-core", "--version"); err == nil {
		st.Installed = true
		st.Version = strings.TrimSpace(out)
	}
	if !st.Installed {
		st.ConfigHint = "未检测到 easytier-core：可执行文件不在 PATH 里。装法见 docs/MESH.md 或让安装脚本处理。"
		return st
	}
	st.BinPath = guessBin(o)
	st.Running = serviceState(o, st.ServiceManager)
	if !st.Running {
		st.ConfigHint = "EasyTier 已安装但服务未运行（服务名 " + o.ServiceName + "）。"
		return st
	}

	if out, err := o.Runner.Run(o.CLIPath, "peer"); err == nil {
		st.Peers = ParsePeers(out)
	} else {
		st.Error = firstLine(out, err)
	}
	if out, err := o.Runner.Run(o.CLIPath, "route"); err == nil {
		st.Routes = ParseRoutes(out)
	}
	if len(st.Peers) <= 1 {
		st.Note = "当前只有本机节点：对端未连上，或对端尚未加入同一网络名。"
	}
	return st
}

// Control 启停组网服务，返回给用户看的说明。
func Control(o Options, action string) (string, error) {
	o = o.normalized()
	mgr := detectServiceManager(o)
	switch mgr {
	case svcSystemd:
		if _, err := o.Runner.Run("systemctl", action, o.ServiceName); err != nil {
			return "", err
		}
		return "systemctl " + action + " " + o.ServiceName + " 已执行", nil
	case svcProcd:
		verb := action
		if action == "restart" {
			verb = "restart"
		}
		if _, err := o.Runner.Run("/etc/init.d/"+o.ServiceName, verb); err != nil {
			return "", err
		}
		return "/etc/init.d/" + o.ServiceName + " " + verb + " 已执行", nil
	}
	return "", exec.ErrNotFound
}

func firstLine(out string, err error) string {
	text := strings.TrimSpace(out)
	if text != "" {
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[:i]
		}
		return text
	}
	if err != nil {
		return err.Error()
	}
	return ""
}
