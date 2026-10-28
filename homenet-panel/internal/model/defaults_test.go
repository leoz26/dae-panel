package model

import (
	"strings"
	"testing"
)

const defaultsCore = `global {
    dial_mode: domain
    tproxy_port: 12345
}

group {
    proxy {
        policy: min_avg10
        filter: subtag(summer)
        check_interval: 30s
        check_tolerance: 1s
    }
}

dns {
    upstream {
        alidns: 'udp://223.5.5.5:53'
    }
    routing {
        request {
            fallback: alidns
        }
    }
}

routing {
    fallback: proxy
}
`

func TestParseGroupsKeepsExtra(t *testing.T) {
	m, err := Parse("", defaultsCore)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(m.Groups) != 1 {
		t.Fatalf("分组数 = %d，期望 1", len(m.Groups))
	}
	extra := strings.Join(m.Groups[0].Extra, "|")
	if !strings.Contains(extra, "check_interval: 30s") || !strings.Contains(extra, "check_tolerance: 1s") {
		t.Fatalf("组级调优参数没被保留：%q", extra)
	}
	_, core := Render(m)
	if !strings.Contains(core, "check_interval: 30s") {
		t.Fatalf("渲染结果丢了组级调优参数：\n%s", core)
	}
}

func TestEnsureGlobalDefaultsInjectsMissingKeys(t *testing.T) {
	out, added := EnsureGlobalDefaults("    dial_mode: domain\n", "dns {\n    upstream {\n        alidns: 'udp://223.5.5.5:53'\n    }\n}\n")
	for _, k := range []string{"check_interval", "check_tolerance", "tcp_check_url", "bootstrap_resolver"} {
		if !strings.Contains(out, k) {
			t.Fatalf("缺 %s：\n%s", k, out)
		}
		if !containsStr(added, k) {
			t.Fatalf("added 里没有 %s：%v", k, added)
		}
	}
	if !strings.Contains(out, "bootstrap_resolver: '223.5.5.5:53'") {
		t.Fatalf("bootstrap_resolver 推导不对：\n%s", out)
	}
	if !strings.Contains(out, "    dial_mode: domain") {
		t.Fatalf("原有内容被破坏：\n%s", out)
	}
}

func TestEnsureGlobalDefaultsKeepsExistingValues(t *testing.T) {
	in := "    check_interval: 15s\n    check_tolerance: 100ms\n"
	out, added := EnsureGlobalDefaults(in, "")
	if !strings.Contains(out, "check_tolerance: 100ms") || strings.Contains(out, "check_tolerance: 500ms") {
		t.Fatalf("现场值被覆盖：\n%s", out)
	}
	if !strings.Contains(out, "check_interval: 15s") {
		t.Fatalf("现场值被覆盖：\n%s", out)
	}
	if containsStr(added, "check_tolerance") || containsStr(added, "check_interval") {
		t.Fatalf("不该报告补入：%v", added)
	}
	// 幂等
	out2, added2 := EnsureGlobalDefaults(out, "")
	if out2 != out || len(added2) != 0 {
		t.Fatalf("不幂等：%v\n%s", added2, out2)
	}
}

func TestDeriveBootstrapResolver(t *testing.T) {
	cases := []struct{ dns, want string }{
		{"dns {\n    upstream {\n        alidns: 'udp://223.5.5.5:53'\n    }\n}", "223.5.5.5:53"},
		{"dns {\n    upstream {\n        cf: 'tls://1.1.1.1:853'\n    }\n}", "1.1.1.1:853"},
		{"dns {\n    upstream {\n        doh: 'https://dns.alidns.com/dns-query'\n    }\n}", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := DeriveBootstrapResolver(c.dns); got != c.want {
			t.Fatalf("DeriveBootstrapResolver(%q) = %q，期望 %q", c.dns, got, c.want)
		}
	}
}

func TestEnsureDNSGroupRule(t *testing.T) {
	rules := []Rule{{Matcher: "domain(geosite:cn)", Out: "direct"}}
	withGroup := []Group{{Name: DNSGroupName, Policy: "min_avg10"}}
	got, added := EnsureDNSGroupRule(rules, withGroup)
	if len(got) != 3 {
		t.Fatalf("规则数 = %d，期望 3：%v", len(got), got)
	}
	if got[1].Matcher != "dip(8.8.8.8, 8.8.4.4) && dport(53)" || got[1].Out != DNSGroupName {
		t.Fatalf("DNS 组规则不对：%+v", got[1])
	}
	if got[2].Matcher != "domain(geosite:cn)" {
		t.Fatalf("原规则被挤到后面以外的地方：%v", got)
	}
	if len(added) != 2 {
		t.Fatalf("added = %v", added)
	}
	// 没有 DNS 组时只补公共 DNS 直连
	got2, _ := EnsureDNSGroupRule(rules, []Group{{Name: "proxy"}})
	if len(got2) != 2 || got2[0].Out != "direct" {
		t.Fatalf("没有 dnsnode 组时不该加 DNS 组规则：%v", got2)
	}
	// 幂等
	again, added3 := EnsureDNSGroupRule(got, withGroup)
	if len(again) != len(got) || len(added3) != 0 {
		t.Fatalf("不幂等：%v", added3)
	}
}

func TestDNSGroupTemplate(t *testing.T) {
	g := DNSGroupTemplate("summer", []string{"香港"})
	if g.Name != DNSGroupName || g.Policy != "min_avg10" {
		t.Fatalf("模板不对：%+v", g)
	}
	if len(g.Filters) != 1 || g.Filters[0] != "subtag(summer) && name(keyword: '香港')" {
		t.Fatalf("filter 不对：%v", g.Filters)
	}
	if len(g.Extra) != 2 {
		t.Fatalf("模板缺组长健康参数：%v", g.Extra)
	}
}

func containsStr(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
