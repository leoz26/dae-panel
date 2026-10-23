package configtx

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"homenet-panel/internal/dae"
)

// mockDae 可编程模拟 dae 行为。
type mockDae struct {
	validateOK  bool
	reloadCode  int
	reloadOut   string
	prog        dae.Progress
	progErr     error
	runningPID  int
	reloadCalls int
}

func (m *mockDae) Validate(string) dae.ValidateResult {
	return dae.ValidateResult{ExitCode: map[bool]int{true: 0, false: 1}[m.validateOK], OK: m.validateOK, Output: "validate-out"}
}
func (m *mockDae) Reload(int) (int, string)        { m.reloadCalls++; return m.reloadCode, m.reloadOut }
func (m *mockDae) RunningPID() int                 { return m.runningPID }
func (m *mockDae) Progress() (dae.Progress, error) { return m.prog, m.progErr }

func newMgr(t *testing.T, d DaeClient) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	target := filepath.Join(dir, "config.dae")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	return New(d, filepath.Join(dir, "backups")), target
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, _ := os.ReadFile(p)
	return string(b)
}

func TestApplySuccess(t *testing.T) {
	d := &mockDae{validateOK: true, reloadCode: 0, prog: dae.Progress{Code: '2', Message: "OK"}, runningPID: 42}
	m, target := newMgr(t, d)
	res, err := m.Apply(Change{Path: target, NewContent: []byte("new")})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Applied || res.RolledBack {
		t.Fatalf("want applied, got %+v", res)
	}
	if readFile(t, target) != "new" {
		t.Fatalf("target not updated")
	}
	if res.BackupPath == "" {
		t.Fatalf("expected backup path")
	}
	if readFile(t, res.BackupPath) != "original" {
		t.Fatalf("backup content wrong")
	}
}

func TestApplyValidateFailRollback(t *testing.T) {
	d := &mockDae{validateOK: false, runningPID: 42}
	m, target := newMgr(t, d)
	res, err := m.Apply(Change{Path: target, NewContent: []byte("new")})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !res.RolledBack {
		t.Fatalf("expected rolledBack, got %+v", res)
	}
	if readFile(t, target) != "original" {
		t.Fatalf("target should be rolled back, got %q", readFile(t, target))
	}
	if d.reloadCalls != 0 {
		t.Fatalf("reload should not be called on validate fail")
	}
}

func TestApplyReloadFailRollback(t *testing.T) {
	d := &mockDae{validateOK: true, reloadCode: 1, reloadOut: "boom", prog: dae.Progress{Code: '3', Message: "boom"}, runningPID: 42}
	m, target := newMgr(t, d)
	res, err := m.Apply(Change{Path: target, NewContent: []byte("new")})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !res.RolledBack {
		t.Fatalf("expected rolledBack")
	}
	if readFile(t, target) != "original" {
		t.Fatalf("target should be rolled back")
	}
	// reload 应被调用两次：一次失败、一次回滚重载
	if d.reloadCalls != 2 {
		t.Fatalf("want 2 reload calls, got %d", d.reloadCalls)
	}
}

func TestApplyZeroNodes(t *testing.T) {
	d := &mockDae{
		validateOK: true,
		reloadCode: 1,
		prog:       dae.Progress{Code: '3', Message: "refusing reload with 0 nodes: all subscription resolving failed"},
		runningPID: 42,
	}
	m, target := newMgr(t, d)
	res, err := m.Apply(Change{Path: target, NewContent: []byte("new")})
	if !errors.Is(err, ErrSubscriptionEmpty) {
		t.Fatalf("want ErrSubscriptionEmpty, got %v", err)
	}
	if !res.SubscriptionEmpty {
		t.Fatalf("want SubscriptionEmpty flag")
	}
	if readFile(t, target) != "original" {
		t.Fatalf("target should be rolled back")
	}
}

func TestApplyPreflightBlocks(t *testing.T) {
	d := &mockDae{validateOK: true, runningPID: 42}
	m, target := newMgr(t, d)
	m.Preflight = func(path string, c []byte) error { return errors.New("blocked") }
	res, err := m.Apply(Change{Path: target, NewContent: []byte("new")})
	if err == nil {
		t.Fatalf("expected preflight block")
	}
	if res.Applied || res.RolledBack {
		t.Fatalf("should not write on preflight fail")
	}
	if readFile(t, target) != "original" {
		t.Fatalf("target must remain unchanged")
	}
}

func TestRollback(t *testing.T) {
	d := &mockDae{validateOK: true, reloadCode: 0, prog: dae.Progress{Code: '2'}, runningPID: 42}
	m, target := newMgr(t, d)
	// 制造一个备份
	if _, err := m.backup(target); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := m.Rollback(target)
	if err != nil {
		t.Fatalf("rollback err: %v", err)
	}
	if !res.Applied {
		t.Fatalf("want applied")
	}
	if readFile(t, target) != "original" {
		t.Fatalf("want original after rollback, got %q", readFile(t, target))
	}
}
