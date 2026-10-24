package access

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// 转发开关的条件触发：只有渲染后的 live 配置真的声明了 lan_interface（即在接管 LAN：
// 旁路由 / 网关形态）时才打开转发并持久化；只代理本机时一律不碰内核参数。
//
// 为什么不能无条件开：只代理本机时开启转发等于把本机变成路由器，扩大攻击面；
// dae 自身也只在绑定 lan_interface 时才要求 conf/<iface>/forwarding=1。
// 为什么不能只靠 dae 的 auto_config_kernel_parameter：配置片段一旦被别的工具覆盖，
// 该选项就消失，转发退回内核默认 0，此后每次 reload 都失败（实机踩过）。

// sysctlRun 便于测试注入（真实环境执行 sysctl -w <key>=1）。
var sysctlRun = func(key string) error {
	return exec.Command("sysctl", "-w", key+"=1").Run()
}

// ipForwardNow 读当前 ip_forward（与 forwardingNow 共用 sysctlProcDir）。
func ipForwardNow() string {
	b, err := os.ReadFile(filepath.Join(sysctlProcDir, "net", "ipv4", "ip_forward"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// EnsureForwardingForInterfaces 直接对给定接口打开转发（供 reload 报错后的自适应修复使用）。
func EnsureForwardingForInterfaces(ifaces []string) (changed bool, err error) {
	clean := make([]string, 0, len(ifaces))
	for _, f := range ifaces {
		if f = strings.TrimSpace(f); f != "" {
			clean = append(clean, f)
		}
	}
	if len(clean) == 0 {
		return false, nil
	}
	return ensureForwardingFor(clean)
}

// EnsureForwardingForConfig 在配置声明了 lan_interface 时确保 ip_forward 与相关接口的
// forwarding 为 1，并写入持久化文件。返回声明的接口列表与本次是否真的做了改动。
// 未声明 lan_interface（或配置读不到）时返回空列表且不做任何修改。
func EnsureForwardingForConfig(configPath string) (ifaces []string, changed bool, err error) {
	if strings.TrimSpace(configPath) == "" {
		return nil, false, nil
	}
	ifaces = daeLanIfaces(configPath)
	if len(ifaces) == 0 {
		return nil, false, nil
	}
	changed, err = ensureForwardingFor(ifaces)
	return ifaces, changed, err
}

// ensureForwardingFor 是真正的开转发实现：先看当前内核值，缺什么补什么。
func ensureForwardingFor(ifaces []string) (changed bool, err error) {
	need := ipForwardNow() != "1"
	for _, f := range ifaces {
		if forwardingNow(f) != "1" {
			need = true
			break
		}
	}
	if !need {
		return false, nil
	}

	var firstErr error
	set := func(key string) {
		if e := sysctlRun(key); e != nil && firstErr == nil {
			firstErr = e
		}
	}
	if ipForwardNow() != "1" {
		set("net.ipv4.ip_forward")
		set("net.ipv4.conf.all.forwarding")
	}
	for _, f := range ifaces {
		if forwardingNow(f) != "1" {
			set("net.ipv4.conf." + f + ".forwarding")
		}
	}

	body := sysctlConfBody(ifaces)
	if cur, ok := readSysctlConf(); !ok || cur != body {
		if e := os.MkdirAll(filepath.Dir(sysctlConfPath), 0o755); e != nil {
			if firstErr == nil {
				firstErr = e
			}
		} else if e := os.WriteFile(sysctlConfPath, []byte(body), 0o644); e != nil && firstErr == nil {
			firstErr = e
		}
	}
	return true, firstErr
}
