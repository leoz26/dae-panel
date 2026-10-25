package model

import (
	"fmt"
	"regexp"
	"strings"
)

// BuiltinOutbounds 是 dae 的四个内置出站。
// 实测（dae v2.1.1，dae validate）：direct / block / must_direct / must_block 都可直接作为出站；
// 而 reject 不是出站，写成 `-> reject` 会得到 outbound (group) \"reject\" not found。
var BuiltinOutbounds = []string{"direct", "block", "must_direct", "must_block"}

// IsBuiltinOutbound 判断名称是否为 dae 内置出站。
func IsBuiltinOutbound(name string) bool {
	for _, b := range BuiltinOutbounds {
		if name == b {
			return true
		}
	}
	return false
}

// Issue 是一条模型静态检查结论（不阻断保存，供界面提示与 apply 前把关）。
type Issue struct {
	Level   string `json:"level"` // error / warn
	Where   string `json:"where"`
	Message string `json:"message"`
}

// Lint 检查出站引用、空规则、fallback，返回发现的全部问题。
func Lint(m *Model) []Issue {
	if m == nil {
		return nil
	}
	groups := map[string]bool{}
	for _, g := range m.Groups {
		groups[strings.TrimSpace(g.Name)] = true
	}
	var out []Issue
	seen := map[string]bool{}
	add := func(level, where, msg string) {
		key := level + "|" + where + "|" + msg
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, Issue{Level: level, Where: where, Message: msg})
	}
	for i, r := range m.Rules {
		where := fmt.Sprintf("规则 %d", i+1)
		matcher := strings.TrimSpace(r.Matcher)
		outName := strings.TrimSpace(r.Out)
		if matcher == "" && outName == "" {
			continue
		}
		if matcher == "" {
			add("error", where, "匹配器为空")
		}
		if outName == "" {
			add("error", where, "出站为空")
			continue
		}
		if outName == "reject" {
			add("error", where, "dae 没有 reject 出站，请改用 block（丢包）或 direct")
			continue
		}
		if IsBuiltinOutbound(outName) || groups[outName] {
			continue
		}
		add("error", where, fmt.Sprintf("出站 %q 未定义：既不是内置出站（%s），也不是已定义的代理组", outName, strings.Join(BuiltinOutbounds, "/")))
	}
	fb := strings.TrimSpace(m.Fallback)
	if fb != "" && !IsBuiltinOutbound(fb) && !groups[fb] {
		add("error", "fallback", fmt.Sprintf("兜底出站 %q 未定义", fb))
	}
	return out
}

var geoTagsRe = regexp.MustCompile(`geosite:([A-Za-z0-9_.!@+\-]+)`)

// GeoRefs 返回规则里引用的所有 geosite tag（保序去重，保持原始大小写）。
func GeoRefs(m *Model) []string {
	if m == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, r := range m.Rules {
		for _, mm := range geoTagsRe.FindAllStringSubmatch(r.Matcher, -1) {
			tag := strings.TrimSpace(mm[1])
			key := strings.ToLower(tag)
			if tag == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, tag)
		}
	}
	return out
}

// LintGeo 用本机 geosite.dat 的 tag 集合校验规则引用；tags 为 nil（数据缺失）时不做判断。
// 起因：旧配置里的 geosite:steam@cn / geosite:microsoft@cn 在当前数据源里根本不存在，
// dae validate 不报错、运行期也静默不匹配 —— 属于看不见的死规则。
func LintGeo(m *Model, tags map[string]bool) []Issue {
	if m == nil || tags == nil {
		return nil
	}
	var out []Issue
	for _, tag := range GeoRefs(m) {
		if tags[strings.ToLower(tag)] {
			continue
		}
		out = append(out, Issue{Level: "error", Where: "geosite:" + tag, Message: fmt.Sprintf("数据源里没有 %q 这个 tag，这条规则永远不会命中（可改用别的 tag 或写成具体域名）", tag)})
	}
	return out
}
