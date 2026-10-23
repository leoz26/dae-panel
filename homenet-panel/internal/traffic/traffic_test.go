package traffic

import (
	"testing"
)

func TestAggregatorOrdering(t *testing.T) {
	a := NewAggregator()
	for _, d := range []string{"b.com", "a.com", "a.com", "c.com", "a.com"} {
		a.Add(d)
	}
	sn := a.Snapshot(2)
	if sn.Queries != 5 {
		t.Fatalf("queries=%d want 5", sn.Queries)
	}
	if sn.Domains != 3 {
		t.Fatalf("domains=%d want 3", sn.Domains)
	}
	if len(sn.Hits) != 2 {
		t.Fatalf("hits=%d want 2 (limit)", len(sn.Hits))
	}
	if sn.Hits[0].Domain != "a.com" || sn.Hits[0].Count != 3 {
		t.Fatalf("top hit=%+v want a.com/3", sn.Hits[0])
	}
}

// buildDNSQuery 构造一个以太网/IPv4/UDP/DNS 查询帧。
func buildDNSQuery(domain string, dstPort int) []byte {
	// DNS
	dns := []byte{0x12, 0x34, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	for _, lab := range splitLabels(domain) {
		dns = append(dns, byte(len(lab)))
		dns = append(dns, []byte(lab)...)
	}
	dns = append(dns, 0x00, 0x00, 0x01, 0x00, 0x01)
	// UDP
	udpLen := 8 + len(dns)
	udp := []byte{0x30, 0x39, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	udp[2] = byte((dstPort >> 8) & 0xff)
	udp[3] = byte(dstPort & 0xff)
	udp[4] = byte((udpLen >> 8) & 0xff)
	udp[5] = byte(udpLen & 0xff)
	udp = append(udp, dns...)
	// IPv4
	total := 20 + len(udp)
	ip := make([]byte, 20)
	ip[0] = 0x45
	ip[2] = byte((total >> 8) & 0xff)
	ip[3] = byte(total & 0xff)
	ip[8] = 64
	ip[9] = 17
	copy(ip[12:16], []byte{192, 168, 1, 85})
	copy(ip[16:20], []byte{223, 5, 5, 5})
	// Ethernet
	eth := make([]byte, 14)
	eth[12] = 0x08
	eth[13] = 0x00
	return append(eth, append(ip, udp...)...)
}

func splitLabels(d string) []string {
	var out []string
	cur := ""
	for _, r := range d {
		if r == '.' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func TestParseQueryFrame(t *testing.T) {
	pkt := buildDNSQuery("www.Example.COM", 53)
	got, ok := parseQueryFrame(pkt)
	if !ok {
		t.Fatal("expected ok")
	}
	if got != "www.example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestParseQueryFrameNotQuery(t *testing.T) {
	// 目的端口不是 53（DNS 响应方向）应被忽略
	pkt := buildDNSQuery("a.com", 5353)
	if _, ok := parseQueryFrame(pkt); ok {
		t.Fatal("expected not-ok for non-53 dst port")
	}
	// 非 IPv4
	bad := make([]byte, 40)
	bad[12] = 0x86
	bad[13] = 0xdd
	if _, ok := parseQueryFrame(bad); ok {
		t.Fatal("expected not-ok for IPv6")
	}
}
