//go:build linux

package traffic

import (
	"fmt"
	"net"
	"sync"
	"syscall"
	"time"
)

// DNSSource 通过 AF_PACKET 原始套接字旁路抓取本机网卡上的 DNS 查询（UDP/53），
// 统计域名查询频度。只读、旁路，不改动 dae 数据面。
type DNSSource struct {
	iface string
	fd    int
	agg   *Aggregator
	done  chan struct{}
	once  sync.Once

	errMu sync.Mutex
	err   error
}

// NewDNSSource 在指定网卡上开启 DNS 旁路抓包。需要 CAP_NET_RAW（面板以 root 运行即可）。
func NewDNSSource(iface string) (Source, error) {
	ifc, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, fmt.Errorf("网卡 %q 不存在: %w", iface, err)
	}
	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, int(htons(syscall.ETH_P_ALL)))
	if err != nil {
		return nil, fmt.Errorf("socket(AF_PACKET): %w", err)
	}
	if err := syscall.SetNonblock(fd, true); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	// 加大接收缓冲，降低突发丢包（失败不致命）。
	_ = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, 4<<20)
	if err := syscall.Bind(fd, &syscall.SockaddrLinklayer{Ifindex: ifc.Index, Protocol: htons(syscall.ETH_P_ALL)}); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("bind %s: %w", iface, err)
	}
	if err := syscall.AttachLsf(fd, dnsFilter()); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("attach BPF filter: %w", err)
	}
	s := &DNSSource{iface: iface, fd: fd, agg: NewAggregator(), done: make(chan struct{})}
	go s.loop()
	return s, nil
}

func htons(v uint16) uint16 { return v<<8 | v>>8 }

// dnsFilter 生成 cBPF 程序：只放行以太网 IPv4/UDP 且源或目的端口为 53 的包。
// 这样内核就把绝大多数数据面流量挡在用户态之外，面板几乎零额外开销。
func dnsFilter() []syscall.SockFilter {
	ldH := syscall.BPF_LD | syscall.BPF_H | syscall.BPF_ABS
	ldB := syscall.BPF_LD | syscall.BPF_B | syscall.BPF_ABS
	jeq := syscall.BPF_JMP | syscall.BPF_JEQ | syscall.BPF_K
	ret := syscall.BPF_RET | syscall.BPF_K
	return []syscall.SockFilter{
		*syscall.LsfStmt(ldH, 12),           // A = ethertype
		*syscall.LsfJump(jeq, 0x0800, 0, 6), // 非 IPv4 -> reject
		*syscall.LsfStmt(ldB, 23),           // A = ip.proto
		*syscall.LsfJump(jeq, 17, 0, 4),     // 非 UDP -> reject
		*syscall.LsfStmt(ldH, 36),           // A = udp dst port (假定无 IP 选项)
		*syscall.LsfJump(jeq, 53, 3, 0),     // dst==53 -> accept
		*syscall.LsfStmt(ldH, 34),           // A = udp src port
		*syscall.LsfJump(jeq, 53, 1, 0),     // src==53 -> accept
		*syscall.LsfStmt(ret, 0),            // reject
		*syscall.LsfStmt(ret, 65535),        // accept (整包)
	}
}

func (s *DNSSource) loop() {
	buf := make([]byte, 65535)
	for {
		select {
		case <-s.done:
			return
		default:
		}
		// 一次性把内核缓冲里的包读干净（BPF 已把流量压到极小），再睡眠，避免忙轮询。
		drained := false
		for {
			n, _, err := syscall.Recvfrom(s.fd, buf, 0)
			if err != nil {
				if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
					break
				}
				if err == syscall.EINTR {
					continue
				}
				s.errMu.Lock()
				s.err = err
				s.errMu.Unlock()
				return
			}
			if n > 0 {
				drained = true
				if d, ok := parseQueryFrame(buf[:n]); ok {
					s.agg.Add(d)
				}
			}
		}
		if !drained {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// Name 返回采集源名。
func (s *DNSSource) Name() string { return "dns-capture" }

// Snapshot 返回当前统计。
func (s *DNSSource) Snapshot(limit int) Snapshot {
	sn := s.agg.Snapshot(limit)
	sn.Source = "dns-capture"
	sn.Iface = s.iface
	s.errMu.Lock()
	if s.err != nil {
		sn.Error = s.err.Error()
	}
	s.errMu.Unlock()
	return sn
}

// Close 停止抓包。
func (s *DNSSource) Close() error {
	s.once.Do(func() {
		close(s.done)
		syscall.Close(s.fd)
	})
	return nil
}
