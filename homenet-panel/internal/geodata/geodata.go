// Package geodata 管理 dae 依赖的 geoip.dat / geosite.dat：状态查看、下载更新（含备份轮转）与 tag 扫描。
//
// 背景：dae 的 geosite/geoip 数据是 v2ray 格式的 dat 文件，放在 /usr/share/dae/ 下；
// 面板接管 goose 之前，这些文件由 goose 的 `update-geo` 每周更新。
package geodata

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	GeoIPName   = "geoip.dat"
	GeoSiteName = "geosite.dat"
	// minSize 用来挡住「下载到 404 页面 / 空响应」这类事故。
	minSize = 1 << 20
)

// DefaultURLs 是默认数据源；与 goose 时代一致，用 Loyalsoldier 的 v2ray-rules-dat（tag 更全：apple-cn / category-ai-!cn 等）。
var DefaultURLs = map[string]string{
	"geoip":   "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat",
	"geosite": "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat",
}

// File 描述一个 dat 文件的本机状态。
type File struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Exists  bool      `json:"exists"`
	Bytes   int64     `json:"bytes"`
	ModTime time.Time `json:"modTime"`
	Tags    int       `json:"tags,omitempty"`
	Managed string    `json:"managed,omitempty"`
	Note    string    `json:"note,omitempty"`
}

// Backup 描述一份历史备份。
type Backup struct {
	Name    string    `json:"name"`
	Bytes   int64     `json:"bytes"`
	ModTime time.Time `json:"modTime"`
}

// Status 是 /api/geo 的返回体。
type Status struct {
	Dir        string            `json:"dir"`
	BackupDir  string            `json:"backupDir"`
	Files      []File            `json:"files"`
	Backups    []Backup          `json:"backups"`
	URLs       map[string]string `json:"urls"`
	TotalBytes int64             `json:"totalBytes"`
}

// Resolve 把 target（geoip/geosite 或文件名）归一成文件名。
func Resolve(target string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "geoip", GeoIPName:
		return GeoIPName, true
	case "geosite", GeoSiteName:
		return GeoSiteName, true
	}
	return "", false
}

var printableRe = regexp.MustCompile(`[ -~]{2,80}`)

// ReadTags 扫描 dat 里的可打印字符串，返回小写 tag 集合。
// dat 是 protobuf，但 tag 本身是明文（如 CN / CATEGORY-ADS-ALL / APPLE-CN），所以不做完整反序列化也能判断「tag 是否存在」。
func ReadTags(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return tagsOf(data)
}

// BackupDir 返回某个 dir 对应的备份目录（<backupRoot>/geo）。
func BackupDir(backupRoot string) string {
	return filepath.Join(backupRoot, "geo")
}

// Inspect 汇总 dir 下两个 dat 的状态与备份列表。
func Inspect(dir, backupRoot string) Status {
	st := Status{Dir: dir, BackupDir: BackupDir(backupRoot), URLs: DefaultURLs}
	for _, name := range []string{GeoIPName, GeoSiteName} {
		p := filepath.Join(dir, name)
		f := File{Name: name, Path: p}
		if li, lerr := os.Lstat(p); lerr == nil && li.Mode()&os.ModeSymlink != 0 {
			if target, terr := os.Readlink(p); terr == nil {
				f.Managed = "symlink → " + target
			}
		}
		if info, err := os.Stat(p); err == nil {
			f.Exists = true
			f.Bytes = info.Size()
			f.ModTime = info.ModTime()
			st.TotalBytes += info.Size()
			if name == GeoSiteName {
				if tags, terr := ReadTags(p); terr == nil {
					f.Tags = len(tags)
				}
			}
		} else {
			f.Note = "不存在"
		}
		st.Files = append(st.Files, f)
	}
	st.Backups = ListBackups(backupRoot)
	return st
}

// ListBackups 列出 <backupRoot>/geo 下的备份（新的在前）。
func ListBackups(backupRoot string) []Backup {
	dir := BackupDir(backupRoot)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Backup
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		out = append(out, Backup{Name: e.Name(), Bytes: info.Size(), ModTime: info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out
}

// Updated 描述一次成功更新。
type Updated struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Bytes  int64  `json:"bytes"`
	Backup string `json:"backup,omitempty"`
	Tags   int    `json:"tags,omitempty"`
}

// Result 是一次更新的汇总。
type Result struct {
	Updated []Updated `json:"updated"`
	Errors  []string  `json:"errors"`
	DryRun  bool      `json:"dryRun"`
}

// UpdateOptions 控制更新行为。
type UpdateOptions struct {
	Dir       string
	BackupDir string
	Targets   []string // geoip / geosite，空=两个都更新
	URLs      map[string]string
	Keep      int // 每个文件保留几份历史备份（0=不清理）
	DryRun    bool
	Timeout   time.Duration
}

// Update 下载并替换 dat 文件：先备份旧文件，再原子写入新文件；DryRun 只下载校验不落盘。
func Update(opt UpdateOptions) (Result, error) {
	res := Result{DryRun: opt.DryRun}
	if opt.Dir == "" {
		return res, fmt.Errorf("未指定 geo 目录")
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 120 * time.Second
	}
	urls := map[string]string{}
	for k, v := range DefaultURLs {
		urls[k] = v
	}
	for k, v := range opt.URLs {
		if v != "" {
			urls[k] = v
		}
	}
	targets := opt.Targets
	if len(targets) == 0 {
		targets = []string{"geoip", "geosite"}
	}
	client := &http.Client{Timeout: opt.Timeout}
	for _, t := range targets {
		name, ok := Resolve(t)
		if !ok {
			res.Errors = append(res.Errors, fmt.Sprintf("未知目标 %q（只支持 geoip / geosite）", t))
			continue
		}
		dst := filepath.Join(opt.Dir, name)
		if li, lerr := os.Lstat(dst); lerr == nil && li.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(dst)
			res.Errors = append(res.Errors, fmt.Sprintf("%s: 是符号链接（→ %s），通常由 v2ray-rules-dat 包管理；请用 apt update && apt install --only-upgrade v2ray-rules-dat 更新，面板不覆盖符号链接", name, target))
			continue
		}
		key := strings.TrimSuffix(name, ".dat")
		url := urls[key]
		data, err := download(client, url)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		if len(data) < minSize {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: 下载内容只有 %d 字节，疑似不是 dat 文件", name, len(data)))
			continue
		}
		up := Updated{Name: name, URL: url, Bytes: int64(len(data))}
		if name == GeoSiteName {
			tags, terr := tagsOf(data)
			if terr == nil {
				up.Tags = len(tags)
				if !tags["cn"] {
					res.Errors = append(res.Errors, fmt.Sprintf("%s: 下载内容缺少 cn tag，疑似不是 geosite.dat", name))
					continue
				}
			}
		}
		if opt.DryRun {
			res.Updated = append(res.Updated, up)
			continue
		}
		if err := os.MkdirAll(opt.Dir, 0o755); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: 创建目录失败: %v", name, err))
			continue
		}
		if b, berr := backup(dst, opt.BackupDir, name); berr == nil {
			up.Backup = b
		}
		if err := writeAtomic(dst, data, 0o644); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: 写入失败: %v", name, err))
			continue
		}
		if opt.Keep > 0 {
			prune(opt.BackupDir, name, opt.Keep)
		}
		res.Updated = append(res.Updated, up)
	}
	return res, nil
}

func download(client *http.Client, url string) ([]byte, error) {
	if url == "" {
		return nil, fmt.Errorf("未配置下载地址")
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "homenet-panel/geo")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 200<<20))
}

func tagsOf(data []byte) (map[string]bool, error) {
	out := make(map[string]bool, 4096)
	for _, m := range printableRe.FindAll(data, -1) {
		out[strings.ToLower(string(m))] = true
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("未扫描到可打印字符串")
	}
	return out, nil
}

func backup(dst, backupRoot, name string) (string, error) {
	if _, err := os.Stat(dst); err != nil {
		return "", err // 首次安装没有旧文件，不算错误
	}
	dir := BackupDir(backupRoot)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	bp := filepath.Join(dir, fmt.Sprintf("%s.%s", name, time.Now().Format("20060102-150405")))
	if err := copyFile(dst, bp); err != nil {
		return "", err
	}
	return bp, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func prune(backupRoot, name string, keep int) {
	all := ListBackups(backupRoot)
	var mine []Backup
	for _, b := range all {
		if strings.HasPrefix(b.Name, name+".") {
			mine = append(mine, b)
		}
	}
	for i := keep; i < len(mine); i++ {
		_ = os.Remove(filepath.Join(BackupDir(backupRoot), mine[i].Name))
	}
}
