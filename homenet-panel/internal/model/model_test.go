package model

import (
	"strings"
	"testing"
)

const coreSample = Header + `
global {
    dial_mode: domain
    tproxy_port: 12345
    wan_interface: auto
}

group {
    hk {
        policy: min_moving_avg
        filter: subtag(dual) && name(keyword: '香港')
    }
    proxy {
        policy: min_moving_avg
        filter: subtag(CF)
        filter: subtag(dual)
    }
}

dns {
    upstream {
        cn: 'udp://223.5.5.5:53'
    }
    routing {
        request {
            fallback: cn
        }
        response {
            fallback: accept
        }
    }
}

routing {
    pname(goose) -> direct
    domain(geosite:youtube) -> hk
    fallback: proxy
}
`

const entrySample = Header + `
include {
    config.d/*.dae
}

subscription {
    CF: 'file://subs/CF.sub'
    dual: 'file://subs/dual.sub'
}
`

func TestParseRenderRoundTrip(t *testing.T) {
	m, err := Parse(entrySample, coreSample)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Subscriptions) != 2 || m.Subscriptions[0].Name != "CF" || m.Subscriptions[0].Source != "file://subs/CF.sub" {
		t.Fatalf("订阅解析错误: %+v", m.Subscriptions)
	}
	if len(m.Groups) != 2 {
		t.Fatalf("分组数量 want 2 got %d: %+v", len(m.Groups), m.Groups)
	}
	if m.Groups[1].Name != "proxy" || len(m.Groups[1].Filters) != 2 {
		t.Fatalf("proxy 组解析错误: %+v", m.Groups[1])
	}
	if len(m.Rules) != 2 || m.Rules[1].Matcher != "domain(geosite:youtube)" || m.Rules[1].Out != "hk" {
		t.Fatalf("规则解析错误: %+v", m.Rules)
	}
	if m.Fallback != "proxy" {
		t.Fatalf("fallback want proxy got %q", m.Fallback)
	}
	if !contains(m.DNS, "udp://223.5.5.5:53") {
		t.Fatalf("dns 未解析: %q", m.DNS)
	}
	if !contains(m.Global, "tproxy_port: 12345") {
		t.Fatalf("global 未解析: %q", m.Global)
	}

	entry, core := Render(m)
	m2, err := Parse(entry, core)
	if err != nil {
		t.Fatal(err)
	}
	entry2, core2 := Render(m2)
	if entry != entry2 || core != core2 {
		t.Fatalf("二次渲染不一致\nentry:\n%s\n---\n%s\ncore:\n%s\n---\n%s", entry, entry2, core, core2)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (len(sub) == 0 || indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestEssentialRulesCoverDHCPBroadcast(t *testing.T) {
	found := false
	for _, r := range EssentialRules {
		if strings.Contains(r.Matcher, "255.255.255.255") && r.Out == "direct" {
			found = true
		}
	}
	if !found {
		t.Fatalf("DHCP 广播 255.255.255.255/32 必须直连，实际 %+v", EssentialRules)
	}
}

func TestEnsureEssentials(t *testing.T) {
	// 缺失时前置补入
	rules := []Rule{{Matcher: "domain(geosite:youtube)", Out: "hk"}}
	out, added := EnsureEssentials(rules)
	if len(added) != len(EssentialRules) {
		t.Fatalf("want %d added got %v", len(EssentialRules), added)
	}
	if len(out) != len(EssentialRules)+1 {
		t.Fatalf("want %d rules got %d", len(EssentialRules)+1, len(out))
	}
	if out[0].Matcher != EssentialRules[0].Matcher || out[len(out)-1].Matcher != "domain(geosite:youtube)" {
		t.Fatalf("essentials not prepended: %+v", out)
	}
	// 已存在则不重复
	again, added2 := EnsureEssentials(out)
	if len(added2) != 0 || len(again) != len(out) {
		t.Fatalf("want no-op got added=%v len=%d", added2, len(again))
	}
	// Render 里也会带上
	_, core := Render(&Model{Rules: rules, Fallback: "proxy"})
	for _, e := range EssentialRules {
		if !strings.Contains(core, e.Matcher+" -> direct") {
			t.Fatalf("rendered core missing %q:\n%s", e.Matcher, core)
		}
	}
}
