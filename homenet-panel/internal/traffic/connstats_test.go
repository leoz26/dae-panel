package traffic

import (
	"testing"
	"time"
)

const ssSample = `Netid State  Recv-Q Send-Q  Local Address:Port  Peer Address:Port Process
tcp   ESTAB  0      0       192.168.1.85:45210  18.142.105.24:36002 users:(("dae",pid=3489,fd=12))
	 cubic wscale:7,7 rtt:0.5/0.2 mss:1448 cwnd:10 bytes_sent:10240 bytes_received:20480
tcp   ESTAB  0      0       192.168.1.85:45211  56.10.123.33:36500 users:(("dae",pid=3489,fd=13))
	 cubic bytes_sent:100 bytes_received:200
tcp   ESTAB  0      0       192.168.1.85:8787   192.168.1.101:50123 users:(("homenet-panel",pid=3500,fd=8))
	 cubic bytes_sent:999 bytes_received:888
udp   ESTAB  0      0       192.168.1.85:51234  54.250.116.159:36500 users:(("dae",pid=3489,fd=20))
	 bytes_sent:0 bytes_received:0
tcp   ESTAB  0      0       192.168.1.85:40000  1.2.3.4:443
	 bytes_sent:7 bytes_received:9
`

func TestParseSS(t *testing.T) {
	socks := ParseSS(ssSample)
	if len(socks) != 5 { // 含无进程信息的那条（上层按进程过滤）
		t.Fatalf("期望 5 条，实际 %d", len(socks))
	}
	if socks[0].Peer != "18.142.105.24:36002" || socks[0].Tx != 10240 || socks[0].Rx != 20480 {
		t.Fatalf("首条解析错误: %+v", socks[0])
	}
	if !isDaeProc(socks[0].Proc) {
		t.Fatalf("应识别为 dae: %q", socks[0].Proc)
	}
	if isDaeProc(socks[2].Proc) {
		t.Fatal("homenet-panel 的连接不应算作 dae")
	}
}

func TestHostOf(t *testing.T) {
	cases := []struct{ in, host, port string }{
		{"192.168.1.85:45210", "192.168.1.85", "45210"},
		{"[::1]:53", "::1", "53"},
		{"*:*", "*", "*"},
		{"1.2.3.4:443", "1.2.3.4", "443"},
	}
	for _, c := range cases {
		h, p := hostOf(c.in)
		if h != c.host || p != c.port {
			t.Errorf("hostOf(%q) = (%q,%q)，期望 (%q,%q)", c.in, h, p, c.host, c.port)
		}
	}
}

func TestIsPrivateHost(t *testing.T) {
	for host, want := range map[string]bool{
		"192.168.1.101": true, "10.0.0.5": true, "172.16.3.4": true, "127.0.0.1": true,
		"169.254.0.11": true, "100.64.1.1": true, "::1": true, "fe80::1": true, "*": true,
		"18.142.105.24": false, "1.1.1.1": false, "2401:c080::1": false,
	} {
		if got := isPrivateHost(host); got != want {
			t.Errorf("isPrivateHost(%q) = %v，期望 %v", host, got, want)
		}
	}
}

func TestParseNetDev(t *testing.T) {
	data := "Inter-|   Receive                                                |  Transmit\n" +
		" face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed\n" +
		"    lo: 1000 1 0 0 0 0 0 0 1000 1 0 0 0 0 0 0\n" +
		"  dae0: 547000 12 0 0 0 0 0 0 266000 34 0 0 0 0 0 0\n"
	rx, tx, err := parseNetDev(data, "dae0")
	if err != nil || rx != 547000 || tx != 266000 {
		t.Fatalf("解析失败 rx=%d tx=%d err=%v", rx, tx, err)
	}
	if _, _, err := parseNetDev(data, "ens2"); err == nil {
		t.Fatal("不存在的网卡应报错")
	}
}

// fakeSource 用固定序列模拟两次采样。
func fakeCollector(samples [][]Sock) *ConnCollector {
	i := 0
	c := NewConnCollector(10*time.Second, "dae0", func(host, port string) string {
		if host == "18.142.105.24" {
			return "香港-01"
		}
		return ""
	})
	c.SockFn = func() ([]Sock, error) {
		s := samples[i]
		if i < len(samples)-1 {
			i++
		}
		return s, nil
	}
	n := 0
	c.IfaceFn = func() (int64, int64, error) {
		n++
		return int64(1000 * n), int64(500 * n), nil
	}
	return c
}

func TestCollectorDeltaAndAttribution(t *testing.T) {
	node := func(tx, rx int64) Sock {
		return Sock{Proto: "tcp", Local: "192.168.1.85:45210", Peer: "18.142.105.24:36002", Tx: tx, Rx: rx, Proc: `users:(("dae",pid=1,fd=2))`}
	}
	client := func(tx, rx int64) Sock {
		return Sock{Proto: "tcp", Local: "192.168.1.85:45211", Peer: "192.168.1.101:50123", Tx: tx, Rx: rx, Proc: `users:(("dae",pid=1,fd=3))`}
	}
	c := fakeCollector([][]Sock{
		{node(1000, 2000), client(50, 60)},   // 首见：只作基线
		{node(1500, 4000), client(150, 460)}, // 增量：节点 tx+500 rx+2000；客户端 tx+100 rx+400
	})
	if err := c.Sample(); err != nil {
		t.Fatal(err)
	}
	sn := c.Snapshot()
	if sn.TotalRx != 0 || sn.TotalTx != 0 {
		t.Fatalf("首轮应全为 0（只作基线），实际 %+v", sn)
	}
	if sn.Nodes == nil || sn.Clients == nil {
		t.Fatal("快照里 nodes/clients 不能是 nil")
	}
	if err := c.Sample(); err != nil {
		t.Fatal(err)
	}
	sn = c.Snapshot()
	if sn.TotalRx != 2400 || sn.TotalTx != 600 {
		t.Fatalf("累计应为 rx=2400 tx=600，实际 rx=%d tx=%d", sn.TotalRx, sn.TotalTx)
	}
	if len(sn.Nodes) != 1 || sn.Nodes[0].Name != "香港-01" || sn.Nodes[0].Rx != 2000 || sn.Nodes[0].Tx != 500 {
		t.Fatalf("节点行错误: %+v", sn.Nodes)
	}
	if len(sn.Clients) != 1 || sn.Clients[0].Key != "192.168.1.101" || sn.Clients[0].Rx != 400 {
		t.Fatalf("客户端行错误: %+v", sn.Clients)
	}
	if sn.IfaceName != "dae0" || sn.IfaceRx != 2000 || sn.IfaceTx != 1000 {
		t.Fatalf("网卡计数错误: %+v", sn)
	}
}

func TestCollectorDisabled(t *testing.T) {
	c := NewConnCollector(0, "dae0", nil)
	sn := c.Snapshot()
	if sn.Enabled {
		t.Fatal("interval=0 时应为关闭")
	}
	var nilC *ConnCollector
	if nilC.Snapshot().Enabled {
		t.Fatal("nil 采集器应为关闭")
	}
	if err := nilC.Close(); err != nil {
		t.Fatal(err)
	}
}
