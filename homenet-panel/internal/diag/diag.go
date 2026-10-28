package diag

import (
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Runner 抽象命令执行，便于测试注入。
type Runner interface {
	Run(name string, args ...string) (string, error)
}

type execRunner struct{}

func (execRunner) Run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return string(out), err
}

// 事件类型
const (
	KindNotAlive       = "not_alive"
	KindProxyIPFailure = "proxy_ip_failure"
	KindReselect       = "reselect"
	KindSelect         = "select"
	KindBootstrap      = "bootstrap"
	KindIOTimeout      = "io_timeout"
	KindServfail       = "servfail"
	KindNoDialer       = "no_dialer"
)

var kindNames = map[string]string{
	KindNotAlive:       "节点被判定不可用（NOT ALIVE）",
	KindProxyIPFailure: "代理 IP 连续失败被强制判死",
	KindReselect:       "分组重新选点",
	KindSelect:         "分组选点",
	KindBootstrap:      "节点域名 bootstrap 解析失败",
	KindIOTimeout:      "拨号/连接超时",
	KindServfail:       "DNS 返回 SERVFAIL",
	KindNoDialer:       "分组内无可用节点（致命）",
}

// KindName 返回中文名。
func KindName(k string) string {
	if n, ok := kindNames[k]; ok {
		return n
	}
	return k
}

type NodeStat struct {
	Name     string    `json:"name"`
	NotAlive int       `json:"notAlive"`
	ProxyIP  int       `json:"proxyIPFailure"`
	LastSeen time.Time `json:"lastSeen"`
}

type GroupStat struct {
	Name         string `json:"name"`
	Selects      int    `json:"selects"`
	Reselects    int    `json:"reselects"`
	NotAlive     int    `json:"notAlive"`
	AliveDialers int    `json:"aliveDialers"`
}

// Bucket 是一分钟内的分类计数。
type Bucket struct {
	TS     time.Time      `json:"ts"`
	Counts map[string]int `json:"counts"`
}

type Report struct {
	WindowMinutes int            `json:"windowMinutes"`
	Source        string         `json:"source"`
	TotalLines    int            `json:"totalLines"`
	Events        int            `json:"events"`
	Counts        map[string]int `json:"counts"`
	Nodes         []NodeStat     `json:"nodes"`
	Groups        []GroupStat    `json:"groups"`
	Series        []Bucket       `json:"series"`
	Advice        []string       `json:"advice"`
	Error         string         `json:"error,omitempty"`
}

func fieldAfter(s, key string) string {
	i := strings.Index(s, key)
	if i < 0 {
		return ""
	}
	rest := s[i+len(key):]
	if j := strings.IndexAny(rest, " 	"); j >= 0 {
		return rest[:j]
	}
	return rest
}

func between(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	rest := s[i+len(start):]
	j := strings.Index(rest, end)
	if j < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:j])
}

func atoiOr(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// splitTS 拆出时间戳与正文；无法解析时间戳时 ts 为零值。
func splitTS(line string) (time.Time, string) {
	i := strings.IndexByte(line, ' ')
	if i <= 0 {
		return time.Time{}, line
	}
	head := line[:i]
	if len(head) >= 20 && head[4] == '-' && head[10] == 'T' {
		if ts, err := time.Parse("2006-01-02T15:04:05-07:00", head); err == nil {
			return ts, line[i+1:]
		}
		if ts, err := time.Parse("2006-01-02T15:04:05Z07:00", head); err == nil {
			return ts, line[i+1:]
		}
	}
	return time.Time{}, line
}

// Classify 判定一行日志属于哪类事件；未知返回空串。
func Classify(msg string) string {
	switch {
	case strings.Contains(msg, "no dialer in this group"):
		return KindNoDialer
	case strings.Contains(msg, "persistent proxy IP failures"):
		return KindProxyIPFailure
	case strings.Contains(msg, "NOT ALIVE:"):
		return KindNotAlive
	case strings.Contains(msg, "Group re-selects dialer"):
		return KindReselect
	case strings.Contains(msg, "Group selects dialer"):
		return KindSelect
	case strings.Contains(msg, "bootstrap resolver returned no usable address"):
		return KindBootstrap
	case strings.Contains(msg, "i/o timeout"):
		return KindIOTimeout
	case strings.Contains(msg, "SERVFAIL"):
		return KindServfail
	}
	return ""
}

// Parse 把日志行聚合为报告。now 只用于生成建议文案。
func Parse(lines []string, windowMinutes int, source string, now time.Time) Report {
	rep := Report{
		WindowMinutes: windowMinutes,
		Source:        source,
		Counts:        map[string]int{},
		Nodes:         []NodeStat{},
		Groups:        []GroupStat{},
		Series:        []Bucket{},
		Advice:        []string{},
	}
	rep.TotalLines = len(lines)
	nodeIdx := map[string]*NodeStat{}
	groupIdx := map[string]*GroupStat{}
	bucketIdx := map[time.Time]*Bucket{}
	ioTargets := map[string]int{}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		kind := Classify(line)
		if kind == "" {
			continue
		}
		ts, msg := splitTS(line)
		rep.Counts[kind]++
		rep.Events++

		if !ts.IsZero() {
			b := ts.Truncate(time.Minute)
			bu := bucketIdx[b]
			if bu == nil {
				bu = &Bucket{TS: b, Counts: map[string]int{}}
				bucketIdx[b] = bu
			}
			bu.Counts[kind]++
		}

		switch kind {
		case KindNotAlive:
			name := between(msg, "dialer=", " group=")
			grp := fieldAfter(msg, "group=")
			if name != "" {
				st := nodeIdx[name]
				if st == nil {
					st = &NodeStat{Name: name}
					nodeIdx[name] = st
				}
				st.NotAlive++
				if ts.After(st.LastSeen) {
					st.LastSeen = ts
				}
			}
			if grp != "" {
				g := groupIdx[grp]
				if g == nil {
					g = &GroupStat{Name: grp}
					groupIdx[grp] = g
				}
				g.NotAlive++
			}
		case KindProxyIPFailure:
			name := between(msg, "dialer=", " network=")
			if name != "" {
				st := nodeIdx[name]
				if st == nil {
					st = &NodeStat{Name: name}
					nodeIdx[name] = st
				}
				st.ProxyIP++
				if ts.After(st.LastSeen) {
					st.LastSeen = ts
				}
			}
		case KindReselect:
			grp := fieldAfter(msg, "group=")
			alive := atoiOr(fieldAfter(msg, "alive_dialers="), -1)
			if grp != "" {
				g := groupIdx[grp]
				if g == nil {
					g = &GroupStat{Name: grp}
					groupIdx[grp] = g
				}
				g.Reselects++
				if alive > g.AliveDialers {
					g.AliveDialers = alive
				}
			}
		case KindSelect:
			grp := fieldAfter(msg, "group=")
			alive := atoiOr(fieldAfter(msg, "alive_dialers="), -1)
			if grp != "" {
				g := groupIdx[grp]
				if g == nil {
					g = &GroupStat{Name: grp}
					groupIdx[grp] = g
				}
				g.Selects++
				if alive > g.AliveDialers {
					g.AliveDialers = alive
				}
			}
		case KindIOTimeout:
			if target := strings.TrimRight(between(msg, "->", " "), ":,;"); target != "" {
				ioTargets[target]++
			}
		}
	}

	for _, st := range nodeIdx {
		rep.Nodes = append(rep.Nodes, *st)
	}
	sort.Slice(rep.Nodes, func(i, j int) bool {
		a := rep.Nodes[i].NotAlive + rep.Nodes[i].ProxyIP
		b := rep.Nodes[j].NotAlive + rep.Nodes[j].ProxyIP
		if a != b {
			return a > b
		}
		return rep.Nodes[i].Name < rep.Nodes[j].Name
	})
	for _, g := range groupIdx {
		rep.Groups = append(rep.Groups, *g)
	}
	sort.Slice(rep.Groups, func(i, j int) bool {
		return rep.Groups[i].Reselects > rep.Groups[j].Reselects
	})
	for _, b := range bucketIdx {
		rep.Series = append(rep.Series, *b)
	}
	sort.Slice(rep.Series, func(i, j int) bool { return rep.Series[i].TS.Before(rep.Series[j].TS) })

	rep.Advice = advise(rep, ioTargets, windowMinutes)
	return rep
}

func advise(rep Report, ioTargets map[string]int, minutes int) []string {
	out := []string{}
	if rep.Counts[KindNoDialer] > 0 {
		out = append(out, "分组内没有可用节点 "+strconv.Itoa(rep.Counts[KindNoDialer])+" 次（致命）：节点池太小或全部被探测误杀，先稀释过滤条件或扩订阅。")
	}
	if rep.Counts[KindBootstrap] > 0 {
		out = append(out, "节点域名解析失败 "+strconv.Itoa(rep.Counts[KindBootstrap])+" 次：dae v2.1.1 的 bootstrap 解析器存在已知缺陷（上游 #1130 才修），请显式配置 bootstrap_resolver 为 ip:port（例 223.5.5.5:53）并让该地址直连。")
	}
	topIO, topN := "", 0
	for k, v := range ioTargets {
		if v > topN {
			topIO, topN = k, v
		}
	}
	if topIO != "" && strings.HasSuffix(topIO, ":853") {
		out = append(out, "超时集中在 DNS 上游 "+topIO+"（DoT 853）：该端口常年被运营商掐，建议把上游改 udp://223.5.5.5:53。")
	} else if topIO != "" {
		out = append(out, "最频繁的超时目标是 "+topIO+"（"+strconv.Itoa(topN)+" 次），优先怀疑该地址/该线路。")
	}
	if len(rep.Nodes) > 0 {
		n := rep.Nodes[0]
		if n.NotAlive+n.ProxyIP >= 2 {
			out = append(out, "节点「"+n.Name+"」被判死 "+strconv.Itoa(n.NotAlive+n.ProxyIP)+" 次，是误杀最多的节点：建议在分组过滤里排除它，或更换探测地址（cp.cloudflare.com 在国内+机场环境下不可靠）。")
		}
	}
	if minutes > 0 {
		total := 0
		for _, g := range rep.Groups {
			total += g.Reselects
		}
		if total*60/minutes >= 10 {
			out = append(out, "选点切换 "+strconv.Itoa(total)+" 次 / "+strconv.Itoa(minutes)+" 分钟：切换过于频繁，建议 check_tolerance: 500ms 且组策略改 min_avg10。")
		}
	}
	if rep.Counts[KindServfail] > 0 {
		out = append(out, "DNS SERVFAIL "+strconv.Itoa(rep.Counts[KindServfail])+" 次：客户端会感到「打不开」，需先修 DNS 上游再看数据面。")
	}
	if len(out) == 0 {
		out = append(out, "窗口内没有发现异常事件。")
	}
	return out
}

// FromCommand 按平台选日志来源并拉取：systemd 用 journalctl，否则用 logread。
func FromCommand(r Runner, unit string, minutes int, now time.Time) Report {
	if r == nil {
		r = execRunner{}
	}
	if minutes <= 0 {
		minutes = 60
	}
	since := strconv.Itoa(minutes) + " min ago"
	out, err := r.Run("journalctl", "-u", unit, "--since", since, "-o", "short-iso", "--no-pager")
	source := "journalctl -u " + unit
	if err != nil || strings.TrimSpace(out) == "" {
		if lo, lerr := r.Run("logread"); lerr == nil {
			out = lo
			source = "logread"
			err = nil
		}
	}
	if err != nil {
		return Report{WindowMinutes: minutes, Source: source, Error: err.Error(), Counts: map[string]int{}, Nodes: []NodeStat{}, Groups: []GroupStat{}, Series: []Bucket{}, Advice: []string{"读取日志失败：" + err.Error()}}
	}
	lines := strings.Split(out, "\n")
	filtered := lines
	if source == "logread" {
		filtered = filtered[:0]
		for _, l := range lines {
			if strings.Contains(l, "dae") {
				filtered = append(filtered, l)
			}
		}
	}
	return Parse(filtered, minutes, source, now)
}
