package route

import (
	"testing"

	"homenet-panel/internal/geo"
)

func mustDB(t *testing.T) *geo.DB {
	t.Helper()
	// 复用 geo 包测试的构造不便，这里直接构造最小数据（suffix example.com）。
	// 用与 geo 包相同的 protobuf 手工编码。
	data := list(geoSite("g", domainMsg(2, "example.com")))
	db, err := geo.Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return db
}

func encVarint(x uint64) []byte {
	var out []byte
	for x >= 0x80 {
		out = append(out, byte(x)|0x80)
		x >>= 7
	}
	return append(out, byte(x))
}
func tag(field, wt int) []byte { return encVarint(uint64(field<<3 | wt)) }
func ld(field int, b []byte) []byte {
	out := tag(field, 2)
	out = append(out, encVarint(uint64(len(b)))...)
	return append(out, b...)
}
func vi(field int, v uint64) []byte { out := tag(field, 0); return append(out, encVarint(v)...) }
func domainMsg(typ int, val string) []byte {
	var b []byte
	b = append(b, vi(1, uint64(typ))...)
	b = append(b, ld(2, []byte(val))...)
	return b
}
func geoSite(code string, ds ...[]byte) []byte {
	var b []byte
	b = append(b, ld(1, []byte(code))...)
	for _, d := range ds {
		b = append(b, ld(2, d)...)
	}
	return b
}
func list(gs ...[]byte) []byte {
	var b []byte
	for _, g := range gs {
		b = append(b, ld(1, g)...)
	}
	return b
}

func TestEvalPredicates(t *testing.T) {
	db := mustDB(t)
	cases := []struct {
		expr string
		host string
		want val
	}{
		{"domain(suffix:example.com)", "a.example.com", vYes},
		{"domain(full:example.com)", "a.example.com", vNo},
		{"domain(keyword:ample)", "example.com", vYes},
		{"domain(geosite:g)", "x.example.com", vYes},
		{"domain(geosite:g)", "nope.org", vNo},
		{"domain(geosite:nope)", "example.com", vUnk},
		{"domain(suffix:example.com) && domain(keyword:zzz)", "example.com", vNo},
		{"domain(suffix:example.com) || domain(keyword:zzz)", "example.com", vYes},
		{"!domain(suffix:example.com)", "example.com", vNo},
		{"ip(1.1.1.1)", "example.com", vUnk},
		{"pname(goose)", "example.com", vCtx},
		{"domain(suffix:x.com) || ip(1.1.1.1)", "example.com", vUnk},
		{"domain(suffix:a.com, suffix:b.com)", "x.b.com", vYes},
		{"domain(suffix: claude.ai, suffix: anthropic.com)", "api.anthropic.com", vYes},
		{"domain(geosite:g, geosite:nope)", "x.example.com", vYes},
		{"domain(geosite:nope, geosite:nope2)", "x.example.com", vUnk},
	}
	for _, c := range cases {
		if got := Eval(c.expr, c.host, db); got != c.want {
			t.Errorf("Eval(%q,%s)=%d want %d", c.expr, c.host, got, c.want)
		}
	}
}

func TestTestResult(t *testing.T) {
	db := mustDB(t)
	rules := []Rule{
		{Matcher: "domain(geosite:g)", Out: "proxy"},
		{Matcher: "domain(keyword:zzz)", Out: "direct"},
	}
	res := Test("a.example.com", rules, "fallback", db)
	if !res.Decided || res.Chosen != "proxy" {
		t.Errorf("decided=%v chosen=%q reason=%q", res.Decided, res.Chosen, res.Reason)
	}
	// 不命中任何规则 → fallback
	rules2 := []Rule{{Matcher: "domain(suffix:other.com)", Out: "direct"}}
	res2 := Test("foo.org", rules2, "fallback", db)
	if !res2.Decided || res2.Chosen != "fallback" {
		t.Errorf("fallback decided=%v chosen=%q", res2.Decided, res2.Chosen)
	}
	// 未知规则在前 → 未决
	rules3 := []Rule{
		{Matcher: "ip(1.1.1.1)", Out: "a"},
		{Matcher: "domain(geosite:g)", Out: "b"},
	}
	res3 := Test("x.example.com", rules3, "fb", db)
	if res3.Decided {
		t.Errorf("expected undecided when unknown before match")
	}
	// 上下文谓词（pname）在域名维度不参与判定，不应阻断结论
	rules4 := []Rule{
		{Matcher: "pname(goose)", Out: "direct"},
		{Matcher: "domain(geosite:g)", Out: "proxy"},
	}
	res4 := Test("x.example.com", rules4, "fb", db)
	if !res4.Decided || res4.Chosen != "proxy" {
		t.Errorf("ctx pred should not block; decided=%v chosen=%q", res4.Decided, res4.Chosen)
	}
}

func TestNormalizeHost(t *testing.T) {
	cases := map[string]string{
		"https://www.claude.com/":    "www.claude.com",
		"http://example.com/a?b=1#c": "example.com",
		"example.com:443":            "example.com",
		"  Example.COM.  ":           "example.com",
		"*.example.com":              "example.com",
		"[::1]:443":                  "::1",
		"user:pw@example.com/path":   "example.com",
		"telegram.org":               "telegram.org",
	}
	for in, want := range cases {
		if got := NormalizeHost(in); got != want {
			t.Errorf("NormalizeHost(%q)=%q，期望 %q", in, got, want)
		}
	}
	if got := Test("https://www.claude.com/", []Rule{{Matcher: "domain(suffix: claude.com)", Out: "ai"}}, "direct", nil); got.Chosen != "ai" || !got.Decided {
		t.Errorf("带协议的 URL 也要能命中：%+v", got)
	}
}
