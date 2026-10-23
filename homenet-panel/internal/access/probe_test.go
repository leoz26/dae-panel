package access

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner map[string]string

func (f fakeRunner) Run(name string, args ...string) (string, error) {
	key := strings.TrimSpace(name + " " + strings.Join(args, " "))
	v, ok := f[key]
	if !ok {
		return "", os.ErrNotExist
	}
	return v, nil
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// mkRoot 造一个假的根目录，模拟一台普通 Linux 主机。
func mkRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "etc/os-release"), "PRETTY_NAME=\"Test Linux 13\"\n")
	writeFile(t, filepath.Join(root, "sys/kernel/btf/vmlinux"), "")
	if err := os.MkdirAll(filepath.Join(root, "sys/fs/bpf"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "proc/sys/net/ipv4/ip_forward"), "1\n")
	writeFile(t, filepath.Join(root, "proc/self/limits"),
		"Limit                     Soft Limit           Hard Limit           Units\n"+
			"Max locked memory         65536                65536                bytes\n")
	writeFile(t, filepath.Join(root, "proc/net/udp"),
		"  sl  local_address rem_address   st\n"+
			"   0: 00000000:0035 00000000:0000 07 0 0 0 0 0 0\n")
	writeFile(t, filepath.Join(root, "sys/class/net/eth0/address"), "aa:bb:cc:dd:ee:ff\n")
	writeFile(t, filepath.Join(root, "sys/class/net/eth0/operstate"), "up\n")
	writeFile(t, filepath.Join(root, "sys/class/net/eth0/device/uevent"), "")
	return root
}

func linuxRunner() fakeRunner {
	return fakeRunner{
		"uname -r":                 "6.12.113-amd64\n",
		"nft list tables":          "table inet filter\ntable inet panel_tproxy\n",
		"ip -4 route show default": "default via 192.168.1.1 dev eth0 proto dhcp metric 100\n",
		"ip -o addr":               "1: lo    inet 127.0.0.1/8 scope host lo\n2: eth0    inet 192.168.1.85/24 brd 192.168.1.255 scope global eth0\n",
		"dae version":              "dae version v2.1.1\n",
		"pgrep -x dae":             "4242\n",
	}
}

func TestCollectParsesFacts(t *testing.T) {
	root := mkRoot(t)
	cfg := filepath.Join(root, "etc/dae/config.dae")
	writeFile(t, cfg, "global {\n    lan_interface: br-lan, eth1\n    wan_interface: auto\n}\n")

	p := Collect(linuxRunner(), root, cfg)

	if p.Platform != "linux" {
		t.Fatalf("platform = %q", p.Platform)
	}
	if !p.HasBPF || !p.HasBTF {
		t.Fatalf("BPF/BTF 应可用: bpf=%v btf=%v", p.HasBPF, p.HasBTF)
	}
	if p.MemlockKB != 64 {
		t.Fatalf("memlockKB = %d", p.MemlockKB)
	}
	if p.DefaultGw != "192.168.1.1" || p.DefaultIface != "eth0" {
		t.Fatalf("默认路由解析错误: %q %q", p.DefaultGw, p.DefaultIface)
	}
	if !p.IPForward {
		t.Fatal("ip_forward 应为 true")
	}
	if p.ServiceManager != "other" {
		t.Fatalf("serviceManager = %q（沙箱里没有 openwrt/systemd 标记）", p.ServiceManager)
	}
	if got := p.BusyPorts["53"]; got != "udp" {
		t.Fatalf("53 端口占用 = %q", got)
	}
	if !p.Nft {
		t.Fatal("nft 应可用")
	}
	if len(p.StaleTables) != 1 || !strings.Contains(p.StaleTables[0], "panel_tproxy") {
		t.Fatalf("遗留面板表未识别: %v", p.StaleTables)
	}
	if !p.DaeInstalled || p.DaeVersion != "dae version v2.1.1" || !p.DaeRunning {
		t.Fatalf("dae 探测: installed=%v version=%q running=%v", p.DaeInstalled, p.DaeVersion, p.DaeRunning)
	}
	if len(p.DaeLanIfaces) != 2 || p.DaeLanIfaces[0] != "br-lan" || p.DaeLanIfaces[1] != "eth1" {
		t.Fatalf("lan_interface 解析错误: %v", p.DaeLanIfaces)
	}
	// 普通 Linux → 默认 B1
	if p.Rec.Mode != ModeBypassManual {
		t.Fatalf("默认档位 = %q，期望 %q", p.Rec.Mode, ModeBypassManual)
	}
	if len(p.Rec.Reasons) == 0 || len(p.Rec.Questions) != 4 {
		t.Fatalf("推荐必须带理由与问卷: reasons=%d questions=%d", len(p.Rec.Reasons), len(p.Rec.Questions))
	}
	// 网卡元信息
	var eth *Iface
	for i := range p.Ifaces {
		if p.Ifaces[i].Name == "eth0" {
			eth = &p.Ifaces[i]
		}
	}
	if eth == nil || eth.MAC != "aa:bb:cc:dd:ee:ff" || !eth.Up || eth.Loopback {
		t.Fatalf("eth0 元信息错误: %+v", eth)
	}
}

func TestDaeLanIfacesFollowsInclude(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "config.dae")
	writeFile(t, entry, "include {\n    config.d/*.dae\n}\n")
	writeFile(t, filepath.Join(root, "config.d/panel-core.dae"),
		"global {\n    lan_interface: br-lan\n    tproxy_port: 12345\n}\n")
	writeFile(t, filepath.Join(root, "config.d/other.dae"),
		"global {\n    lan_interface: br-lan, eth1\n}\n")

	got := daeLanIfaces(entry)
	want := []string{"br-lan", "eth1"}
	if len(got) != len(want) {
		t.Fatalf("daeLanIfaces = %v，期望 %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("daeLanIfaces = %v，期望 %v", got, want)
		}
	}
}

func TestDetectDaeFallsBackToDashVersion(t *testing.T) {
	r := fakeRunner{"dae --version": "dae version v9.9.9\n"}
	ok, v := detectDae(r)
	if !ok || v != "dae version v9.9.9" {
		t.Fatalf("detectDae = %v %q", ok, v)
	}
	// 只支持 version 子命令的旧版本
	r2 := fakeRunner{"dae version": "v1.0.0\n"}
	if ok, v := detectDae(r2); !ok || v != "v1.0.0" {
		t.Fatalf("detectDae(version) = %v %q", ok, v)
	}
}

func TestCollectOpenWrtRecommendsGateway(t *testing.T) {
	root := mkRoot(t)
	writeFile(t, filepath.Join(root, "etc/openwrt_release"), "DISTRIB_ID='ImmortalWrt'\n")
	writeFile(t, filepath.Join(root, "sbin/procd"), "")
	p := Collect(linuxRunner(), root, "")

	if p.Platform != "openwrt" {
		t.Fatalf("platform = %q", p.Platform)
	}
	if p.ServiceManager != "procd" {
		t.Fatalf("serviceManager = %q", p.ServiceManager)
	}
	if p.Rec.Mode != ModeGateway {
		t.Fatalf("OpenWrt 默认档位 = %q，期望 gateway", p.Rec.Mode)
	}
}

func TestRecommendAnswersDriveMode(t *testing.T) {
	yes, no := true, false
	base := &Probe{Platform: "linux", HasBPF: true, HasBTF: true, DefaultGw: "192.168.1.1"}

	if got := Recommend(base, Answers{}).Mode; got != ModeBypassManual {
		t.Fatalf("无回答 → %q", got)
	}
	if got := Recommend(base, Answers{DHCPGateway: &yes}).Mode; got != ModeBypassRouterDHCP {
		t.Fatalf("能改 DHCP 网关 → %q", got)
	}
	if got := Recommend(base, Answers{DHCPGateway: &no}).Mode; got != ModeBypassManual {
		t.Fatalf("不能改 DHCP 网关 → %q", got)
	}
	// 策略路由只作提示，不改变默认档
	r := Recommend(base, Answers{PBR: &yes})
	if r.Mode != ModeBypassManual {
		t.Fatalf("PBR 不应改变默认档: %q", r.Mode)
	}
	joined := strings.Join(r.Reasons, "|")
	if !strings.Contains(joined, "策略路由") {
		t.Fatalf("缺少策略路由提示: %v", r.Reasons)
	}
	// 关 DHCP 只是"允许 B3"，不自动选
	if got := Recommend(base, Answers{CanDisableDHCP: &yes}).Mode; got != ModeBypassManual {
		t.Fatalf("能关 DHCP 不应自动切 B3: %q", got)
	}
}

func TestRecommendBlockers(t *testing.T) {
	// 非 Linux 平台
	r := Recommend(&Probe{Platform: "darwin", HasBPF: true, HasBTF: true}, Answers{})
	if r.Mode != ModeObserve || len(r.Blockers) == 0 {
		t.Fatalf("macOS 应只允许 observe: mode=%q blockers=%v", r.Mode, r.Blockers)
	}
	// 缺 BTF
	r = Recommend(&Probe{Platform: "linux", HasBPF: true}, Answers{})
	if r.Mode != ModeObserve {
		t.Fatalf("缺 BTF 应 observe: %q", r.Mode)
	}
	// 提示类不影响档位
	r = Recommend(&Probe{Platform: "linux", HasBPF: true, HasBTF: true, DefaultGw: "10.0.0.1",
		MemlockKB: 8192, StaleTables: []string{"table inet panel_tproxy"}, DaeInstalled: true}, Answers{})
	if r.Mode != ModeBypassManual {
		t.Fatalf("提示项不应降级: %q", r.Mode)
	}
	joined := strings.Join(r.Reasons, "|")
	for _, want := range []string{"memlock", "遗留", "lan_interface"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("缺少提示 %q: %v", want, r.Reasons)
		}
	}
}

func TestGuideAndLabels(t *testing.T) {
	modes := []AccessMode{ModeGateway, ModeBypassManual, ModeBypassRouterDHCP, ModeBypassSelfDHCP, ModeBypassPBR, ModeObserve}
	for _, m := range modes {
		if !m.Valid() {
			t.Fatalf("%q 应有效", m)
		}
		if m.Label() == "" || m.Label() == string(m) {
			t.Fatalf("%q 缺中文标签", m)
		}
		if len(Guide(m)) < 2 {
			t.Fatalf("%q 缺操作要点", m)
		}
	}
	if AccessMode("nope").Valid() {
		t.Fatal("未知档位不应有效")
	}
}

func TestParseDefaultRoute(t *testing.T) {
	gw, iface := parseDefaultRoute("default via 192.168.0.1 dev br-lan proto static\n")
	if gw != "192.168.0.1" || iface != "br-lan" {
		t.Fatalf("got %q %q", gw, iface)
	}
	if gw, iface := parseDefaultRoute(""); gw != "" || iface != "" {
		t.Fatal("空输出应返回空")
	}
}
