package preflight

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Writer struct {
	PID     int    `json:"pid"`
	Comm    string `json:"comm"`
	Cmdline string `json:"cmdline"`
}

type Report struct {
	ConfigPath     string   `json:"configPath"`
	GooseHeader    bool     `json:"gooseHeader"`
	GooseInstalled bool     `json:"gooseInstalled"`
	GooseBinary    string   `json:"gooseBinary,omitempty"`
	GooseRunning   []int    `json:"gooseRunning,omitempty"`
	OtherWriters   []Writer `json:"otherWriters,omitempty"`
	SubErrors      []string `json:"subErrors,omitempty"`
	SubNotes       []string `json:"subNotes,omitempty"`
}

var fileURLRe = regexp.MustCompile(`file://([^'"\s)]+)`)

// DetectGooseHeader 判断配置是否由外部生成器（goose）生成。
func DetectGooseHeader(content []byte) bool {
	head := string(content)
	if len(head) > 512 {
		head = head[:512]
	}
	l := strings.ToLower(head)
	return strings.Contains(l, "goose") && (strings.Contains(l, "生成") || strings.Contains(l, "generated"))
}

// FindGooseBinary 查找 goose 可执行文件。
func FindGooseBinary() string {
	cands := []string{"/usr/local/bin/goose", "/usr/bin/goose", "/bin/goose"}
	for _, c := range cands {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// FindProcess 扫描 /proc 找 comm==name 的进程（非 linux 返回空）。
func FindProcess(name string) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var pids []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if pid == os.Getpid() {
			continue
		}
		b, err := os.ReadFile(filepath.Join("/proc", e.Name(), "comm"))
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(b)) == name {
			pids = append(pids, pid)
		}
	}
	return pids
}

// FindOpenWriters 扫描 /proc/*/fd 找正打开给定文件（用于写的其它进程）。
func FindOpenWriters(path string) []Writer {
	abs, _ := filepath.Abs(path)
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	self := os.Getpid()
	var ws []Writer
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		fdDir := filepath.Join("/proc", e.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			tgt, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if tgt == abs {
				comm, _ := os.ReadFile(filepath.Join("/proc", e.Name(), "comm"))
				cmdline, _ := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
				ws = append(ws, Writer{
					PID:     pid,
					Comm:    strings.TrimSpace(string(comm)),
					Cmdline: strings.ReplaceAll(string(cmdline), "\x00", " "),
				})
				break
			}
		}
	}
	return ws
}

// CheckSubscriptions 就配置文本做订阅预检（R2 相对路径；同时提示空配置）。
func CheckSubscriptions(configPath string, content []byte) (errs []string, notes []string) {
	baseDir := filepath.Dir(configPath)
	matches := fileURLRe.FindAllStringSubmatch(string(content), -1)
	hasSubBlock := strings.Contains(string(content), "subscription {")
	hasNodeBlock := strings.Contains(string(content), "node {")
	for _, m := range matches {
		p := m[1]
		if strings.HasPrefix(p, "/") {
			errs = append(errs, fmt.Sprintf("订阅 %q 使用绝对路径，dae file:// 仅支持相对配置目录的路径（R2）", p))
			continue
		}
		full := filepath.Join(baseDir, p)
		if st, err := os.Stat(full); err != nil {
			errs = append(errs, fmt.Sprintf("订阅文件不存在: %s", full))
		} else if st.Size() == 0 {
			errs = append(errs, fmt.Sprintf("订阅文件为空: %s", full))
		}
	}
	if !hasSubBlock && !hasNodeBlock {
		notes = append(notes, "配置既无 subscription 块也无内联 node，重载会因 0 节点被 dae 拒绝（R1）")
	}
	return errs, notes
}

// Detect 汇总双写与订阅预检结果。
func Detect(configPath string, content []byte) Report {
	r := Report{ConfigPath: configPath}
	r.GooseHeader = DetectGooseHeader(content)
	r.GooseBinary = FindGooseBinary()
	r.GooseInstalled = r.GooseBinary != ""
	r.GooseRunning = FindProcess("goose")
	r.OtherWriters = FindOpenWriters(configPath)
	r.SubErrors, r.SubNotes = CheckSubscriptions(configPath, content)
	return r
}

// Blocking 判断是否应拒绝进入可编辑态（双写风险 / 订阅路径错误）。
func (r Report) Blocking() bool {
	if len(r.GooseRunning) > 0 || len(r.OtherWriters) > 0 {
		return true
	}
	if r.GooseHeader && r.GooseInstalled {
		return true
	}
	return len(r.SubErrors) > 0
}

// Reasons 返回阻断原因（用于错误信息）。
func (r Report) Reasons() []string {
	var rs []string
	if len(r.GooseRunning) > 0 {
		rs = append(rs, fmt.Sprintf("goose 进程仍在运行(pids=%v)", r.GooseRunning))
	}
	if len(r.OtherWriters) > 0 {
		rs = append(rs, fmt.Sprintf("检测到其它进程正打开配置文件(%v)", r.OtherWriters))
	}
	if r.GooseHeader && r.GooseInstalled {
		rs = append(rs, "配置文件由 goose 生成且 goose 仍安装，存在双写风险（E25）")
	}
	rs = append(rs, r.SubErrors...)
	return rs
}
