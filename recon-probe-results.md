# dae 实机只读侦察报告（recon-probe-results）

> **执行者：** prober（设备侦察/只读实测工程师）
> **目标机：** Debian-te（192.168.1.85，x86_64），dae v2.1.1，systemd dae.service 运行中（PID 708）
> **核查日期：** 2026-10-23
> **数据来源：** 实机 SSH 只读探测（`ssh -i ~/.ssh/dae_test_ed25519 root@192.168.1.85`）
> **约束遵守：** 全程未 restart/stop/reload/suspend，未改 /etc/dae/*，未删系统 geo，未 ip link set / tc 增删。所有测试配置写在 `/tmp/dae-eval-prober/`（已保留）。
> **标注：** ✅ 已实测确认 / ⬜ 只得到间接信号（需后续受控 run）/ ❌ 与核对表冲突

---

## 0. 一句话结论

dae CLI 能力**基本与核对表 A~D 一致，且比预期更好**：`dae --version`/`validate`/`export outline`/`sysdump`/`trace`/`reload`/`suspend` **在本机二进制中全部真实存在**（`trace` 存在，推翻 D8 的"大概率没有"假设）。关键实测确认：
- **D1 盲区成立**：404 订阅 URL 的配置 `validate` 退出 **0**（不解析订阅）。
- **validate 不需要 root**：nobody 用户跑 validate 退出 **0**。
- **validate 有文件权限闸门**：配置文件 0644 直接被拒（要求 0640/0600）——面板必须 chmod 0600。
- **geo 查找是"搜索路径列表"**（config 目录 → ~/.local/share/dae → /usr/local/share/dae → /usr/share/dae），不是单纯"按配置文件目录推断"（细化 D3）。
- **本机已装第三方工具 goose 0.5.18**（非 daed），它是 /etc/dae/config.dae 的**当前唯一写入方**（文件头明写"由 Goose 生成，真源是 config.yaml"）——即"双写"风险的前置事实已存在。
- **流量拦截在 v2.1.1 走 netkit 设备（dae0↔dae0peer，netns daens），不是 tc-bpf**：三处 clsact qdisc 均存在但 `tc filter show` 全空、无 ip rule、无 nft 表。

---

## E1 实际版本与架构 — ✅

**命令：** `dae --version`

**退出码：** 0

**关键输出原文（多行）：**
```
dae version v2.1.1
go runtime go1.26.8-X:heapminimum512kib,newinliner,simd linux/amd64
Copyright (c) 2022-2026 @daeuniverse
License GNU AGPLv3 <https://github.com/daeuniverse/dae/blob/main/LICENSE>
```

**结论：** 版本 v2.1.1；GOOS/GOARCH = **linux/amd64**（`uname -m` = x86_64）。多行文本，需按行解析。
**与 A.1 一致性：** ✅ 完全一致（多行：版本 + go runtime + 版权/license 行）。

---

## E2 子命令全集 / trace 是否存在 — ✅（trace 存在，推翻 D8）

**命令：** `dae --help`、`dae help <cmd>`（各子命令逐一）

**退出码：** 均为 0

**关键输出原文（`dae --help` 子命令表）：**
```
Available Commands:
  export      To export some information for UI developers.
  help        Help about any command
  honk        Let dae call for you.
  reload      To reload config file without interrupt connections.
  run         To run dae in the foreground.
  suspend     To suspend dae. This command puts dae into no-load state. Recover it by 'dae reload'.
  sysdump     To dump up system network config
  trace       To trace traffic
  validate    To validate dae config.
```

**各子命令 help 全部可解析：** export / trace / validate / reload / suspend / sysdump / honk / completion 均返回 0。
- `dae help trace` 输出真实 flags：`--drop-only`、`-4/--ipv4`、`-6/--ipv6`、`-p/--l4-proto`(默认 tcp)、`-o/--output`(默认 /dev/stdout)、`-P/--port`(默认 80)。
- `dae help completion` 存在（bash|zsh|fish）。
- 裸 `dae`（无子命令）打印 help，**退出 0**（与 A.1 一致）。

**结论：** `trace` **在本机默认二进制中确实存在并可用**（非 build tag 裁剪）。
**与核对表一致性：** ⚠️ **与 D8 冲突**——D8 称"官方默认二进制里很可能没有 `dae trace`"；本机实测**有**。面板仍应按"命令可能不存在"做稳健探测（不同发行/构建可能不同），但当前这台机器上可直接用。

---

## E3 `run --help` 逐项对照 A.2 — ✅

**命令：** `dae run --help`

**退出码：** 0

**关键输出原文：**
```
Flags:
  -c, --config string            Config file of dae.(required)
      --disable-pidfile          Not generate /var/run/dae.pid.
      --disable-sudo             Disable sudo prompt ,may cause startup failure due to insufficient permissions
      --disable-timestamp        Disable timestamp.
  -h, --help                     help for run
      --logfile string           Log file to write. Empty means writing to stdout and stderr.
      --logfile-maxbackups int   The maximum number of old log files to retain. (default 3)
      --logfile-maxsize int      Unit: MB. The maximum size in megabytes of the log file before it gets rotated. (default 30)
```

**逐项对照 A.2：**
| A.2 参数 | 实测 | 结论 |
|---|---|---|
| `-c/--config`（必填） | 有，(required) | ✅ |
| `--logfile`（空=stdout/stderr） | 有，描述一致 | ✅ |
| `--logfile-maxsize` 默认 30 | 有，(default 30) | ✅ |
| `--logfile-maxbackups` 默认 3 | 有，(default 3) | ✅ |
| `--disable-timestamp` 默认 false | 有 | ✅ |
| `--disable-pidfile` 默认 false | 有 | ✅ |
| `--disable-sudo` 默认 false | 有 | ✅ |

**结论：** 与 A.2 **逐项完全一致**，无缺失、无新增。
**与 A.2 一致性：** ✅

---

## E4 非 root 跑 `validate` — ✅（可用；但有配置文件权限闸门）

> 本机**未安装 sudo**（`sudo: 未找到命令`，EXIT=127），改用 `setpriv` / `runuser`。

**命令与结果：**

1) nobody 读 root-owned 0600 配置（权限不足路径）：
```
setpriv --reuid 65534 --regid 65534 --clear-groups dae validate -c /tmp/dae-eval-prober/valid.dae
EXIT=1
failed to read config file /tmp/dae-eval-prober/valid.dae: open /tmp/dae-eval-prober/valid.dae: permission denied
```

2) nobody 读自己所有 0600 的合法配置（`/tmp/dae-eval-prober/nb.dae`，chown nobody:600）：
```
setpriv --reuid 65534 --regid 65534 --clear-groups dae validate -c /tmp/dae-eval-prober/nb.dae
EXIT=0
```
`runuser -u nobody -- dae validate -c /tmp/dae-eval-prober/nb.dae` → **EXIT=0**

**结论：** `dae validate` **本身不需要 root**（nobody 无提权、不触发 AutoSu，退出 0），与 A.1 "validate root=否" 一致 ✅。但受**配置文件读权限**限制（必须对该用户可读）。
**与核对表一致性：** ✅（"不需要 root，但可能受配置文件权限限制"——实测两者都验证到了）。

---

## E6 故意写坏一行 routing → validate — ✅

**命令：** `dae validate -c /tmp/dae-eval-prober/badsyntax.dae`（第 6 行 `pname(test) ->` 故意残缺）

**退出码：** 1

**关键输出原文：**
```
failed to parse config file /tmp/dae-eval-prober/badsyntax.dae:
line 6:12     fallback: direct
                      ^: mismatched input ':' expecting '}'
```

**结论：** 语法错误能被抓出，退出码非 0，**错误含行号、列号、脱字符定位**，可读性高。
**与判定标准（D2/E6）：** ✅

---

## E7 引用不存在的 group `-> notexist_group` — ✅

**命令：** `dae validate -c /tmp/dae-eval-prober/badgroup.dae`（routing 内 `pname(test) -> notexist_group`，group 段已定义合法组 grp）

**退出码：** 1

**关键输出原文：**
```
invalid routing rules: failed to parse 'pname(test)': outbound (group) "notexist_group" not found; please define it in section "group"
```

**结论：** validate 做 outbound（group）存在性检查，报错明确提示"未在 group 段定义"。
**与判定标准：** ✅（"应报 outbound (group) 未找到"——完全命中）。

> 附注：初次测试只写了不完整 group（`grp{}`），validate 先报 `section "grp" requires param "policy" but not found`——说明 group 定义缺 policy 也会被前置校验抓出。

---

## E8 404 订阅 URL → validate（验证 D1 盲区）— ✅ 盲区确认

**命令：** `dae validate -c /tmp/dae-eval-prober/badsub.dae`
配置内容：`subscription { deadsub: "http://127.0.0.1:9/notexist-404" }` + 合法 global/routing（文件 0600）

**退出码：** **0（无任何输出）**

**关键输出原文：**
```
EXIT=0
```

**结论：** **订阅不可达 / 404 完全不会被 validate 发现**（validate 不联网、不解析订阅），退出 0。这**实证了 D1**：validate 对订阅类错误是盲区，订阅错误只在 run（构建控制面）阶段才暴露。
**与判定标准：** ✅（"预期发现不了"——命中）。**只做了 validate，未 run**，符合约束。

---

## E10 缺 geo 数据时 validate 的报错 — ✅（含搜索路径原文）

**基线（geo 存在）：** `dae validate -c /tmp/dae-eval-prober/geosite.dae`（含 `domain(geosite:cn)`）→ **EXIT=0**（geo 正常加载）。

**缺 geo 模拟（不碰系统 geo）：** 用**私有 mount namespace** 把空 tmpfs 挂到 /usr/share/dae（只影响该私有 ns，不影响宿主机）：
```
unshare -m --propagation private sh -c 'mount -t tmpfs none /usr/share/dae && dae validate -c /tmp/dae-eval-prober/geosite.dae; echo EXIT=$?'
EXIT=1
ApplyRulesOptimizers error:
geosite.dat: file does not exist in [/tmp/dae-eval-prober, /root/.local/share/dae, /usr/local/share/dae, /usr/share/dae]
```

**结论与细化 D3：**
- 缺 geo 时 validate 退出 1，报错可读（`ApplyRulesOptimizers error: geosite.dat: file does not exist in [...]`）。
- **重要细化：geo 查找是一个搜索路径列表**（按顺序）：**①配置文件所在目录 → ②~/.local/share/dae → ③/usr/local/share/dae → ④/usr/share/dae**。不是 D3 所述"仅按配置文件目录推断"。系统 geo 现状：`/usr/share/dae/geoip.dat`(16.5MB) + `/usr/share/dae/geosite.dat`(11.2MB) 存在且世界可读。
**与判定标准：** ✅（错误文本可用于面板错误分类）；**与 D3 措辞有出入**（D3 应改为"搜索路径列表"）。

---

## E11 validate 退出码全谱 — ✅

| 场景 | 命令 | 退出码 | 输出 |
|---|---|---|---|
| 合法 | `dae validate -c /tmp/dae-eval-prober/valid.dae`（0600） | **0** | 无输出 |
| 非法（语法） | `dae validate -c .../badsyntax.dae` | **1** | 行/列/脱字符解析错误 |
| 非法（group 引用） | `dae validate -c .../badgroup.dae` | **1** | outbound "notexist_group" not found |
| 缺 `-c` | `dae validate` | **1** | `Argument "--config" or "-c" is required but not provided.`（stdout） |
| 文件权限过松 | 0644 配置文件 | **1** | `permissions 0644 ... are too open; requires ... suggest 0640 or 0600` |
| 文件不可读 | nobody 读 root-600 | **1** | `failed to read config file ...: permission denied` |

**结论：** 退出码谱干净：合法→0，非法/缺参/权限→1，且**提示打到 stdout**。**附加发现：validate 会主动拒绝 0644 配置文件**（安全闸门，面板写配置必须 0600/0640）。
**与判定标准：** ✅（"`-c` 缺失时退出 1 且提示打到 stdout"——命中）。

---

## E9 二进制是否内含 Clash/YAML 解析能力 — ⬜ 静态信号：**无 YAML/Clash 配置解析**（待受控 run 终判）

**命令：**（本机无 `strings`，改用 `grep -a` 直接扫二进制）
```
for p in gopkg.in/yaml yaml.v3 clash proxy-providers sip008 SIP008 base64 \
         ResolveSubscription ResolveFile subscription ss:// vmess:// hysteria; do
  printf "%-22s %s\n" "$p" "$(grep -aoiF "$p" /usr/bin/dae | wc -l)"; done
```

**关键计数原文：**
```
gopkg.in/yaml          0
yaml.v3                0
gopkg.in/yaml.v3       0
clash                  7
proxy-providers        0
sip008                 8
base64                 73
ResolveSubscription    8
ResolveFile            2
subscription           96
vmess://               2
hysteria               324
```
`proxies:` 作为配置键计数 = **0**

**"clash" 命中上下文（证明是协议库而非配置解析）：**
```
nfig.proto\x12\x15clash.transport.v
m/Dreamacro/clash/transport/v
```

**推断：**
- **无 `gopkg.in/yaml` 字符串**（Go 二进制通常会保留包导入路径）→ 极可能**未链接任何 YAML 解析器**。
- 无 `proxy-providers`、无 `proxies:` 配置键 → 无 Clash 配置结构处理。
- `clash` 的 7 处命中均是 `Dreamacro/clash/transport`（clash 的传输层库，被 hysteria/ss 等协议复用）与 `clash.transport` proto，**不是 Clash YAML 配置解析器**。
- 反面：`sip008`(8)、`base64`(73)、`ResolveSubscription`(8)、`ResolveFile`(2) 均在 → **订阅解析器（base64 / SIP008 / 明文 / file）确实存在**。

**结论：** **强静态信号支持 C.3 —— dae 不解析 Clash YAML**（只认 base64/SIP008/明文节点行/file）。⚠️ **E9 的 run 测试按约束未做**（留待受控 run 才能终判"喂真实 Clash YAML 时的报错文本形态"）。

---

## E19 内核版本 / BPF 内核配置 — ✅

**命令：** `uname -a`；`grep BPF /boot/config-$(uname -r)`

**关键输出原文：**
```
Linux Debian-te 6.12.113+deb13-amd64 #1 SMP PREEMPT_DYNAMIC Debian 6.12.113-1 (2026-10-15) x86_64 GNU/Linux
```
BPF 相关内核选项（全部满足 dae 要求）：
```
CONFIG_BPF=y
CONFIG_BPF_SYSCALL=y
CONFIG_BPF_JIT=y
CONFIG_BPF_JIT_DEFAULT_ON=y
CONFIG_BPF_UNPRIV_DEFAULT_OFF=y
CONFIG_BPF_LSM=y
CONFIG_CGROUP_BPF=y
CONFIG_BPF_STREAM_PARSER=y
CONFIG_DEBUG_INFO_BTF=y
CONFIG_DEBUG_INFO_BTF_MODULES=y
CONFIG_BPF_EVENTS=y
# CONFIG_BPF_JIT_ALWAYS_ON is not set
```

**结论：** 内核 6.12.113，BPF 全套 + BTF 齐备，满足 dae eBPF 要求。
**与判定标准：** ✅

---

## E20 eBPF 相关系统限制 — ✅（记录实际值）

**命令 / 关键输出原文：**
```
ulimit -l        → 8192      (soft, KB)
ulimit -Hl       → 8192      (hard, KB)
/proc/sys/kernel/unprivileged_bpf_disabled → 2
sysctl kernel.unprivileged_bpf_disabled     → 2
sysctl net.core.rmem_max  → 212992
sysctl net.core.wmem_max  → 212992
sysctl kernel.panic_on_oops → 0
```

**结论：**
- memlock soft=hard=**8192 KB（约 8MB）**，并非 unlimited；dae 仍能运行（v2.1.1 netkit 路线对 memlock 依赖较低，或运行时自行处理）。**注意：systemd unit 未设 `LimitMEMLOCK`**（见 E26）。若后续切到需要大 memlock 的路径，可能需补 `LimitMEMLOCK=infinity`。
- `unprivileged_bpf_disabled=2`（禁止非特权 BPF）——dae 以 root 运行，无影响。
- rmem/wmem 均为内核默认 212992；panic_on_oops=0。
**与判定标准：** ✅（已记录实际值）

---

## E21 tc 钩子现状（只读）— ✅（v2.1.1 用 netkit，非 tc-bpf）

**命令（只 show，未增删）：** `ip -br link`；`ip route show default`；`tc qdisc show dev <iface>`；`tc [-s] filter show dev <iface> [ingress|egress]`；`ip rule`；`nft list tables`；`ip netns list`；`ip -d link show dae0`；`ip netns exec daens ...`

**关键输出原文：**
```
# ip -br link
lo       UNKNOWN  00:00:00:00:00:00 <LOOPBACK,UP,LOWER_UP>
ens2     UP       06:98:e8:0d:41:15 <BROADCAST,MULTICAST,UP,LOWER_UP>     # 物理网卡
dae0@if3 UP       42:df:d0:53:4c:c0 <BROADCAST,MULTICAST,UP,LOWER_UP>     # dae 设备

# ip route show default
default via 192.168.1.1 dev ens2 proto dhcp src 192.168.1.85 metric 1002

# tc qdisc show dev ens2
qdisc fq_codel 0: root ...
qdisc clsact ffff: parent ffff:fff1

# tc qdisc show dev dae0  → qdisc noqueue 0: root ; qdisc clsact ffff: parent ffff:fff1

# tc -s filter show dev ens2 ingress   → (空)  rc=0
# tc -s filter show dev ens2 egress    → (空)  rc=0
# tc filter show dev ens2 (all)        → (空)
# tc -s qdisc show dev ens2 里 clsact：Sent 0 bytes 0 pkt

# ip rule → 只有 0/local, 32766/main, 32767/default（无自定义规则）
# nft list tables → (空)

# ip -d link show dae0
dae0@if3 ... netkit mode l2 type primary policy forward peer policy forward ... link-netns daens

# ip netns list → daens (id: 0)
# 在 daens 里：dae0peer@if4 的 clsact 存在，但 filter 同样为空
```

**结论：**
- 三个设备（ens2、dae0、netns 内 dae0peer）都挂了 **clsact qdisc，但 ingress/egress 的 `tc filter` 列表全为空**（rc=0），clsact 统计 "Sent 0 bytes"。
- 无自定义 ip rule、无 nft 表。
- **dae0 是 netkit l2 设备**（`link-netns daens`），即 **v2.1.1 的流量拦截走 netkit 设备 + 独立 netns，而非传统 tc-bpf filter 挂在物理网卡上**。这解释了 tc filter 全空但代理仍在工作（日志有 group 选路）。
- **对面板的含义：** E21 原假设的"tc filter 残留 → 停服务是否干净卸载"这一断网风险点，在本版本形态下**不是通过 tc filter 体现**；netkit 设备/BPF 的挂载点需 `bpftool` 才能进一步确认（**本机未装 bpftool**，无法只读枚举 BPF prog；装包会改系统，未做）。**建议后续受控环境用 bpftool 复核 netkit 上的 BPF attach 与 dae 停止后的清理。**
**与判定标准：** ✅（已记录 ingress/egress 现状：clsact 在、filter 空）；卸载验证**未能做**（需停服务，被约束禁止）。

---

## E22 `/var/run` 可写性与 pid/progress 文件 — ✅

**命令 / 关键输出原文：**
```
ls -ld /var/run            → lrwxrwxrwx /var/run -> /run
touch /var/run/.dae-write-test → WRITABLE (成功并已删除)
/var/run/dae.pid           → -rw-r--r-- root root 3 bytes ; 内容 = 708
/var/run/dae.progress      → -rw-r--r-- root root 1 byte  ; 内容 = '2'(0x32)
/var/run/dae.abort         → 不存在（无该文件）
```

**结论：** `/var/run`（→/run，tmpfs）**可写**；pid/progress 文件已由 dae 正常落地（pid 内容为 708，progress 为单字节 '2'）；abort 文件当前不存在。容器/只读 rootfs 场景需另测（本机为常规 Debian，/run 为 tmpfs 可写）。
**与判定标准：** ✅

---

## E24 现有 dae 安装形态 — ✅

**命令 / 关键输出原文：**
```
ls -l /usr/bin/dae → -rwxr-xr-x 35483810 bytes, ELF 64-bit LSB statically linked, Go BuildID..., stripped
dpkg -S /usr/bin/dae → "没有找到与 /usr/bin/dae 相匹配的路径"（非 apt 安装）
/etc/dae 结构：
  config.dae  (-rw------- root root, 987 bytes, 51 行)
  backups/    (16 份 *.dae 历史快照)
  subs/       (CF.sub, dual.sub, *.meta.json — 订阅缓存，drwx------ )
  # 无 config.d/ 目录；config.dae 内 grep include = 0（未使用 include 拆分配置）

二进制来源证据：
/root/bundle-final/dae-install/ 内有 dae-x86_64(35483810 bytes，与 /usr/bin/dae 字节数完全一致)、
dae-arm64、example.dae、geo/{geoip,geosite}.dat；另有 /root/goose-deploy-bundle-*/
```

**结论：**
- 二进制来源：**手动从发布 bundle（`dae-x86_64`）拷贝安装**，非 apt/opkg（dpkg 无归属）；stat ic Go、stripped、v2.1.1。
- `/etc/dae`：**单文件 config.dae（51 行）**，配 `backups/`（历史快照）与 `subs/`（订阅缓存）两个目录。
- **无 `config.d/`、无 `include` 段**——即核对表 C.2/F4 设想的"只托管 config.d/panel-*.dae"在当前安装中**尚未落地**，是纯单文件形态。
- config.dae 由 **goose 生成**（首行注："由 Goose 生成 —— 真源是 config.yaml，请勿手改本文件"），且 **0600 权限**（恰好满足 validate 权限闸门）。
**与判定标准：** ✅（二进制路径/来源/服务定义已记录）

---

## E25 是否装第三方 Web UI（daed 等）+ 是否写同一份配置 — ✅（装的是 goose，非 daed）

**命令 / 关键输出原文：**
```
ls /usr/bin/daed /usr/local/bin/daed → 均不存在
systemctl list-units | grep daed → daed.service not-found inactive dead
which goose /usr/local/bin → /usr/local/bin/goose ; goose --version → goose 0.5.18
/etc/goose/: config.yaml(1592B), auth.json, .session-key, backups/
/usr/local/bin/: goose + 十余个 goose-0.5.x*.bak 历史备份
ps aux | grep goose → 当前无 goose 进程（未以守护服务常驻）
systemctl（无 goose.service）；crontab 无 goose 项
```

**goose 与 dae 配置的关系（关键）：**
- goose 的 `/etc/goose/config.yaml`（YAML 真源）含 `global/dns/routing/subscriptions/groups/auto_update`，`groups` 用 `subtag(CF)/subtag(dual)` 过滤——**goose 自己就是把 YAML 编译成 dae 配置的"模式 B 式"面板**。
- `/etc/dae/config.dae` 头部明写"由 Goose 生成，真源是 config.yaml"→ **goose 是 /etc/dae 的当前唯一写入方**（备份/订阅缓存亦由它管理）。

**结论：**
- **未装 daed**（dae 官方 Web UI），daed.service 仅为残留的 Conflicts 引用。
- **已装第三方面板 goose 0.5.18**，且它**正是 /etc/dae/config.dae 的生成者**。当前 goose 未常驻运行（无进程/无 service），但其产物是活的。
- **对架构的直接含义：** "双写同一份配置"的风险**已经现实存在**——新面板若要接管 /etc/dae，必须与 goose 协调（或明确"面板禁用/替代 goose"），否则会与 goose 的 config.yaml→config.dae 生成流程互相覆盖。这是 E25 最重要的结论。
**与判定标准：** ✅（"若存在，确认它是否也写同一份配置"——已确认：是）。

---

## E26 systemd unit 内容 — ✅（自定义 unit，非上游默认）

**命令：** `systemctl cat dae.service`

**关键输出原文：**
```
# /usr/lib/systemd/system/dae.service
[Unit]
Description=dae Service
Documentation=https://github.com/daeuniverse/dae
After=network-online.target docker.service systemd-sysctl.service
Wants=network-online.target
# 与 daed 互斥（daed 单元亦声明 Conflicts=dae.service）。
# Goose 的切换流程仍显式执行「先停 daed、再起 dae」，便于控制与回滚。
Conflicts=daed.service
StartLimitIntervalSec=300
StartLimitBurst=5

[Service]
Type=notify
User=root
LimitNPROC=4096
LimitNOFILE=1048576
OOMScoreAdjust=-100
# 启动前干跑校验: 与 run 相同的规则管线, 但不挂 BPF、不发网络请求。
ExecStartPre=/usr/bin/dae validate -c /etc/dae/config.dae
ExecStart=/usr/bin/dae run --disable-timestamp -c /etc/dae/config.dae
ExecReload=/usr/bin/dae reload $MAINPID
Restart=on-abnormal
RestartSec=5s
TimeoutStartSec=120
TimeoutStopSec=30

[Install]
WantedBy=multi-user.target
```

**结论：**
- `Type=notify`、`User=root`、`ExecStart=dae run --disable-timestamp -c /etc/dae/config.dae`（与已知事实一致）。
- **`ExecStartPre=dae validate -c ...`**——启动前已做一次 validate 干跑（良好实践，但注意它只覆盖 routing/dns，不覆盖订阅，见 E8/D1）。
- **`ExecReload=dae reload $MAINPID`**——**正确**地用二进制 reload（而非 SIGHUP），符合 B 节/D4 建议 ✅。
- `Conflicts=daed.service` 且注释提到 "Goose 的切换流程先停 daed 再起 dae"——**说明该 unit 被人为定制**（含中文注释与 goose/daed 切换逻辑），非 dae 上游默认 unit。
- **未设 `LimitMEMLOCK`**（与 E20 的 8192KB 偏低相呼应，但当前运行正常）。
**与判定标准：** ✅（Type/ExecStart/权限均已确认）

---

## 附：汇总——与核对表 A~D 的冲突/细化点

| 编号 | 核对表原说法 | 实测 | 处理建议 |
|---|---|---|---|
| **D8** | "官方默认二进制里很可能没有 `dae trace`" | ❌ **冲突**：本机 v2.1.1 有 `trace` 且可用 | 文档改为"取决于构建；本机实测有，但面板仍应稳健探测命令是否存在" |
| **D3** | "validate 需要读 geo，按配置文件目录推断位置" | ⚠️ **细化**：geo 是**搜索路径列表**（config目录→~/.local/share/dae→/usr/local/share/dae→/usr/share/dae） | 文档补全搜索路径列表；缺 geo 报错原文已抓到 |
| **D1** | "validate 不解析订阅，订阅错误只在 run 暴露" | ✅ **实证**：404 订阅 URL validate 退出 0 | 保持，且证明必须由面板自做订阅预检 |
| **A.1 validate root=否** | 不需要 root | ✅ **实证**：nobody 跑 validate 退出 0 | 保持；补充"配置文件须该用户可读" |
| **C.2/F4 include/config.d 拆分** | 建议只托管 config.d/panel-*.dae | ⬜ **当前未用**：本机是单文件 config.dae，无 include/config.d | 落地前需先与 goose 的生成流程协调 |
| **E21 tc 钩子** | 假设靠 tc filter 挂载 | ⚠️ **形态变化**：v2.1.1 走 **netkit 设备 + netns daens**，tc filter 全空 | 断网风险评估需按 netkit 形态重做（需 bpftool，受控环境） |

## 附：需要后续受控 run 才能定的项

- **E8**（已实证 validate 盲区）→ 还需**受控 run** 确认"404 订阅在 run 时到底报什么、进程是否仍存活"，以定面板订阅预检的错误分类。
- **E9**（静态已强信号：无 YAML/Clash 配置解析）→ 仍需**受控 run** 喂真实 Clash YAML 订阅，抓 run 阶段真实报错文本形态，才能把 §4.2 从"模式 A/B 待定"钉死。
- **E12~E18（reload/suspend 契约）**、**E21 停止后清理**：均需对线上 dae 做变更/停止类操作，本次只读任务**未做**，需安排在受控窗口。

## 附：测试产物路径（已保留）
`/tmp/dae-eval-prober/`：valid.dae / badsyntax.dae / badgroup.dae / badsub.dae / geodep.dae / geosite.dae / nb.dae / bare.out，以及临时 geo 软链 geoip.dat、geosite.dat。
