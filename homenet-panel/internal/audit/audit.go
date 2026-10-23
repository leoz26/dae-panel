package audit

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Entry 是一条审计记录（JSONL 一行）。
type Entry struct {
	TS     string `json:"ts"`
	Actor  string `json:"actor"`
	IP     string `json:"ip,omitempty"`
	Action string `json:"action"`
	Target string `json:"target,omitempty"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// 默认保留策略：单文件 5MB，保留 3 份历史（当前 + 3 份 = 最多 20MB）。
// 路由器（OpenWrt，flash 通常 16~128MB）建议 --audit-max-mb 2 --audit-keep 2（最多 6MB）。
const (
	DefaultMaxBytes int64 = 5 << 20
	DefaultKeep           = 3
)

var (
	mu       sync.Mutex
	maxBytes = DefaultMaxBytes
	keep     = DefaultKeep
)

// SetRetention 设置轮转阈值与保留份数；max<=0 回默认，keep<0 视为只保留当前文件。
func SetRetention(max int64, k int) {
	mu.Lock()
	defer mu.Unlock()
	if max <= 0 {
		max = DefaultMaxBytes
	}
	if k < 0 {
		k = 0
	}
	maxBytes, keep = max, k
}

// Retention 返回当前轮转阈值与保留份数。
func Retention() (int64, int) {
	mu.Lock()
	defer mu.Unlock()
	return maxBytes, keep
}

// rotated 返回第 i 代历史文件路径（i>=1）。
func rotated(path string, i int) string { return path + "." + strconv.Itoa(i) }

// rotate 由新到旧滚动历史文件，并删除超出保留份数的最旧一代。
func rotate(path string) {
	k := keep
	if k <= 0 {
		return
	}
	_ = os.Remove(rotated(path, k))
	for i := k - 1; i >= 1; i-- {
		_ = os.Rename(rotated(path, i), rotated(path, i+1))
	}
	_ = os.Rename(path, rotated(path, 1))
}

// Append 追加一条审计记录。失败时返回错误，但调用方通常可忽略（不阻断主流程）。
func Append(path string, e Entry) error {
	if path == "" {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	if e.TS == "" {
		e.TS = time.Now().UTC().Format(time.RFC3339)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if fi, err := os.Stat(path); err == nil && fi.Size() >= maxBytes {
		rotate(path)
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

// tailFile 读单个文件，返回按时间正序的记录。
func tailFile(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		t := strings.TrimSpace(sc.Text())
		if t == "" {
			continue
		}
		var e Entry
		if json.Unmarshal([]byte(t), &e) == nil {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}

// ReadAll 按时间正序读出当前文件与所有历史代的记录（导出用）。
func ReadAll(path string) ([]Entry, error) {
	_, k := Retention()
	out := []Entry{}
	for i := k; i >= 1; i-- {
		es, err := tailFile(rotated(path, i))
		if err != nil {
			return out, err
		}
		out = append(out, es...)
	}
	es, err := tailFile(path)
	if err != nil {
		return out, err
	}
	return append(out, es...), nil
}

// Tail 返回最近 n 条记录（新的在前），跨历史文件（从当前文件往前找）。
func Tail(path string, n int) ([]Entry, error) {
	if n <= 0 {
		n = 100
	}
	_, k := Retention()
	files := []string{path}
	for i := 1; i <= k; i++ {
		files = append(files, rotated(path, i))
	}
	out := []Entry{}
	for _, f := range files {
		es, err := tailFile(f)
		if err != nil {
			return out, err
		}
		for i := len(es) - 1; i >= 0 && len(out) < n; i-- {
			out = append(out, es[i])
		}
		if len(out) >= n {
			break
		}
	}
	return out, nil
}

// Files 返回当前存在的日志文件（当前 + 历史代）及总字节数。
func Files(path string) ([]string, int64) {
	_, k := Retention()
	cands := []string{path}
	for i := 1; i <= k; i++ {
		cands = append(cands, rotated(path, i))
	}
	var files []string
	var total int64
	for _, f := range cands {
		if fi, err := os.Stat(f); err == nil {
			files = append(files, f)
			total += fi.Size()
		}
	}
	return files, total
}

// CSV 把记录导出为 CSV（带 UTF-8 BOM，方便 Excel 直接打开中文）。
func CSV(entries []Entry) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("\ufeff")
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"时间", "操作者", "来源IP", "动作", "对象", "结果", "详情"})
	for _, e := range entries {
		res := "失败"
		if e.OK {
			res = "成功"
		}
		if err := w.Write([]string{e.TS, e.Actor, e.IP, e.Action, e.Target, res, e.Detail}); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Describe 给前端一句话描述保留策略。
func Describe() string {
	max, k := Retention()
	return fmt.Sprintf("单文件上限 %dMB，保留 %d 份历史（最多约 %dMB）", max>>20, k, (int64(k)+1)*max>>20)
}
