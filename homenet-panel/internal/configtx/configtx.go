package configtx

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"homenet-panel/internal/dae"
)

// DaeClient 抽象 dae 操作，便于单元测试注入模拟实现。
type DaeClient interface {
	Validate(path string) dae.ValidateResult
	Reload(pid int) (int, string)
	RunningPID() int
	Progress() (dae.Progress, error)
}

// ErrSubscriptionEmpty 表示 dae 因订阅解析出 0 节点而拒绝重载（R1）。
var ErrSubscriptionEmpty = errors.New("订阅解析出 0 节点，dae 拒绝重载（R1）")

// ErrDaeNotRunning 表示 reload 前读不到 dae 的 pid（进程没在跑，或 /var/run/dae.pid 不存在）。
var ErrDaeNotRunning = errors.New("dae 没有在运行")

// DaeDownHint 是给用户的操作指引：dae 没跑时应用配置只会失败，先把 dae 拉起来。
const DaeDownHint = "读不到 " + dae.DefaultPidFile + "。" +
	"先在「安装与初始化」页点『生成初始配置』和『启动 dae』（或用 systemctl start dae；OpenWrt 用 /etc/init.d/dae start），" +
	"确认 dae 在运行后再点应用。"

// explainDaeOut 把 dae 的原始报错翻译成用户能读懂的话（识别不了就原样返回）。
func explainDaeOut(out string) string {
	if strings.Contains(out, "failed to read pid file") || strings.Contains(out, "no such file or directory") && strings.Contains(out, "dae.pid") {
		return ErrDaeNotRunning.Error() + "（" + DaeDownHint + "）；原始输出: " + firstLineOf(out)
	}
	return out
}

func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

type Manager struct {
	Dae       DaeClient
	BackupDir string
	// Preflight 可选：写入前校验钩子（如双写检测、订阅路径检查），返回 err 则放弃写入。
	Preflight func(path string, content []byte) error
}

func New(a DaeClient, backupDir string) *Manager {
	return &Manager{Dae: a, BackupDir: backupDir}
}

type Change struct {
	Path       string
	NewContent []byte
}

type Result struct {
	Applied           bool         `json:"applied"`
	RolledBack        bool         `json:"rolledBack"`
	BackupPath        string       `json:"backupPath,omitempty"`
	ValidateOut       string       `json:"validateOut,omitempty"`
	ReloadCode        int          `json:"reloadCode"`
	ReloadOut         string       `json:"reloadOut,omitempty"`
	Progress          dae.Progress `json:"progress"`
	SubscriptionEmpty bool         `json:"subscriptionEmpty,omitempty"`
	Message           string       `json:"message"`
}

func (m *Manager) backup(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if err := os.MkdirAll(m.BackupDir, 0o700); err != nil {
		return "", err
	}
	ts := time.Now().Format("20060102-150405.000")
	dst := filepath.Join(m.BackupDir, fmt.Sprintf("config-%s.dae", ts))
	if err := os.WriteFile(dst, b, 0o600); err != nil {
		return "", err
	}
	return dst, nil
}

func (m *Manager) restore(backup, path string) error {
	if backup == "" {
		return nil
	}
	b, err := os.ReadFile(backup)
	if err != nil {
		return err
	}
	return writeAtomic(path, b, 0o600)
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".hnp-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func isZeroNode(msg string) bool {
	l := strings.ToLower(msg)
	return strings.Contains(l, "0 nodes") ||
		strings.Contains(l, "all subscription resolving failed") ||
		strings.Contains(l, "no node found") ||
		strings.Contains(l, "no valid node")
}

// Apply: 预检→备份→写入(0600)→validate→reload→失败回滚。
func (m *Manager) Apply(ch Change) (Result, error) {
	var res Result

	if m.Preflight != nil {
		if err := m.Preflight(ch.Path, ch.NewContent); err != nil {
			res.Message = "预检未通过，放弃写入"
			return res, err
		}
	}

	backup, err := m.backup(ch.Path)
	if err != nil {
		return res, fmt.Errorf("备份失败: %w", err)
	}
	res.BackupPath = backup

	if err := writeAtomic(ch.Path, ch.NewContent, 0o600); err != nil {
		return res, fmt.Errorf("写入失败: %w", err)
	}

	vr := m.Dae.Validate(ch.Path)
	res.ValidateOut = vr.Output
	if !vr.OK {
		if rerr := m.restore(backup, ch.Path); rerr == nil && backup != "" {
			res.RolledBack = true
		}
		res.Message = "validate 失败，已回滚"
		return res, fmt.Errorf("validate 失败(exit %d): %s", vr.ExitCode, vr.Output)
	}

	pid := m.Dae.RunningPID()
	if pid <= 0 {
		_ = m.restore(backup, ch.Path)
		res.RolledBack = true
		res.Message = "dae 没有在运行，已回滚"
		return res, fmt.Errorf("%w：%s", ErrDaeNotRunning, DaeDownHint)
	}
	code, out := m.Dae.Reload(pid)
	res.ReloadCode = code
	res.ReloadOut = out
	if prog, perr := m.Dae.Progress(); perr == nil {
		res.Progress = prog
	}

	if code != 0 || res.Progress.IsError() {
		_ = m.restore(backup, ch.Path)
		pid2 := m.Dae.RunningPID()
		_, rollbackOut := m.Dae.Reload(pid2)
		res.RolledBack = true
		msg := res.Progress.Message
		if msg == "" {
			msg = out
		}
		if isZeroNode(msg) || isZeroNode(out) {
			res.SubscriptionEmpty = true
			res.Message = "订阅 0 节点，dae 拒绝重载，已回滚"
			_ = rollbackOut
			return res, fmt.Errorf("%w: %s", ErrSubscriptionEmpty, msg)
		}
		msg = explainDaeOut(msg)
		res.Message = "reload 失败，已回滚: " + msg
		_ = rollbackOut
		return res, fmt.Errorf("reload 失败(exit %d): %s", code, msg)
	}

	res.Applied = true
	res.Message = "已应用"
	return res, nil
}

func (m *Manager) LatestBackup() (string, error) {
	entries, err := os.ReadDir(m.BackupDir)
	if err != nil {
		return "", err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("无可用备份: %s", m.BackupDir)
	}
	sort.Strings(names)
	return filepath.Join(m.BackupDir, names[len(names)-1]), nil
}

func (m *Manager) Rollback(target string) (Result, error) {
	var res Result
	backup, err := m.LatestBackup()
	if err != nil {
		return res, err
	}
	res.BackupPath = backup
	if err := m.restore(backup, target); err != nil {
		return res, fmt.Errorf("恢复失败: %w", err)
	}
	pid := m.Dae.RunningPID()
	if pid <= 0 {
		res.Message = "文件已恢复，但 dae 没有在运行，未重载"
		return res, fmt.Errorf("%w：文件已恢复但未重载；%s", ErrDaeNotRunning, DaeDownHint)
	}
	code, out := m.Dae.Reload(pid)
	res.ReloadCode = code
	res.ReloadOut = out
	if prog, perr := m.Dae.Progress(); perr == nil {
		res.Progress = prog
	}
	if code != 0 || res.Progress.IsError() {
		res.Message = "回滚 reload 失败"
		return res, fmt.Errorf("回滚 reload 失败(exit %d): %s", code, out)
	}
	res.Applied = true
	res.Message = "已回滚到最近备份"
	return res, nil
}

// ApplyMulti 多文件事务：预检→逐份备份→写入→validate(validatePath)→reload→失败全部回滚。
// 用于内容层一次同时更新入口配置(subscription)与主体配置(global/group/dns/routing)。
func (m *Manager) ApplyMulti(validatePath string, chs []Change) (Result, error) {
	var res Result
	if len(chs) == 0 {
		return res, fmt.Errorf("无变更")
	}
	if m.Preflight != nil {
		for _, ch := range chs {
			if err := m.Preflight(ch.Path, ch.NewContent); err != nil {
				res.Message = "预检未通过，放弃写入"
				return res, err
			}
		}
	}
	type bk struct{ path, backup string }
	bks := make([]bk, 0, len(chs))
	for _, ch := range chs {
		b, err := m.backup(ch.Path)
		if err != nil {
			return res, fmt.Errorf("备份失败: %w", err)
		}
		bks = append(bks, bk{ch.Path, b})
	}
	res.BackupPath = bks[0].backup
	restoreAll := func() {
		for _, b := range bks {
			_ = m.restore(b.backup, b.path)
		}
	}
	for _, ch := range chs {
		if err := writeAtomic(ch.Path, ch.NewContent, 0o600); err != nil {
			restoreAll()
			res.RolledBack = true
			return res, fmt.Errorf("写入失败: %w", err)
		}
	}
	vr := m.Dae.Validate(validatePath)
	res.ValidateOut = vr.Output
	if !vr.OK {
		restoreAll()
		res.RolledBack = true
		res.Message = "validate 失败，已回滚"
		return res, fmt.Errorf("validate 失败(exit %d): %s", vr.ExitCode, vr.Output)
	}
	pid := m.Dae.RunningPID()
	if pid <= 0 {
		restoreAll()
		res.RolledBack = true
		res.Message = "dae 没有在运行，已回滚"
		return res, fmt.Errorf("%w：%s", ErrDaeNotRunning, DaeDownHint)
	}
	code, out := m.Dae.Reload(pid)
	res.ReloadCode = code
	res.ReloadOut = out
	if prog, perr := m.Dae.Progress(); perr == nil {
		res.Progress = prog
	}
	if code != 0 || res.Progress.IsError() {
		restoreAll()
		pid2 := m.Dae.RunningPID()
		_, rollbackOut := m.Dae.Reload(pid2)
		res.RolledBack = true
		msg := res.Progress.Message
		if msg == "" {
			msg = out
		}
		if isZeroNode(msg) || isZeroNode(out) {
			res.SubscriptionEmpty = true
			res.Message = "订阅 0 节点，dae 拒绝重载，已回滚"
			_ = rollbackOut
			return res, fmt.Errorf("%w: %s", ErrSubscriptionEmpty, msg)
		}
		res.Message = "reload 失败，已回滚: " + msg
		_ = rollbackOut
		return res, fmt.Errorf("reload 失败(exit %d): %s", code, msg)
	}
	res.Applied = true
	res.Message = "已应用"
	return res, nil
}
