package traffic

// 内网客户端流量归集（数据源：/proc/net/nf_conntrack）。
//
// 为什么需要另一个数据源：dae 用 tproxy 在本地终结连接，`ss` 只能看到
// dae ↔ 上游节点/目标的 socket，看不到「这条连接是内网哪台设备发起的」。所以按
// socket 归集的 clients 表在旁路由场景下永远是空的（实测 n6：conntrack 里有
// src=192.168.5.83 的 96 条连接，而 ss 里只有 dae 自己的 34 条）。
//
// conntrack 表里保留着原始五元组与两个方向的字节计数，正好补上「谁在用」。
//
// 只读、无副作用；读不到（未加载 nf_conntrack 模块 / 无权限）只影响客户端表，
// 不影响节点表与网卡总量。

import (
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ConnEntry 是 conntrack 里一条连接，已归一到「哪个内网客户端 ↔ 哪个远端」。
type ConnEntry struct {
	Proto  string `json:"proto"`
	Client string `json:"client"`
	Remote string `json:"remote"`
	Dport  string `json:"dport,omitempty"`
	Out    int64  `json:"out"` // 客户端 → 远端 的字节
	In     int64  `json:"in"`  // 远端 → 客户端 的字节
}

// conntrackPaths 是内核导出连接跟踪表的常见位置。
var conntrackPaths = []string{"/proc/net/nf_conntrack", "/proc/net/ip_conntrack"}

// ConntrackPath 返回第一个可读的连接跟踪文件；都不存在返回空串。
func ConntrackPath() string {
	for _, p := range conntrackPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ReadConntrack 读取并解析连接跟踪表。空路径表示不可用。
func ReadConntrack(path string) ([]ConnEntry, error) {
	if path == "" {
		return nil, os.ErrNotExist
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseConntrack(string(b)), nil
}

// readConntrackLocal 读取连接跟踪表，并把本机子网视为客户端侧。
func readConntrackLocal(path string, local []*net.IPNet) ([]ConnEntry, error) {
	if path == "" {
		return nil, os.ErrNotExist
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseConntrackLocal(string(b), local), nil
}

// ParseConntrack 解析 /proc/net/nf_conntrack 的文本内容。纯函数，便于单测。
//
// 一行形如：
//
//	ipv4 2 tcp 6 431999 ESTABLISHED src=192.168.5.83 dst=140.82.112.4 sport=51514 dport=443
//	     src=140.82.112.4 dst=192.168.5.83 sport=443 dport=51514 [ASSURED] mark=0 use=1
//
// 有些内核会在每个方向上额外给出 bytes= / packets=。没有 bytes= 时按 0 处理
// （那只是「这条连接没有字节信息」，不是错误）。
func ParseConntrack(data string) []ConnEntry {
	return parseConntrack(data, isPrivateHost)
}

// ParseConntrackLocal 与 ParseConntrack 相同，但把「本机所在子网」也视为客户端侧。
// 家庭网络里的内网设备常常持有公网 IPv6（SLAAC），只靠私有地址判断会漏掉它们。
func ParseConntrackLocal(data string, local []*net.IPNet) []ConnEntry {
	pred := func(ip string) bool { return isPrivateHost(ip) || inNets(ip, local) }
	return parseConntrack(data, pred)
}

func parseConntrack(data string, isLocal func(string) bool) []ConnEntry {
	var out []ConnEntry
	for _, line := range strings.Split(data, "\n") {
		e, ok := parseConntrackLine(line, isLocal)
		if ok {
			out = append(out, e)
		}
	}
	return out
}

func parseConntrackLine(line string, isLocal func(string) bool) (ConnEntry, bool) {
	f := strings.Fields(line)
	if len(f) < 6 {
		return ConnEntry{}, false
	}
	if f[0] != "ipv4" && f[0] != "ipv6" {
		return ConnEntry{}, false // 表头或非 IP 协议（如 bridge）
	}
	proto := f[2]

	// 按 src= 出现切分方向：每行最多两个方向（原始 + 回复）。
	var tuples [2]map[string]string
	var cur map[string]string
	for _, tok := range f {
		k, v, ok := strings.Cut(tok, "=")
		if !ok {
			continue
		}
		if k == "src" {
			if cur != nil {
				if tuples[0] == nil {
					tuples[0] = cur
				} else if tuples[1] == nil {
					tuples[1] = cur
				}
				cur = nil
			}
			cur = map[string]string{}
		}
		if cur != nil {
			cur[k] = v
		}
	}
	if cur != nil {
		if tuples[0] == nil {
			tuples[0] = cur
		} else if tuples[1] == nil {
			tuples[1] = cur
		}
	}
	if tuples[0] == nil {
		return ConnEntry{}, false
	}

	first, second := tuples[0], tuples[1]
	s1, d1 := first["src"], first["dst"]
	e := ConnEntry{Proto: proto}
	switch {
	case isLocal(s1) && !isLocal(d1):
		e.Client, e.Remote, e.Dport = s1, d1, first["dport"]
		e.Out = bytesOf(first)
		e.In = bytesOf(second)
	case isLocal(d1) && !isLocal(s1):
		e.Client, e.Remote, e.Dport = d1, s1, second["dport"]
		e.Out = bytesOf(second)
		e.In = bytesOf(first)
	default:
		return ConnEntry{}, false // 两端都本地（内网互访）或都远端（不是客户端流量）
	}
	if e.Client == "" || e.Remote == "" {
		return ConnEntry{}, false
	}
	return e, true
}

func bytesOf(t map[string]string) int64 {
	if t == nil {
		return 0
	}
	v, err := strconv.ParseInt(t["bytes"], 10, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

// connKey 是判定「同一条连接」的键（同五元组在同一个采样周期内视为一条）。
func connKey(e ConnEntry) string {
	return e.Proto + "|" + e.Client + "|" + e.Remote + "|" + e.Dport
}

// topRemotes 取前 n 个远端，拼成可读的一行。
func topRemotes(m map[string]int64, n int) string {
	if len(m) == 0 || n <= 0 {
		return ""
	}
	type kv struct {
		k string
		v int64
	}
	list := make([]kv, 0, len(m))
	for k, v := range m {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].v != list[j].v {
			return list[i].v > list[j].v
		}
		return list[i].k < list[j].k
	})
	var b strings.Builder
	for i, it := range list {
		if i >= n {
			break
		}
		if i > 0 {
			b.WriteString(" · ")
		}
		b.WriteString(it.k)
		b.WriteString(" ")
		b.WriteString(humanBytes(it.v))
	}
	return b.String()
}

// humanBytes 把字节数写成 1.2MB 这类短文本（UI 明细列用）。
func humanBytes(v int64) string {
	const unit = 1024
	if v < unit {
		return strconv.FormatInt(v, 10) + "B"
	}
	units := []string{"KB", "MB", "GB", "TB"}
	f := float64(v)
	for _, u := range units {
		f /= unit
		if f < unit {
			return strconv.FormatFloat(f, 'f', 1, 64) + u
		}
	}
	return strconv.FormatFloat(f, 'f', 1, 64) + "PB"
}

// clientAgg 是单个客户端在一次采样周期内的增量。
type clientAgg struct {
	tx, rx  int64
	conns   int
	remotes map[string]int64 // 字节口径（需内核开 acct）
	counts  map[string]int64 // 连接数口径（acct 关闭时的兜底展示）
}

// sampleClients 把 conntrack 的增量归到内网客户端行。
// 调用方已持有 c.mu。首个采样周期只记基线（与 socket 侧口径一致）。
func (c *ConnCollector) sampleClients(entries []ConnEntry, err error, now time.Time, elapsed float64) {
	if err != nil {
		c.hasCT = false
		c.ctErr = "读不到连接跟踪表（旁路由/网关场景需要 nf_conntrack）: " + err.Error()
		return
	}
	c.hasCT = true
	c.ctErr = ""
	c.acct = conntrackAcctOn()
	if c.AcctFn != nil {
		c.acct = c.AcctFn()
	}

	cur := make(map[string][2]int64, len(entries))
	agg := map[string]*clientAgg{}
	hasBytes := false
	for _, e := range entries {
		if e.Out+e.In > 0 {
			hasBytes = true
		}
		key := connKey(e)
		cur[key] = [2]int64{e.Out, e.In}
		prev, seen := c.ctLast[key]
		var dOut, dIn int64
		if seen {
			if e.Out >= prev[0] {
				dOut = e.Out - prev[0]
			}
			if e.In >= prev[1] {
				dIn = e.In - prev[1]
			}
		}
		a := agg[e.Client]
		if a == nil {
			a = &clientAgg{remotes: map[string]int64{}, counts: map[string]int64{}}
			agg[e.Client] = a
		}
		a.conns++
		a.tx += dOut
		a.rx += dIn
		if dOut+dIn > 0 {
			a.remotes[e.Remote+portSuffix(e.Dport)] += dOut + dIn
		}
		if !c.acct || !hasBytes {
			a.counts[e.Remote+portSuffix(e.Dport)]++
		}
	}
	c.ctLast = cur
	c.sampled = len(entries) > 0
	// acct 打开但表里全是 0 字节：说明该内核没真正统计（实测 6.12 Debian 如此），按连接数口径展示。
	c.bytesUse = c.acct && hasBytes

	for ip, a := range agg {
		r := c.clients[ip]
		if r == nil {
			r = &Row{Key: ip, Name: ip}
			if c.localIPs[ip] {
				r.Name = ip + "（本机）"
			}
			c.clients[ip] = r
		}
		r.Rx += a.rx
		r.Tx += a.tx
		r.Conns = a.conns
		r.LastSeen = now
		if elapsed > 0 {
			r.RateRx = float64(a.rx) / elapsed
			r.RateTx = float64(a.tx) / elapsed
		} else {
			r.RateRx, r.RateTx = 0, 0
		}
		m := c.clientRemotes[ip]
		if m == nil {
			m = map[string]int64{}
			c.clientRemotes[ip] = m
		}
		for k, v := range a.remotes {
			m[k] += v
		}
		if !c.bytesUse {
			cm := c.clientConns[ip]
			if cm == nil {
				cm = map[string]int64{}
				c.clientConns[ip] = cm
			}
			for k, v := range a.counts {
				cm[k] += v
			}
			r.Detail = topCounts(cm, 3)
		} else {
			r.Detail = topRemotes(m, 3)
		}
	}
	// 本轮已消失的客户端：保留累计字节，速率与连接数归零。
	for ip, r := range c.clients {
		if _, ok := agg[ip]; ok {
			continue
		}
		r.RateRx, r.RateTx, r.Conns = 0, 0, 0
	}
}

func portSuffix(port string) string {
	if port == "" {
		return ""
	}
	return ":" + port
}

// source 说明本次快照用到的数据源。
func (c *ConnCollector) source() string {
	if c.hasCT {
		return "ss-sockets+conntrack"
	}
	return "ss-sockets"
}

// clientSource 说明客户端表的来源（空 = 不可用）。
func (c *ConnCollector) clientSource() string {
	if c.hasCT {
		return "conntrack"
	}
	return ""
}

// LocalSubnets 返回本机各网卡上配置的子网（回环除外）。
// 用来判断 conntrack 里哪个地址「属于我们这一侧」——内网设备也可能持有公网 IPv6。
func LocalSubnets() []*net.IPNet {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []*net.IPNet
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok || ipn.IP.IsLoopback() {
			continue
		}
		out = append(out, ipn)
	}
	return out
}

func inNets(ipStr string, nets []*net.IPNet) bool {
	if len(nets) == 0 || ipStr == "" {
		return false
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n != nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

// localAddrs 返回本机各网卡上的地址（用于在客户端表里标注「本机」）。
func localAddrs() map[string]bool {
	out := map[string]bool{}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return out
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok {
			out[ipn.IP.String()] = true
		}
	}
	return out
}

// conntrackAcctOn 报告内核是否在连接跟踪里记账字节数（net.netfilter.nf_conntrack_acct）。
// 默认多数发行版是 0：表里有字节字段但永远是 0，只能看连接数与目的地址。
func conntrackAcctOn() bool {
	b, err := os.ReadFile("/proc/sys/net/netfilter/nf_conntrack_acct")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(b)) == "1"
}

// clientNote 给出客户端表的已知局限（不隐藏，也不假装精确）。
func (c *ConnCollector) clientNote() string {
	if !c.hasCT {
		return ""
	}
	if !c.acct {
		return "内核未开启连接字节记账（net.netfilter.nf_conntrack_acct=0）：客户端表只能看到连接数与主要目的，" +
			"要统计字节请执行 sysctl -w net.netfilter.nf_conntrack_acct=1 并持久化到 /etc/sysctl.d/"
	}
	if c.sampled && !c.bytesUse {
		return "连接跟踪记账已开，但本机内核没有真的往表里写字节数（实测部分 6.x 内核如此）：" +
			"客户端表只能看到连接数与主要目的；字节总量请看上方「dae0 网卡总量」与「按节点」两张表。"
	}
	return ""
}

// topCounts 取前 n 个远端（按连接数），acct 关闭时用来显示「主要目的」。
func topCounts(m map[string]int64, n int) string {
	if len(m) == 0 || n <= 0 {
		return ""
	}
	type kv struct {
		k string
		v int64
	}
	list := make([]kv, 0, len(m))
	for k, v := range m {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].v != list[j].v {
			return list[i].v > list[j].v
		}
		return list[i].k < list[j].k
	})
	var b strings.Builder
	for i, it := range list {
		if i >= n {
			break
		}
		if i > 0 {
			b.WriteString(" · ")
		}
		b.WriteString(it.k)
		b.WriteString(" ")
		b.WriteString(strconv.FormatInt(it.v, 10))
		b.WriteString(" 条")
	}
	return b.String()
}
