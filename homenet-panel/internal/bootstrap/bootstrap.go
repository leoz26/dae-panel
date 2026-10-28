package bootstrap

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"homenet-panel/internal/model"
)

// CmdRunner 执行外部命令（测试注入）。
type CmdRunner interface {
	Run(name string, args ...string) (string, error)
}

type execRunner struct{}

func (execRunner) Run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return msg, fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), firstLine(msg))
	}
	return string(out), nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Options 控制安装/初始化行为。
type Options struct {
	Runner    CmdRunner
	Fetch     func(url string) ([]byte, error)
	EtcDir    string      // 默认 /etc/dae
	EntryPath string      // 默认 <EtcDir>/config.dae
	CorePath  string      // 默认 <EtcDir>/config.d/panel-core.dae
	DryRun    bool        // 只打印将执行的步骤，不改动系统
	IsRoot    func() bool // 测试注入；默认 os.Geteuid()==0
}

func (o Options) normalized() Options {
	if o.Runner == nil {
		o.Runner = execRunner{}
	}
	if o.Fetch == nil {
		o.Fetch = fetchURL
	}
	if o.EtcDir == "" {
		o.EtcDir = "/etc/dae"
	}
	if o.EntryPath == "" {
		o.EntryPath = filepath.Join(o.EtcDir, "config.dae")
	}
	if o.CorePath == "" {
		o.CorePath = filepath.Join(o.EtcDir, "config.d", "panel-core.dae")
	}
	if o.IsRoot == nil {
		o.IsRoot = func() bool { return os.Geteuid() == 0 }
	}
	return o
}

func fetchURL(url string) ([]byte, error) {
	// 不只用来看 APT 源文件：还要下 dae 官方静态包（~12MB），上限要放宽；
	// 超时按「单次尝试」计（多镜像回退，卡住一个就换下一个）。
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

// Status 是「安装与初始化」页要展示的现场状态。
type Status struct {
	Root           bool   `json:"root"`
	ServiceManager string `json:"serviceManager"`
	HasApt         bool   `json:"hasApt"`
	DaeInstalled   bool   `json:"daeInstalled"`
	DaeVersion     string `json:"daeVersion,omitempty"`
	DaeRunning     bool   `json:"daeRunning"`
	ConfigExists   bool   `json:"configExists"`
	ConfigBytes    int64  `json:"configBytes"`
	ConfigManaged  bool   `json:"configManaged"`
	CoreExists     bool   `json:"coreExists"`
	EntryPath      string `json:"entryPath"`
	CorePath       string `json:"corePath"`
	BTF            bool   `json:"btf"`
	MemlockKB      int    `json:"memlockKB"`
	Note           string `json:"note,omitempty"`
}

// Detect 采集当前状态（只读）。
func Detect(o Options) Status {
	o = o.normalized()
	st := Status{Root: o.IsRoot(), EntryPath: o.EntryPath, CorePath: o.CorePath}
	if _, err := os.Stat("/etc/openwrt_release"); err == nil {
		st.ServiceManager = "procd"
	} else if _, err := os.Stat("/run/systemd/system"); err == nil {
		st.ServiceManager = "systemd"
	} else {
		st.ServiceManager = "none"
	}
	if _, err := o.Runner.Run("apt-get", "--version"); err == nil {
		st.HasApt = true
	}
	if out, err := o.Runner.Run("dae", "--version"); err == nil {
		st.DaeInstalled = true
		st.DaeVersion = firstLine(strings.TrimSpace(out))
	}
	if daeProcessRunning(o.Runner) {
		st.DaeRunning = true
	}
	if info, err := os.Stat(o.EntryPath); err == nil {
		st.ConfigExists = true
		st.ConfigBytes = info.Size()
		if data, rerr := os.ReadFile(o.EntryPath); rerr == nil {
			st.ConfigManaged = strings.Contains(string(data), model.Header)
		}
	}
	if info, err := os.Stat(o.CorePath); err == nil && info.Size() > 0 {
		st.CoreExists = true
	}
	if _, err := os.Stat("/sys/kernel/btf/vmlinux"); err == nil {
		st.BTF = true
	}
	st.MemlockKB = memlockKB()
	switch {
	case !st.Root:
		st.Note = "面板不是以 root 运行的，安装/初始化按钮会拒绝执行（只读巡视可用）"
	case !st.BTF:
		st.Note = "内核缺少 BTF（/sys/kernel/btf/vmlinux），dae 无法运行"
	case !st.DaeInstalled:
		st.Note = "还没有 dae：点「安装 dae」可自动加官方源安装（APT 系）"
	case !st.ConfigExists || st.ConfigBytes == 0:
		st.Note = "dae 已安装但还没有配置：点「生成初始配置」写一份最小可运行配置"
	case !st.DaeRunning:
		st.Note = "配置已存在但 dae 没在运行：点「启动 dae」"
	}
	return st
}

// daeProcessRunning 判断 dae 是否在跑。pgrep -x 在 busybox/OpenWrt 上匹配不到，
// 退化到 dae 自己的 pid 文件（/var/run/dae.pid）+ kill -0。
func daeProcessRunning(r CmdRunner) bool {
	if out, err := r.Run("pgrep", "-x", "dae"); err == nil && strings.TrimSpace(out) != "" {
		return true
	}
	if out, err := r.Run("cat", "/var/run/dae.pid"); err == nil {
		if pid := strings.TrimSpace(out); pid != "" {
			if _, e := r.Run("kill", "-0", pid); e == nil {
				return true
			}
		}
	}
	return false
}

func memlockKB() int {
	data, err := os.ReadFile("/proc/self/limits")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "Max locked memory") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			return 0
		}
		if fields[3] == "unlimited" {
			return -1
		}
		kb, _ := strconv.Atoi(fields[3])
		return kb / 1024
	}
	return 0
}

// Step 是一条执行记录。
type Step struct {
	Title   string `json:"title"`
	Command string `json:"command,omitempty"`
	Output  string `json:"output,omitempty"`
	OK      bool   `json:"ok"`
	Skipped bool   `json:"skipped,omitempty"`
}

// Result 是一次安装/初始化动作的结果。
type Result struct {
	Steps  []Step `json:"steps"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	DryRun bool   `json:"dryRun"`
}

func (r *Result) add(title string) *Step {
	r.Steps = append(r.Steps, Step{Title: title})
	return &r.Steps[len(r.Steps)-1]
}

func (r *Result) runCmd(o Options, title, name string, args ...string) bool {
	step := r.add(title)
	step.Command = name + " " + strings.Join(args, " ")
	if o.DryRun {
		step.OK = true
		step.Skipped = true
		return true
	}
	out, err := o.Runner.Run(name, args...)
	step.Output = strings.TrimSpace(out)
	step.OK = err == nil
	if err != nil {
		r.OK = false
		r.Error = title + " 失败: " + err.Error()
		return false
	}
	return true
}

const (
	sourcesURL = "https://daeuniverse.pages.dev/daeuniverse.sources"
	listURL    = "https://daeuniverse.pages.dev/daeuniverse.list"
	keyURL     = "https://daeuniverse.pages.dev/daeuniverse-archive-goose.gpg"
)

// InstallDae 通过官方 APT 源安装 dae（仅 APT 系；已安装则跳过）。
func InstallDae(o Options) Result {
	o = o.normalized()
	res := Result{OK: true, DryRun: o.DryRun}
	if !o.IsRoot() {
		res.OK = false
		res.Error = "需要 root 权限"
		return res
	}
	if out, err := o.Runner.Run("dae", "--version"); err == nil {
		step := res.add("dae 已安装，跳过")
		step.OK = true
		step.Skipped = true
		step.Output = firstLine(strings.TrimSpace(out))
		return res
	}
	if _, err := o.Runner.Run("apt-get", "--version"); err != nil {
		// 没有 APT（OpenWrt/iStoreOS/Alpine 等）：走官方静态包，见 dae_static.go
		return installDaeStatic(o, res)
	}
	srcURL, srcPath := sourcesURL, "/etc/apt/sources.list.d/daeuniverse.sources"
	if out, err := o.Runner.Run("apt-get", "--version"); err == nil {
		if m := aptMajor(out); m > 0 && m < 3 {
			srcURL, srcPath = listURL, "/etc/apt/sources.list.d/daeuniverse.list"
		}
	}
	res.add(fmt.Sprintf("准备 APT 源（%s）", filepath.Base(srcPath)))
	if !o.DryRun {
		src, err := o.Fetch(srcURL)
		if err != nil {
			res.OK = false
			res.Error = "下载 APT 源失败: " + err.Error()
			return res
		}
		if err := writeFile(srcPath, src, 0o644); err != nil {
			res.OK = false
			res.Error = "写 APT 源失败: " + err.Error()
			return res
		}
		key, err := o.Fetch(keyURL)
		if err != nil {
			res.OK = false
			res.Error = "下载 GPG key 失败: " + err.Error()
			return res
		}
		if err := writeFile("/usr/share/keyrings/daeuniverse-archive-goose.gpg", key, 0o644); err != nil {
			res.OK = false
			res.Error = "写 GPG key 失败: " + err.Error()
			return res
		}
	}
	if !res.runCmd(o, "apt-get update", "apt-get", "update", "-q") {
		return res
	}
	if !res.runCmd(o, "apt-get install -y dae", "apt-get", "install", "-y", "-q", "dae") {
		return res
	}
	step := res.add("校验安装结果")
	if o.DryRun {
		step.OK = true
		step.Skipped = true
		return res
	}
	if out, err := o.Runner.Run("dae", "--version"); err != nil {
		step.OK = false
		res.OK = false
		res.Error = "安装后仍找不到 dae"
	} else {
		step.OK = true
		step.Output = firstLine(strings.TrimSpace(out))
	}
	return res
}

func aptMajor(out string) int {
	for _, f := range strings.Fields(out) {
		if f == "apt" {
			continue
		}
		if v, err := strconv.Atoi(strings.SplitN(f, ".", 2)[0]); err == nil && v > 0 && v < 10 {
			return v
		}
	}
	return 0
}

// DefaultModel 是「生成初始配置」用的最小可用模型：只代理本机、其余直连。
func DefaultModel() model.Model {
	return model.Model{
		Global: "    tproxy_port: 12345\n    wan_interface: auto",
		DNS: "    upstream {\n        cn: 'udp://223.5.5.5:53'\n    }\n" +
			"    routing {\n        request {\n            fallback: cn\n        }\n" +
			"        response {\n            fallback: accept\n        }\n    }",
		Fallback: "direct",
	}
}

// WriteInitConfig 写入最小可用配置（入口 + panel-core.dae），validate 失败则回滚。
func WriteInitConfig(o Options, force bool) Result {
	o = o.normalized()
	res := Result{OK: true, DryRun: o.DryRun}
	if !o.IsRoot() {
		res.OK = false
		res.Error = "需要 root 权限"
		return res
	}
	// 只在「空白设备」上写默认配置：已有任何内容都要求显式 force，避免把线上规则冲掉。
	if !force && hasContent(o.EntryPath) || !force && hasContent(o.CorePath) {
		res.OK = false
		managed := "非面板托管"
		if data, err := os.ReadFile(o.EntryPath); err == nil && strings.Contains(string(data), model.Header) {
			managed = "面板托管"
		}
		res.Error = "已存在配置（" + managed + "）：" + o.EntryPath + "；拒绝覆盖（生成初始配置只用于空白设备）。确认要替换请带 force=true（会先备份）"
		return res
	}
	m := DefaultModel()
	entry, core := model.Render(&m)
	step := res.add("写入 " + o.EntryPath)
	step.Output = fmt.Sprintf("%d 字节", len(entry))
	if !o.DryRun {
		if err := backupIfExists(o.EntryPath); err != nil {
			res.OK = false
			res.Error = err.Error()
			return res
		}
		if err := os.MkdirAll(filepath.Dir(o.EntryPath), 0o700); err != nil {
			res.OK = false
			res.Error = err.Error()
			return res
		}
		if err := writeFile(o.EntryPath, []byte(entry), 0o600); err != nil {
			res.OK = false
			res.Error = "写入口配置失败: " + err.Error()
			return res
		}
	}
	step.OK = true

	step2 := res.add("写入 " + o.CorePath)
	step2.Output = fmt.Sprintf("%d 字节", len(core))
	if !o.DryRun {
		if err := backupIfExists(o.CorePath); err != nil {
			res.OK = false
			res.Error = err.Error()
			return res
		}
		if err := os.MkdirAll(filepath.Dir(o.CorePath), 0o700); err != nil {
			res.OK = false
			res.Error = err.Error()
			return res
		}
		if err := writeFile(o.CorePath, []byte(core), 0o600); err != nil {
			res.OK = false
			res.Error = "写核心配置失败: " + err.Error()
			return res
		}
	}
	step2.OK = true

	if !res.runCmd(o, "dae validate", "dae", "validate", "-c", o.EntryPath) {
		if !o.DryRun {
			_ = restore(o.EntryPath)
			_ = restore(o.CorePath)
			res.Steps = append(res.Steps, Step{Title: "已回滚到写入前的配置", OK: true})
		}
		return res
	}
	return res
}

// StartDae 启动并设置开机自启（systemd / procd）。
func StartDae(o Options) Result {
	o = o.normalized()
	res := Result{OK: true, DryRun: o.DryRun}
	if !o.IsRoot() {
		res.OK = false
		res.Error = "需要 root 权限"
		return res
	}
	if _, err := os.Stat("/etc/openwrt_release"); err == nil {
		if !res.runCmd(o, "启动 dae（procd）", "/etc/init.d/dae", "restart") {
			return res
		}
		res.add("提示").Output = "OpenWrt 上开机自启请执行 /etc/init.d/dae enable"
		return res
	}
	if !res.runCmd(o, "systemctl enable dae", "systemctl", "enable", "dae") {
		return res
	}
	if !res.runCmd(o, "systemctl restart dae", "systemctl", "restart", "dae") {
		return res
	}
	step := res.add("检查运行状态")
	if o.DryRun {
		step.OK = true
		step.Skipped = true
		return res
	}
	for i := 0; i < 10; i++ {
		if out, err := o.Runner.Run("systemctl", "is-active", "dae"); err == nil && strings.TrimSpace(out) == "active" {
			step.OK = true
			step.Output = "active"
			return res
		}
		time.Sleep(500 * time.Millisecond)
	}
	step.OK = false
	res.OK = false
	res.Error = "dae 启动后仍是 inactive：请查看 journalctl -u dae -n 50"
	return res
}

// StopDae 一键停用 dae（只停服务，不改配置、不改开机自启）。
func StopDae(o Options) Result {
	o = o.normalized()
	res := Result{OK: true, DryRun: o.DryRun}
	if !o.IsRoot() {
		res.OK = false
		res.Error = "需要 root 权限"
		return res
	}
	if _, err := os.Stat("/etc/openwrt_release"); err == nil {
		if !res.runCmd(o, "停止 dae（procd）", "/etc/init.d/dae", "stop") {
			return res
		}
		res.add("提示").Output = "OpenWrt 上如需开机不再自启，执行 /etc/init.d/dae disable"
		return res
	}
	if !res.runCmd(o, "systemctl stop dae", "systemctl", "stop", "dae") {
		return res
	}
	step := res.add("检查运行状态")
	if o.DryRun {
		step.OK = true
		step.Skipped = true
		return res
	}
	for i := 0; i < 10; i++ {
		out, _ := o.Runner.Run("systemctl", "is-active", "dae")
		state := strings.TrimSpace(firstLine(strings.TrimSpace(out)))
		if state == "" {
			state = "inactive"
		}
		if state != "active" {
			step.OK = true
			step.Output = state
			return res
		}
		time.Sleep(500 * time.Millisecond)
	}
	step.OK = false
	res.OK = false
	res.Error = "dae 仍在运行：请查看 journalctl -u dae -n 50"
	return res
}

// RestartDae 重启 dae（不重新生成配置）。注意：restart 会拆装拦截设备，个别内核上可能挂起挂死，能 reload 就别 restart。
func RestartDae(o Options) Result {
	o = o.normalized()
	res := Result{OK: true, DryRun: o.DryRun}
	if !o.IsRoot() {
		res.OK = false
		res.Error = "需要 root 权限"
		return res
	}
	if _, err := os.Stat("/etc/openwrt_release"); err == nil {
		res.runCmd(o, "重启 dae（procd）", "/etc/init.d/dae", "restart")
		return res
	}
	if !res.runCmd(o, "systemctl restart dae", "systemctl", "restart", "dae") {
		return res
	}
	step := res.add("检查运行状态")
	if o.DryRun {
		step.OK = true
		step.Skipped = true
		return res
	}
	for i := 0; i < 20; i++ {
		if out, err := o.Runner.Run("systemctl", "is-active", "dae"); err == nil && strings.TrimSpace(firstLine(strings.TrimSpace(out))) == "active" {
			step.OK = true
			step.Output = "active"
			return res
		}
		time.Sleep(500 * time.Millisecond)
	}
	step.OK = false
	res.OK = false
	res.Error = "dae 重启后仍是 inactive：请查看 journalctl -u dae -n 50"
	return res
}

func writeFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func hasContent(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

func backupIfExists(path string) error {
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		return nil
	}
	data, rerr := os.ReadFile(path)
	if rerr != nil {
		return rerr
	}
	bak := path + ".bak-" + time.Now().Format("20060102-150405")
	return os.WriteFile(bak, data, info.Mode().Perm())
}

func restore(path string) error {
	matches, _ := filepath.Glob(path + ".bak-*")
	if len(matches) == 0 {
		return nil
	}
	newest := matches[len(matches)-1]
	data, err := os.ReadFile(newest)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
