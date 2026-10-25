package model

import "testing"

func TestLintOutbounds(t *testing.T) {
	m := &Model{
		Groups:   []Group{{Name: "hk", Policy: "min_moving_avg"}},
		Fallback: "direct",
		Rules: []Rule{
			{Matcher: "pname(dnsmasq)", Out: "must_direct"},
			{Matcher: "domain(geosite:cn)", Out: "direct"},
			{Matcher: "domain(geosite:youtube)", Out: "hk"},
			{Matcher: "domain(suffix:claude.ai)", Out: "sg"},
			{Matcher: "l4proto(udp) && dport(443)", Out: "reject"},
			{Matcher: "dport(53)", Out: ""},
		},
	}
	issues := Lint(m)
	errs := 0
	msgs := ""
	for _, it := range issues {
		if it.Level == "error" {
			errs++
		}
		msgs += it.Message + "\n"
	}
	if errs != 3 {
		t.Fatalf("期望 3 条 error（sg 未定义 / reject / 出站为空），实际 %d：%+v", errs, issues)
	}
	if !contains(msgs, "reject") || !contains(msgs, "\"sg\"") {
		t.Fatalf("提示文案缺失: %s", msgs)
	}
}

func TestLintAcceptsAllBuiltins(t *testing.T) {
	var rules []Rule
	for _, b := range BuiltinOutbounds {
		rules = append(rules, Rule{Matcher: "domain(suffix:" + b + ".test)", Out: b})
	}
	if issues := Lint(&Model{Rules: rules, Fallback: "direct"}); len(issues) != 0 {
		t.Fatalf("内置出站不应报错: %+v", issues)
	}
}

func TestLintGeo(t *testing.T) {
	m := &Model{Rules: []Rule{
		{Matcher: "domain(geosite:cn, geosite:apple-cn)", Out: "direct"},
		{Matcher: "domain(geosite:steam@cn)", Out: "direct"},
		{Matcher: "domain(geosite:CN)", Out: "direct"},
	}}
	issues := LintGeo(m, map[string]bool{"cn": true, "apple-cn": true})
	if len(issues) != 1 || !contains(issues[0].Message, "steam@cn") {
		t.Fatalf("期望只有 steam@cn 报错: %+v", issues)
	}
	refs := GeoRefs(m)
	if len(refs) != 3 || refs[0] != "cn" {
		t.Fatalf("GeoRefs 错误: %v", refs)
	}
	if LintGeo(m, nil) != nil {
		t.Fatal("tags 为 nil 时应跳过校验")
	}
}
