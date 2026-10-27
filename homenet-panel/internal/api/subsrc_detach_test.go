package api

import (
	"testing"

	"homenet-panel/internal/model"
)

func TestDetachSubscription(t *testing.T) {
	m := &model.Model{
		Subscriptions: []model.Subscription{{Name: "dual", Source: "file://subs/dual.sub"}, {Name: "CF", Source: "file://subs/CF.sub"}},
		Groups: []model.Group{
			{Name: "hk", Filters: []string{"subtag(dual) && name(keyword: '香港')", "subtag(dual) && name(keyword: 'hk')"}},
			{Name: "CF", Filters: []string{"subtag(CF)"}},
			{Name: "sg", Filters: []string{"name(keyword: '新加坡')"}},
		},
	}
	removed, warn := detachSubscription(m, "dual")
	if removed != 1 {
		t.Fatalf("应摘掉 1 条订阅，实际 %d", removed)
	}
	if len(m.Subscriptions) != 1 || m.Subscriptions[0].Name != "CF" {
		t.Fatalf("剩余订阅不对: %+v", m.Subscriptions)
	}
	if len(warn) != 1 || warn[0] != "hk" {
		t.Fatalf("应只提示 hk 分组，实际 %v", warn)
	}
	// 幂等：再摘一次没有可摘的，也不该崩
	removed2, _ := detachSubscription(m, "dual")
	if removed2 != 0 {
		t.Fatalf("重复摘除应为 0，实际 %d", removed2)
	}
}
