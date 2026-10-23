package subs

// AggStats 聚合统计。
type AggStats struct {
	Total      int `json:"total"`      // 合并前总数
	Unique     int `json:"unique"`     // 去重后
	Duplicates int `json:"duplicates"` // 被去掉的重复数
}

// Aggregate 合并多个订阅的节点并按完整链接去重（保留首次出现）。
func Aggregate(sources []Source) ([]Node, AggStats) {
	seen := make(map[string]bool)
	var out []Node
	st := AggStats{}
	for _, s := range sources {
		for _, nd := range s.Nodes {
			st.Total++
			key := nd.Raw
			if key == "" {
				key = nd.Scheme + "|" + nd.Host + "|" + nd.Port + "|" + nd.Tag
			}
			if seen[key] {
				st.Duplicates++
				continue
			}
			seen[key] = true
			out = append(out, nd)
		}
	}
	st.Unique = len(out)
	return out, st
}
