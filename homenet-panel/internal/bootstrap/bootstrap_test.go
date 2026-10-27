package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"homenet-panel/internal/model"
)

type fakeRunner struct {
	calls []string
	fail  map[string]error
	out   map[string]string
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, key)
	if f.fail != nil {
		if err, ok := f.fail[key]; ok {
			return f.out[key], err
		}
	}
	if f.out != nil {
		return f.out[key], nil
	}
	return "", nil
}

func testOpts(t *testing.T) (Options, *fakeRunner) {
	t.Helper()
	dir := t.TempDir()
	r := &fakeRunner{out: map[string]string{
		"dae --version":           "dae version v2.1.1",
		"systemctl is-active dae": "active",
	}}
	return Options{
		Runner:    r,
		EtcDir:    dir,
		EntryPath: filepath.Join(dir, "config.dae"),
		CorePath:  filepath.Join(dir, "config.d", "panel-core.dae"),
		Fetch:     func(url string) ([]byte, error) { return []byte("stub"), nil },
		IsRoot:    func() bool { return true },
	}, r
}

func TestWriteInitConfig(t *testing.T) {
	opts, _ := testOpts(t)
	res := WriteInitConfig(opts, false)
	if !res.OK {
		t.Fatalf("应成功: %+v", res)
	}
	entry, err := os.ReadFile(opts.EntryPath)
	if err != nil {
		t.Fatalf("入口配置未写入: %v", err)
	}
	if !strings.Contains(string(entry), model.Header) || !strings.Contains(string(entry), "config.d/*.dae") {
		t.Fatalf("入口配置内容不对: %s", entry)
	}
	core, err := os.ReadFile(opts.CorePath)
	if err != nil {
		t.Fatalf("核心配置未写入: %v", err)
	}
	for _, want := range []string{"global {", "tproxy_port: 12345", "fallback: direct", "dip(192.168.0.0/16) -> direct"} {
		if !strings.Contains(string(core), want) {
			t.Errorf("核心配置缺少 %q", want)
		}
	}
	if info, _ := os.Stat(opts.CorePath); info.Mode().Perm() != 0o600 {
		t.Errorf("核心配置权限应为 0600，实际 %v", info.Mode().Perm())
	}
}

func TestWriteInitConfigRefusesForeignConfig(t *testing.T) {
	opts, _ := testOpts(t)
	if err := os.MkdirAll(filepath.Dir(opts.EntryPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(opts.EntryPath, []byte("routing {\n    fallback: direct\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res := WriteInitConfig(opts, false)
	if res.OK {
		t.Fatal("已存在配置时应拒绝覆盖")
	}
	if !strings.Contains(res.Error, "拒绝覆盖") {
		t.Errorf("错误信息不明确: %s", res.Error)
	}
	// force 时必须先备份再写
	res = WriteInitConfig(opts, true)
	if !res.OK {
		t.Fatalf("force 应成功: %+v", res)
	}
	matches, _ := filepath.Glob(opts.EntryPath + ".bak-*")
	if len(matches) != 1 {
		t.Fatalf("应留一份备份，实际 %v", matches)
	}
}

func TestWriteInitConfigRollsBackWhenValidateFails(t *testing.T) {
	opts, r := testOpts(t)
	if err := os.MkdirAll(filepath.Dir(opts.EntryPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(opts.EntryPath, []byte(model.Header+"\ninclude {\n    config.d/*.dae\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.fail = map[string]error{"dae validate -c " + opts.EntryPath: os.ErrInvalid}
	res := WriteInitConfig(opts, true)
	if res.OK {
		t.Fatal("validate 失败时不应报告成功")
	}
	found := false
	for _, s := range res.Steps {
		if strings.Contains(s.Title, "回滚") {
			found = true
		}
	}
	if !found {
		t.Errorf("应记录回滚步骤: %+v", res.Steps)
	}
}

func TestInstallDaeDryRun(t *testing.T) {
	opts, r := testOpts(t)
	r.fail = map[string]error{"dae --version": os.ErrNotExist}
	r.out = map[string]string{
		"apt-get --version": "apt 2.6.1 (amd64)",
	}
	opts.DryRun = true
	res := InstallDae(opts)
	if !res.OK {
		t.Fatalf("dry-run 应成功: %+v", res)
	}
	if len(res.Steps) == 0 {
		t.Fatal("应有步骤记录")
	}
	if _, err := os.Stat("/etc/apt/sources.list.d/daeuniverse.list"); err == nil {
		t.Fatal("dry-run 不应真的写系统文件")
	}
}

func TestStartDaeSystemd(t *testing.T) {
	opts, r := testOpts(t)
	res := StartDae(opts)
	if !res.OK {
		t.Fatalf("应成功: %+v", res)
	}
	joined := strings.Join(r.calls, "\n")
	if !strings.Contains(joined, "systemctl enable dae") || !strings.Contains(joined, "systemctl restart dae") {
		t.Errorf("未按预期调用 systemctl: %s", joined)
	}
}

func TestStopDaeSystemd(t *testing.T) {
	opts, r := testOpts(t)
	r.out["systemctl is-active dae"] = "inactive"
	res := StopDae(opts)
	if !res.OK {
		t.Fatalf("应成功: %+v", res)
	}
	if !strings.Contains(strings.Join(r.calls, "\n"), "systemctl stop dae") {
		t.Errorf("未调用 systemctl stop dae: %v", r.calls)
	}
}

func TestStopDaeStaysActive(t *testing.T) {
	opts, r := testOpts(t)
	r.out["systemctl is-active dae"] = "active"
	opts.DryRun = true // 跳过 5 秒轮询，只验命令路径
	res := StopDae(opts)
	if !res.OK {
		t.Fatalf("dry-run 应成功: %+v", res)
	}
	if len(res.Steps) == 0 || !strings.Contains(res.Steps[0].Command, "systemctl stop dae") {
		t.Errorf("dry-run 也应记录命令: %+v", res.Steps)
	}
	_ = r
}

func TestStopDaeNotRoot(t *testing.T) {
	opts, _ := testOpts(t)
	opts.IsRoot = func() bool { return false }
	res := StopDae(opts)
	if res.OK || !strings.Contains(res.Error, "root") {
		t.Fatalf("非 root 应被拒绝: %+v", res)
	}
}

func TestRestartDaeSystemd(t *testing.T) {
	opts, r := testOpts(t)
	res := RestartDae(opts)
	if !res.OK {
		t.Fatalf("应成功: %+v", res)
	}
	if !strings.Contains(strings.Join(r.calls, "\n"), "systemctl restart dae") {
		t.Errorf("未调用 systemctl restart dae: %v", r.calls)
	}
}

func TestDetectStatus(t *testing.T) {
	opts, r := testOpts(t)
	r.out["apt-get --version"] = "apt 2.6.1"
	r.out["pgrep -x dae"] = "12345"
	st := Detect(opts)
	if !st.HasApt || !st.DaeInstalled || !st.DaeRunning {
		t.Fatalf("状态探测不对: %+v", st)
	}
	if st.ConfigExists {
		t.Error("临时目录里不应有配置")
	}
	if st.ServiceManager != "none" && st.ServiceManager != "systemd" {
		t.Errorf("服务管理器识别异常: %s", st.ServiceManager)
	}
}

func TestWriteInitConfigRefusesManagedConfigToo(t *testing.T) {
	opts, _ := testOpts(t)
	if err := os.MkdirAll(filepath.Dir(opts.EntryPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(opts.EntryPath, []byte(model.Header+"\ninclude {\n    config.d/*.dae\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(opts.CorePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(opts.CorePath, []byte(model.Header+"\nrouting {\n    fallback: proxy\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if res := WriteInitConfig(opts, false); res.OK {
		t.Fatal("已经由面板托管且有内容的配置也必须要求 force，否则会冲掉线上规则")
	}
}
