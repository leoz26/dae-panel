package dae

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

const (
	DefaultBinary   = "dae"
	DefaultConfig   = "/etc/dae/config.dae"
	DefaultPidFile  = "/var/run/dae.pid"
	DefaultProgress = "/var/run/dae.progress"
)

type Adapter struct {
	Binary       string
	ConfigPath   string
	PidFile      string
	ProgressFile string

	// BeforeReload 可选回调：每次 reload 前先执行（面板用它做"条件转发"自检）。
	BeforeReload func()

	// OnReloadError 可选回调：reload 返回非 0 时被调用；返回 true 表示已修复环境，重试一次。
	OnReloadError func(code int, out string) bool
}

func New() *Adapter {
	return &Adapter{
		Binary:       DefaultBinary,
		ConfigPath:   DefaultConfig,
		PidFile:      DefaultPidFile,
		ProgressFile: DefaultProgress,
	}
}

func run(cmd *exec.Cmd) (int, string) {
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = -1
			if buf.Len() > 0 {
				buf.WriteString("\n")
			}
			buf.WriteString(err.Error())
		}
	}
	return code, strings.TrimSpace(buf.String())
}

func (a *Adapter) Detect() (string, error) {
	out, err := exec.Command(a.Binary, "--version").CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("dae --version 失败: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

type ValidateResult struct {
	ExitCode int    `json:"exitCode"`
	Output   string `json:"output"`
	OK       bool   `json:"ok"`
}

func (a *Adapter) Validate(path string) ValidateResult {
	code, out := run(exec.Command(a.Binary, "validate", "-c", path))
	return ValidateResult{ExitCode: code, Output: out, OK: code == 0}
}

func (a *Adapter) Reload(pid int) (int, string) {
	if a.BeforeReload != nil {
		a.BeforeReload()
	}
	code, out := a.reloadOnce(pid)
	if code != 0 && a.OnReloadError != nil && a.OnReloadError(code, out) {
		code, out = a.reloadOnce(pid)
	}
	return code, out
}

// reloadOnce 执行一次 dae reload，不做任何修复。
func (a *Adapter) reloadOnce(pid int) (int, string) {
	args := []string{"reload"}
	if pid > 0 {
		args = append(args, strconv.Itoa(pid))
	}
	return run(exec.Command(a.Binary, args...))
}

func (a *Adapter) Suspend(pid int) (int, string) {
	args := []string{"suspend"}
	if pid > 0 {
		args = append(args, strconv.Itoa(pid))
	}
	return run(exec.Command(a.Binary, args...))
}

func (a *Adapter) Outline() (string, error) {
	out, err := exec.Command(a.Binary, "export", "outline").Output()
	return string(out), err
}

// Progress 读取 dae 进度文件。
func (a *Adapter) Progress() (Progress, error) {
	return ReadProgress(a.ProgressFile)
}

// RunningPID 从 pidfile 读取 pid 并确认进程存活；读不到或已退出返回 0。
func (a *Adapter) RunningPID() int {
	b, err := os.ReadFile(a.PidFile)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return 0
	}
	if err := syscall.Kill(pid, 0); err != nil {
		return 0
	}
	return pid
}

func (a *Adapter) ServiceActive() string {
	out, err := exec.Command("systemctl", "is-active", "dae.service").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
