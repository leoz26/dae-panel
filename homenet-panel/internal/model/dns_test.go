package model

import (
	"reflect"
	"strings"
	"testing"
)

const liveDNS = `    upstream {
        cn: 'udp://223.5.5.5:53'
    }
    routing {
        request {
            fallback: cn
        }
        response {
            fallback: accept
        }
    }`

const richDNS = `# 由面板托管
    upstream {
        alidsns: 'udp://223.5.5.5'
        googledns: "tcp://8.8.8.8:53"
    }
    routing {
        request {
            qname(keyword: lucky, geosite:cn) -> alidsns
            qname(suffix: apple.com.cn, suffix: icloud.com.cn) -> alidsns
            qtype(aaaa) -> reject
            fallback: googledns   # 兜底
        }
        response {
            fallback: accept
        }
    }`

func TestParseDNSLiveConfig(t *testing.T) {
	s := ParseDNS(liveDNS)
	if len(s.Upstreams) != 1 || s.Upstreams[0].Name != "cn" || s.Upstreams[0].Addr != "udp://223.5.5.5:53" {
		t.Fatalf("upstreams wrong: %+v", s.Upstreams)
	}
	if s.RequestFallback != "cn" || s.ResponseFallback != "accept" {
		t.Fatalf("fallbacks wrong: %+v", s)
	}
	if len(s.Request) != 0 || len(s.Response) != 0 {
		t.Fatalf("unexpected rules: %+v", s)
	}
}

func TestParseDNSWithRules(t *testing.T) {
	s := ParseDNS(richDNS)
	if len(s.Upstreams) != 2 {
		t.Fatalf("want 2 upstreams, got %+v", s.Upstreams)
	}
	if s.Upstreams[1].Addr != "tcp://8.8.8.8:53" {
		t.Fatalf("quoted addr not unquoted: %+v", s.Upstreams[1])
	}
	if len(s.Request) != 3 {
		t.Fatalf("want 3 request rules, got %+v", s.Request)
	}
	if s.Request[2].Out != "reject" || !strings.HasPrefix(s.Request[2].Matcher, "qtype(aaaa)") {
		t.Fatalf("qtype rule wrong: %+v", s.Request[2])
	}
	if s.RequestFallback != "googledns" {
		t.Fatalf("inline comment not stripped: %q", s.RequestFallback)
	}
}

func TestRenderDNSRoundTrip(t *testing.T) {
	s := ParseDNS(richDNS)
	text := RenderDNS(s)
	if !strings.Contains(text, "alidsns: 'udp://223.5.5.5'") {
		t.Fatalf("render lost upstream:\n%s", text)
	}
	if !strings.Contains(text, "qtype(aaaa) -> reject") {
		t.Fatalf("render lost rule:\n%s", text)
	}
	again := ParseDNS(text)
	if !reflect.DeepEqual(s, again) {
		t.Fatalf("round trip mismatch:\nfirst=%+v\nagain=%+v", s, again)
	}
}

func TestDefaultDNSSpecValid(t *testing.T) {
	s := DefaultDNSSpec()
	if w := LintDNS(s); len(w) != 0 {
		t.Fatalf("default dns should lint clean, got %v", w)
	}
	text := RenderDNS(s)
	if !strings.Contains(text, "cn: 'udp://223.5.5.5:53'") {
		t.Fatalf("render wrong:\n%s", text)
	}
	if !strings.Contains(text, "fallback: cn") || !strings.Contains(text, "fallback: accept") {
		t.Fatalf("render missing fallbacks:\n%s", text)
	}
}

func TestLintDNS(t *testing.T) {
	s := DNSSpec{
		Upstreams:       []DNSUpstream{{Name: "cn", Addr: "223.5.5.5:53"}, {Name: "cn", Addr: "udp://1.1.1.1:53"}},
		Request:         []DNSRule{{Matcher: "qname(geosite:cn)", Out: "typo"}},
		RequestFallback: "missing",
	}
	w := LintDNS(s)
	joined := strings.Join(w, " | ")
	for _, want := range []string{"缺少协议前缀", "名字重复", "不存在"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("warnings should mention %q, got %v", want, w)
		}
	}
}

func TestLintDNSRejectsInvalidActions(t *testing.T) {
	s := DNSSpec{
		Upstreams: []DNSUpstream{{Name: "cn", Addr: "udp://223.5.5.5:53"}},
		Request: []DNSRule{
			{Matcher: "qname(geolocation-cn)", Out: "cn"},
			{Matcher: "qtype(aaaa)", Out: "drop"},
			{Matcher: "qname(geosite:cn)", Out: "accept"},
		},
	}
	joined := strings.Join(LintDNS(s), " | ")
	for _, want := range []string{"geolocation", "drop", "accept"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("warnings should mention %q, got %s", want, joined)
		}
	}
}
