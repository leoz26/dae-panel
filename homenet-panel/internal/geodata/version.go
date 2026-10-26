package geodata

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CmdRunner 用于执行外部命令（dpkg-query / apt-cache），便于测试注入。
type CmdRunner interface {
	Run(name string, args ...string) (string, error)
}

type execRunner struct{}

func (execRunner) Run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return string(out), err
}

const defaultAPIURL = "https://api.github.com/repos/Loyalsoldier/v2ray-rules-dat/releases/latest"

// State 记录面板上次成功更新时拿到的远端构建信息（落盘，供版本对比）。
type State struct {
	Tag       string               `json:"tag,omitempty"`
	FetchedAt time.Time            `json:"fetchedAt,omitempty"`
	Files     map[string]StateFile `json:"files,omitempty"`
}

// StateFile 是单个文件的记录。
type StateFile struct {
	Source    string    `json:"source,omitempty"`
	Tag       string    `json:"tag,omitempty"`
	Bytes     int64     `json:"bytes,omitempty"`
	SHA256    string    `json:"sha256,omitempty"`
	FetchedAt time.Time `json:"fetchedAt,omitempty"`
}

// LoadState 读取版本记录；文件不存在或损坏时返回空 State。
func LoadState(path string) State {
	st := State{}
	if path == "" {
		return st
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return st
	}
	_ = json.Unmarshal(data, &st)
	return st
}

// SaveState 原子写入版本记录。
func SaveState(path string, st State) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, data, 0o644)
}

// FileVersion 描述单个 dat 的本地/远端版本对照。
type FileVersion struct {
	Name           string    `json:"name"`
	Managed        string    `json:"managed,omitempty"`
	PkgInstalled   string    `json:"pkgInstalled,omitempty"`
	PkgCandidate   string    `json:"pkgCandidate,omitempty"`
	LocalBytes     int64     `json:"localBytes"`
	LocalModTime   time.Time `json:"localModTime"`
	LocalTag       string    `json:"localTag,omitempty"`
	LocalSHA256    string    `json:"localSha256,omitempty"`
	RemoteBytes    int64     `json:"remoteBytes"`
	RemoteModified time.Time `json:"remoteModified"`
	UpToDate       bool      `json:"upToDate"`
	Verdict        string    `json:"verdict"`
}

// VersionReport 是 /api/geo/versions 的返回体。
type VersionReport struct {
	RemoteTag       string        `json:"remoteTag,omitempty"`
	RemotePublished time.Time     `json:"remotePublished,omitempty"`
	Files           []FileVersion `json:"files"`
	Hint            string        `json:"hint,omitempty"`
	Errors          []string      `json:"errors"`
}

// VersionOptions 控制版本检测。
type VersionOptions struct {
	Dir       string
	URLs      map[string]string
	StatePath string
	APIURL    string
	Runner    CmdRunner
	Timeout   time.Duration
	HTTP      *http.Client
}

// DetectVersions 对比本机 dat 与上游最新构建；只做 HEAD 请求与本地检查，不下载正文、不写盘。
func DetectVersions(o VersionOptions) VersionReport {
	rep := VersionReport{}
	if o.Runner == nil {
		o.Runner = execRunner{}
	}
	if o.Timeout <= 0 {
		o.Timeout = 15 * time.Second
	}
	client := o.HTTP
	if client == nil {
		client = &http.Client{Timeout: o.Timeout}
	}
	urls := o.URLs
	if len(urls) == 0 {
		urls = DefaultURLs
	}
	apiURL := o.APIURL
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	st := LoadState(o.StatePath)
	pkgVer, pkgCand := aptVersion(o.Runner)

	// 先把本地状态摸清；apt 托管的文件不再探测 GitHub HEAD（apt 才是权威信息源，也避免慢网络下长时间等待）。
	files := make([]FileVersion, 0, 2)
	needProbe := map[string]bool{}
	for _, name := range []string{GeoIPName, GeoSiteName} {
		key := strings.TrimSuffix(name, ".dat")
		fv := FileVersion{Name: name}
		p := filepath.Join(o.Dir, name)
		info, serr := os.Stat(p)
		if li, lerr := os.Lstat(p); lerr == nil && li.Mode()&os.ModeSymlink != 0 {
			fv.Managed = "apt（符号链接）"
			fv.PkgInstalled = pkgVer
			fv.PkgCandidate = pkgCand
			fv.LocalTag = pkgVersionTag(pkgVer)
		} else if serr == nil {
			fv.Managed = "面板/手工文件"
			if sf, ok := st.Files[key]; ok {
				fv.LocalTag = sf.Tag
				fv.LocalSHA256 = sf.SHA256
			}
		}
		if serr != nil {
			fv.Verdict = "本机没有这个文件"
			files = append(files, fv)
			continue
		}
		fv.LocalBytes = info.Size()
		fv.LocalModTime = info.ModTime()
		// 只有自定义源（镜像）才逐文件 HEAD；默认 GitHub 源的大小由 release API 一次给出。
		if fv.Managed == "面板/手工文件" && urls[key] != "" && urls[key] != DefaultURLs[key] {
			needProbe[name] = true
		}
		files = append(files, fv)
	}

	// 远端探测并发进行：一次 GitHub API 取构建号 + 每个待探测文件一次 HEAD。
	type probe struct {
		size int64
		mod  time.Time
		err  error
	}
	probes := map[string]probe{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var tag string
	var published time.Time
	var tagErr error
	var rel releaseInfo
	wg.Add(1)
	go func() {
		defer wg.Done()
		rel, tagErr = fetchLatestRelease(client, apiURL)
		if tagErr == nil {
			tag = rel.tag
			published = rel.published
		}
	}()
	for name := range needProbe {
		name := name
		url := urls[strings.TrimSuffix(name, ".dat")]
		wg.Add(1)
		go func() {
			defer wg.Done()
			size, mod, err := headSize(client, url)
			mu.Lock()
			probes[name] = probe{size: size, mod: mod, err: err}
			mu.Unlock()
		}()
	}
	wg.Wait()

	if tagErr == nil {
		rep.RemoteTag = tag
		rep.RemotePublished = published
		// GitHub release 里已带资产大小：默认源直接用，不必逐个 HEAD（HEAD 在部分网络下会被卡住）
		for i := range files {
			fv := &files[i]
			if urls[strings.TrimSuffix(fv.Name, ".dat")] != DefaultURLs[strings.TrimSuffix(fv.Name, ".dat")] {
				continue
			}
			if sz, ok := rel.sizes[fv.Name]; ok {
				fv.RemoteBytes = sz
				fv.RemoteModified = published
			}
		}
	} else {
		rep.Errors = append(rep.Errors, "取上游构建号失败: "+tagErr.Error())
	}

	for i := range files {
		fv := &files[i]
		if pr, ok := probes[fv.Name]; ok {
			if pr.err != nil {
				rep.Errors = append(rep.Errors, fv.Name+": 探测上游失败: "+pr.err.Error())
			} else {
				fv.RemoteBytes = pr.size
				fv.RemoteModified = pr.mod
			}
		}
		fv.UpToDate, fv.Verdict = verdictFor(*fv, rep.RemoteTag, pkgVer, pkgCand)
	}
	rep.Files = files

	if pkgCand != "" && pkgVer != "" && pkgCand != pkgVer {
		rep.Hint = "apt 有可升级版本 " + pkgCand + "（当前 " + pkgVer + "）：apt install --only-upgrade v2ray-rules-dat"
	}
	if rep.Hint == "" {
		for _, f := range files {
			if !f.UpToDate && strings.HasPrefix(f.Managed, "apt") {
				rep.Hint = "上游已发布新构建，apt 源尚未提供：先 apt update 再检测一次，或等 daeuniverse 源同步"
				break
			}
		}
	}
	return rep
}

// verdictFor 依据「大小是否一致 / 构建号是否一致 / apt 是否有新版本」给出结论。
func verdictFor(fv FileVersion, remoteTag, pkgVer, pkgCand string) (bool, string) {
	if remoteTag != "" && fv.LocalTag != "" && fv.LocalTag == remoteTag {
		return true, "与上游 " + remoteTag + " 一致"
	}
	if fv.Managed != "" && strings.HasPrefix(fv.Managed, "apt") && remoteTag != "" && fv.LocalTag != "" && fv.LocalTag != remoteTag {
		if pkgCand != "" && pkgVer != "" && pkgCand != pkgVer {
			return false, "apt 有可升级版本 " + pkgCand + "（当前 " + pkgVer + "）"
		}
		return false, "本机 " + fv.LocalTag + "，上游已发布 " + remoteTag + "（apt 源尚未提供；可先 apt update 再看）"
	}
	if fv.Managed != "" && strings.HasPrefix(fv.Managed, "apt") {
		switch {
		case pkgCand != "" && pkgVer != "" && pkgCand != pkgVer:
			return false, "apt 源已有新版本 " + pkgCand + "（当前 " + pkgVer + "）"
		case pkgVer != "":
			return true, "apt 已是最新 " + pkgVer
		default:
			return true, "由 apt 托管，请用 apt 检查更新"
		}
	}
	if fv.RemoteBytes > 0 && fv.LocalBytes == fv.RemoteBytes {
		msg := "与上游构建大小一致"
		if remoteTag != "" {
			msg += "（上游 " + remoteTag + "）"
		}
		return true, msg
	}
	if fv.RemoteBytes > 0 {
		return false, fmt.Sprintf("上游构建大小不同（本地 %d / 上游 %d 字节），可能已有新版本", fv.LocalBytes, fv.RemoteBytes)
	}
	return true, "无法探测上游，仅显示本机状态"
}

// releaseInfo 是一次 GitHub release 查询的结果。
type releaseInfo struct {
	tag       string
	published time.Time
	sizes     map[string]int64
}

// fetchLatestRelease 读 GitHub release 的 tag、发布时间与资产大小（一次请求，避免逐个 HEAD）。
func fetchLatestRelease(client *http.Client, apiURL string) (releaseInfo, error) {
	info := releaseInfo{sizes: map[string]int64{}}
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return info, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "homenet-panel-geo")
	resp, err := client.Do(req)
	if err != nil {
		return info, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return info, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var body struct {
		TagName   string    `json:"tag_name"`
		Published time.Time `json:"published_at"`
		Assets    []struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return info, err
	}
	if body.TagName == "" {
		return info, fmt.Errorf("响应里没有 tag_name")
	}
	info.tag = body.TagName
	info.published = body.Published
	for _, a := range body.Assets {
		if a.Size > 0 {
			info.sizes[a.Name] = a.Size
		}
	}
	return info, nil
}

// headSize 用 HEAD 取上游文件大小与 Last-Modified（跟随重定向，不下载正文）。
func headSize(client *http.Client, url string) (int64, time.Time, error) {
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return 0, time.Time{}, err
	}
	req.Header.Set("User-Agent", "homenet-panel-geo")
	resp, err := client.Do(req)
	if err != nil {
		return 0, time.Time{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, time.Time{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var mod time.Time
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		if t, perr := http.ParseTime(lm); perr == nil {
			mod = t
		}
	}
	return resp.ContentLength, mod, nil
}

// aptVersion 读 apt 里 v2ray-rules-dat 的已装/候选版本。
func aptVersion(r CmdRunner) (string, string) {
	installed := ""
	if out, err := r.Run("dpkg-query", "-W", "-f=${Version}", "v2ray-rules-dat"); err == nil {
		installed = strings.TrimSpace(out)
	}
	candidate := ""
	if out, err := r.Run("apt-cache", "policy", "v2ray-rules-dat"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Candidate:") {
				candidate = strings.TrimSpace(strings.TrimPrefix(line, "Candidate:"))
				break
			}
		}
	}
	return installed, candidate
}

// pkgVersionTag 把 apt 版本号（如 202610222206.0.0）折成上游 tag（202610222206）。
func pkgVersionTag(v string) string {
	if i := strings.IndexByte(v, '.'); i > 0 {
		return v[:i]
	}
	return v
}

// FileSHA256 计算文件摘要（用于面板记录与排障）。
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
