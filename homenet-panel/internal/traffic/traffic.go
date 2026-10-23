// Package traffic 采集「按域名命中 / 流量」统计。
//
// 设计原则：
//   - 面板只做旁路观测，绝不改动 dae 的数据面；采集失败也不影响代理。
//   - 采集源以 Source 接口抽象：当前实现 DNS 旁路抓包（方案③）；
//     预留 eBPF 直读 dae map（方案②，见 EBpfSource），未来实现同一接口即可替换。
package traffic

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrReserved 表示该采集源为预留接口，尚未实现。
var ErrReserved = errors.New("eBPF 采集源尚未实现（预留接口）")

// Hit 是单个域名的累计命中。
type Hit struct {
	Domain   string    `json:"domain"`
	Count    int64     `json:"count"`
	LastSeen time.Time `json:"lastSeen"`
}

// Snapshot 是一次统计快照。
type Snapshot struct {
	Source    string    `json:"source"`
	Iface     string    `json:"iface,omitempty"`
	StartedAt time.Time `json:"startedAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Queries   int64     `json:"queries" `
	Domains   int       `json:"domains" `
	Dropped   int64     `json:"dropped" `
	Hits      []Hit     `json:"hits"`
	Error     string    `json:"error,omitempty"`
}

// Source 是可插拔的统计采集源。未来接入 eBPF（②）时实现该接口即可替换，无需改动上层。
type Source interface {
	Name() string
	Snapshot(limit int) Snapshot
	Close() error
}

// Aggregator 线程安全地累计域名查询计数。
type Aggregator struct {
	mu      sync.Mutex
	started time.Time
	updated time.Time
	queries int64
	dropped int64
	m       map[string]*agg
}

type agg struct {
	n    int64
	last time.Time
}

// NewAggregator 创建聚合器。
func NewAggregator() *Aggregator {
	now := time.Now()
	return &Aggregator{started: now, updated: now, m: map[string]*agg{}}
}

// Add 记录一次域名查询。
func (a *Aggregator) Add(domain string) {
	if domain == "" {
		return
	}
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.queries++
	a.updated = now
	c := a.m[domain]
	if c == nil {
		c = &agg{}
		a.m[domain] = c
	}
	c.n++
	c.last = now
}

// Drop 记录一次解析失败的包。
func (a *Aggregator) Drop() {
	a.mu.Lock()
	a.dropped++
	a.mu.Unlock()
}

// Snapshot 返回按次数降序（同次数按域名升序）的前 limit 条。
func (a *Aggregator) Snapshot(limit int) Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	sn := Snapshot{
		StartedAt: a.started,
		UpdatedAt: a.updated,
		Queries:   a.queries,
		Dropped:   a.dropped,
		Domains:   len(a.m),
	}
	hits := make([]Hit, 0, len(a.m))
	for d, c := range a.m {
		hits = append(hits, Hit{Domain: d, Count: c.n, LastSeen: c.last})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Count != hits[j].Count {
			return hits[i].Count > hits[j].Count
		}
		return hits[i].Domain < hits[j].Domain
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	sn.Hits = hits
	return sn
}

// EBpfSource 预留：直读 dae eBPF map 的字节级计量采集源（方案②）。
// 目前仅占位，Snapshot 返回 ErrReserved，供上层接口与前端展示预留状态。
type EBpfSource struct{}

// Name 返回采集源名。
func (EBpfSource) Name() string { return "ebpf-dae-maps" }

// Snapshot 预留实现，返回未实现提示。
func (EBpfSource) Snapshot(limit int) Snapshot {
	return Snapshot{Source: "ebpf-dae-maps", Error: ErrReserved.Error(), Hits: []Hit{}}
}

// Close 预留实现。
func (EBpfSource) Close() error { return nil }

// SourceInfo 描述一个采集源的可用状态，供前端展示。
type SourceInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ready | disabled | reserved
	Detail string `json:"detail"`
}

// Sources 返回采集源清单。dnsReady 表示 DNS 旁路抓包是否就绪，dnsDetail 为附加说明。
func Sources(dnsReady bool, dnsDetail string) []SourceInfo {
	ds := "ready"
	if !dnsReady {
		ds = "disabled"
	}
	detail := "旁路抓取本地 UDP/53 查询（只读，不改数据面），反映域名查询频度而非字节量。"
	if dnsDetail != "" {
		detail += " " + dnsDetail
	}
	return []SourceInfo{
		{Name: "dns-capture", Status: ds, Detail: detail},
		{Name: "ebpf-dae-maps", Status: "reserved", Detail: "预留：直读 dae eBPF map 的字节级计量（精度最高、开销最低，但依赖 dae 内部实现，随版本易碎）。"},
	}
}

// parseQueryFrame 从以太网帧解析出 DNS 查询（目的端口 53）的域名。
// 仅处理 IPv4/UDP，其余返回 ok=false。纯函数，便于单测。
func parseQueryFrame(pkt []byte) (string, bool) {
	if len(pkt) < 14+20+8 {
		return "", false
	}
	if pkt[12] != 0x08 || pkt[13] != 0x00 { // ethertype IPv4
		return "", false
	}
	ip := pkt[14:]
	if ip[0]>>4 != 4 {
		return "", false
	}
	ihl := int(ip[0]&0x0f) * 4
	if ihl < 20 || len(ip) < ihl+8 {
		return "", false
	}
	if ip[9] != 17 { // UDP
		return "", false
	}
	if total := int(ip[2])<<8 | int(ip[3]); total > 0 && total <= len(ip) {
		ip = ip[:total]
	}
	udp := ip[ihl:]
	if len(udp) < 8 {
		return "", false
	}
	dp := int(udp[2])<<8 | int(udp[3])
	if dp != 53 { // 只统计查询（发往 53）
		return "", false
	}
	return qname(udp[8:])
}

// qname 解析 DNS 报文问题段的第一个域名。
func qname(p []byte) (string, bool) {
	if len(p) < 12 {
		return "", false
	}
	if qd := int(p[4])<<8 | int(p[5]); qd < 1 {
		return "", false
	}
	i := 12
	var sb strings.Builder
	for i < len(p) {
		l := int(p[i])
		if l == 0 {
			break
		}
		if l&0xc0 != 0 { // 压缩指针不应出现在问题段
			return "", false
		}
		i++
		if i+l > len(p) {
			return "", false
		}
		if sb.Len() > 0 {
			sb.WriteByte('.')
		}
		sb.Write(p[i : i+l])
		i += l
		if sb.Len() > 253 {
			return "", false
		}
	}
	if sb.Len() == 0 {
		return "", false
	}
	return strings.ToLower(sb.String()), true
}
