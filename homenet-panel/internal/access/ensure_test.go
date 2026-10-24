package access

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// withSysctlSandbox 把转发相关路径与 sysctl 调用替换成临时沙箱，返回已打开的 key 列表。
func withSysctlSandbox(t *testing.T) (*[]string, string, string) {
	t.Helper()
	dir := t.TempDir()
	proc := filepath.Join(dir, "proc")
	conf := filepath.Join(dir, "99-homenet-panel.conf")
	oldConf, oldProc, oldRun := sysctlConfPath, sysctlProcDir, sysctlRun
	sysctlConfPath, sysctlProcDir = conf, proc
	calls := []string{}
	sysctlRun = func(key string) error {
		calls = append(calls, key)
		return nil
	}
	t.Cleanup(func() { sysctlConfPath, sysctlProcDir, sysctlRun = oldConf, oldProc, oldRun })
	return &calls, proc, conf
}

func writeProc(t *testing.T, path, val string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(val+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureForwardingSkipsWithoutLanInterface(t *testing.T) {
	calls, proc, conf := withSysctlSandbox(t)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.dae")
	if err := os.WriteFile(cfg, []byte("global {\n  dial_mode: domain\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeProc(t, filepath.Join(proc, "net", "ipv4", "ip_forward"), "0")

	ifaces, changed, err := EnsureForwardingForConfig(cfg)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if len(ifaces) != 0 || changed {
		t.Fatalf("未声明 lan_interface 时不应改动: ifaces=%v changed=%v", ifaces, changed)
	}
	if len(*calls) != 0 {
		t.Fatalf("不应调用 sysctl: %v", *calls)
	}
	if _, e := os.Stat(conf); !os.IsNotExist(e) {
		t.Fatal("不应写持久化文件")
	}
}

func TestEnsureForwardingEnablesAndPersists(t *testing.T) {
	calls, proc, conf := withSysctlSandbox(t)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.dae")
	body := "global {\n  tproxy_port: 12345\n  lan_interface: eth9\n}\n"
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	writeProc(t, filepath.Join(proc, "net", "ipv4", "ip_forward"), "0")
	writeProc(t, filepath.Join(proc, "net", "ipv4", "conf", "eth9", "forwarding"), "0")

	ifaces, changed, err := EnsureForwardingForConfig(cfg)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if !changed || len(ifaces) != 1 || ifaces[0] != "eth9" {
		t.Fatalf("应识别 eth9 并改动: ifaces=%v changed=%v", ifaces, changed)
	}
	sort.Strings(*calls)
	want := []string{"net.ipv4.conf.all.forwarding", "net.ipv4.conf.eth9.forwarding", "net.ipv4.ip_forward"}
	if strings.Join(*calls, ",") != strings.Join(want, ",") {
		t.Fatalf("sysctl 调用不符: got %v want %v", *calls, want)
	}
	b, e := os.ReadFile(conf)
	if e != nil {
		t.Fatalf("应写入持久化文件: %v", e)
	}
	got := string(b)
	if !strings.Contains(got, "net.ipv4.ip_forward = 1") || !strings.Contains(got, "net.ipv4.conf.eth9.forwarding = 1") {
		t.Fatalf("持久化内容不符:\n%s", got)
	}
}

func TestEnsureForwardingNoopWhenAlreadyOn(t *testing.T) {
	calls, proc, conf := withSysctlSandbox(t)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.dae")
	if err := os.WriteFile(cfg, []byte("global {\n  lan_interface: eth9,eth10\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeProc(t, filepath.Join(proc, "net", "ipv4", "ip_forward"), "1")
	writeProc(t, filepath.Join(proc, "net", "ipv4", "conf", "eth9", "forwarding"), "1")
	writeProc(t, filepath.Join(proc, "net", "ipv4", "conf", "eth10", "forwarding"), "1")

	ifaces, changed, err := EnsureForwardingForConfig(cfg)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if changed || len(*calls) != 0 {
		t.Fatalf("已经打开时不应再调用 sysctl: changed=%v calls=%v", changed, *calls)
	}
	if len(ifaces) != 2 {
		t.Fatalf("应识别两个接口: %v", ifaces)
	}
	if _, e := os.Stat(conf); !os.IsNotExist(e) {
		t.Fatal("无改动时不应写持久化文件")
	}
}
