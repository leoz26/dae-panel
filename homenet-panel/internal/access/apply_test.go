package access

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"homenet-panel/internal/dae"
)

type nftRunner struct {
	calls     []string
	failOn    string
	tablesOut string
}

func (f *nftRunner) Run(name string, args ...string) (string, error) {
	cmd := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, cmd)
	if f.failOn != "" && strings.Contains(cmd, f.failOn) {
		return "", errors.New("fake nft failure")
	}
	if cmd == "nft list tables" {
		return f.tablesOut, nil
	}
	return "", nil
}

func (f *nftRunner) saw(sub string) bool {
	for _, c := range f.calls {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

type fakeDae struct {
	code     int
	out      string
	progress dae.Progress
}

func (f *fakeDae) Validate(p string) dae.ValidateResult {
	return dae.ValidateResult{OK: true, ExitCode: 0, Output: "validate ok"}
}
func (f *fakeDae) Reload(pid int) (int, string) { return f.code, f.out }
func (f *fakeDae) RunningPID() int              { return 4242 }
func (f *fakeDae) Progress() (dae.Progress, error) {
	return f.progress, nil
}

func newTestApplier(t *testing.T) (*Applier, *nftRunner, *fakeDae, string) {
	t.Helper()
	dir := t.TempDir()
	accessPath := filepath.Join(dir, "config.d", "panel-access.dae")
	if err := os.MkdirAll(filepath.Dir(accessPath), 0o700); err != nil {
		t.Fatal(err)
	}
	r := &nftRunner{}
	d := &fakeDae{progress: dae.Progress{Code: dae.ProgressReloadDone}}
	a := &Applier{
		Runner:     r,
		Dae:        d,
		EntryPath:  filepath.Join(dir, "config.dae"),
		AccessPath: accessPath,
		BackupDir:  filepath.Join(dir, "backups"),
		Probe:      testProbe(),
	}
	return a, r, d, dir
}

func TestApplyDryRunDoesNotWrite(t *testing.T) {
	a, r, _, _ := newTestApplier(t)
	opt := PlanOptions{}
	res, err := a.Apply(opt, "", false)
	if err != nil {
		t.Fatalf("dry-run 不应报错: %v", err)
	}
	if res.Applied || !res.DryRun {
		t.Fatalf("dry-run 结果不对: applied=%v dryRun=%v", res.Applied, res.DryRun)
	}
	if _, err := os.Stat(a.AccessPath); !os.IsNotExist(err) {
		t.Fatal("dry-run 不能写文件")
	}
	if len(r.calls) != 0 {
		t.Fatalf("dry-run 不能执行 nft，实际: %v", r.calls)
	}
	if !strings.Contains(res.Content, "lan_interface: en0") {
		t.Fatalf("应渲染 lan_interface: %s", res.Content)
	}
	if len(res.Commands) == 0 || !strings.Contains(res.Commands[0], "panel_dns") {
		t.Fatalf("应包含 panel_dns 命令: %v", res.Commands)
	}
}

func TestApplyFingerprintMismatch(t *testing.T) {
	a, r, _, _ := newTestApplier(t)
	_, err := a.Apply(PlanOptions{}, "deadbeefdeadbeef", true)
	if err == nil || !strings.Contains(err.Error(), "计划已变化") {
		t.Fatalf("指纹不一致应拒绝: %v", err)
	}
	if _, statErr := os.Stat(a.AccessPath); !os.IsNotExist(statErr) {
		t.Fatal("指纹不一致时不能写文件")
	}
	if len(r.calls) != 0 {
		t.Fatal("指纹不一致时不能执行 nft")
	}
}

func TestApplySuccess(t *testing.T) {
	a, r, _, _ := newTestApplier(t)
	opt := PlanOptions{}
	fp := PlanFor(testProbe(), opt).Fingerprint
	res, err := a.Apply(opt, fp, true)
	if err != nil {
		t.Fatalf("apply 应成功: %v", err)
	}
	if !res.Applied || res.DryRun {
		t.Fatalf("结果不对: %+v", res)
	}
	b, err := os.ReadFile(a.AccessPath)
	if err != nil {
		t.Fatalf("片段应已写入: %v", err)
	}
	if !strings.Contains(string(b), "tproxy_port: 12345") || !strings.Contains(string(b), "lan_interface: en0") {
		t.Fatalf("片段内容不对: %s", string(b))
	}
	st, _ := os.Stat(a.AccessPath)
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("片段应为 0600，实际 %v", st.Mode().Perm())
	}
	if !r.saw("add table inet panel_dns") {
		t.Fatalf("应创建 panel_dns 表: %v", r.calls)
	}
	if r.saw("add table inet panel_snat") {
		t.Fatalf("SNAT=auto 时不应创建 panel_snat: %v", r.calls)
	}
}

func TestApplyNftFailureRollsBack(t *testing.T) {
	a, r, _, _ := newTestApplier(t)
	r.failOn = "udp dport 53"
	opt := PlanOptions{}
	fp := PlanFor(testProbe(), opt).Fingerprint
	res, err := a.Apply(opt, fp, true)
	if err == nil {
		t.Fatal("nft 失败应报错")
	}
	if !res.RolledBack {
		t.Fatalf("应标记已回滚: %+v", res)
	}
	if _, statErr := os.Stat(a.AccessPath); !os.IsNotExist(statErr) {
		t.Fatal("回滚后片段应被移除")
	}
	if !r.saw("delete table inet panel_dns") {
		t.Fatalf("应删除自有表: %v", r.calls)
	}
}

func TestApplyReloadFailureRestoresOldFile(t *testing.T) {
	a, _, d, _ := newTestApplier(t)
	old := "# 旧片段\n"
	if err := os.WriteFile(a.AccessPath, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	d.code = 1
	d.out = "reload failed"
	d.progress = dae.Progress{Code: dae.ProgressReloadError}
	opt := PlanOptions{}
	fp := PlanFor(testProbe(), opt).Fingerprint
	res, err := a.Apply(opt, fp, true)
	if err == nil {
		t.Fatal("reload 失败应报错")
	}
	if !res.RolledBack {
		t.Fatalf("应标记已回滚: %+v", res)
	}
	b, rerr := os.ReadFile(a.AccessPath)
	if rerr != nil {
		t.Fatalf("回滚后旧片段应存在: %v", rerr)
	}
	if string(b) != old {
		t.Fatalf("旧片段应被还原，实际: %q", string(b))
	}
}

func TestRollbackRemovesFile(t *testing.T) {
	a, r, _, _ := newTestApplier(t)
	if err := os.WriteFile(a.AccessPath, []byte("# x"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := a.Rollback(true)
	if err != nil {
		t.Fatalf("回滚应成功: %v", err)
	}
	if !res.Applied {
		t.Fatalf("结果不对: %+v", res)
	}
	if _, statErr := os.Stat(a.AccessPath); !os.IsNotExist(statErr) {
		t.Fatal("回滚后片段应被删除")
	}
	if res.BackupPath == "" {
		t.Fatal("应备份被删除的片段")
	}
	if _, statErr := os.Stat(res.BackupPath); statErr != nil {
		t.Fatalf("备份文件应存在: %v", statErr)
	}
	for _, tb := range []string{"panel_dns", "panel_snat", "panel_tproxy"} {
		if !r.saw("delete table inet " + tb) {
			t.Fatalf("应删除自有表 %s: %v", tb, r.calls)
		}
	}
}

func TestRollbackDryRun(t *testing.T) {
	a, r, _, _ := newTestApplier(t)
	if err := os.WriteFile(a.AccessPath, []byte("# x"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := a.Rollback(false)
	if err != nil {
		t.Fatalf("dry-run 不应报错: %v", err)
	}
	if res.Applied || !res.DryRun {
		t.Fatalf("dry-run 结果不对: %+v", res)
	}
	if len(res.Commands) != 4 {
		t.Fatalf("应列出 3 条删表 + 1 条删文件，实际 %v", res.Commands)
	}
	if len(r.calls) != 0 {
		t.Fatalf("dry-run 不能执行任何命令: %v", r.calls)
	}
	if _, statErr := os.Stat(a.AccessPath); statErr != nil {
		t.Fatal("dry-run 不能删文件")
	}
}

func TestApplyRecordsIPForwardAndRollbackRestores(t *testing.T) {
	a, r, _, dir := newTestApplier(t)
	p := filepath.Join(dir, "ip_forward")
	if err := os.WriteFile(p, []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := sysIPForwardPath
	sysIPForwardPath = p
	defer func() { sysIPForwardPath = old }()

	opt := PlanOptions{}
	fp := PlanFor(testProbe(), opt).Fingerprint
	if _, err := a.Apply(opt, fp, true); err != nil {
		t.Fatalf("apply 应成功: %v", err)
	}
	st, err := os.ReadFile(a.statePath())
	if err != nil {
		t.Fatalf("应记录基线状态: %v", err)
	}
	if !strings.Contains(string(st), "originalIpForward") {
		t.Fatalf("状态文件应含 originalIpForward: %s", string(st))
	}
	// 模拟 dae 的 auto_config_kernel_parameter 把内核打开了转发
	if err := os.WriteFile(p, []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Rollback(true); err != nil {
		t.Fatalf("回滚应成功: %v", err)
	}
	if !r.saw("net.ipv4.ip_forward=0") {
		t.Fatalf("回滚应还原 ip_forward: %v", r.calls)
	}
	if _, err := os.Stat(a.statePath()); !os.IsNotExist(err) {
		t.Fatal("回滚后应删除状态文件")
	}
}

func TestApplyPersistsForwardingAndRollbackRemovesIt(t *testing.T) {
	a, r, _, dir := newTestApplier(t)
	conf := filepath.Join(dir, "sysctl.d", "99-homenet-panel.conf")
	oldConf := sysctlConfPath
	sysctlConfPath = conf
	defer func() { sysctlConfPath = oldConf }()

	opt := PlanOptions{}
	fp := PlanFor(testProbe(), opt).Fingerprint
	res, err := a.Apply(opt, fp, true)
	if err != nil {
		t.Fatalf("apply 应成功: %v", err)
	}
	if !r.saw("net.ipv4.ip_forward=1") {
		t.Fatalf("apply 应先打开转发（dae 绑定 lan_interface 的前置条件）: %v", r.calls)
	}
	if !r.saw("net.ipv4.conf.en0.forwarding=1") {
		t.Fatalf("apply 应打开 LAN 接口转发: %v", r.calls)
	}
	b, err := os.ReadFile(conf)
	if err != nil {
		t.Fatalf("应写入持久化文件: %v", err)
	}
	if !strings.Contains(string(b), "net.ipv4.ip_forward = 1") || !strings.Contains(string(b), "net.ipv4.conf.en0.forwarding = 1") {
		t.Fatalf("持久化文件内容不对: %s", string(b))
	}
	found := false
	for _, s := range res.Steps {
		if s.Name == "sysctl-persist" && s.OK {
			found = true
		}
	}
	if !found {
		t.Fatal("结果里应有 sysctl-persist 步骤")
	}

	if _, err := a.Rollback(true); err != nil {
		t.Fatalf("回滚应成功: %v", err)
	}
	if _, err := os.Stat(conf); !os.IsNotExist(err) {
		t.Fatalf("回滚应删除自有持久化文件: %v", err)
	}
}

func TestApplyRestoresPreexistingSysctlFile(t *testing.T) {
	a, _, _, dir := newTestApplier(t)
	conf := filepath.Join(dir, "sysctl.d", "99-homenet-panel.conf")
	if err := os.MkdirAll(filepath.Dir(conf), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "net.ipv4.tcp_syncookies = 1\n"
	if err := os.WriteFile(conf, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	oldConf := sysctlConfPath
	sysctlConfPath = conf
	defer func() { sysctlConfPath = oldConf }()

	opt := PlanOptions{}
	fp := PlanFor(testProbe(), opt).Fingerprint
	if _, err := a.Apply(opt, fp, true); err != nil {
		t.Fatalf("apply 应成功: %v", err)
	}
	if _, err := a.Rollback(true); err != nil {
		t.Fatalf("回滚应成功: %v", err)
	}
	b, err := os.ReadFile(conf)
	if err != nil {
		t.Fatalf("回滚应写回应用前的文件: %v", err)
	}
	if string(b) != original {
		t.Fatalf("应逐字还原原文件，实际 %q", string(b))
	}
}

func TestRebuildTablesAfterReboot(t *testing.T) {
	a, r, _, _ := newTestApplier(t)
	fp := PlanFor(testProbe(), PlanOptions{}).Fingerprint
	if _, err := a.Apply(PlanOptions{}, fp, true); err != nil {
		t.Fatalf("apply 应成功: %v", err)
	}
	// 模拟重启：内核里的自有表全丢
	r.calls = nil
	r.tablesOut = "table ip nat\ntable ip filter\n"
	steps, rebuilt, err := a.RebuildTables()
	if err != nil {
		t.Fatalf("重建不应报错: %v", err)
	}
	if !rebuilt {
		t.Fatal("表缺失时应重建")
	}
	if !r.saw("add table inet panel_dns") {
		t.Fatalf("应重建 panel_dns: %v", r.calls)
	}
	if len(steps) == 0 {
		t.Fatal("应留下步骤记录")
	}
}

func TestRebuildTablesNoopWhenPresent(t *testing.T) {
	a, r, _, _ := newTestApplier(t)
	fp := PlanFor(testProbe(), PlanOptions{}).Fingerprint
	if _, err := a.Apply(PlanOptions{}, fp, true); err != nil {
		t.Fatalf("apply 应成功: %v", err)
	}
	r.calls = nil
	r.tablesOut = "table inet panel_dns\ntable ip nat\n"
	_, rebuilt, err := a.RebuildTables()
	if err != nil || rebuilt {
		t.Fatalf("表已存在时不该重建: rebuilt=%v err=%v", rebuilt, err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("只应执行一次 nft list tables，实际: %v", r.calls)
	}
}

func TestRebuildTablesWithoutRecord(t *testing.T) {
	a, r, _, _ := newTestApplier(t)
	r.tablesOut = "table ip nat\n"
	_, rebuilt, err := a.RebuildTables()
	if err != nil || rebuilt {
		t.Fatalf("没有记录时不该重建: rebuilt=%v err=%v", rebuilt, err)
	}
	if len(r.calls) != 0 {
		t.Fatalf("没有记录时不应执行任何命令: %v", r.calls)
	}
}

func TestRebuildTablesUsesRecordedCommands(t *testing.T) {
	a, r, _, _ := newTestApplier(t)
	fp := PlanFor(testProbe(), PlanOptions{}).Fingerprint
	if _, err := a.Apply(PlanOptions{}, fp, true); err != nil {
		t.Fatalf("apply 应成功: %v", err)
	}
	st := a.readState()
	if len(st.NftCommands) == 0 {
		t.Fatal("apply 后应把 nft 命令记录进状态文件")
	}
	r.calls = nil
	r.tablesOut = ""
	if _, _, err := a.RebuildTables(); err != nil {
		t.Fatalf("重建不应报错: %v", err)
	}
	if !r.saw("add chain inet panel_dns prerouting") {
		t.Fatalf("应按记录重建链与规则: %v", r.calls)
	}
}
