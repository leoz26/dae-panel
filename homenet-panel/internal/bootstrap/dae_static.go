package bootstrap

// 无 APT 的系统（OpenWrt / iStoreOS / Alpine 等）怎么装 dae。
//
// 硬事实：dae 不在任何发行版官方源里，OpenWrt 的 opkg 源也没有它
// （opkg list | grep ^dae 为空；dae 官方只提供 Debian 自建 apt 源 + GitHub 静态包）。
// 所以这里走官方 release 的静态 tar.xz：按 CPU 能力从最优到最通用逐个尝试。

import (
	"fmt"
	"os"
	"strings"
)

const (
	daeReleaseBase    = "https://github.com/daeuniverse/dae/releases/download/"
	daeReleaseVersion = "v2.1.1"
	daeBinPath        = "/usr/bin/dae"
	daeProcdInit      = "/etc/init.d/dae"
	daeExamplePath    = "/etc/dae/example.dae"
)

// daeDownloadPrefixes：官方地址优先，其次两个常见的 GitHub 加速前缀（国内直连 GitHub
// 经常 EOF；这些镜像只是回退，装完可以在面板里看到实际用的地址）。
var daeDownloadPrefixes = []string{
	"",
	"https://ghfast.top/",
	"https://gh-proxy.com/",
}

// DaeAssetCandidates 按 uname -m 与 /proc/cpuinfo 的 flags 给出候选包名。
// 顺序 = 从最优到最通用：先试 avx2 版本，404 了再退回通用版。
func DaeAssetCandidates(machine, cpuinfo string) []string {
	avx2 := false
	for _, f := range strings.Fields(cpuinfo) {
		if f == "avx2" {
			avx2 = true
			break
		}
	}
	switch strings.ToLower(strings.TrimSpace(machine)) {
	case "x86_64", "amd64":
		if avx2 {
			return []string{
				"dae-linux-x86_64_v3_avx2.tar.xz",
				"dae-linux-x86_64_v2.tar.xz",
				"dae-linux-x86_64.tar.xz",
			}
		}
		return []string{"dae-linux-x86_64_v2.tar.xz", "dae-linux-x86_64.tar.xz"}
	case "aarch64", "arm64":
		return []string{"dae-linux-arm64.tar.xz", "dae-linux-arm64_v2.tar.xz"}
	case "armv7l", "armv7", "armhf":
		return []string{"dae-linux-armv7.tar.xz", "dae-linux-armv6.tar.xz"}
	case "armv6l", "armv6":
		return []string{"dae-linux-armv6.tar.xz", "dae-linux-armv7.tar.xz"}
	case "i386", "i486", "i586", "i686", "386":
		return []string{"dae-linux-386.tar.xz", "dae-linux-i386.tar.xz"}
	}
	return nil
}

// procdInit 是 OpenWrt 下的 init 脚本（没有 systemd）。
func procdInit() []byte {
	return []byte(`#!/bin/sh /etc/rc.common
# 由 HomeNet Panel 生成：dae 的 procd 服务定义
USE_PROCD=1
START=90
STOP=10

start_service() {
	procd_open_instance
	procd_set_param command /bin/sh -c "exec /usr/bin/dae run --disable-timestamp -c /etc/dae/config.dae"
	procd_set_param respawn 3600 5 0
	procd_set_param stdout 1
	procd_set_param stderr 1
	procd_set_param limits memlock=unlimited
	procd_close_instance
}

reload_service() {
	/usr/bin/dae reload 2>/dev/null && return 0
	stop
	start
}
`)
}

// readFirst 依次尝试多个路径，返回第一个读成功的。
func readFirst(paths ...string) ([]byte, error) {
	var lastErr error
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err == nil {
			return b, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// installDaeStatic 走官方静态包安装（APT 不可用时）。
func installDaeStatic(o Options, res Result) Result {
	machine, _ := o.Runner.Run("uname", "-m")
	cpuinfo, _ := o.Runner.Run("sh", "-c", "grep -m1 '^flags' /proc/cpuinfo 2>/dev/null || true")
	cands := DaeAssetCandidates(machine, cpuinfo)
	if len(cands) == 0 {
		res.OK = false
		res.Error = fmt.Sprintf("不认识的 CPU 架构 %q；请按 https://github.com/daeuniverse/dae 手动安装", strings.TrimSpace(machine))
		return res
	}

	step := res.add("下载 dae 官方静态包")
	var data []byte
	var lastErr error
	for _, name := range cands {
		if data != nil {
			break
		}
		for _, prefix := range daeDownloadPrefixes {
			url := prefix + daeReleaseBase + daeReleaseVersion + "/" + name
			if o.DryRun {
				step.Command = "curl -fsSL -o /tmp/dae.tar.xz " + url
				step.OK, step.Skipped = true, true
				data = []byte{0}
				break
			}
			b, err := o.Fetch(url)
			if err != nil {
				lastErr = fmt.Errorf("%s: %w", url, err)
				continue
			}
			step.Command = "curl -fsSL -o /tmp/dae.tar.xz " + url
			step.OK = true
			step.Output = fmt.Sprintf("%s（%d 字节）", name, len(b))
			data = b
			break
		}
	}
	if o.DryRun {
		if !res.runCmd(o, "解包静态包", "sh", "-c", "xz -dc /tmp/dae.tar.xz > /tmp/dae.tar && mkdir -p /tmp/dae-pkg && tar -xf /tmp/dae.tar -C /tmp/dae-pkg") {
			return res
		}
		s := res.add("安装二进制到 " + daeBinPath)
		s.Command = "cp /tmp/dae-pkg/dae " + daeBinPath + " && chmod 755 " + daeBinPath
		s.OK, s.Skipped = true, true
		if _, err := os.Stat("/etc/openwrt_release"); err == nil {
			s2 := res.add("写 procd 服务定义 " + daeProcdInit)
			s2.Command = "/etc/init.d/dae enable && /etc/init.d/dae start"
			s2.OK, s2.Skipped = true, true
		}
		v := res.add("校验安装结果")
		v.Command = "dae --version"
		v.OK, v.Skipped = true, true
		return res
	}
	if data == nil || o.DryRun && step.Command == "" {
		res.OK = false
		if lastErr != nil {
			res.Error = "下载 dae 静态包失败（已尝试 " + strings.Join(cands, "、") + "）: " + lastErr.Error()
		} else {
			res.Error = "下载 dae 静态包失败"
		}
		return res
	}
	tarball := "/tmp/dae.tar.xz"
	if err := writeFile(tarball, data, 0o644); err != nil {
		res.OK = false
		res.Error = "写临时文件失败: " + err.Error()
		return res
	}
	if !res.runCmd(o, "解包静态包", "sh", "-c",
		"xz -dc "+tarball+" > /tmp/dae.tar && rm -rf /tmp/dae-pkg && mkdir -p /tmp/dae-pkg && tar -xf /tmp/dae.tar -C /tmp/dae-pkg") {
		return res
	}
	// 官方静态包是「整棵文件系统」布局（usr/bin/dae、etc/dae/example.dae），
	// 也有版本是平铺的 ./dae，两种都认。
	bin, err := readFirst("/tmp/dae-pkg/dae", "/tmp/dae-pkg/usr/bin/dae")
	if err != nil {
		res.OK = false
		res.Error = "静态包里没有 dae 可执行文件: " + err.Error()
		return res
	}
	s := res.add("安装二进制到 " + daeBinPath)
	if err := writeFile(daeBinPath, bin, 0o755); err != nil {
		s.OK = false
		res.OK = false
		res.Error = "写 " + daeBinPath + " 失败: " + err.Error()
		return res
	}
	s.OK = true
	s.Output = fmt.Sprintf("%d 字节", len(bin))
	if ex, err := readFirst("/tmp/dae-pkg/example.dae", "/tmp/dae-pkg/etc/dae/example.dae"); err == nil {
		_ = writeFile(daeExamplePath, ex, 0o644)
	}

	if _, err := os.Stat("/etc/openwrt_release"); err == nil {
		if _, err := os.Stat(daeProcdInit); err != nil {
			p := res.add("写 procd 服务定义 " + daeProcdInit)
			if err := writeFile(daeProcdInit, procdInit(), 0o755); err != nil {
				p.OK = false
				res.OK = false
				res.Error = "写 init 脚本失败: " + err.Error()
				return res
			}
			p.OK = true
			res.runCmd(o, "开机自启 dae（/etc/init.d/dae enable）", "sh", "-c", "/etc/init.d/dae enable")
		}
		// 换掉二进制后 procd 不会自动改用新文件（重试耗尽就停在 inactive），
		// 有配置时顺手把服务拉起来；失败只记录，不算安装失败。
		if _, err := os.Stat(o.EntryPath); err == nil {
			rs := res.add("重启 dae 服务（让新二进制生效）")
			rs.Command = "/etc/init.d/dae restart"
			if out, err := o.Runner.Run("sh", "-c", "/etc/init.d/dae restart"); err != nil {
				rs.OK = false
				rs.Output = firstLine(strings.TrimSpace(out))
			} else {
				rs.OK = true
			}
		}
	}

	v := res.add("校验安装结果")
	if out, err := o.Runner.Run("dae", "--version"); err != nil {
		v.OK = false
		res.OK = false
		res.Error = "安装后仍找不到 dae"
	} else {
		v.OK = true
		v.Output = firstLine(strings.TrimSpace(out))
	}
	return res
}
