package access

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// 面板托管的转发开关。
//
// 为什么需要它：dae 绑定 LAN 接口（lan_interface）时要求该接口的 ipforward 为 1，
// 否则 reload 直接失败（ipforward on <iface> is off）。dae 只在配置里带
// auto_config_kernel_parameter: true 时才自己去开，而配置一旦被别的片段覆盖，
// 转发就退回内核默认 0，此后每次 reload 都失败——形成"改不动配置"的死结。
//
// 因此面板自己负责：应用接入时打开转发并写一个自有持久化文件（重启后依然有效），
// 回滚接入时按应用前的原值还原并删除该文件（恢复到系统默认关闭）。
var (
	// sysctlConfPath 便于测试注入（真实环境是 /etc/sysctl.d/99-homenet-panel.conf）。
	sysctlConfPath = "/etc/sysctl.d/99-homenet-panel.conf"
	// sysctlProcDir 便于测试注入（真实环境是 /proc/sys）。
	sysctlProcDir = "/proc/sys"
)

const sysctlConfHeader = "# 由 HomeNet Panel 生成（panel-managed），请勿手改。\n" +
	"# 用途：LAN 接管（dae 绑定 lan_interface）所需的转发开关。\n" +
	"# 撤回：面板「接入向导 → 回滚」会删除本文件并把转发恢复为应用前的值。\n"

// sysctlConfBody 渲染持久化文件内容：除 ip_forward 外，把每个 LAN 接口的 forwarding 也置 1。
func sysctlConfBody(ifaces []string) string {
	var b strings.Builder
	b.WriteString(sysctlConfHeader)
	b.WriteString("net.ipv4.ip_forward = 1\n")
	for _, f := range ifaces {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		fmt.Fprintf(&b, "net.ipv4.conf.%s.forwarding = 1\n", f)
	}
	return b.String()
}

// readSysctlConf 读取应用前的持久化文件（不存在时 exists=false）。
func readSysctlConf() (string, bool) {
	b, err := os.ReadFile(sysctlConfPath)
	if err != nil {
		return "", false
	}
	return string(b), true
}

// forwardingNow 读接口当前 forwarding 值（读不到返回空串）。
func forwardingNow(iface string) string {
	b, err := os.ReadFile(filepath.Join(sysctlProcDir, "net", "ipv4", "conf", iface, "forwarding"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// EnableForwarding 打开转发并写持久化文件。返回的步骤供前端展示。
func (a *Applier) EnableForwarding(ifaces []string) []ApplyStep {
	a.ensure()
	steps := []ApplyStep{}

	set := func(key, human string) {
		cmd := []string{"-w", key + "=1"}
		_, err := a.Runner.Run("sysctl", cmd...)
		steps = append(steps, ApplyStep{Name: "sysctl", OK: err == nil, Detail: human + errText(err), Command: "sysctl " + strings.Join(cmd, " ")})
	}
	set("net.ipv4.ip_forward", "打开 IPv4 转发")
	set("net.ipv4.conf.all.forwarding", "打开全局接口转发")
	for _, f := range ifaces {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		set("net.ipv4.conf."+f+".forwarding", "打开接口 "+f+" 的转发")
	}

	// 持久化：写进 /etc/sysctl.d，重启后仍然有效（用户要求：只在关闭服务/回滚时恢复默认）
	body := []byte(sysctlConfBody(ifaces))
	err := os.MkdirAll(filepath.Dir(sysctlConfPath), 0o755)
	if err == nil {
		err = os.WriteFile(sysctlConfPath, body, 0o644)
	}
	steps = append(steps, ApplyStep{Name: "sysctl-persist", OK: err == nil, Detail: errText(err), Command: "写入 " + sysctlConfPath})
	return steps
}

// DisableForwarding 按应用前的基线还原：写回原文件内容或删除自有文件，然后把转发关回去。
func (a *Applier) DisableForwarding(st *accessState) []ApplyStep {
	a.ensure()
	steps := []ApplyStep{}

	var err error
	if st != nil && st.HadSysctlFile {
		err = os.WriteFile(sysctlConfPath, []byte(st.OriginalSysctl), 0o644)
		if err == nil {
			steps = append(steps, ApplyStep{Name: "sysctl-restore-file", OK: true, Detail: "已写回应用前的 " + sysctlConfPath})
		}
	} else {
		err = os.Remove(sysctlConfPath)
		if err != nil && os.IsNotExist(err) {
			err = nil
		}
		if err == nil {
			steps = append(steps, ApplyStep{Name: "sysctl-restore-file", OK: true, Detail: "已删除自有文件 " + sysctlConfPath})
		}
	}
	if err != nil {
		steps = append(steps, ApplyStep{Name: "sysctl-restore-file", OK: false, Detail: errText(err)})
	}

	// 只有应用前是 0 才关回去（应用前就是 1 说明系统本来就开了，不动它）
	if st != nil && st.OriginalIPForward != "0" {
		return steps
	}
	if cur := readIPForward(); cur == "0" {
		return steps
	}
	cmd := []string{"-w", "net.ipv4.ip_forward=0"}
	_, cerr := a.Runner.Run("sysctl", cmd...)
	steps = append(steps, ApplyStep{Name: "sysctl-restore", OK: cerr == nil, Detail: errText(cerr), Command: "sysctl -w net.ipv4.ip_forward=0"})
	return steps
}
