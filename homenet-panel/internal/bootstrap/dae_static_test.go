package bootstrap

import (
	"errors"
	"strings"
	"testing"
)

func TestDaeAssetCandidates(t *testing.T) {
	cases := []struct {
		machine string
		cpuinfo string
		want    string
		first   string
	}{
		{"x86_64", "flags : fpu avx avx2 sse4_2", "x86_64", "dae-linux-x86_64_v3_avx2.tar.xz"},
		{"x86_64", "flags : fpu sse2", "x86_64", "dae-linux-x86_64_v2.tar.xz"},
		{"aarch64", "", "arm64", "dae-linux-arm64.tar.xz"},
		{"armv7l", "", "armv7", "dae-linux-armv7.tar.xz"},
		{"i686", "", "386", "dae-linux-386.tar.xz"},
	}
	for _, c := range cases {
		got := DaeAssetCandidates(c.machine, c.cpuinfo)
		if len(got) == 0 {
			t.Fatalf("%s: 没有候选", c.machine)
		}
		if got[0] != c.first {
			t.Errorf("%s/%q: 首选 %s，想要 %s", c.machine, c.cpuinfo, got[0], c.first)
		}
		if !strings.Contains(got[0], c.want) {
			t.Errorf("%s: 首选 %s 不含 %s", c.machine, got[0], c.want)
		}
	}
	if got := DaeAssetCandidates("mips", ""); got != nil {
		t.Errorf("mips 不该有候选，得到 %v", got)
	}
}

// 没有 apt-get 时必须走官方静态包（而不是直接报错放弃）。
func TestInstallDaeFallsBackToStaticPackage(t *testing.T) {
	dir := t.TempDir()
	r := &fakeRunner{
		out: map[string]string{
			"uname -m": "x86_64\n",
			"sh -c grep -m1 '^flags' /proc/cpuinfo 2>/dev/null || true": "flags\t\t: fpu avx2 sse4_2",
		},
		fail: map[string]error{"apt-get --version": errNotFound(), "dae --version": errNotFound()},
	}
	opts := Options{Runner: r, EtcDir: dir, DryRun: true, IsRoot: func() bool { return true }}
	res := InstallDae(opts.normalized())
	if !res.OK {
		t.Fatalf("dry-run 应成功: %s", res.Error)
	}
	if !strings.Contains(res.Steps[0].Title, "下载 dae 官方静态包") {
		t.Fatalf("第一步应是下载静态包，实际 %q", res.Steps[0].Title)
	}
	if !strings.Contains(res.Steps[0].Command, "dae-linux-x86_64_v3_avx2.tar.xz") {
		t.Errorf("命令没选中 avx2 包: %s", res.Steps[0].Command)
	}
	joined := ""
	for _, s := range res.Steps {
		joined += s.Title + "|" + s.Command + "\n"
	}
	if !strings.Contains(joined, "安装二进制到 /usr/bin/dae") {
		t.Errorf("缺少安装二进制步骤:\n%s", joined)
	}
}

func TestInstallDaeStaticUnknownArch(t *testing.T) {
	r := &fakeRunner{
		out:  map[string]string{"uname -m": "mips\n"},
		fail: map[string]error{"apt-get --version": errNotFound(), "dae --version": errNotFound()},
	}
	opts := Options{Runner: r, DryRun: true, IsRoot: func() bool { return true }}.normalized()
	res := InstallDae(opts)
	if res.OK || !strings.Contains(res.Error, "不认识的 CPU 架构") {
		t.Fatalf("应报架构不支持，得到 ok=%v err=%q", res.OK, res.Error)
	}
}

func errNotFound() error { return errors.New("not found") }
