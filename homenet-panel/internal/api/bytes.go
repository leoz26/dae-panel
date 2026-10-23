package api

// 字节级流量统计的 HTTP 侧：/api/traffic/bytes
// 数据全部来自 traffic.ConnCollector（只读：ss 的 socket 计数器 + /proc/net/dev）。

import (
	"net"
	"net/http"
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

// nodeIndex 构建 ip:port -> 节点名 的索引，带 10 分钟缓存（含 DNS 解析，不在每次采样时做）。
func (s *Server) nodeIndex() map[string]string {
	s.bytesMu.Lock()
	if s.bytesIdx != nil && time.Since(s.bytesIdxAt) < 10*time.Minute {
		idx := s.bytesIdx
		s.bytesMu.Unlock()
		return idx
	}
	s.bytesMu.Unlock()

	idx := map[string]string{}
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
			idx[n.Host+":"+n.Port] = name
			// 节点多半是域名，解析成 IP 才能和 ss 里的对端地址对上
			addrs, err := net.LookupHost(n.Host)
			if err != nil {
				continue
			}
			for _, a := range addrs {
				idx[a+":"+n.Port] = name
			}
		}
	}

	s.bytesMu.Lock()
	s.bytesIdx = idx
	s.bytesIdxAt = time.Now()
	s.bytesMu.Unlock()
	return idx
}
