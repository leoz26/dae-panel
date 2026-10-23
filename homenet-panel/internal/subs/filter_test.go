package subs

import "testing"

func TestMatchGroupReal(t *testing.T) {
	hk := Node{Subtag: "dual", Tag: "🇭🇰 广东-香港 DP"}
	sg := Node{Subtag: "dual", Tag: "🇸🇬 新加坡 01"}
	cf := Node{Subtag: "CF", Tag: "comet95.cf"}

	if !MatchGroup([]string{"subtag(dual) && name(keyword: '香港')"}, hk) {
		t.Errorf("hk should match")
	}
	if MatchGroup([]string{"subtag(dual) && name(keyword: '香港')"}, sg) {
		t.Errorf("sg should not match hk filter")
	}
	if !MatchGroup([]string{"subtag(CF)", "subtag(dual)"}, cf) {
		t.Errorf("cf should match multi-line OR")
	}
	if !MatchGroup([]string{"subtag(CF)", "subtag(dual)"}, sg) {
		t.Errorf("sg should match via subtag(dual)")
	}
}

func TestMatchGroupRegexAndParen(t *testing.T) {
	n := Node{Subtag: "dual", Tag: "香港 01"}
	if !MatchGroup([]string{"name(regex: '香港|新加坡') && (subtag(dual) || subtag(CF))"}, n) {
		t.Errorf("regex+paren should match")
	}
	if MatchGroup([]string{"subtag(CF) && name(suffix: 'x')"}, n) {
		t.Errorf("should not match")
	}
}

func TestAggregate(t *testing.T) {
	a := Source{Name: "a", Nodes: []Node{
		{Raw: "vless://1", Host: "h1"}, {Raw: "vless://2", Host: "h2"},
	}}
	b := Source{Name: "b", Nodes: []Node{
		{Raw: "vless://2", Host: "h2"}, {Raw: "vless://3", Host: "h3"},
	}}
	merged, st := Aggregate([]Source{a, b})
	if st.Total != 4 || st.Unique != 3 || st.Duplicates != 1 {
		t.Fatalf("stats=%+v", st)
	}
	if len(merged) != 3 {
		t.Fatalf("merged=%d", len(merged))
	}
}
