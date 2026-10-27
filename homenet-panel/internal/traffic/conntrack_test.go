package traffic

import (
	"net"
	"strings"
	"testing"
	"time"
)

const conntrackSample = `ipv4     2 tcp      6 431999 ESTABLISHED src=192.168.5.83 dst=140.82.112.4 sport=51514 dport=443 src=140.82.112.4 dst=192.168.5.83 sport=443 dport=51514 [ASSURED] mark=0 use=1
ipv4     2 tcp      6 118 ESTABLISHED src=140.82.112.4 dst=192.168.5.83 sport=443 dport=51514 packets=9 bytes=1800 src=192.168.5.83 dst=140.82.112.4 sport=51514 dport=443 packets=6 bytes=900 [ASSURED] mark=0 use=1
ipv4     2 udp      17 29 src=192.168.5.2 dst=192.168.5.83 sport=53 dport=40000 src=192.168.5.83 dst=192.168.5.2 sport=40000 dport=53 mark=0 use=1
ipv4     2 tcp      6 82 ESTABLISHED src=192.168.5.83 dst=192.168.5.70 sport=40001 dport=22 src=192.168.5.70 dst=192.168.5.83 sport=22 dport=40001 [ASSURED] mark=0 use=1
ipv6     2 tcp      6 120 ESTABLISHED src=2408:8256:5087:31e9::1000 dst=2606:4700:4700::1111 sport=51000 dport=853 packets=2 bytes=1024 src=2606:4700:4700::1111 dst=2408:8256:5087:31e9::1000 sport=853 dport=51000 packets=3 bytes=4096 [ASSURED] mark=0 use=1
`

func TestParseConntrack(t *testing.T) {
	got := ParseConntrack(conntrackSample)
	if len(got) != 2 {
		t.Fatalf("按私有地址判断，应只保留 2 条「内网客户端↔公网」连接（公网 IPv6 客户端看不到），得到 %d 条: %+v", len(got), got)
	}
	want := map[string]struct {
		out, in int64
	}{
		"192.168.5.83|140.82.112.4": {900, 1800}, // Out 客户端发出 900，In 接收到 1800
		"192.168.5.83|192.168.5.83": {},
	}
	byKey := map[string]ConnEntry{}
	for _, e := range got {
		byKey[e.Client+"|"+e.Remote] = e
	}
	for k := range want {
		if _, ok := byKey[k]; !ok && k != "192.168.5.83|192.168.5.83" {
			t.Errorf("缺少 %s", k)
		}
	}
	a := byKey["192.168.5.83|140.82.112.4"]
	if a.Out != 900 || a.In != 1800 {
		t.Errorf("方向归一错误：out=%d in=%d", a.Out, a.In)
	}
	if a.Dport != "443" {
		t.Errorf("dport 应为 443，得到 %q", a.Dport)
	}
}

func TestParseConntrackLocalSeesIPv6Clients(t *testing.T) {
	_, lan, err := net.ParseCIDR("2408:8256:5087:31e9::/64")
	if err != nil {
		t.Fatal(err)
	}
	got := ParseConntrackLocal(conntrackSample, []*net.IPNet{lan})
	if len(got) != 3 {
		t.Fatalf("按本机子网判断应保留 3 条（含公网 IPv6 客户端），得到 %d 条: %+v", len(got), got)
	}
	var v6 *ConnEntry
	for i := range got {
		if strings.Contains(got[i].Remote, "2606:") {
			v6 = &got[i]
		}
	}
	if v6 == nil {
		t.Fatalf("没解析出 IPv6 客户端连接：%+v", got)
	}
	if v6.Client != "2408:8256:5087:31e9::1000" || v6.Out != 1024 || v6.In != 4096 || v6.Dport != "853" {
		t.Errorf("IPv6 客户端归一错误：%+v", *v6)
	}
}

func TestParseConntrackSkipsNoise(t *testing.T) {
	for _, s := range []string{"", "nf_conntrack version 0.5.0 (65536 buckets, 262144 max)", "bridge 2 0"} {
		if got := ParseConntrack(s); len(got) != 0 {
			t.Errorf("%q 应被跳过，得到 %+v", s, got)
		}
	}
}

func TestCollectorClientAggregation(t *testing.T) {
	c := NewConnCollector(time.Second, "", nil)
	defer c.Close()
	c.SockFn = func() ([]Sock, error) { return nil, nil }
	c.IfaceFn = func() (int64, int64, error) { return 0, 0, nil }

	clientConn := func(out, in int64) []ConnEntry {
		return []ConnEntry{{Proto: "tcp", Client: "192.168.5.83", Remote: "140.82.112.4", Dport: "443", Out: out, In: in}}
	}

	// 第一次采样：只记基线
	c.ClientFn = func() ([]ConnEntry, error) { return clientConn(1000, 2000), nil }
	if err := c.Sample(); err != nil {
		t.Fatal(err)
	}
	sn := c.Snapshot()
	if len(sn.Clients) != 1 || sn.Clients[0].Tx != 0 || sn.Clients[0].Rx != 0 {
		t.Fatalf("首轮应只建基线，得到 %+v", sn.Clients)
	}
	if sn.ClientSource != "conntrack" || !strings.Contains(sn.Source, "conntrack") {
		t.Errorf("数据源标识不对：%q / %q", sn.Source, sn.ClientSource)
	}

	// 第二次采样：增量应记到客户端行
	c.ClientFn = func() ([]ConnEntry, error) { return clientConn(3000, 6000), nil }
	if err := c.Sample(); err != nil {
		t.Fatal(err)
	}
	sn = c.Snapshot()
	r := sn.Clients[0]
	if r.Tx != 2000 || r.Rx != 4000 {
		t.Fatalf("增量累计错误：tx=%d rx=%d", r.Tx, r.Rx)
	}
	if r.Conns != 1 {
		t.Errorf("连接数应为 1，得到 %d", r.Conns)
	}
	if !strings.Contains(r.Detail, "140.82.112.4:443") {
		t.Errorf("明细里应含主要目的，得到 %q", r.Detail)
	}
	if r.RateTx <= 0 || r.RateRx <= 0 {
		t.Errorf("速率应为正：%+v", r)
	}

	// 第三次采样：连接消失 → 累计保留、连接数与速率归零
	c.ClientFn = func() ([]ConnEntry, error) { return nil, nil }
	if err := c.Sample(); err != nil {
		t.Fatal(err)
	}
	sn = c.Snapshot()
	if sn.Clients[0].Tx != 2000 || sn.Clients[0].Conns != 0 || sn.Clients[0].RateTx != 0 {
		t.Errorf("连接消失后的行不对：%+v", sn.Clients[0])
	}
}

func TestCollectorConntrackErrorKeepsSocketPath(t *testing.T) {
	c := NewConnCollector(time.Second, "", nil)
	defer c.Close()
	sent := int64(500)
	recv := int64(100)
	c.SockFn = func() ([]Sock, error) {
		return []Sock{{Proto: "tcp", Local: "192.168.5.2:40000", Peer: "140.82.112.4:443", Tx: sent, Rx: recv}}, nil
	}
	c.IfaceFn = func() (int64, int64, error) { return 0, 0, nil }
	c.ClientFn = func() ([]ConnEntry, error) { return nil, errNoConntrack }
	if err := c.Sample(); err != nil {
		t.Fatal(err)
	}
	sent, recv = 900, 400
	c.Sample()
	sn := c.Snapshot()
	if sn.ClientSource != "" || sn.ClientError == "" {
		t.Errorf("conntrack 不可用时应给出原因：%+v", sn)
	}
	if len(sn.Nodes) != 1 {
		t.Errorf("节点表不应受 conntrack 影响：%+v", sn.Nodes)
	}
}

var errNoConntrack = errString("no such file")

type errString string

func (e errString) Error() string { return string(e) }

func TestCollectorAcctEnabledButNoBytes(t *testing.T) {
	c := NewConnCollector(time.Second, "", nil)
	defer c.Close()
	c.SockFn = func() ([]Sock, error) { return nil, nil }
	c.IfaceFn = func() (int64, int64, error) { return 0, 0, nil }
	c.AcctFn = func() bool { return true }

	entries := []ConnEntry{{Proto: "tcp", Client: "192.168.5.83", Remote: "140.82.112.4", Dport: "443"}}
	c.ClientFn = func() ([]ConnEntry, error) { return entries, nil }
	if err := c.Sample(); err != nil {
		t.Fatal(err)
	}
	if err := c.Sample(); err != nil {
		t.Fatal(err)
	}
	sn := c.Snapshot()
	if len(sn.Clients) != 1 || sn.Clients[0].Tx != 0 {
		t.Fatalf("没有字节计数时应为 0：%+v", sn.Clients)
	}
	if !strings.Contains(sn.Clients[0].Detail, "140.82.112.4:443") {
		t.Errorf("应回落到连接数口径的明细，得到 %q", sn.Clients[0].Detail)
	}
	if !strings.Contains(sn.ClientNote, "没有真的往表里写字节数") {
		t.Errorf("应提示字节不可用，得到 %q", sn.ClientNote)
	}
}

func TestCollectorAcctWithBytesReportsNoteFree(t *testing.T) {
	c := NewConnCollector(time.Second, "", nil)
	defer c.Close()
	c.SockFn = func() ([]Sock, error) { return nil, nil }
	c.IfaceFn = func() (int64, int64, error) { return 0, 0, nil }
	c.AcctFn = func() bool { return true }

	c.ClientFn = func() ([]ConnEntry, error) {
		return []ConnEntry{{Proto: "tcp", Client: "192.168.5.83", Remote: "140.82.112.4", Dport: "443", Out: 1000, In: 2000}}, nil
	}
	if err := c.Sample(); err != nil {
		t.Fatal(err)
	}
	c.ClientFn = func() ([]ConnEntry, error) {
		return []ConnEntry{{Proto: "tcp", Client: "192.168.5.83", Remote: "140.82.112.4", Dport: "443", Out: 3000, In: 6000}}, nil
	}
	if err := c.Sample(); err != nil {
		t.Fatal(err)
	}
	sn := c.Snapshot()
	if sn.Clients[0].Tx != 2000 || sn.Clients[0].Rx != 4000 {
		t.Fatalf("字节增量错误：%+v", sn.Clients[0])
	}
	if sn.ClientNote != "" {
		t.Errorf("可以统计字节时不应有提示，得到 %q", sn.ClientNote)
	}
}
