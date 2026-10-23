package traffic

// 字节级流量统计（方案 B：轮询内核 socket 计数器）。
//
// 为什么不是 eBPF 直读 dae 的 map：dae 的 map 布局随版本变化（v2.1.1 → 下一版就可能破），
// 属易碎依赖；且面板是零第三方依赖（go.mod 无 require），引入 ebpf 库会带来生命周期与
// 内核兼容面的额外风险。而我们真正要回答的是「每个节点用了多少字节 / 每台内网设备用了
// 多少字节 / 拦截网卡总共进出多少」——这些内核已经算好了。
//
// 数据源（全部只读，采集失败不影响数据面）：
//   1. `ss -tunip`：dae 每条到节点的 socket 累计 bytes_sent / bytes_received —— 逐节点精确字节；
//   2. /proc/net/dev：dae0 网卡收发总量 —— 含 UDP/QUIC，是「有没有漏算」的兜底口径。
//
// 已知局限（UI 与文档都写明，不假装精确）：
//   - socket 字节计数器只对 TCP 稳定；UDP socket 多数没有计数，QUIC 流量只体现在 dae0 总量里；
//   - 采样间隔内「建立即关闭」的连接会漏计（默认 10s，长连接为主时影响很小）；
//   - 采集器重启后，首次看到的 socket 只作为基线，不把它的历史字节算进「本次统计」。

import (
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Row 是「一个节点 / 一个客户端」的累计字节与最近一次采样速率。
type Row struct {
	Key      string    `json:"key"`
	Name     string    `json:"name,omitempty"`
	Rx       int64     `json:"rx"`
	Tx       int64     `json:"tx"`
	RateRx   float64   `json:"rateRx"`
	RateTx   float64   `json:"rateTx"`
	Conns    int       `json:"conns"`
	LastSeen time.Time `json:"lastSeen"`
}

// BytesSnapshot 是一次字节统计快照。
type BytesSnapshot struct {
	Enabled     bool      `json:"enabled"`
	Source      string    `json:"source"`
	StartedAt   time.Time `json:"startedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	IntervalMs  int       `json:"intervalMs"`
	Sockets     int       `json:"sockets"`
	TotalRx     int64     `json:"totalRx"`
	TotalTx     int64     `json:"totalTx"`
	RateRx      float64   `json:"rateRx"`
	RateTx      float64   `json:"rateTx"`
	IfaceName   string    `json:"ifaceName"`
	IfaceRx     int64     `json:"ifaceRx"`
	IfaceTx     int64     `json:"ifaceTx"`
	IfaceRateRx float64   `json:"ifaceRateRx"`
	IfaceRateTx float64   `json:"ifaceRateTx"`
	Nodes       []Row     `json:"nodes"`
	Clients     []Row     `json:"clients"`
	Error       string    `json:"error,omitempty"`
}

// Sock 是 ss 输出里的一条 socket。
type Sock struct {
	Proto string
	Local string
	Peer  string
	Tx    int64 // bytes_sent
	Rx    int64 // bytes_received
	Proc  string
}

var (
	ssHeadRe  = regexp.MustCompile(`^(tcp|udp|raw)\s+(\S+)\s+(\d+)\s+(\d+)\s+(\S+)\s+(\S+)\s*(.*)$`)
	bytesSent = regexp.MustCompile(`bytes_sent:(\d+)`)
	bytesRecv = regexp.MustCompile(`bytes_received:(\d+)`)
)

// ParseSS 解析 `ss -tunip` 输出。纯函数，便于单测。
func ParseSS(out string) []Sock {
	var socks []Sock
	var cur *Sock
	flush := func() {
		if cur != nil {
			socks = append(socks, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if m := ssHeadRe.FindStringSubmatch(line); m != nil {
			flush()
			cur = &Sock{Proto: m[1], Local: m[5], Peer: m[6], Proc: m[7]}
			continue
		}
		if cur == nil {
			continue
		}
		if m := bytesSent.FindStringSubmatch(line); m != nil {
			if v, err := strconv.ParseInt(m[1], 10, 64); err == nil {
				cur.Tx = v
			}
		}
		if m := bytesRecv.FindStringSubmatch(line); m != nil {
			if v, err := strconv.ParseInt(m[1], 10, 64); err == nil {
				cur.Rx = v
			}
		}
	}
	flush()
	return socks
}

// isDaeProc 判断这条 socket 是否属于 dae（ss -p 在 root 下可见）。
func isDaeProc(proc string) bool {
	return strings.Contains(proc, `("dae"`)
}

// hostOf 从 addr:port 里取 host 与 port（兼容 [::1]:53、*:*）。
func hostOf(addr string) (host, port string) {
	if addr == "" {
		return "", ""
	}
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return strings.Trim(addr, "[]"), ""
	}
	return strings.Trim(addr[:i], "[]"), addr[i+1:]
}

// isPrivateHost 判断是否内网/本机地址（这类对端算「客户端」而不是「节点」）。
func isPrivateHost(h string) bool {
	if h == "" || h == "*" {
		return true
	}
	if strings.Contains(h, ":") { // IPv6：回环与链路本地算本机
		return h == "::1" || strings.HasPrefix(h, "fe80") || strings.HasPrefix(h, "fd")
	}
	parts := strings.Split(h, ".")
	if len(parts) != 4 {
		return true
	}
	a, _ := strconv.Atoi(parts[0])
	b, _ := strconv.Atoi(parts[1])
	switch {
	case a == 10, a == 127:
		return true
	case a == 172 && b >= 16 && b <= 31:
		return true
	case a == 192 && b == 168:
		return true
	case a == 169 && b == 254:
		return true
	case a == 100 && b >= 64 && b <= 127:
		return true
	}
	return false
}

// ConnCollector 周期性采样 dae 的 socket 字节计数器与拦截网卡总量。
type ConnCollector struct {
	mu       sync.Mutex
	interval time.Duration
	iface    string
	started  time.Time
	updated  time.Time
	lastAt   time.Time
	lastSock map[string][2]int64
	nodes    map[string]*Row
	clients  map[string]*Row
	totRx    int64
	totTx    int64
	rateRx   float64
	rateTx   float64
	socks    int
	errText  string
	ifLast   [2]int64
	ifTot    [2]int64
	ifRate   [2]float64
	nameFn   func(host, port string) string
	stop     chan struct{}

	// 便于单测替换
	SockFn  func() ([]Sock, error)
	IfaceFn func() (int64, int64, error)
}

// NewConnCollector 创建采集器。interval<=0 表示关闭。nameFn 可为 nil（不显示节点名）。
func NewConnCollector(interval time.Duration, iface string, nameFn func(host, port string) string) *ConnCollector {
	c := &ConnCollector{
		interval: interval,
		iface:    iface,
		started:  time.Now(),
		lastSock: map[string][2]int64{},
		nodes:    map[string]*Row{},
		clients:  map[string]*Row{},
		nameFn:   nameFn,
		stop:     make(chan struct{}),
	}
	c.SockFn = func() ([]Sock, error) { return ssSocks() }
	c.IfaceFn = func() (int64, int64, error) { return netDevCounters(c.iface) }
	return c
}

// Enabled 表示采集是否开启。
func (c *ConnCollector) Enabled() bool { return c != nil && c.interval > 0 }

// Start 启动后台采样。
func (c *ConnCollector) Start() {
	if !c.Enabled() {
		return
	}
	go func() {
		_ = c.Sample()
		t := time.NewTicker(c.interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				_ = c.Sample()
			case <-c.stop:
				return
			}
		}
	}()
}

// Close 停止采样。
func (c *ConnCollector) Close() error {
	if c == nil {
		return nil
	}
	select {
	case <-c.stop:
	default:
		close(c.stop)
	}
	return nil
}

// ssSocks 读取 dae 当前所有 socket 的字节计数器。
func ssSocks() ([]Sock, error) {
	out, err := exec.Command("ss", "-tunip").Output()
	if err != nil {
		return nil, err
	}
	all := ParseSS(string(out))
	dae := make([]Sock, 0, len(all))
	for _, s := range all {
		if isDaeProc(s.Proc) {
			dae = append(dae, s)
		}
	}
	return dae, nil
}

// netDevCounters 读 /proc/net/dev 里某网卡的累计收/发字节。
func netDevCounters(iface string) (int64, int64, error) {
	if iface == "" {
		return 0, 0, nil
	}
	b, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0, err
	}
	return parseNetDev(string(b), iface)
}

// parseNetDev 从 /proc/net/dev 内容里取指定网卡的 (rx, tx) 字节。纯函数。
func parseNetDev(data, iface string) (int64, int64, error) {
	for _, line := range strings.Split(data, "\n") {
		i := strings.Index(line, ":")
		if i < 0 {
			continue
		}
		if strings.TrimSpace(line[:i]) != iface {
			continue
		}
		f := strings.Fields(line[i+1:])
		if len(f) < 9 {
			continue
		}
		rx, e1 := strconv.ParseInt(f[0], 10, 64)
		tx, e2 := strconv.ParseInt(f[8], 10, 64)
		if e1 != nil || e2 != nil {
			break
		}
		return rx, tx, nil
	}
	return 0, 0, errIfaceMissing
}

var errIfaceMissing = &ifaceErr{}

type ifaceErr struct{}

func (*ifaceErr) Error() string {
	return "网卡不存在或不可读（未启用拦截时属正常）"
}

// Sample 采样一次并累计。
func (c *ConnCollector) Sample() error {
	socks, err := c.SockFn()
	ifcRx, ifcTx, ifcErr := c.IfaceFn()

	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(c.lastAt).Seconds()
	if c.lastAt.IsZero() || elapsed <= 0 {
		elapsed = 0
	}
	c.lastAt = now

	if err != nil {
		c.errText = "读取 socket 统计失败: " + err.Error()
	} else {
		c.errText = ""
	}

	cur := make(map[string][2]int64, len(socks))
	rowRate := map[string][2]float64{}
	rowConns := map[string]int{}
	var dRx, dTx int64
	for _, s := range socks {
		key := s.Proto + "|" + s.Local + "|" + s.Peer
		cur[key] = [2]int64{s.Tx, s.Rx}
		prev, seen := c.lastSock[key]
		if !seen {
			continue // 新 socket 首见：只记基线
		}
		dt, dr := int64(0), int64(0)
		if s.Tx >= prev[0] {
			dt = s.Tx - prev[0]
		}
		if s.Rx >= prev[1] {
			dr = s.Rx - prev[1]
		}
		if dt == 0 && dr == 0 {
			continue
		}
		host, port := hostOf(s.Peer)
		var bucket map[string]*Row
		var bkey string
		if isPrivateHost(host) {
			bucket, bkey = c.clients, host
		} else {
			bucket, bkey = c.nodes, s.Peer
		}
		r := bucket[bkey]
		if r == nil {
			r = &Row{Key: bkey}
			if bucket != nil && bkey != "" && !isPrivateHost(host) && c.nameFn != nil {
				r.Name = c.nameFn(host, port)
			}
			bucket[bkey] = r
		}
		r.Rx += dr
		r.Tx += dt
		r.LastSeen = now
		v := rowRate[bkey]
		rowRate[bkey] = [2]float64{v[0] + float64(dr), v[1] + float64(dt)}
		rowConns[bkey]++
		dRx += dr
		dTx += dt
	}
	c.lastSock = cur
	c.socks = len(socks)
	c.totRx += dRx
	c.totTx += dTx
	if elapsed > 0 {
		c.rateRx = float64(dRx) / elapsed
		c.rateTx = float64(dTx) / elapsed
	}
	for _, m := range []map[string]*Row{c.nodes, c.clients} {
		for k, r := range m {
			if v, ok := rowRate[k]; ok {
				r.RateRx = v[0] / maxF(elapsed, 1e-9)
				r.RateTx = v[1] / maxF(elapsed, 1e-9)
			} else {
				r.RateRx, r.RateTx = 0, 0
			}
			r.Conns = rowConns[k]
		}
	}
	if ifcErr == nil {
		if c.ifLast != [2]int64{} && ifcRx >= c.ifLast[0] && ifcTx >= c.ifLast[1] && elapsed > 0 {
			c.ifRate = [2]float64{float64(ifcRx-c.ifLast[0]) / elapsed, float64(ifcTx-c.ifLast[1]) / elapsed}
		}
		c.ifLast = [2]int64{ifcRx, ifcTx}
		c.ifTot = [2]int64{ifcRx, ifcTx}
	}
	c.updated = now
	return err
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// Snapshot 返回当前快照。
func (c *ConnCollector) Snapshot() BytesSnapshot {
	if c == nil || !c.Enabled() {
		return BytesSnapshot{Enabled: false, Source: "ss-sockets", Nodes: []Row{}, Clients: []Row{}}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	sn := BytesSnapshot{
		Enabled:     true,
		Source:      "ss-sockets",
		StartedAt:   c.started,
		UpdatedAt:   c.updated,
		IntervalMs:  int(c.interval / time.Millisecond),
		Sockets:     c.socks,
		TotalRx:     c.totRx,
		TotalTx:     c.totTx,
		RateRx:      c.rateRx,
		RateTx:      c.rateTx,
		IfaceName:   c.iface,
		IfaceRx:     c.ifTot[0],
		IfaceTx:     c.ifTot[1],
		IfaceRateRx: c.ifRate[0],
		IfaceRateTx: c.ifRate[1],
		Error:       c.errText,
		Nodes:       copyRows(c.nodes),
		Clients:     copyRows(c.clients),
	}
	return sn
}

func copyRows(m map[string]*Row) []Row {
	out := make([]Row, 0, len(m))
	for _, r := range m {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Rx+out[i].Tx, out[j].Rx+out[j].Tx
		if a != b {
			return a > b
		}
		return out[i].Key < out[j].Key
	})
	return out
}
