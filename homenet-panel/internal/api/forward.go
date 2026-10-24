package api

import (
	"log"
	"regexp"

	"homenet-panel/internal/access"
)

// 条件转发：面板启动时与每次 dae reload 前各做一次自检，只有当 live 配置声明了
// lan_interface（真在接管 LAN）才打开转发并持久化；只代理本机时什么都不做。
//
// 另一条自适应路径：dae 有时并未在配置里声明 lan_interface（例如 wan_interface: auto
// 解析到同一块网卡），却仍要求该接口 forwarding=1 并让 reload 失败。此时从错误里取出
// 接口名，只对 dae 明确要求的那个接口打开转发后重试一次——依然满足"有条件才开"。
var ipforwardErrRe = regexp.MustCompile(`ipforward on (\S+) is off`)

// forwardIfaceFromError 从 dae 的 reload 错误里取出它要求的接口名（取不到返回空串）。
func forwardIfaceFromError(out string) string {
	m := ipforwardErrRe.FindStringSubmatch(out)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func (s *Server) ensureForwarding() {
	ifaces, changed, err := access.EnsureForwardingForConfig(s.EntryPath)
	if len(ifaces) == 0 {
		return
	}
	if err != nil {
		log.Printf("转发自检: 配置声明了 lan_interface %v，但打开转发失败: %v", ifaces, err)
		return
	}
	if changed {
		log.Printf("转发自检: 配置声明了 lan_interface %v，已打开 ip_forward 并写入 /etc/sysctl.d/99-homenet-panel.conf", ifaces)
	}
}

// healForwarding 在 reload 报 "ipforward on X is off" 时打开该接口的转发，返回是否值得重试。
func (s *Server) healForwarding(code int, out string) bool {
	iface := forwardIfaceFromError(out)
	if iface == "" {
		return false
	}
	changed, err := access.EnsureForwardingForInterfaces([]string{iface})
	if err != nil {
		log.Printf("转发自检: dae 要求接口 %s 打开转发，但 sysctl 失败: %v", iface, err)
		return false
	}
	if changed {
		log.Printf("转发自检: dae 要求接口 %s 的转发（%s），已打开并持久化，重试 reload", iface, out)
	}
	return changed
}
