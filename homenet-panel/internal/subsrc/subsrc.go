package subsrc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"homenet-panel/internal/clashconv"
	"homenet-panel/internal/subs"
)

// Entry 描述一条订阅的「上游 URL ↔ 本地镜像文件」关系及最近一次检查结果。
// 说明：面板只把上游内容镜像到本地文件（0600），dae 侧仍通过 file:// 引用镜像，
// 因此更新订阅不会改变配置结构，也不需要改写 config.dae。
type Entry struct {
	Name        string `json:"name"`              // 订阅名（对应 dae subscription 名 / subtag）
	URL         string `json:"url"`               // 上游地址（http/https/或 file://）
	UA          string `json:"ua,omitempty"`      // 拉取上游用的 User-Agent（空=默认）
	Format      string `json:"format,omitempty"`  // 上游内容格式：links / clash
	Skipped     int    `json:"skipped,omitempty"` // Clash 转换时被跳过的代理数
	Source      string `json:"source"`            // 本地镜像在配置里的引用，如 file://subs/CF.sub
	File        string `json:"file"`              // 本地镜像绝对路径
	LastCheck   string `json:"lastCheck,omitempty"`
	LastOK      bool   `json:"lastOk"`
	Err         string `json:"err,omitempty"`
	RemoteCount int    `json:"remoteCount"`
	LocalCount  int    `json:"localCount"`
	Changed     bool   `json:"changed"`
	Added       int    `json:"added"`
	Removed     int    `json:"removed"`
	// AddedKeys/RemovedKeys 是变化明细（最多保留 maxKeys 条），便于界面直接展示变化内容。
	AddedKeys   []string `json:"addedKeys,omitempty"`
	RemovedKeys []string `json:"removedKeys,omitempty"`
	LastApplied string   `json:"lastApplied,omitempty"`
	// InModel 表示该订阅仍被配置（内容层）引用。它是每次请求现算的派生字段：
	// Load 一律清零、从不写回盘（早期版本把它持久化，订阅从内容层移除后仍显示「已引用」）。
	InModel bool `json:"inModel,omitempty"`
}

// Store 是订阅源登记表，持久化为 JSON（0600）。
type Store struct {
	Path       string  `json:"-"`
	CheckHours int     `json:"checkHours"`
	LastRun    string  `json:"lastRun,omitempty"`
	Items      []Entry `json:"items"`
}

// Load 读取登记表；文件不存在或损坏时返回空表（不报错，避免面板因此起不来）。
func Load(path string) *Store {
	st := &Store{Path: path, CheckHours: 6}
	if b, err := os.ReadFile(path); err == nil {
		var t Store
		if json.Unmarshal(b, &t) == nil {
			t.Path = path
			if t.CheckHours <= 0 {
				t.CheckHours = 6
			}
			if t.Items == nil {
				t.Items = []Entry{}
			}
			// InModel 是派生字段，旧版本可能把它写进文件；读入一律清零，由调用方按模型现算。
			for i := range t.Items {
				t.Items[i].InModel = false
			}
			return &t
		}
	}
	return st
}

// Save 原子写入登记表（0600）。
func (s *Store) Save() error {
	if s.Items == nil {
		s.Items = []Entry{}
	}
	// 派生字段不落盘：写盘前清零，避免旧值被带进文件。
	cp := *s
	cp.Items = make([]Entry, len(s.Items))
	for i, e := range s.Items {
		e.InModel = false
		cp.Items[i] = e
	}
	b, err := json.MarshalIndent(&cp, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(s.Path, b, 0o600)
}

// Find 返回指定订阅的指针（没有则 nil）。
func (s *Store) Find(name string) *Entry {
	for i := range s.Items {
		if s.Items[i].Name == name {
			return &s.Items[i]
		}
	}
	return nil
}

// Delete 从登记表里移除一条登记项（不动磁盘上的镜像文件）。返回是否确有该条。
func (s *Store) Delete(name string) bool {
	for i := range s.Items {
		if s.Items[i].Name == name {
			s.Items = append(s.Items[:i], s.Items[i+1:]...)
			return true
		}
	}
	return false
}

// Upsert 新增或更新一条登记（按 Name 匹配）。
func (s *Store) Upsert(e Entry) *Entry {
	if cur := s.Find(e.Name); cur != nil {
		cur.URL = e.URL
		cur.UA = e.UA
		if e.Source != "" {
			cur.Source = e.Source
		}
		if e.File != "" {
			cur.File = e.File
		}
		return cur
	}
	s.Items = append(s.Items, e)
	return &s.Items[len(s.Items)-1]
}

// Keys 生成节点的去重标识集合（不含凭据）。
func Keys(nodes []subs.Node) map[string]bool {
	out := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		k := n.Scheme + "|" + n.Host + "|" + n.Port + "|" + n.Tag
		out[k] = true
	}
	return out
}

// Diff 比较两个标识集合，返回新增与移除的标识。
func Diff(old, new map[string]bool) (added, removed []string) {
	for k := range new {
		if !old[k] {
			added = append(added, k)
		}
	}
	for k := range old {
		if !new[k] {
			removed = append(removed, k)
		}
	}
	return
}

// fetchCacheTTL 是上游拉取结果的缓存有效期。
// 为什么必须缓存：实测该机场上游（Cloudflare 后）对短时间内的重复请求会直接挂起
// （第 1 次 2s 返回 200，紧接着的第 2 次 30s 超时），而面板的主要使用顺序是
// 「检查 → 看差异 → 应用」，若两次都真去拉取，应用必然超时。
var fetchCacheTTL = 120 * time.Second

type cachedPayload struct {
	data    []byte
	raw     []byte
	format  string
	skipped []string
	at      time.Time
}

var fetchCache = map[string]cachedPayload{}

// SetCacheTTL 设置缓存有效期（0 表示每次都重新拉取）。
func SetCacheTTL(d time.Duration) { fetchCacheTTL = d }

// ClearCache 清空缓存（测试或手动强制刷新时用）。
func ClearCache() {
	for k := range fetchCache {
		delete(fetchCache, k)
	}
}

func cacheKey(e *Entry) string { return strings.TrimSpace(e.URL) + "|" + e.UA }

// tokenRe 匹配订阅 URL 里的 token 参数（错误信息/审计日志里不应留下明文凭证）。
var tokenRe = regexp.MustCompile(`(?i)(token=)[^&\s"']+`)

// maskToken 把订阅链接里的 token 打码。
func maskToken(s string) string { return tokenRe.ReplaceAllString(s, "${1}***") }

// FriendlyErr 给上游错误补上人话提示，并给凭证打码。
func FriendlyErr(err error) string {
	if err == nil {
		return ""
	}
	msg := maskToken(err.Error())
	if strings.Contains(msg, "context deadline exceeded") || strings.Contains(msg, "Client.Timeout") {
		msg += "（上游疑似限流：短时间内重复拉取会被挂起，请等 1 分钟再试）"
	}
	return msg
}

// FetchPayload 拉取上游并归一化为「dae 可消费的节点链接列表」，优先用缓存。
// 关键：Clash/Mihomo 的 YAML 配置 dae 无法直接消费（实测 v2.1.1 不支持），
// 这里就地转换成 ss/vmess/trojan/vless/hysteria2/tuic 链接；其余内容原样返回。
// 只读，不写盘。
func FetchPayload(e *Entry, baseDir string) (data []byte, format string, skipped []string, err error) {
	if fetchCacheTTL > 0 {
		if c, ok := fetchCache[cacheKey(e)]; ok && time.Since(c.at) < fetchCacheTTL {
			return c.data, c.format, c.skipped, nil
		}
	}
	return FetchPayloadForce(e, baseDir)
}

// FetchPayloadRaw 与 FetchPayload 相同，但额外返回上游原始内容（留档用）；命中缓存时也能拿到原始内容。
func FetchPayloadRaw(e *Entry, baseDir string) (data, raw []byte, format string, skipped []string, err error) {
	if fetchCacheTTL > 0 {
		if c, ok := fetchCache[cacheKey(e)]; ok && time.Since(c.at) < fetchCacheTTL {
			return c.data, c.raw, c.format, c.skipped, nil
		}
	}
	return FetchPayloadForceRaw(e, baseDir)
}

// FetchPayloadForce 忽略缓存强制拉取上游，成功后刷新缓存。
func FetchPayloadForce(e *Entry, baseDir string) (data []byte, format string, skipped []string, err error) {
	data, _, format, skipped, err = FetchPayloadForceRaw(e, baseDir)
	return data, format, skipped, err
}

// FetchPayloadForceRaw 同 FetchPayloadForce，另外返回上游原始内容（用于留档，便于排查被跳过的节点类型）。
func FetchPayloadForceRaw(e *Entry, baseDir string) (data, raw []byte, format string, skipped []string, err error) {
	raw, err = subs.FetchRawUA(baseDir, strings.TrimSpace(e.URL), e.UA)
	if err != nil {
		return nil, nil, "", nil, err
	}
	if lookLikeHTML(raw) {
		return nil, raw, "html", nil, fmt.Errorf("上游返回的是网页（HTML）而不是订阅内容：该地址多半不是订阅链接，或链接已失效/需要浏览器登录，也可能要换 User-Agent")
	}
	if !clashconv.LooksLikeClash(raw) {
		data, format, skipped = raw, "links", nil
	} else {
		res, cerr := clashconv.Convert(raw)
		if cerr != nil {
			return nil, raw, "clash", res.Skipped, cerr
		}
		data, format, skipped = []byte(strings.Join(res.Links, "\n")), "clash", res.Skipped
	}
	if fetchCacheTTL > 0 {
		fetchCache[cacheKey(e)] = cachedPayload{data: data, raw: raw, format: format, skipped: skipped, at: time.Now()}
	}
	return data, raw, format, skipped, nil
}

// lookLikeHTML 判断上游是不是返回了一个网页（404 页、登录页、面板首页等）。
func lookLikeHTML(raw []byte) bool {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return false
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "<!doctype") || strings.HasPrefix(lower, "<html") {
		return true
	}
	if strings.HasPrefix(s, "<") && strings.Contains(lower, "</") {
		return true
	}
	return false
}

// Fetch 拉取上游订阅并解析为节点（只读，不写盘）。
func Fetch(e *Entry, baseDir string) (subs.Source, string, []string) {
	out := subs.Source{Name: e.Name, URL: e.URL}
	data, format, skipped, err := FetchPayloadForce(e, baseDir)
	if err != nil {
		out.Error = FriendlyErr(err)
		return out, format, skipped
	}
	nodes, err := subs.Parse(data)
	if err != nil {
		out.Error = err.Error()
		return out, format, skipped
	}
	for i := range nodes {
		nodes[i].Subtag = e.Name
	}
	out.Nodes = nodes
	out.Count = len(nodes)
	return out, format, skipped
}

// maxKeys 是登记表里保存的变化明细上限（避免登记表被明细撑大）。
const maxKeys = 30

// capKeys 截断并排序变化明细（排序保证展示稳定）。
func capKeys(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := append([]string{}, in...)
	sort.Strings(out)
	if len(out) > maxKeys {
		out = append(out[:maxKeys], "…")
	}
	return out
}

// Summary 汇总「有待更新变化的订阅数」与最近一次巡检时间（供状态栏提醒）。
func Summary(st *Store) (changed int, names []string) {
	for _, e := range st.Items {
		if e.Changed {
			changed++
			names = append(names, e.Name)
		}
	}
	return changed, names
}

// Check 对比上游与本地镜像，填充 e 的检查结果字段（只读）。
func Check(e *Entry, baseDir string) {
	e.LastCheck = time.Now().UTC().Format(time.RFC3339)
	url := strings.TrimSpace(e.URL)
	if url == "" {
		e.LastOK = false
		e.Changed = false
		e.Err = "未配置上游 URL"
		return
	}
	remote, format, skipped := Fetch(e, baseDir)
	e.Format = format
	e.Skipped = len(skipped)
	if remote.Error != "" {
		e.LastOK = false
		e.Changed = false
		e.Err = "拉取上游失败: " + remote.Error
		return
	}
	local := subs.Resolve(baseDir, e.Name, e.Source)
	added, removed := Diff(Keys(local.Nodes), Keys(remote.Nodes))
	e.RemoteCount = remote.Count
	e.LocalCount = local.Count
	e.Added = len(added)
	e.Removed = len(removed)
	e.AddedKeys = capKeys(added)
	e.RemovedKeys = capKeys(removed)
	e.Changed = e.Added > 0 || e.Removed > 0
	e.LastOK = true
	e.Err = ""
	if local.Error != "" {
		e.LocalCount = 0
		e.Changed = true
		e.Err = "本地镜像不可读: " + local.Error
	}
}

// WriteFileAtomic 写临时文件后 rename，文件权限 0600（配置/订阅类文件要求）。
func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
