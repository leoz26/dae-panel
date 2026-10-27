package mesh

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const addrOut = `1: lo    inet 127.0.0.1/8 scope host lo
2: eth0    inet 192.168.1.85/24 brd 192.168.1.255 scope global eth0
3: tun0    inet 10.144.0.2/24 scope global tun0
4: dae0    inet 169.254.0.11/32 scope global dae0
`

func lanRunner() fakeRunner {
	return fakeRunner{
		out: map[string]string{
			"ip -4 -o addr show":       addrOut,
			"ip -4 route show default": "default via 192.168.1.1 dev eth0 proto static",
			"easytier-cli route":       routeOut,
		},
		err: map[string]error{},
	}
}

func TestDetectLanFacts(t *testing.T) {
	r := lanRunner()
	r.err["nft list table ip hnp_mesh"] = errNotFound()
	r.err["systemctl is-enabled hnp-mesh-nat"] = errNotFound()
	st := DetectLan(Options{Runner: r, RootDir: t.TempDir()})
	if st.MeshIface != "tun0" {
		t.Fatalf("meshIface=%q", st.MeshIface)
	}
	if len(st.MeshAddrs) != 1 || st.MeshAddrs[0] != "10.144.0.2/24" {
		t.Fatalf("meshAddrs=%v", st.MeshAddrs)
	}
	if st.LanIface != "eth0" {
		t.Fatalf("lanIface=%q", st.LanIface)
	}
	if len(st.LanCIDRs) != 1 || st.LanCIDRs[0] != "192.168.1.0/24" {
		t.Fatalf("lanCidrs=%v", st.LanCIDRs)
	}
	if len(st.PeerCIDRs) != 1 || st.PeerCIDRs[0] != "192.168.5.0/24" {
		t.Fatalf("peerCidrs=%v", st.PeerCIDRs)
	}
	if st.Enabled || st.NftTable || st.ServiceEnabled {
		t.Fatalf("enabled=%v nft=%v svc=%v", st.Enabled, st.NftTable, st.ServiceEnabled)
	}
	if st.Note == "" {
		t.Fatal("note empty")
	}
}

func TestDetectLanEnabled(t *testing.T) {
	r := lanRunner()
	r.out["nft list table ip hnp_mesh"] = "table ip hnp_mesh {\n\tchain post {\n\t\ttype nat hook postrouting priority srcnat; policy accept;\n\t\toifname \"tun0\" ip saddr 192.168.1.0/24 masquerade\n\t}\n}"
	r.out["systemctl is-enabled hnp-mesh-nat"] = "enabled"
	st := DetectLan(Options{Runner: r, RootDir: t.TempDir()})
	if !st.Enabled || !st.NftTable || !st.ServiceEnabled {
		t.Fatalf("enabled=%v nft=%v svc=%v", st.Enabled, st.NftTable, st.ServiceEnabled)
	}
	if len(st.Rules) != 1 || !strings.Contains(st.Rules[0], "masquerade") {
		t.Fatalf("rules=%v", st.Rules)
	}
}

func TestDetectLanNoteWhenNoPeerCIDR(t *testing.T) {
	r := lanRunner()
	r.out["easytier-cli route"] = ""
	r.err["nft list table ip hnp_mesh"] = errNotFound()
	r.err["systemctl is-enabled hnp-mesh-nat"] = errNotFound()
	st := DetectLan(Options{Runner: r, RootDir: t.TempDir()})
	if len(st.PeerCIDRs) != 0 || !strings.Contains(st.Note, "-n") {
		t.Fatalf("peer=%v note=%q", st.PeerCIDRs, st.Note)
	}
}

func TestRenderNatScriptAndUnit(t *testing.T) {
	s := RenderNatScript(LanConfig{LanCIDRs: []string{"192.168.1.0/24"}, MeshIface: "tun0"})
	if !strings.Contains(s, "nft add table ip hnp_mesh") {
		t.Fatalf("script=%s", s)
	}
	if !strings.Contains(s, `oifname "tun0" ip saddr 192.168.1.0/24 masquerade`) {
		t.Fatalf("script=%s", s)
	}
	if strings.Contains(s, "ens2") {
		t.Fatal("不应出现其他网卡")
	}
	u := RenderNatUnit()
	if !strings.Contains(u, "ExecStart="+meshNatScript) || !strings.Contains(u, "RemainAfterExit=yes") {
		t.Fatalf("unit=%s", u)
	}
}

func TestApplyLanDryRunTouchesNothing(t *testing.T) {
	root := t.TempDir()
	r := lanRunner()
	res := ApplyLan(Options{Runner: r, RootDir: root}, LanConfig{LanCIDRs: []string{"192.168.1.0/24"}, MeshIface: "tun0"}, true)
	if !res.OK || !res.DryRun {
		t.Fatalf("res=%+v", res)
	}
	want := false
	for _, st := range res.Steps {
		if strings.Contains(st.Command, "masquerade") {
			want = true
		}
	}
	if !want {
		t.Fatalf("steps=%+v", res.Steps)
	}
	if _, err := os.Stat(filepath.Join(root, "usr/local/sbin/hnp-mesh-nat.sh")); err == nil {
		t.Fatal("dry-run 不应写文件")
	}
}

func TestApplyLanWritesScriptAndUnit(t *testing.T) {
	root := t.TempDir()
	r := lanRunner()
	r.out["sh "+meshNatScript] = ""
	r.out["systemctl daemon-reload"] = ""
	r.out["systemctl enable --now hnp-mesh-nat"] = ""
	res := ApplyLan(Options{Runner: r, RootDir: root}, LanConfig{LanCIDRs: []string{"192.168.1.0/24"}, MeshIface: "tun0"}, false)
	if !res.OK {
		t.Fatalf("res=%+v", res)
	}
	sp := filepath.Join(root, "usr/local/sbin/hnp-mesh-nat.sh")
	data, err := os.ReadFile(sp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "192.168.1.0/24 masquerade") {
		t.Fatalf("script=%s", data)
	}
	info, _ := os.Stat(sp)
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode=%v", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(root, "etc/systemd/system/hnp-mesh-nat.service")); err != nil {
		t.Fatalf("unit 缺失: %v", err)
	}
}

func TestApplyLanRequiresCIDR(t *testing.T) {
	r := lanRunner()
	res := ApplyLan(Options{Runner: r, RootDir: t.TempDir()}, LanConfig{}, true)
	if res.OK || !strings.Contains(res.Error, "网段") {
		t.Fatalf("res=%+v", res)
	}
}

func TestApplyLanFillsMeshIface(t *testing.T) {
	r := lanRunner()
	r.out["sh "+meshNatScript] = ""
	r.out["systemctl daemon-reload"] = ""
	r.out["systemctl enable --now hnp-mesh-nat"] = ""
	root := t.TempDir()
	res := ApplyLan(Options{Runner: r, RootDir: root}, LanConfig{LanCIDRs: []string{"192.168.1.0/24"}}, false)
	if !res.OK {
		t.Fatalf("res=%+v", res)
	}
	data, _ := os.ReadFile(filepath.Join(root, "usr/local/sbin/hnp-mesh-nat.sh"))
	if !strings.Contains(string(data), "oifname \"tun0\"") {
		t.Fatalf("script=%s", data)
	}
}

func TestRemoveLanDryRun(t *testing.T) {
	r := lanRunner()
	res := RemoveLan(Options{Runner: r, RootDir: t.TempDir()}, true)
	if !res.OK || len(res.Steps) != 2 {
		t.Fatalf("res=%+v", res)
	}
}

func TestRemoveLanRuns(t *testing.T) {
	r := lanRunner()
	r.out["systemctl disable --now hnp-mesh-nat"] = ""
	r.out["nft delete table ip hnp_mesh"] = ""
	res := RemoveLan(Options{Runner: r, RootDir: t.TempDir()}, false)
	if !res.OK {
		t.Fatalf("res=%+v", res)
	}
}

func errNotFound() error { return errors.New("not found") }
