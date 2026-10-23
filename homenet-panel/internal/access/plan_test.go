package access

import (
	"strings"
	"testing"
)

func testProbe() *Probe {
	return &Probe{
		Platform: "linux", Arch: "amd64", ServiceManager: "systemd",
		HasBPF: true, HasBTF: true, Nft: true, IPForward: false,
		Ifaces: []Iface{
			{Name: "lo", Addrs: []string{"127.0.0.1/8"}, Loopback: true, Up: true},
			{Name: "en0", Addrs: []string{"192.168.1.85/24"}, Up: true, MAC: "aa:bb"},
			{Name: "dae0", Up: true, Virtual: true},
		},
		DaeInstalled: true, DaeVersion: "dae version v2.1.1", DaeRunning: true,
		Rec: Rec{Mode: ModeBypassManual},
	}
}

func joinSteps(steps []Step) string {
	out := ""
	for _, s := range steps {
		out += s.Title + "|" + s.Detail + "|" + s.Snippet + "|"
	}
	return out
}

func TestPlanDefaultsToBypassManual(t *testing.T) {
	pl := PlanFor(testProbe(), PlanOptions{})
	if pl.Mode != ModeBypassManual {
		t.Fatalf("默认档位应为 B1，实际 %s", pl.Mode)
	}
	if pl.ModeExplicit {
		t.Fatal("未显式指定档位时 ModeExplicit 应为 false")
	}
	if !pl.DryRun {
		t.Fatal("计划必须是 dry-run")
	}
	if pl.TProxyPort != DefaultTProxyPort {
		t.Fatalf("默认 tproxy 端口应为 %d，实际 %d", DefaultTProxyPort, pl.TProxyPort)
	}
	if len(pl.LanIfaces) != 1 || pl.LanIfaces[0] != "en0" {
		t.Fatalf("应探测到内网网卡 en0，实际 %v", pl.LanIfaces)
	}
	if pl.ProxyIP != "192.168.1.85" {
		t.Fatalf("应取到内网 IPv4，实际 %q", pl.ProxyIP)
	}
	all := joinSteps(pl.Steps)
	if !strings.Contains(all, "lan_interface: en0") {
		t.Fatalf("dae global 片段应包含 lan_interface: en0：%s", all)
	}
	if !strings.Contains(all, "tproxy_port: 12345") {
		t.Fatalf("dae global 片段应包含 tproxy_port：%s", all)
	}
	if !strings.Contains(all, "panel_dns") {
		t.Fatal("应渲染自有 nft 表 panel_dns")
	}
	m := joinSteps(pl.ManualSteps)
	if !strings.Contains(m, "网关: 192.168.1.85") || !strings.Contains(m, "DNS: 192.168.1.85") {
		t.Fatalf("B1 的手动步骤应包含设备端改网关与 DNS：%s", m)
	}
	if len(pl.Checks) == 0 {
		t.Fatal("应带自检清单")
	}
	if pl.Fingerprint == "" {
		t.Fatal("应有计划指纹")
	}
}

func TestPlanRouterDHCPSteps(t *testing.T) {
	pl := PlanFor(testProbe(), PlanOptions{Mode: ModeBypassRouterDHCP, TestDevice: "客厅手机"})
	if pl.Mode != ModeBypassRouterDHCP || !pl.ModeExplicit {
		t.Fatalf("应使用显式档位 B2，实际 %s", pl.Mode)
	}
	m := joinSteps(pl.ManualSteps)
	if !strings.Contains(m, "DHCP") || !strings.Contains(m, "原值") {
		t.Fatalf("B2 应提醒先记录 DHCP 原值：%s", m)
	}
	rb := joinSteps(pl.Rollback)
	if !strings.Contains(rb, "nft delete table inet panel_dns") {
		t.Fatalf("回滚应包含删自有 nft 表：%s", rb)
	}
	w := strings.Join(pl.Warnings, "；")
	if !strings.Contains(w, "断网") {
		t.Fatalf("B2 应提示断网风险：%s", w)
	}
}

func TestPlanObserveHasNoSteps(t *testing.T) {
	pl := PlanFor(testProbe(), PlanOptions{Mode: ModeObserve})
	if len(pl.Steps) != 0 {
		t.Fatalf("观察模式不应有面板侧变更步骤，实际 %d 条", len(pl.Steps))
	}
	if len(pl.Checks) != 1 {
		t.Fatalf("观察模式只保留 dae 运行自检，实际 %d 条", len(pl.Checks))
	}
}

func TestPlanExplicitLanIfacesWin(t *testing.T) {
	pl := PlanFor(testProbe(), PlanOptions{LanIfaces: []string{"br-lan"}, TProxyPort: 7891, DNSMode: "none", SNAT: "on"})
	if len(pl.LanIfaces) != 1 || pl.LanIfaces[0] != "br-lan" {
		t.Fatalf("显式 lanIfaces 应优先，实际 %v", pl.LanIfaces)
	}
	if pl.TProxyPort != 7891 {
		t.Fatalf("应使用显式端口，实际 %d", pl.TProxyPort)
	}
	all := joinSteps(pl.Steps)
	if !strings.Contains(all, "DNS 重定向已关闭") {
		t.Fatal("DNSMode=none 时不应生成重定向规则")
	}
	if !strings.Contains(all, "masquerade") {
		t.Fatal("SNAT=on 时应渲染 masquerade 规则")
	}
}

func TestPlanFingerprintStableAndSensitive(t *testing.T) {
	a := PlanFor(testProbe(), PlanOptions{})
	b := PlanFor(testProbe(), PlanOptions{})
	if a.Fingerprint != b.Fingerprint {
		t.Fatal("相同输入的计划指纹应一致")
	}
	c := PlanFor(testProbe(), PlanOptions{TProxyPort: 23456})
	if a.Fingerprint == c.Fingerprint {
		t.Fatal("端口变化后指纹应变化")
	}
}

func TestVerifyLevels(t *testing.T) {
	pr := testProbe()
	res := Verify(VerifyInput{Probe: pr, Clients: 3, DNSQueries: 12, IfaceRx: 1000, IfaceTx: 2000})
	for _, c := range res {
		if c.Level == "fail" {
			t.Fatalf("%s 不应为 fail：%s", c.Key, c.Detail)
		}
	}
	if len(res) != 6 {
		t.Fatalf("应有 6 项自检，实际 %d", len(res))
	}
	empty := Verify(VerifyInput{Probe: pr})
	warns := 0
	for _, c := range empty {
		if c.Level == "warn" {
			warns++
		}
	}
	if warns != 3 {
		t.Fatalf("无流量时应 3 项 warn（clients/dae0/dns），实际 %d", warns)
	}
	pr2 := testProbe()
	pr2.StaleTables = []string{"panel_tproxy"}
	pr2.DaeRunning = false
	res2 := Verify(VerifyInput{Probe: pr2})
	var fail, stale int
	for _, c := range res2 {
		if c.Key == "dae_running" && c.Level == "fail" {
			fail++
		}
		if c.Key == "stale_tables" && c.Level == "warn" {
			stale++
		}
	}
	if fail != 1 || stale != 1 {
		t.Fatalf("dae 未运行应 fail、遗留表应 warn，实际 fail=%d stale=%d", fail, stale)
	}
}
