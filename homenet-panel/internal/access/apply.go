package access

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"homenet-panel/internal/configtx"
	"homenet-panel/internal/dae"
)

// DaeLike 抽象 dae 操作，便于测试注入。
type DaeLike interface {
	Validate(path string) dae.ValidateResult
	Reload(pid int) (int, string)
	RunningPID() int
	Progress() (dae.Progress, error)
}

// Applier 执行接入变更：写自有片段 → validate → reload →（成功后）建自有 nft 表。
// 任何一步失败都会回滚：删自有表 + 删除/还原片段 + 再 reload。
type Applier struct {
	Runner     Runner
	Dae        DaeLike
	EntryPath  string
	AccessPath string
	BackupDir  string
	Probe      *Probe // 可选：测试注入；nil 则现场只读探测
}

// ApplyStep 是一步执行记录。
type ApplyStep struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Detail  string `json:"detail,omitempty"`
	Command string `json:"command,omitempty"`
}

// ApplyResult 是一次接入变更的结果（dry-run 时只填渲染结果）。
type ApplyResult struct {
	DryRun      bool        `json:"dryRun"`
	Applied     bool        `json:"applied"`
	RolledBack  bool        `json:"rolledBack"`
	Fingerprint string      `json:"fingerprint"`
	AccessPath  string      `json:"accessPath"`
	Content     string      `json:"content"`
	Commands    []string    `json:"commands"`
	Steps       []ApplyStep `json:"steps"`
	BackupPath  string      `json:"backupPath,omitempty"`
	ReloadCode  int         `json:"reloadCode"`
	ReloadOut   string      `json:"reloadOut,omitempty"`
	Message     string      `json:"message"`
}

var ownTables = []string{"panel_dns", "panel_snat", "panel_tproxy"}

func (a *Applier) ensure() {
	if a.Runner == nil {
		a.Runner = execRunner{}
	}
	if a.EntryPath == "" {
		a.EntryPath = dae.DefaultConfig
	}
	if a.AccessPath == "" {
		a.AccessPath = filepath.Join(filepath.Dir(a.EntryPath), "config.d", "panel-access.dae")
	}
	if a.BackupDir == "" {
		a.BackupDir = "/var/lib/homenet-panel/backups"
	}
}

// nftPlan 返回要执行的自有表命令（[][]string 直接喂 nft，argv 形式避免 shell 注入）。
func nftPlan(pl Plan) [][]string {
	cmds := [][]string{}
	if pl.DNSMode != "none" {
		target := pl.ProxyIP
		if target == "" {
			target = "127.0.0.1"
		}
		cmds = append(cmds,
			[]string{"add", "table", "inet", "panel_dns"},
			[]string{"add", "chain", "inet", "panel_dns", "prerouting", "{", "type", "nat", "hook", "prerouting", "priority", "dstnat;", "policy", "accept;", "}"},
		)
		ifaces := pl.LanIfaces
		if len(ifaces) == 0 {
			ifaces = []string{""}
		}
		for _, f := range ifaces {
			pre := []string{"add", "rule", "inet", "panel_dns", "prerouting"}
			if f != "" {
				pre = append(pre, "iifname", f)
			}
			cmds = append(cmds, append(append([]string{}, pre...), "ip", "daddr", target, "return"))
			cmds = append(cmds, append(append([]string{}, pre...), "udp", "dport", "53", "dnat", "ip", "to", target+":53"))
			cmds = append(cmds, append(append([]string{}, pre...), "tcp", "dport", "53", "dnat", "ip", "to", target+":53"))
		}
	}
	if pl.SNAT == "on" {
		cmds = append(cmds,
			[]string{"add", "table", "inet", "panel_snat"},
			[]string{"add", "chain", "inet", "panel_snat", "postrouting", "{", "type", "nat", "hook", "postrouting", "priority", "srcnat;", "policy", "accept;", "}"},
			[]string{"add", "rule", "inet", "panel_snat", "postrouting", "ip", "saddr", "192.168.0.0/16", "oifname", "!=", "lo", "masquerade"},
		)
	}
	return cmds
}

// NftCommands 把计划渲染成可读的 nft 命令文本（仅供展示/复制）。
func NftCommands(pl Plan) []string {
	out := []string{}
	for _, c := range nftPlan(pl) {
		out = append(out, "nft "+strings.Join(c, " "))
	}
	return out
}

// Apply 执行（或 dry-run）一次接入变更。confirm=false 时只渲染，绝不写盘。
func (a *Applier) Apply(opt PlanOptions, expectFingerprint string, confirm bool) (ApplyResult, error) {
	a.ensure()
	probe := a.Probe
	if probe == nil {
		probe = Collect(nil, "/", a.EntryPath)
	}
	plan := PlanFor(probe, opt)
	res := ApplyResult{DryRun: !confirm, Fingerprint: plan.Fingerprint, AccessPath: a.AccessPath}
	if plan.Mode == ModeObserve {
		return res, fmt.Errorf("观察模式不写任何东西：请先选定接入档位")
	}
	res.Content = RenderDaeGlobal(plan.LanIfaces, plan.TProxyPort)
	res.Commands = NftCommands(plan)
	if !confirm {
		res.Message = "dry-run：已渲染片段与 nft 命令，未做任何修改"
		return res, nil
	}
	if expectFingerprint != "" && expectFingerprint != plan.Fingerprint {
		return res, fmt.Errorf("计划已变化（当前 %s ≠ 提交 %s）：请重新生成计划再应用", plan.Fingerprint, expectFingerprint)
	}

	// 0) 记录改动机器前的内核基线（dae 的 auto_config_kernel_parameter 会把 ip_forward 置 1）
	ipFwdBefore := readIPForward()
	sysctlBefore, hadSysctlFile := readSysctlConf()

	// 0.5) 先打开转发并持久化：dae 绑定 lan_interface 时要求 ipforward 为 1，
	//      否则下面的 reload 会直接失败（ipforward on <iface> is off）。
	res.Steps = append(res.Steps, a.EnableForwarding(plan.LanIfaces)...)

	// 1) 写片段 + validate + reload（失败由配置事务自动回滚文件）
	tx := configtx.New(a.Dae, a.BackupDir)
	txr, err := tx.ApplyMulti(a.EntryPath, []configtx.Change{{Path: a.AccessPath, NewContent: []byte(res.Content)}})
	res.BackupPath = txr.BackupPath
	res.ReloadCode = txr.ReloadCode
	res.ReloadOut = txr.ReloadOut
	res.Steps = append(res.Steps, ApplyStep{Name: "write+validate+reload", OK: err == nil, Detail: txr.Message + " " + txr.ValidateOut})
	if err != nil {
		res.RolledBack = txr.RolledBack
		res.Message = "片段写入/校验/reload 失败，已回滚文件"
		rbSt := accessState{OriginalIPForward: ipFwdBefore, HadSysctlFile: hadSysctlFile, OriginalSysctl: sysctlBefore}
		res.Steps = append(res.Steps, a.DisableForwarding(&rbSt)...)
		return res, err
	}

	// 2) 幂等：先清掉自有表，保证最终规则集合恰好等于计划（重复应用不叠加）
	a.DeleteTables()
	res.Steps = append(res.Steps, ApplyStep{Name: "nft-cleanup", OK: true, Detail: "已清理自有表（幂等），随后按计划重建"})

	// 3) 建自有 nft 表（失败则整体回滚）
	for _, c := range nftPlan(plan) {
		cmdText := "nft " + strings.Join(c, " ")
		if _, e := a.Runner.Run("nft", c...); e != nil {
			a.DeleteTables()
			_, rbErr := a.RestoreAccess()
			rbSt := accessState{OriginalIPForward: ipFwdBefore, HadSysctlFile: hadSysctlFile, OriginalSysctl: sysctlBefore}
			res.Steps = append(res.Steps, a.DisableForwarding(&rbSt)...)
			res.RolledBack = true
			res.Steps = append(res.Steps, ApplyStep{Name: "nft", OK: false, Detail: e.Error(), Command: cmdText})
			if rbErr != nil {
				res.Message = "nft 建表失败，且回滚不完整"
				return res, fmt.Errorf("nft 失败: %s: %w（回滚异常: %v）", cmdText, e, rbErr)
			}
			res.Message = "nft 建表失败，已回滚"
			return res, fmt.Errorf("nft 失败: %s: %w", cmdText, e)
		}
		res.Steps = append(res.Steps, ApplyStep{Name: "nft", OK: true, Command: cmdText})
	}

	res.Applied = true
	// 基线只在第一次记录：重复 apply 时内核可能已被 dae 改过，不能用「当前值」覆盖原始值
	st := a.readState()
	if st.OriginalIPForward == "" {
		st.OriginalIPForward = ipFwdBefore
		st.HadSysctlFile = hadSysctlFile
		st.OriginalSysctl = sysctlBefore
	}
	st.AppliedAt = time.Now().Format(time.RFC3339)
	st.Fingerprint = plan.Fingerprint
	st.NftCommands = nftPlan(plan)
	a.writeState(st)
	res.Message = "已应用：自有片段已写入并 reload 成功"
	if len(res.Commands) > 0 {
		res.Message += fmt.Sprintf("，并创建 %d 条自有 nft 规则", len(res.Commands))
	}
	return res, nil
}

// accessState 记录应用前的系统基线，回滚时据此还原（ip_forward 与面板自有的 sysctl 持久化文件）。
type accessState struct {
	AppliedAt         string `json:"appliedAt"`
	Fingerprint       string `json:"fingerprint"`
	OriginalIPForward string `json:"originalIpForward,omitempty"`
	HadSysctlFile     bool   `json:"hadSysctlFile,omitempty"`
	OriginalSysctl    string `json:"originalSysctl,omitempty"`
	// NftCommands 记录本次应用时真正执行过的 nft argv，供面板启动时按需重建（内核运行态不落盘）。
	NftCommands [][]string `json:"nftCommands,omitempty"`
}

func (a *Applier) statePath() string { return filepath.Join(a.BackupDir, "access-state.json") }

// sysIPForwardPath 便于测试注入（真实环境是 /proc/sys/net/ipv4/ip_forward）。
var sysIPForwardPath = "/proc/sys/net/ipv4/ip_forward"

// readIPForward 读内核当前值（读不到返回空串，例如在 macOS 上测试）。
func readIPForward() string {
	b, err := os.ReadFile(sysIPForwardPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func (a *Applier) writeState(st accessState) {
	_ = os.MkdirAll(a.BackupDir, 0o700)
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(a.statePath(), b, 0o600)
}

func (a *Applier) readState() accessState {
	var st accessState
	b, err := os.ReadFile(a.statePath())
	if err != nil {
		return st
	}
	_ = json.Unmarshal(b, &st)
	return st
}

// restoreSysctl 还原应用前的内核参数（dae 的 auto_config_kernel_parameter 会把 ip_forward 置 1，
// 回滚时 dae 不会自动改回来，需要面板自己还原）。
func (a *Applier) restoreSysctl(st accessState) []ApplyStep {
	steps := []ApplyStep{}
	if st.OriginalIPForward != "0" {
		return steps
	}
	if readIPForward() == "0" {
		return steps // 已经回到基线
	}
	cmd := []string{"-w", "net.ipv4.ip_forward=0"}
	_, err := a.Runner.Run("sysctl", cmd...)
	steps = append(steps, ApplyStep{Name: "sysctl-restore", OK: err == nil, Detail: errText(err), Command: "sysctl -w net.ipv4.ip_forward=0"})
	return steps
}

// DeleteTables 删除面板自有的三张表（不存在时忽略错误）。
func (a *Applier) DeleteTables() []ApplyStep {
	a.ensure()
	steps := []ApplyStep{}
	for _, t := range ownTables {
		cmd := []string{"delete", "table", "inet", t}
		out, err := a.Runner.Run("nft", cmd...)
		ok, detail := err == nil, errText(err)
		if err != nil && (strings.Contains(detail, "No such file or directory") || strings.Contains(out, "No such file or directory")) {
			ok, detail = true, "表不存在，跳过"
		}
		steps = append(steps, ApplyStep{Name: "nft-delete", OK: ok, Detail: detail, Command: "nft " + strings.Join(cmd, " ")})
	}
	return steps
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// RestoreAccess 备份并删除自有片段，然后 reload（把 dae 恢复成接入前的行为）。
func (a *Applier) RestoreAccess() (string, error) {
	a.ensure()
	b, err := os.ReadFile(a.AccessPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // 本来就没有片段：无需恢复
		}
		return "", err
	}
	if err := os.MkdirAll(a.BackupDir, 0o700); err != nil {
		return "", err
	}
	backup := filepath.Join(a.BackupDir, "access-"+time.Now().Format("20060102-150405.000")+".dae")
	if err := os.WriteFile(backup, b, 0o600); err != nil {
		return "", err
	}
	if err := os.Remove(a.AccessPath); err != nil {
		return backup, err
	}
	if a.Dae != nil {
		a.Dae.Reload(a.Dae.RunningPID())
	}
	return backup, nil
}

// Rollback 回滚一次接入变更：删自有表 → 备份并删除片段 → reload。confirm=false 只列出将做什么。
func (a *Applier) Rollback(confirm bool) (ApplyResult, error) {
	a.ensure()
	res := ApplyResult{DryRun: !confirm, AccessPath: a.AccessPath}
	for _, t := range ownTables {
		res.Commands = append(res.Commands, "nft delete table inet "+t)
	}
	if _, err := os.Stat(a.AccessPath); err == nil {
		res.Commands = append(res.Commands, "rm "+a.AccessPath+"（会先备份到 "+a.BackupDir+"）")
	}
	if !confirm {
		res.Message = "dry-run：以上是回滚将执行的动作，未做任何修改"
		return res, nil
	}
	res.Steps = append(res.Steps, a.DeleteTables()...)
	backup, err := a.RestoreAccess()
	res.BackupPath = backup
	if err != nil {
		res.Message = "回滚未完成"
		return res, fmt.Errorf("回滚失败: %w", err)
	}
	st := a.readState()
	res.Steps = append(res.Steps, a.DisableForwarding(&st)...)
	_ = os.Remove(a.statePath())

	res.Applied = true
	res.Message = "已回滚：自有 nft 表已删除，片段已移除并 reload"
	if st.OriginalIPForward == "0" && readIPForward() == "0" {
		res.Message += "，内核 ip_forward 已还原为 0"
	}
	return res, nil
}

// OwnTableNames 返回面板自有的 nft 表名（回滚/重建只碰这些）。
func OwnTableNames() []string { return append([]string{}, ownTables...) }

// MissingOwnTables 返回当前内核里不存在的自有 nft 表名。
func (a *Applier) MissingOwnTables() []string {
	a.ensure()
	out, err := a.Runner.Run("nft", "list", "tables")
	present := map[string]bool{}
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			f := strings.Fields(line)
			if len(f) >= 3 && f[0] == "table" {
				present[f[2]] = true
			}
		}
	}
	missing := []string{}
	for _, t := range ownTables {
		if !present[t] {
			missing = append(missing, t)
		}
	}
	return missing
}

// RebuildTables 供面板启动自检使用：接入计划建的自有 nft 表是内核运行态、不落盘，
// 重启后必然消失；这里按 apply 时记录的命令重建，不改 dae 配置、不 reload。
// 从未应用过接入计划时返回 rebuilt=false（不做任何事）。
func (a *Applier) RebuildTables() (steps []ApplyStep, rebuilt bool, err error) {
	a.ensure()
	st := a.readState()
	if len(st.NftCommands) == 0 {
		return nil, false, nil
	}
	// panel_dns 由 DNSMode 决定是否该存在；只要求「记录过的命令涉及的表」存在。
	wanted := map[string]bool{}
	for _, c := range st.NftCommands {
		for _, tok := range c {
			for _, own := range ownTables {
				if tok == own {
					wanted[own] = true
				}
			}
		}
	}
	missing := []string{}
	for _, t := range a.MissingOwnTables() {
		if wanted[t] {
			missing = append(missing, t)
		}
	}
	if len(missing) == 0 {
		return nil, false, nil
	}
	steps = append(steps, ApplyStep{Name: "nft-rebuild", OK: true, Detail: "自有 nft 表缺失：" + strings.Join(missing, ",") + "（重启后内核运行态丢失），按上次接入计划重建"})
	steps = append(steps, a.DeleteTables()...)
	for _, c := range st.NftCommands {
		cmdText := "nft " + strings.Join(c, " ")
		if _, e := a.Runner.Run("nft", c...); e != nil {
			steps = append(steps, ApplyStep{Name: "nft", OK: false, Command: cmdText, Detail: e.Error()})
			return steps, false, fmt.Errorf("重建 nft 表失败: %s: %w", cmdText, e)
		}
		steps = append(steps, ApplyStep{Name: "nft", OK: true, Command: cmdText})
	}
	return steps, true, nil
}
