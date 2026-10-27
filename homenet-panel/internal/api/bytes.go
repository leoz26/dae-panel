package api

// 字节级流量统计的 HTTP 侧：/api/traffic/bytes
// 数据全部来自 traffic.ConnCollector（只读：ss 的 socket 计数器 + /proc/net/dev）。

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"homenet-panel/internal/traffic"
)

// StartBytes 启动字节统计采集（interval<=0 表示关闭）。
func (s *Server) StartBytes(interval time.Duration, iface string) {
	if interval <= 0 {
		return
	}
	if iface == "" {
		iface = "dae0"
	}
	s.Bytes = traffic.NewConnCollector(interval, iface, s.bytesNodeName)
	s.Bytes.Start()
	// 节点索引首次构建要解析节点域名，先丢到后台，别让第一个请求等它。
	go s.buildNodeIndex()
}

func (s *Server) handleTrafficBytes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.Bytes.Snapshot())
}

// bytesNodeName 把「节点 IP:端口」映射回订阅里的节点名（对不上就返回空，前端显示原始地址）。
func (s *Server) bytesNodeName(host, port string) string {
	idx := s.nodeIndex()
	if n, ok := idx[host+":"+port]; ok {
		return n
	}
	return ""
}

// nodeIndex 返回当前索引（10 分钟缓存）。缓存过期时触发后台重建并先返回旧值，
// 绝不阻塞请求：OpenWrt 上节点域名解析可能很慢，串行解析能把请求拖到分钟级。
func (s *Server) nodeIndex() map[string]string {
	s.bytesMu.Lock()
	idx, at, building := s.bytesIdx, s.bytesIdxAt, s.bytesBuilding
	fresh := idx != nil && time.Since(at) < 10*time.Minute
	if fresh || building {
		s.bytesMu.Unlock()
		return idx
	}
	s.bytesBuilding = true
	s.bytesMu.Unlock()

	go s.buildNodeIndex()
	return idx
}

// buildNodeIndex 解析订阅节点，构建 host:port / ip:port -> 节点名 的索引。
// 解析并发有上限、单次有超时，避免一个不可解析的域名拖住整轮。
func (s *Server) buildNodeIndex() {
	names := map[string]string{} // host:port -> name（含未解析的域名键）
	sources, _ := s.allSources()
	for _, src := range sources {
		for _, n := range src.Nodes {
			if n.Host == "" {
				continue
			}
			name := n.Tag
			if name == "" {
				name = n.Scheme + "://" + n.Host
			}
			names[n.Host+":"+n.Port] = name
		}
	}

	hosts := make([]string, 0, len(names))
	seen := map[string]bool{}
	for k := range names {
		h, _, err := net.SplitHostPort(k)
		if err != nil || net.ParseIP(h) != nil || seen[h] {
			continue
		}
		seen[h] = true
		hosts = append(hosts, h)
	}

	idx := make(map[string]string, len(names))
	for k, v := range names {
		idx[k] = v
	}
	resolver := &net.Resolver{PreferGo: true}
	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, h := range hosts {
		wg.Add(1)
		sem <- struct{}{}
		go func(host string) {
			defer wg.Done()
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			addrs, err := resolver.LookupHost(ctx, host)
			if err != nil {
				return
			}
			mu.Lock()
			for k, v := range names {
				hh, pp, e := net.SplitHostPort(k)
				if e != nil || hh != host {
					continue
				}
				for _, a := range addrs {
					idx[a+":"+pp] = v
				}
			}
			mu.Unlock()
		}(h)
	}
	wg.Wait()

	s.bytesMu.Lock()
	s.bytesIdx = idx
	s.bytesIdxAt = time.Now()
	s.bytesBuilding = false
	s.bytesMu.Unlock()
}
