# HomeNet Panel：总体架构与开发结构设计

> **文档版本：** v0.21  
> **整理日期：** 2026-10-23  
> **修订（v0.2）：** 依据 `dae-cli-capability-checklist.md`（dae v2.1.1 源码核对底稿）把 CLI 能力与能力边界正式并入 §2.4、§4.1、§4.2、§4.3、§4.5、新增 §4.7 与新增 §13，对应核对表 D/F 条目，详见文末「本版变更清单」。  
> **修订（v0.3）：** 依据 2026-10-23 对 192.168.1.85（dae v2.1.1）的只读实机实测，回填并修正 §4.1、§4.2、§4.4、§4.5、§4.7（新增 §4.8 共存风险）与 §13（D3/D8 及实机验证补充），详见 §13 末与 §14。  
> **修订（v0.4）：** 用户决策定稿——goose 是面板前身，面板**取代** goose（非共存/非不管）。据此定稿 §4.8 取代决策、§4.7 迁移路径、§11 禁止双写硬性验收、§4.5 回滚语义、§2.3 接管边界，详见 §14。  
> **修订（v0.5）：** 依核对表 E 节实机结论（dae v2.1.1 @192.168.1.85，E1–E26 全部跑完）收口：E9 定论 Clash YAML 不支持 → §4.2 模式 A 定为首发、模式 B 正式立项；R1–R5 关键发现并入 §4.2/§4.5/§4.1；§4.5 订阅预检强化（reload 0 节点拒绝 R1 + 订阅可达 R3 + progress 按 R4 解析）；§4.1 补 validate 盲区/本地订阅相对路径 R2 与 pid 契约 E17；§4.4/§13 补 E16/E21/E25/E26；§11 新增“启动/重载 0 节点保护”“应急 suspend 可恢复性”两条验收。详见 §14。
> **修订（v0.6）：** 新增 §8「goose→面板 迁移里程碑 M1–M5」（对应 §4.7 五步迁移，含 gating 与可回滚/不可逆标注），详见 §14。
> **修订（v0.7）：** goose→面板迁移 M1–M4 已在 192.168.1.85 实机完成并验证；更正 E25（goose 存在 `goose.service`，实测 disabled+inactive）；新增「迁移后实际配置布局」「迁移后回滚到 goose 与残留风险」；§11 双写检测现状更新。详见 §14。
> **修订（v0.8）：** P0 代码从零落地并在 192.168.1.85 部署运行：systemd 服务化、账号密码登录（取代静态 Bearer token）、内容层（订阅/分组/规则结构化编辑 + 渲染应用事务）、订阅节点聚合（只读解析订阅列出节点）。新增 §14「实现现状与 API」。详见 §14。
> **修订（v0.9）：** 前端本地化（Vue 内嵌，去 CDN 依赖）；节点测速（并发 TCP 建连延迟）+ 按分组 filter 本地求值命中；模式 B 自建聚合（多订阅合并去重，生成 panel-merged.sub）。新增 §14.6。详见 §14。
> **修订（v0.10）：** 补记前端本地化方案取舍分析（§14.7）；新增模式 B 一键安全切换（POST /api/aggregate/switch，事务化改订阅引用+分组 filter 并校验可回滚）。详见 §14.7 / §14.8。
> **修订（v0.11）：** 启用 HTTPS/TLS（`serve --tls-cert/--tls-key`，cookie 置 Secure，自签 SAN 证书）。详见 §14.9。
> **修订（v0.12）：** 新增分流命中调试器（geosite 解码 + 规则本地求值，§14.10）；分流规则可视化编辑增强（排序、出站联表、未保存规则试算，§14.11）。
> **修订（v0.13）：** 修复 v0.12 尾部截断（§13 的 D8–D12 与整节 §14 丢失，已按实机实测与现有代码重建）；并新增 §14.12「流量 / 命中统计（DNS 旁路）」记录 ③ DNS 查询频度统计与 ② eBPF 预留接口的实现、口径与开销实测。
> **修订（v0.14）：** 新增 §14.13「订阅变更提醒 + 审计日志导出与保留」：状态栏订阅变化红点与差异明细（只提醒、人工应用）、CSV/JSONL 审计导出、按大小轮转与可配置保留份数（含路由器/NAS 磁盘口径建议），并修复 `/api/audit` 死路由（此前前端审计页 404）。  
> **修订（v0.15）：** 流量路径实测（脚本见 `tools/netdiag/`）发现两大事实：① dae 的 `fallback: proxy` 把**本机 DHCP/DNS 与面板自身出站**也代理了（`lAddr=192.168.1.85:68 … proxy_addr=192.168.1.1:67`），已由面板强制注入「安全直连规则」（§14.14）；② 当前**没有任何 LAN 客户端经本机上网**，Mac 网关是 192.168.1.11。并更正 §14.12 的「可疑 DNS 心跳」误判。  
> **修订（v0.16）：** ① goose 存量归档到 `/root/goose-retired/`（315MB，mv 不删，含恢复说明）；② 新增 §14.15 字节级流量统计（方案 B：`ss` socket 计数器 + `/proc/net/dev` 的 `dae0` 总量；明确不用 eBPF 的理由）与前端「字节统计」tab，实机验证通过。  
> **修订（v0.17）：** §4.4 重写为「网络接入与 LAN 接管」：两类流量命运、三张流程图（网关/旁路由/旁路由+策略路由）、**五种接入档位**（gateway / bypass-manual / bypass-router-dhcp / bypass-self-dhcp / bypass-pbr）+ observe、自动选档算法（含中兴光猫与中兴路由的默认假设）、`AccessProfile`/`Plan`/`PBRSpec` 数据模型（策略路由作为保留接口，只出片段不远程执行）、"只写自有 nft 表"的边界、七项自检矩阵、P-A…P-E 实施阶段与风险红线。  
> **修订（v0.18）：** §4.4 去厂商耦合（面向开源）：
> **修订（v0.19）：** 实现 §4.4.7 的 P-A 阶段——新增 `internal/access`（只读能力探测 + 选档推荐）与 `/api/access/probe`、`/api/access/recommend`、前端「接入向导」tab；实机（192.168.1.85）跑通并修掉 3 个真机 bug（`dae version` 子命令不存在 / 未展开 `include` / `nft list tables` 字段数假设），详见 §14.16。
> **修订（v0.21）：** 落地接入向导 P-B（只出计划与只读自检，不执行）：新增 §14.17，定稿"默认 B1、其余档位仅留生成入口"，并新增 README/LICENSE（MIT）用于开源。
> **修订（v0.20）：** 补入 §4.4.3 的现场实测样本：中兴家用固件「地址管理」页提供可编辑的**「自定义网关地址」**（= DHCP Option 3），说明"家用路由普遍改不了 DHCP 网关"不成立 → B2 在部分家用固件上可行；同时记录"该值可能影响路由器自身出口、填错会整段断网"的风险与操作顺序。详见 §4.4.3。  
> · 「中兴默认假设」改为**厂商画像 preset 表**（OpenWrt/软路由/光猫/家用路由/Mesh/未知，仅预填与导航，不参与选档逻辑）+ **通用默认档 B1** + 与厂商无关的向导三问；
> · 新增 §4.4.9「开源通用性约束」（不写死网络参数/服务管理器/厂商，探测失败 fail-open 到 observe，无 root 也能只读启动，变更前必出 Plan）。  
> **建议路径：** `docs/architecture.md`  
> **项目状态：** 架构草案；§13 部分条目已在 192.168.1.85 实机复核（文中标注“实测”），E1–E26 已于 2026-10-23 全部实机实测完成

## 1. 项目定位

HomeNet Panel 是一个轻量、开源、低资源占用的家庭网络管理面板，以 dae 为首期核心，提供核心与运行环境管理、聚合订阅、精准分流、网络接口信息和安全配置运维能力。

项目不重新实现代理内核，而是把 dae 的配置、订阅、规则和运行维护能力组织成可视化、可校验、可回滚的工作流。

### 1.1 核心价值

1. **核心与驱动管理**：管理 dae 官方二进制、版本、服务状态、内核能力与运行环境。
2. **聚合订阅**：统一管理多个订阅来源与手动节点，提供标准化、去重、重命名、过滤和策略组编排。
3. **精准分流**：通过结构化规则模型和配置编译器，降低 dae 规则配置门槛。
4. **安全运维**：提供差异预览、配置校验、备份、应用、健康检查与回滚。
5. **跨平台适配**：先在 Armbian/Linux 上验证，再适配 OpenWrt。
6. **可扩展性**：为未来 Tailscale 系统客户端与可选 tsnet 实现预留接口。

### 1.2 明确的职责边界

- **dae**：负责实际网络流量处理。
- **HomeNet Panel**：负责核心管理、配置生成、订阅聚合、分流编排、诊断和安全运维。
- **OpenWrt**：继续负责 DHCP、设备管理、bridge、VLAN、防火墙和底层网络配置。
- **Linux / Armbian**：由平台适配器处理服务、文件、接口和运行环境差异。
- **Tailscale / tsnet**：首期不作为运行依赖，只预留扩展点。

---

## 2. 核心架构决策：dae 独立运行

### 2.1 首期选择

**dae 继续以官方二进制、独立进程运行；HomeNet Panel 负责管理它。**

面板可以统一提供检测、配置、服务控制和升级流程，但不将 dae 强行嵌入 Go 后端进程。

### 2.2 为什么采用独立进程

- dae 崩溃或升级时，面板不必同时退出。
- 面板升级不必强制升级 dae，降低网络中断风险。
- 保留官方二进制和原有服务管理方式，便于迁移与排错。
- 避免依赖未经确认的库级嵌入接口。
- 管理层与流量处理层故障隔离更清晰。

### 2.3 管理边界

- 默认优先检测并接管现有 dae 安装，不擅自重装。
- **“接管”包含配置生成权的接管（v0.4）**：接管现有安装时，须一并接管 `/etc/dae/config.dae` 的生成权——即停用旧的配置生成器（本机为 goose，面板前身，见 §4.8），使面板成为配置的唯一生成者；未接管生成权不算完成接管。
- 修改服务定义或替换二进制前，应展示变更内容并要求用户确认。
- 核心升级前备份二进制、配置与服务定义。
- 升级失败时恢复旧版本，并验证核心能否重新启动。
- 不允许 Web 请求直接拼接或执行任意 Shell 命令。

### 2.4 核心管理接口示意

```go
type CoreManager interface {
    Detect(ctx context.Context) (CoreInfo, error)                // → dae --version（多行输出，按行解析版本与 GOOS/GOARCH）
    Status(ctx context.Context) (CoreStatus, error)             // → 服务状态 + /var/run/dae.pid
    Validate(ctx context.Context, configPath string) error      // → dae validate -c <path>（退出码 + stdout 错误文本）
    Apply(ctx context.Context) error                            // → dae reload（退出码 + /var/run/dae.progress 错误文本）
    Suspend(ctx context.Context) error                          // → dae suspend（应急降为无负载态，dae reload 恢复）
    Outline(ctx context.Context) (Outline, error)               // → dae export outline（缓存，用于表单生成与版本兼容 diff）
    Dump(ctx context.Context) (string, error)                   // → dae sysdump（cwd 指定到临时目录）
    Logs(ctx context.Context, limit int) ([]LogEntry, error)    // → journald / procd 或 --logfile
}
```

实际 dae 命令映射（核对表 F3，基于 dae v2.1.1 源码核对）：

| 方法 | dae 命令 | 行为与注意 |
|---|---|---|
| Detect | `dae --version` | 多行输出（版本行 + `go runtime go1.x linux/arch` + 版权行），需按行解析，不能当单行字符串 |
| Validate | `dae validate -c <path>` | 不碰 BPF，退出码 + stdout 错误文本；**不解析订阅**（见 D1，§4.5） |
| Apply | `dae reload` | 发 SIGUSR1 并轮询 `/var/run/dae.progress`，最多等 60s；结果以 `.progress` 的 ReloadError 文本为准 |
| Suspend | `dae suspend` | 发 SIGUSR2，进入无负载态；`-a` 额外中止已建立连接；用 `dae reload` 恢复 |
| Outline | `dae export outline` | 打印 JSON，含 leaves 字段路径与 structure 结构树；面板缓存用于表单与兼容 diff |
| Dump | `dae sysdump` | 打包到**当前工作目录**，面板必须把 cwd 指定到临时目录（D7） |

> **Apply 失败时进程可能仍在跑旧配置**（待 E13 实机确认）。因此回滚不是“重启服务”，而是“写回文件 + 再 reload”；且 `reload` / `suspend` 内部会 `AutoSu()`，非 root 调用可能卡在 sudo 提示，面板调用应显式加 `--disable-sudo` 并确保自身权限（D5）。

这是管理层抽象。不同核心可能具有不同配置格式和运行机制；dae 专属行为仍应保留在 dae 适配器中。

---

## 3. 总体技术架构

```text
Web 管理界面
  Vue 3 + TypeScript + Vite
  ├── 仪表盘
  ├── 核心管理
  ├── 订阅中心
  ├── 分流规则
  ├── 网络接口
  └── 诊断中心
          │ REST API / JSON / 鉴权
          ▼
Go 后端（模块化单体）
  ├── 核心管理与运行环境检测
  ├── 订阅聚合与节点管理
  ├── 分流规则模型与配置编译器
  ├── 配置事务、版本与回滚
  ├── 网络信息与诊断
  ├── 任务管理
  └── 未来私网接入抽象
          │
          ▼
统一适配接口
  ├── dae 核心适配器
  ├── Linux / Armbian 平台适配器
  └── OpenWrt 平台适配器
          │
          ▼
独立运行的 dae 官方二进制
```

### 3.1 技术选型

| 层次 | 方案 | 说明 |
|---|---|---|
| 后端 | Go | 适合交叉编译与低资源设备 |
| 前端 | Vue 3 + TypeScript + Vite | 便于构建管理页面 |
| 通信 | REST API + JSON | 易于调试与维护 |
| 配置 | 原始配置保留 + 结构化模型 | 减少编辑器对高级配置的破坏 |
| 持久化 | 首期文件系统 | 需要复杂查询时再评估 SQLite |
| Linux | systemd、iproute2 等 | 以实机检测结果为准 |
| OpenWrt | procd、UCI、ubus、fw4 | 遵循 OpenWrt 原生管理方式 |
| 发布 | Go 交叉编译 + 前端静态资源 | 按目标架构构建 |
| 私网扩展 | 预留接口 | 首期不引入 tsnet 依赖 |

首期不需要 Redis、消息队列、微服务或远端 Agent。前端资源可以嵌入 Go 程序，但需先测量 OpenWrt 的闪存和内存占用，再决定是否同时提供分离资源包。

---

## 4. 六个核心模块

### 4.1 核心与驱动管理

负责管理 dae 官方二进制及其运行条件。

**核心生命周期管理**
- 检测二进制路径、版本、架构、启动命令和服务状态。
- 检测现有安装并确认配置路径。
- 升级前备份二进制、配置和服务定义。
- 升级后检查服务状态和关键健康状态。
- 保留旧版本，支持有保护措施的回退。

**内核与运行环境检测**
- 检查内核版本、必要内核配置和权限。
- 检查 dae 所需的 eBPF 相关能力及系统限制。
- 检查网卡、路由、DNS 与接口绑定的常见问题。
- 输出检测结果、错误原因和修复建议。

**边界**
- 本项目中的“驱动管理”首期主要指内核及运行依赖的检测与管理。
- 首期不自动替换系统内核、不自行编译驱动。
- 实际兼容性必须依据目标 dae 版本官方文档和设备实测确认。

**与 dae 实际 CLI 的对齐（v0.2，核对表 F3）**

核心管理接口（见 §2.4 `CoreManager`）直接映射到 dae 实际命令：`Detect → dae --version`（多行解析）、`Validate → dae validate -c <path>`、`Apply → dae reload`、`Suspend → dae suspend`、`Outline → dae export outline`、`Dump → dae sysdump`（cwd 指定临时目录）。要点：

- **应用用 reload 而非重启**：`Apply` 走 `dae reload`，不断已建立连接；失败时进程可能仍在跑旧配置，故回滚是“写回文件 + 再 reload”，不是“重启服务”。
- **应急摘流量用 suspend**：`dae suspend` 是真正的无负载态（不动服务、不做 teardown），`dae reload` 恢复（D6）。
- **reload / suspend 需权限**：内部 `AutoSu()`，非 root 调用可能卡在 sudo 提示，应显式加 `--disable-sudo` 并确保自身权限（D5）。
- **sysdump 注意 cwd**：输出到当前工作目录，必须指定临时目录，避免污染 `/` 或 `/etc/dae`（D7）。
- **validate 不判订阅可用（E8/R3，v0.5）**：`dae validate` 不解析订阅、不联网取订阅，订阅不可达或格式不支持只报 **WARN 非致命**、以空组继续 → `Validate` 不能作为订阅可用性判据，订阅预检见 §4.5 第 2 步。
- **本地订阅用相对路径（R2，v0.5）**：`file://` 仅接受**相对配置目录**的路径，绝对路径被拒（`not support absolute path`，WARN、0 节点）→ 面板托管本地订阅文件须固定相对目录布局（如 `subs/`）。

**实机已验证的环境事实（v0.3，192.168.1.85 / dae v2.1.1，只读实测）**

以下为环境检测与错误分类必须覆盖的实测项：

- **geo 数据定位（修正 D3）**：geo 不是“按配置目录推断”，而是按**搜索路径列表**查找——配置目录 → `~/.local/share/dae` → `/usr/local/share/dae` → `/usr/share/dae`。缺 geo 时 `validate` 报 `geosite.dat: file does not exist in [...]`。环境检测应逐路径核对，并把该报错文本归入“缺 geo 资源”错误分类，提示先下载 geo。
- **流量拦截形态（修正 E21）**：本机 v2.1.1 拦截走 **netkit 设备对（`dae0` ↔ `dae0peer`，netns `daens`）**，不是传统 tc-bpf；`ens2`/`dae0`/`dae0peer` 有 clsact qdisc 但 **`tc filter` 为空**。诊断与卸载检查应针对 netkit 设备与 `daens` netns。
- **运行环境实测项**：memlock soft=hard=8192KB（**非 unlimited**，unit 未设 `LimitMEMLOCK`，可能压 eBPF）；`kernel.unprivileged_bpf_disabled=2`；服务 unit 为定制版 `Type=notify`、`ExecStartPre=dae validate`、`ExecReload=dae reload MAINPID`（正确，非 SIGHUP）。环境检测应核对 memlock 限额、bpf 开关与 unit 定义。

**v0.5 实测补充（E17 / E26）**

- **reload 目标进程（E17）**：`--disable-pidfile` 启动时无 `/var/run/dae.pid`，`dae reload`（无 pid）报 `failed to read pid file` 失败，须显式 `dae reload <pid>`。面板管理 dae 进程时若禁用 pidfile，必须自己持有 pid。
- **启动限流（E26）**：定制 unit 为 `Type=notify` + `ExecStartPre=dae validate` + `ExecReload=dae reload MAINPID` + `Restart=on-abnormal` + `StartLimit 5次/5min`；反复失败会进入 start-limit，恢复流程须含 `systemctl reset-failed`。

### 4.2 聚合订阅中心

核心流程：

```text
多个订阅来源 + 手动节点
          ↓
获取、解析与标准化
          ↓
节点去重、重命名、标签、过滤
          ↓
策略组与节点编排
          ↓
生成 dae 配置
          ↓
预览、校验、应用
```

首版功能：
- 多订阅来源管理与手动节点管理。
- 订阅刷新、失败提示、更新时间。
- 节点统一展示、重命名、过滤（去重能力受模式约束，见下方订阅数据通路）。
- 来源标签、启用/禁用状态。
- 基础策略组生成。
- 刷新前后差异预览，显示新增、删除和变化的节点。
- 对不支持或无法解析的格式给出明确错误，不静默丢弃节点。

**订阅数据通路：两种模式（明确二选一，不并列）**

依据核对表 F1 / C.3（dae v2.1.1 源码核对），下面两种模式是**不同的数据通路**，实现对同一批节点只能选其一：

- **模式 A（推荐首发）：托管 dae 原生订阅。**
  面板只写 `subscription { tag: 'url' }` 与 `group { filter: subtag(tag) && ... }`，节点拉取与解析完全交给 dae。
  面板的“去重 / 重命名”能力随之**降级**为**组级过滤与命名规则**（使用 dae 原生 `group.filter` 的 `subtag(...)`、`name(keyword: ...)` 等），**跨来源去重不可做**。
  优点：实现量小、与官方行为一致、升级风险低。
  约束（C.3 + E9 定论）：dae 原生订阅只支持 **base64 / SIP008 / 明文节点行 / 本地文件（`file://`，且须为**相对配置目录**的路径，见 R2）** 四种格式；**E9 受控 `dae run` 实测已定论：Clash / Clash.Meta YAML 订阅解析为 0 节点**（`subscription resolved to 0 nodes`，仅 WARN 非致命）→ **不支持 Clash YAML**。因此大量只提供 Clash YAML 的商业机场订阅无法走模式 A，需靠模式 B 补齐。

- **模式 B（后续可选，已正式立项列入路线图）：自建聚合。**
  面板自己抓取并解析（需扩展支持 Clash YAML 等更多格式），**只产出 `node { }` 段**。
  优点：真正的跨来源去重 / 重命名 / 过滤。
  代价：要自行维护多格式解析器（相当于重做一套 subconverter 能力），且同一批节点不能同时走 dae 原生订阅（两条通路互斥）。

> **不要把两者并列写成“优先利用 dae 原生能力 + 同时提供聚合编排”。** 模式 A 与模式 B 对同一批节点是互斥的数据通路，必须显式二选一。首发默认模式 A（E9 已实证 Clash YAML 不支持，模式 A 胜出）；模式 B 已正式列入路线图（见 §8 P4）。

**实测结论（v0.5，E8 / E9 已定论）**

- **E8（实证）**：404 订阅后 `dae validate` 退出 0、未报错 → **D1 盲区成立**，§4.5 订阅预检为必需步骤。
- **E9（定论）**：受控 `dae run` 实测，Clash YAML 订阅静默返回 **0 节点**（`subscription resolved to 0 nodes`，WARN 非致命，dae 仍建控制面并加载 BPF）→ **确认不支持 Clash YAML**。故 **模式 A 定为首发**，模式 B 正式立项列入路线图。
- **R1 / R3（重要）**：`dae reload` 对解析出 0 节点的配置会**拒绝应用**（`refusing reload with 0 nodes... keeping the current generation`，progress=3），而**启动 `run` 却容忍 0 节点继续跑**——启动宽松 / 重载严格非对称。故面板订阅预检不能只看 `validate`（D1），还必须覆盖“reload 的 0 节点拒绝 + 订阅可达 + 节点数 > 0”，见 §4.5。

**其余设计原则**

- 将订阅来源与标准化节点分开建模。
- 明确声明支持的订阅格式，不把未知格式伪装成成功。
- 订阅 URL、访问令牌和节点凭据应作为敏感信息处理。

### 4.3 精准分流规则引擎

用户通过结构化规则模型编辑规则，后端编译为目标 dae 版本支持的配置语法。

首版覆盖范围应以目标 dae 版本官方文档为准，可能包括：
- 域名与域名规则集。
- 目标 IP、来源地址。
- 端口与协议。
- 进程相关条件（需确认运行环境和 dae 版本支持情况）。
- 规则优先级、动作与默认策略。
- 规则集与策略组关联。

重点能力：
- 可视化规则编辑。
- 常用规则模板，例如内网直连、指定域名走代理、指定设备使用特定策略。
- 规则顺序调整与潜在遮蔽提示。
- 生成配置预览和原生语法校验。
- 保留可视化模型无法表达的原始配置，避免保存时静默删除。

**编译器前的 outline 校验关卡（v0.2，核对表 A.3）**

在“结构化规则模型 → 配置编译器”链路中插入一道基于 `dae export outline` 的校验关卡：

- 缓存每个 dae 版本的 outline（`leaves` 字段路径列表 + `structure` 的 `type` / `defaultValue` / `required` / `desc`）。
- 编译产物写出前，逐 key 与 outline 的 `leaves` 比对，做**字段存在性校验**：结构化模型产出的 key 在当前 dae 版本不存在即报错，避免生成“看起来合法、运行时才失败”的配置。
- 升级 dae 前后 diff outline，识别“字段被删除 / 默认值变更”，作为**版本兼容检测**提前告警（见 §9 与核对表 G）。

**禁止直接把前端表单拼接成 dae 配置文本。** 规则模型、配置编译器和应用事务必须分层设计。

### 4.4 网络接入与 LAN 接管（v0.17 重写：接入档位 + 计划 + 自检）

> 本节回答一个此前一直悬空的问题：**面板把 dae 配好了，可流量凭什么经过 dae？** 结论：把"接入"做成**按能力分档的向导**（不是多套代码），默认自动选档、可人工开关，策略路由接口保留。

#### 4.4.0 三条设计底线

1. **不碰传统路由功能**：不接管 DHCP 服务（默认）、不做 PPPoE/NAT 网关、不改 `/etc/config/firewall`、`/etc/config/dhcp`（只读）。面板只做"把流量送到 dae 面前"这一件事。
2. **只写自己的东西**：网络规则一律写入**面板自己命名的 nft 表**（`table inet panel_tproxy` / `panel_snat` / `panel_dns`）+ 面板自己的配置文件；需要宿主改的地方，输出成"可粘贴片段 + 人话步骤"，由用户执行。
3. **任何接入变更都可一键回滚**，且失败不影响既有网络（先自检、再启用；启用后有 watchdog）。

#### 4.4.1 两类流量命运（理解接入的前提）

经过本机的包只有两种命运，决定了接入设计的全部差异：

| | 被**劫持**的流量（TCP/UDP 命中 dae 拦截） | 被**转发**的流量（未劫持：ICMP、内网互访、部分 IPv6） |
|---|---|---|
| 谁终结连接 | 本机 dae（客户端与 dae 握手，dae 用自己的 socket 出去） | 内核纯转发，源 IP 仍是客户端 |
| 回程 | dae 本地 socket 收到 → 写回客户端 → **天然对称** | 由主路由按客户端 IP 直接送回 → **回程不经过本机** |
| 是否需要 SNAT | 不需要 | **跨网段 / 主路由严格 zone 或 rp_filter 时需要** |

> 更正（v0.17）：早期把"SNAT 回程"当旁路由通病，说法不严谨。旁路由真正的问题是"包进不来（未开启转发/劫持）"和"DNS 不经过本机"，而非劫持流量的回程。

#### 4.4.2 三种物理形态

**图 A：装在 OpenWrt 上（网关形态）**

```
 [手机/PC] gw=主路由IP
      │ 帧: 目的MAC=路由器, 目的IP=8.8.8.8
      ▼
 ┌──────────────── OpenWrt（与 dae/面板同机） ────────────────┐
 │  br-lan ─► 内核转发(ip_forward=1，路由器天生)              │
 │             ├─► dae 分流阀(netkit)：direct → wan 出去      │
 │             │                        proxy → 隧道到节点    │
 │  wan(pppoe/eth) ─► 外网                                    │
 │  NAT：路由器自己做；DNS：dnsmasq 就在本机 53               │
 └────────────────────────────────────────────────────────────┘
 面板只需声明 lan_interface=br-lan，其余路由功能一概不碰
```

**图 B：旁路由（客户端网关指向本机）**

```
 [PC] gw=<旁路由IP>（手改 或 主路由 DHCP 下发）
      │ 帧: 目的MAC=旁路由, 目的IP=8.8.8.8
      ▼
 ┌──────── 旁路由 <旁路由IP>（普通主机，单/多网口） ────────┐
 │ eth0 ─► ① 内核必须"愿意收"：ip_forward=1 + tproxy 劫持规则  │
 │           ├─ ② 被劫持 ─► dae 本地 socket ─► eth0 再发出     │
 │           │              (源IP=<旁路由IP>，回程天然回来)  │
 │           └─ ③ 未劫持 ─► 纯转发(源IP=客户端)，回程绕开本机 │
 │ eth0 ─► 主路由 ─► 外网                                     │
 └────────────────────────────────────────────────────────────┘
 三个必解坑：① 收流(转发+tproxy) ② 回程(跨网段才需 SNAT) ③ DNS 必须经过本机
```

**图 C：旁路由 + 主路由策略路由（手术式）**

```
 [PC-A] gw=主路由          [PC-B] gw=主路由
      │                        │
      ▼                        ▼
 ┌──── 主路由（DHCP/DNS/NAT 仍在它）────┐
 │ 策略路由：仅 PC-B / 指定域名集的下一跳→.85 │
 └───────┬──────────────────────┬────────┘
         │ PC-A 直出             │ PC-B → 旁路由 → dae → 外网
         ▼                       ▼
       外网                    （只影响被命中的流量）
```

#### 4.4.3 五种接入档位（同一份代码，不同模板）

| 档 | 名称 | 主路由需具备 | 流量由谁送来 | 影响范围 | 面板自动化程度 |
|---|---|---|---|---|---|
| **A** | `gateway` | OpenWrt（或能做网关的 Linux）+ 内核支持 dae | 本机就是网关 | 全屋 | 高（同机，但 DHCP/DNS 配置只读） |
| **B1** | `bypass-manual` | **无要求** | 用户在设备上手改网关/DNS | 单台设备 | 半自动（dae 侧全自动 + 人话步骤） |
| **B2** | `bypass-router-dhcp` | 能改 DHCP 下发的**网关/DNS 选项** | 主路由 DHCP | 全屋 | 半自动（生成步骤 + 自检） |
| **B3** | `bypass-self-dhcp` | 仅需能**关闭**主路由 DHCP | 面板托管的 DHCP（**默认关闭**，需显式授权） | 全屋 | 高，但风险最高 |
| **C** | `bypass-pbr` | 能做 `ip rule` / nftset / PBR（基本只有 OpenWrt/软路由） | 主路由按规则挑流量 | 指定设备/域名 | 只生成片段 + 离线语法校验，不远程执行 |
| — | `observe` | 无 | 谁都不经过 | 无（只观测） | 只读：DNS 命中 + 字节统计 |

**默认选档算法（自动，带理由展示）**：

```
检测到 /etc/openwrt_release 且内核 arch∈{x86_64,aarch64} 且 BPF/BTF 可用
      → 推荐 A（gateway）
否则（普通 Linux 主机，无 OpenWrt 特征）
      → 推荐 B1（bypass-manual）：先把 dae 侧准备好 + 引导"手改一台设备验证"
         若用户回答"主路由可改 DHCP 网关" → 升级 B2
         若用户回答"主路由可做策略路由"   → 提供 C 片段（不改变默认档）
         若用户回答"可以关掉主路由 DHCP" → 才允许启用 B3（需二次确认）
内核 arch 不支持 BPF / BTF 缺失  → 只允许 observe，并明确说明原因
```

**通用默认假设（面向开源，不绑定任何厂商）**：

1. **统计判断（标注为判断）**：家用场景里"能自定义 DHCP 下发网关"的比例低——多见于 OpenWrt/软路由/部分企业级设备；运营商光猫一体机与多数家用路由器只提供 DNS 自定义、不提供网关项。因此 **默认档 = B1（bypass-manual）**。
2. **唯一升档开关**：用户回答"主路由能改 DHCP 网关" → 升 **B2**；回答"能做策略路由" → 提供 **C** 片段（不改变默认档）；回答"能关掉主路由 DHCP" → 才允许 **B3**（二次确认 + 30 分钟自动失效）。
3. **面板不做厂商指纹猜测**：厂商/型号只用于**预填答案与导航**（告诉用户去哪个页面找什么），最终档位由"探测结果 + 用户回答"决定；任何预设值都必须显示为「预设，可修改」。

**厂商画像 preset（只是预填与导航，不是权威结论；全部标注为"需现场确认"）**：

| 画像 | 识别线索（探测） | DHCP 网关项 | 策略路由 | 常见默认档 |
|---|---|---|---|---|
| OpenWrt / ImmortalWrt | `/etc/openwrt_release` 存在 | 有（`uci show dhcp` 的 `option 3`） | 有 | 同机 → A；旁挂 → B2/C |
| 软路由（iKuai / pfSense / OPNsense / ROS） | `/etc/os-release` 或厂商线索 | 通常有 | 通常有 | B2 / C |
| 运营商光猫（含中兴/华为等一体机） | 默认网关为 `.1`、TR-069 痕迹、登录页运营商标识 | 多数**没有**（判断） | 无 | B1 |
| 家用路由器（中兴/华为/TP-Link/小米/荣耀等） | 管理页 `192.168.0.1`/`192.168.1.1` | 多数**没有或仅 DNS**（判断） | 无 | B1（确认有网关项→B2） |
| Mesh 系统（Deco / Orbi / 小米 Mesh 等） | 多节点、App 管理 | 有限 | 无 | B1 |
| 未知 / 用户不清楚 | — | 未知 | 未知 | B1（最安全） |

> 新增一个厂商画像 = 加一条 preset 数据（名称 + 识别线索 + 页面导航 + 预设答案），**不改任何核心逻辑**；这是开源项目的扩展点。

> **实测样本（2026-10-23，中兴家用固件「地址管理」页）**：该页把 DHCP 的 Option 3 做成了可编辑字段 **「自定义网关地址」**（同页「DNS 服务器」「备用 DNS 服务器」也可编辑，DHCP 地址池 192.168.5.2–254，租期可自定义），说明"家用路由普遍不能改 DHCP 网关"这一**统计假设并不成立**——至少该批中兴固件可以。结论不变（默认仍 B1，用户答"能改"则升 B2），但向导应把该字段的常见叫法列全（**自定义网关地址 / 默认网关 / 路由器地址 / 网关 / DHCP Option 3**），并提示风险：该页原文警告"无效的网关地址会导致路由器无法上网"，部分固件把这个值也用于路由器自身出口，**填错会让整台路由断网**；操作必须"先记录原值 → 改 → 立刻验证 → 失败立即改回"。

**向导必问的三件事（与厂商无关，任何路由都能回答）**：
1. 主路由后台的 DHCP 设置里，能否修改**下发的网关地址**？
2. 能否修改**下发的 DNS 地址**？
3. 能否**关闭**主路由的 DHCP 服务？
> 三项都答"不知道/不能" → 仍可工作：走 B1 单机验证，面板不因信息缺失而卡死。

#### 4.4.4 数据模型：AccessProfile + Plan（保留策略路由接口）

```go
type AccessMode string // gateway | bypass-manual | bypass-router-dhcp | bypass-self-dhcp | bypass-pbr | observe

type AccessProfile struct {
    Mode        AccessMode `json:"mode"`
    LanIfaces   []string   `json:"lanIfaces"`   // 接管哪些网卡的流量 → dae global.lan_interface
    WanIface    string     `json:"wanIface"`    // dae global.wan_interface（auto 或具体网卡）
    TProxyPort  int        `json:"tproxyPort"`  // 默认 12345
    DNSMode     string     `json:"dnsMode"`     // hijack53 | forward-from-router | none
    SNAT        string     `json:"snat"`        // auto | on | off（跨网段/严格主路由时用）
    AllowDHCP   bool       `json:"allowDhcp"`   // B3 专用，默认 false，且有 30 分钟自动失效
    PBR         *PBRSpec   `json:"pbr,omitempty"` // 策略路由接口（保留）：只生成片段
}

type PBRSpec struct {                 // C 档保留接口
    MatchBy   string   `json:"matchBy"`   // device | domain-set | ip-set
    Devices   []string `json:"devices,omitempty"`
    DomainSet string   `json:"domainSet,omitempty"`
    NextHop   string   `json:"nextHop"`   // 通常是本机 IP
}

type Plan struct {
    Mode        AccessMode        `json:"mode"`
    Steps       []PlanStep        `json:"steps"`       // 面板能自动执行的部分（有序、可回滚）
    ManualSteps []ManualStep      `json:"manualSteps"` // 必须用户在主路由/设备上做的部分
    Checks      []CheckSpec       `json:"checks"`      // 应用后的自检项
    Risk        []string          `json:"risk"`        // 人话风险提示（断网面、影响范围）
    Fingerprint string            `json:"fingerprint"` // 计划哈希，apply 时校验未被篡改
}
```

- `PlanStep`：`{Kind: dae-config|nft-table|sysctl|service, Path/Table, Content, Revert}`，全部可逆。
- `ManualStep`：`{Where: "<主路由后台>"|"手机WiFi设置", What: "把网关/DNS 改为 <旁路由IP>", Why, Verify}`。
- `PBRSpec` 是**保留接口**：当前只用于"生成 OpenWrt uci/nftset 片段 + 离线语法校验 + 文档指引"，面板**不远程登录主路由**。将来若支持 SSH 到 OpenWrt 主路由，只填这一步执行器即可，模型不变。

#### 4.4.5 渲染产物与"只写自己的表"

| 产物 | 内容 | 归属 |
|---|---|---|
| dae 入口配置 | `global { lan_interface: <LanIfaces>, wan_interface: <WanIface>, tproxy_port: 12345, auto_config_kernel_parameter: true }` | 面板（`config.d/panel-*.dae`） |
| 收流规则 | `table inet panel_tproxy`：PREROUTING 里对 LAN 入方向 TCP/UDP 做 tproxy 到 12345 + `ip rule fwmark` + `ip route add local` | 面板独占表 |
| 回程 SNAT | `table inet panel_snat`：仅 `SNAT=on` 时生成 | 面板独占表 |
| DNS | `table inet panel_dns`：把 LAN 侧 53 劫持到 dae 的 DNS；或（`DNSMode=forward-from-router`）只生成"主路由 dnsmasq 转发到本机"的步骤 | 面板独占表 / 仅步骤 |
| 内核参数 | `ip_forward=1`、`send_redirects=0`：**dae 的 `auto_config_kernel_parameter: true` 已代劳**，面板只读校验 | dae |

**红线**：`/etc/config/firewall`、`/etc/config/dhcp`、主路由的任何配置一律不改；按名字删表（`nft delete table inet panel_*`）即完全卸载，不影响宿主规则。

#### 4.4.6 自检矩阵（"接线成功"必须可证明）

| 检查项 | 方法 | 通过标准 | 失败时提示 |
|---|---|---|---|
| dae 运行 | `/api/status`（service+pid+progress） | active 且 reload 码正常 | 先修 dae 再谈接入 |
| **LAN 流量真的来了** | `/api/traffic/bytes` 的 `clients` 表 | 出现内网地址 | "设备网关/DNS 还没指向本机" |
| 收流生效 | 从设备侧访问一个外部地址，看 `dae0` 计数增长 | 增长 | "转发/tproxy 未生效" |
| 回程对称 | 双向大包传输（如 10MB 下载 + 上传） | 双向速率均正常、无明显卡死 | "可能需开启 SNAT 或主路由 rp_filter" |
| DNS 劫持 | 从设备 `dig` 随机域名，看 dae 日志/命中表 | 命中表出现该域名 | "DNS 未经过本机，域名分流失效" |
| 未夺权 | 读 `/etc/config/*`、比对 md5 | 未被面板改动 | 出现改动即视为 bug |
| 双写 | 检测其它写者（goose/手改） | 仅面板写 | 阻断并提示 |

#### 4.4.7 实施阶段与风险控制

| 阶段 | 内容 | 风险 |
|---|---|---|
| **P-A（先做）** | 只读探测 + 选档推荐（`GET /api/access/probe`）：OpenWrt 检测、arch/BPF/BTF、网卡/默认路由、既有 tproxy/nft 冲突、DHCP 端口占用 | 零（不改任何东西） |
| **P-B** | `bypass-manual`：dae `lan_interface` + `panel_tproxy` + DNS 劫持 + `/api/access/verify` | 低（可 db 删表回滚） |
| **P-C** | `bypass-router-dhcp` / `bypass-pbr`：生成步骤 + 片段 + 离线语法校验 | 低（不执行远端） |
| **P-D** | `bypass-self-dhcp`：默认关闭、显式授权、**30 分钟自动失效**（挂钟 watchdog）+ 立即回滚 | 中（全屋 IP 分配） |
| **P-E** | `gateway`（OpenWrt）：procd 适配 + 能力探测 + **不依赖面板的直连兜底看门狗** | 高（面板挂了全屋断网） |

**风险红线**：进入任何"改变转发路径"的档位前，必须先通过自检矩阵的前两项；apply 前一律备份（nft 全表 dump + 配置文件副本）；任何档位都提供 `POST /api/access/revert` 一键卸载。

#### 4.4.9 开源通用性约束（面向社区，不只服务单一部署）

本项目计划开源，因此接入层从第一天起就必须是"可移植"的，避免把当前测试环境当默认：

| 约束 | 要求 | 反例（禁止） |
|---|---|---|
| 不写死网络参数 | 网关/网段/网卡名一律**探测** | 硬编码 `192.168.1.1`、`ens2`、`192.168.1.85` |
| 不写死服务管理器 | 抽象 `service` 接口：systemd / procd / 其他；缺失则降级只读 | 到处直接 `systemctl` |
| 不写死厂商 | 厂商只作 preset 预填 | `if vendor == "ZTE" { mode = B1 }` |
| 不远程改宿主 | 一切"要改主路由/宿主"的动作 = **步骤 + 可粘贴片段 + 校验**，永不代执行 | 面板 SSH 去改主路由 |
| 探测失败不阻塞 | 任何探测项失败 → fail-open 到 `observe` 并说明原因，不抛错中断 | 没有 BTF 就整个页面打不开 |
| 变更可见可逆 | 任何写入前先展示 `Plan`（含风险与回滚方式），dry-run 可看 | 点一下就直接改转发路径 |
| 无 root / 无 dae 也能跑 | 面板必须能"只看不写"启动（degraded 模式） | 缺权限直接 panic |
| 术语可翻译 | 面向新手的文案集中管理，先中文 + 预留 i18n 词条 | 中文散落在代码各处手拼 |
| 文档即产品 | 每个档位都有"前提 / 步骤 / 自检 / 回滚"四段说明 | 只写代码不写怎么接线 |


#### 4.4.8 平台差异与既有实测结论（沿用）

| 能力 | Linux / Armbian | OpenWrt |
|---|---|---|
| 网卡与 IP 信息 | iproute2 | ubus |
| bridge / VLAN 关系 | 系统网络信息 | ubus、UCI |
| dae 处理范围 | dae 配置（`lan_interface`） | 同左 |
| 服务管理 | systemd | **procd**（需 `service` 接口抽象，勿把 `systemctl` 写死） |
| 网络配置修改 | 只写自有 nft 表 | 只写自有 nft 表，`/etc/config/*` 只读 |

必须区分：① **dae 处理范围**（哪些接口流量交给 dae，`lan_interface` 只决定"处理哪些"，不等于隔离）；② **网络隔离状态**（bridge/VLAN/防火墙，属宿主，面板不碰）；③ **物理接口关系**（端口属于哪个 bridge/VLAN）。

**流量拦截形态（v0.3 实测，E21）**：dae v2.1.1 通过 **netkit 设备对（`dae0` ↔ `dae0peer`，netns `daens`）** 拦截；`ens2`/`dae0`/`dae0peer` 有 clsact qdisc 但 `tc filter` 为空，**并非传统 tc-bpf 挂载**。卸载检查应针对 netkit 设备与 `daens` netns；断网风险按 netkit 生命周期评估。

**v0.5（E16）**：`dae suspend` **不摘除** `dae0`（保持 UP、仅转无负载态），故 suspend ≠ 摘钩子；停 dae 服务后 netkit 钩子才随之清理。

**OpenWrt 上的 dae 前提（判断，需实机确认）**：内核需 BPF 且多见于 x86_64 / aarch64；**MIPS 系路由器基本不可用**。故 A 档入口必须先做能力探测，不满足即降级 `observe` 并说明原因，而不是装完就崩。

### 4.5 配置事务与安全管理

所有 dae 配置变更统一经过以下流程：

1. 生成候选配置。
2. **订阅预检（v0.2 新增，v0.5 强化）**：对候选配置中每个订阅来源做 **URL 可达 + 内容可解析 + 节点数 > 0** 预检。理由：`dae validate` 不解析订阅、不联网取订阅（D1；E8 实证 validate 抓不到 404 订阅）；且订阅不可达 / 格式不支持时 dae 仅报 **WARN 非致命**（R3），以空组继续。**同时 `dae reload` 对解析出 0 节点的配置会直接拒绝应用（R1）**，故预检必须与 reload 的 0 节点拒绝语义对齐，避免“预检放过 → reload 拒绝”的错配。
3. 备份当前配置并记录摘要。
4. 展示差异，供用户确认。
5. 检查版本是否发生并发变化。
6. 使用 `dae validate -c <path>` 校验候选配置（退出码 + stdout 错误文本）。
7. 原子写入配置并应用：应用通过 `dae reload`（SIGUSR1）完成，**而非重启服务**；reload 内部轮询 `/var/run/dae.progress` 回报结果。注：dae 级**不设超时**（E15：3000 规则 + 真实节点 reload 仅 0.52s），真正的硬超时来自 systemd `TimeoutStartUSec`（本机 2min），面板应自设合理超时并提示。
8. 检查服务状态、日志与必要的连通性；reload 失败时以 `/var/run/dae.progress` 的内容作回滚判定依据（D10 / R4）：该文件**首字节是 ASCII 码点**（`'2'`=ReloadDone、`'3'`=ReloadError），换行后为消息文本 → 面板须按此格式解析 ReloadError 文本，比“看日志猜”可靠。
9. 成功后记录修订；失败时**写回旧文件并再次 `dae reload`** 完成回滚，并验证恢复结果（不是“重启服务”——**E13 已实证**：坏配置 reload 失败后 dae 仍 active、同 pid、旧配置继续跑，故“写回 + 再 reload”成立）。

> **仅当 reload 不可用**（例如改动了不支持热更的 `global` 字段）时，才降级为 restart；restart 会中断连接，需单独提示风险。

附加保护：
- 同一时间只允许一个配置应用任务。`dae reload` 自带互斥（`.progress` 的 ReloadBusy），面板不必自造锁，但必须把 **ReloadBusy 映射为“已有变更进行中”的 API 错误**（如 HTTP 409）。
- 校验、重载和健康检查必须有超时。
- `reload` / `suspend` 内部会 `AutoSu()`，非 root 调用可能卡在 sudo 密码提示，面板调用应显式加 `--disable-sudo` 并确保自身权限（D5）。
- **配置权限约束（v0.3 实测）**：`dae validate` 会主动拒绝 `0644` 配置（要求 `0600` / `0640`）。面板原子写入后必须 `chmod 0600`，否则 validate / apply 会失败。
- **独占写入域前提（v0.4）**：因面板取代 goose（见 §4.7、§4.8），`/etc/dae/config.dae` 与 `config.d/` 的写入域归面板独占，故“写回旧文件 + 再 reload”的回滚语义重新成立；**前提是 goose 已停用**。若检测到外部生成器仍在写同一文件，回滚不可信，应先禁止双写。
- **应急 suspend 可恢复性（v0.5，E16/R5）**：`dae suspend` rc=0 但**不摘 `dae0`**（保持 UP、转无负载态）；suspend 后若配置解析出 **0 节点**，`dae reload` 同样拒绝 → 会**卡在 no-load**。故应急 suspend 后必须用**带 ≥1 有效节点**的配置才能恢复；面板须对这一前提做保护（见 §11 验收）。
- **reload 目标进程（v0.5，E17）**：`--disable-pidfile` 启动时无 `/var/run/dae.pid`，`dae reload`（无 pid）报 `failed to read pid file` 失败，须 `dae reload <pid>`；面板若禁用 pidfile 必须自持 pid。
- 记录操作者、时间、变更摘要与结果，但不得记录敏感凭据。
- 管理链路可能受影响时，提供风险提示和本地恢复方案。
- 网络拓扑修改与 dae 配置事务分离，不能用简单文件回滚冒充整网恢复。

### 4.6 诊断与未来扩展

首版诊断范围：
- 核心状态、版本与启动错误。
- 内核与运行环境能力。
- DNS、接口绑定和基础路由检查。
- 配置校验错误和最近变更记录。

未来私网扩展：
- `SystemTailscale`：读取系统 Tailscale 客户端状态。
- `EmbeddedTSNet`：未来可选的 tsnet 实现。

首期只预留必要接口和数据模型，不引入 tsnet 依赖。应用私网接入、子网路由和互联网出口是不同能力，后续应分别设计。

### 4.7 配置所有权

依据核对表 C.2，dae 的 `include` 支持拆分配置：相对路径**相对入口配置文件所在目录**解析（不是 cwd），出于安全只允许包含入口配置目录下的文件，合并后按 section 名去重。据此明确配置所有权边界：

- 取代 goose 后，面板是 `/etc/dae/config.dae`（及其 `config.d/`）的**唯一生成者**，默认**只写 `config.d/panel-*.dae`** 分组文件（`panel-node.dae`、`panel-group.dae`、`panel-route.dae`、`panel-dns.dae` 等）。
- 面板**不覆盖、不重排**用户手写且仍需保留的内容；如用户仍要手改 `config.dae`，面板通过 `include` 段与其并存，而非重写整份文件。
- 入口 `config.dae` 的 `include { config.d/*.dae }` 段由面板在接管时写入（首次接管需用户确认），此后由面板维护。
- 面板只对自己的文件负责：§4.5 的“原子写入 + 回滚”范围限于 `panel-*.dae` 与面板维护的入口 `include` 段。

**落地路径（v0.4，取代 goose）**：include 隔离不是“目标形态”，而是**当前安装上要实际落地的路径**——先让 goose 退役，再切到面板驱动 `config.dae`。迁移步骤：

1. **导出 / 接管**：读取现有 `/etc/goose/config.yaml` 与其生成的 `/etc/dae/config.dae`，以及现有订阅（subs）与节点，导入面板模型。
2. **校验等价**：面板生成的首版配置与 goose 现有 `config.dae` 做差异/语义比对，`dae validate` 必须通过，且关键行为（routing / group / dns）等价。
3. **停 goose**：实测存在 `/etc/systemd/system/goose.service`（`Type=simple`，`ExecStart=/usr/local/bin/goose serve --port 5889`，goose 本身是一个 Web 面板／控制面），当前 **disabled + inactive**（vendor preset enabled，`is-enabled=disabled`）；迁移验证通过后 `systemctl disable --now goose` 并移除其它触发入口；无 timer、无 cron、无其它进程。保留一份只读备份（如 `/etc/dae/config.dae.goose.bak`）供追溯。
4. **面板接管**：面板成为唯一生成者，按本节写 `config.d/panel-*.dae` 并维护入口 `include` 段。
5. **验证无回退**：确认无外部生成器仍在写 `config.dae`（见 §11 禁止双写检测）；重载 dae 后行为正常、无回退。

这条把“面板 vs 用户自定义 / 旧生成器”的冲突从架构层面消除，也给配置事务划出清晰边界。
**迁移后实际配置布局（v0.7 实机落地）**

- 入口 `/etc/dae/config.dae`（权限 `0600`）：panel 头注释 + `include { config.d/*.dae }` + `subscription { CF: 'file://subs/CF.sub', dual: 'file://subs/dual.sub' }`。
- **关键决策：`subscription` 块放在【入口文件】内，而非 `config.d/*.dae`。** 因为 dae 的 `file://` 相对【入口配置文件所在目录】(`/etc/dae`) 解析（R2）；若把 `subscription` 放进 `config.d` 子目录，相对路径基准会随之改变 → 解析 0 节点 → `dae reload` 拒绝（R1）。此为 R2 / R1 的落地结论。
- `/etc/dae/config.d/panel-core.dae`（权限 `0600`）：`global` + `group` + `dns` + `routing` 主体。
- 说明：当前面板“生成”= **采用/重写**当前等价配置并落 ownership（真正从节点模型生成仍属聚合模块 P2 / P3，尚未建设）；迁移后配置所有权已从 goose 转到面板。

**回滚到 goose 单文件（可回滚）**：

```
cp /root/hnp/migration-m1/config.dae.orig /etc/dae/config.dae && chmod 600 /etc/dae/config.dae
rm -rf /etc/dae/config.d/
dae reload <pid>   # 验证 /var/run/dae.progress 为 ReloadDone（首字节码点 '2'）
```

### 4.8 取代既有第三方工具（goose）与双写风险（E25；v0.4 定稿取代）

本机 192.168.1.85 实测发现已安装第三方工具 **goose 0.5.18**（`/usr/local/bin/goose`、`/etc/goose/config.yaml`），且当前 `/etc/dae/config.dae` 正是 **goose 由 `config.yaml` 生成**的单文件配置（无 `config.d/`、无 `include`）。经用户确认：**goose 是 HomeNet Panel 的前身，面板要“取代”goose**——不是共存，也不是放任不管。

**决策（v0.4 定稿）：以面板取代并停用 goose。**

- **面板接管配置生成权 = 面板成为 `/etc/dae/config.dae` 的唯一生成者。** 迁移完成后停用 goose：`systemctl disable --now goose`，并移除其 timer / 触发钩子；如需追溯，保留一份只读备份（如 `/etc/dae/config.dae.goose.bak`、`/etc/goose/config.yaml` 只读留存）。
- **迁移步骤**见 §4.7（导出 / 接管 → 校验等价 → 停 goose → 面板接管 → 验证无回退）。
- **禁止双写检测**：面板启动时探测 goose（或其它进程 / timer）是否仍在写 `/etc/dae/config.dae` 或 `config.d/`；若检测到，拒绝进入可编辑态并告警（见 §11 硬性验收项）。
- **双写风险为事件驱动（v0.5 起，v0.7 更正）**：实测 goose 存在 `goose.service`（`Type=simple`，`serve --port 5889`），但当前 **disabled + inactive**、无 timer、无 cron、无其它进程；故双写不是常驻进程竞争，而是“有人手动 `goose serve` 或 `systemctl start goose` 即从 `/etc/goose/config.yaml` 重生成并覆盖入口 `config.dae`”。面板双写检测应覆盖“生成物 mtime / 内容被非面板进程改写”的事件式探测，而不仅查常驻进程。
- **双写冲突的本质**：只要旧生成器仍在跑，面板写入就会被下一次 goose 生成冲掉、回滚语义随之失效；因此“取代 + 停用”是回滚能成立（§4.5）与配置所有权能成立（§4.7）的共同前提。
- **迁移后残留风险（v0.7）**：M1–M4 完成后面板已成为唯一生成者，但 **goose 二进制与 `goose.service` 仍在**（未删除，保留可逆）。若有人手动 `goose serve` 或 `systemctl start goose`，会从 `/etc/goose/config.yaml` 重生成并覆盖入口 `config.dae`。因此 M5（观察期）建议停用 `goose.service` 并在备份后移除二进制（**不可逆，需用户确认**）。

---

## 5. 项目目录结构

```text
homenet-panel/
├── cmd/
│   └── homenet-panel/
│       └── main.go
├── internal/
│   ├── api/                 # REST API、请求校验
│   ├── auth/                # 本地鉴权与会话
│   ├── core/                # 核心管理接口与生命周期
│   ├── dae/                 # dae 专属适配器
│   ├── runtimecheck/        # 内核与运行环境检测
│   ├── subscriptions/       # 订阅获取、解析、更新
│   ├── nodes/               # 标准化节点、去重与过滤
│   ├── routing/             # 结构化分流规则模型
│   ├── compiler/            # 生成 dae 配置
│   ├── network/              # 接口、bridge、VLAN 模型
│   ├── configtx/             # 差异、校验、应用、回滚
│   ├── revisions/            # 备份和修订历史
│   ├── jobs/                 # 异步任务、超时、互斥
│   ├── diagnostics/           # 状态检查与诊断
│   ├── platform/
│   │   ├── linux/
│   │   └── openwrt/
│   └── privatenet/            # 未来 Tailscale / tsnet 抽象
├── web/
│   ├── src/
│   │   ├── api/
│   │   ├── components/
│   │   ├── layouts/
│   │   ├── views/
│   │   │   ├── Dashboard/
│   │   │   ├── Core/
│   │   │   ├── Subscriptions/
│   │   │   ├── Routing/
│   │   │   ├── Network/
│   │   │   └── Diagnostics/
│   │   ├── stores/
│   │   └── types/
│   └── package.json
├── docs/
│   ├── architecture.md
│   ├── api.md
│   ├── config-model.md
│   ├── platform-linux.md
│   ├── platform-openwrt.md
│   └── development.md
├── scripts/
├── tests/
├── Makefile
└── go.mod
```

### 模块依赖规则

- `subscriptions` 不直接操作系统服务。
- `routing` 不直接写配置文件。
- `compiler` 只负责生成配置，不负责重启 dae。
- `configtx` 负责安全变更流程。
- `dae` 负责核心特定行为。
- `platform` 负责系统差异。
- `privatenet` 首期不参与核心运行路径。

---

## 6. 核心数据模型

| 模型 | 职责 |
|---|---|
| `CoreInfo` | 核心版本、路径、架构与能力 |
| `RuntimeReport` | 内核、依赖和运行环境检测结果 |
| `SubscriptionSource` | 订阅来源、更新时间、启用状态 |
| `ProxyNode` | 标准化节点、来源、协议和属性 |
| `ProxyGroup` | 节点集合与策略组类型 |
| `RoutingRule` | 匹配条件、优先级和动作 |
| `GeneratedConfig` | 生成配置摘要、版本与校验结果 |
| `ConfigRevision` | 备份位置、前后摘要和变更结果 |
| `ApplyJob` | 任务阶段、进度、错误和最终结果 |
| `NetworkInterface` | 接口、地址、bridge、VLAN 关系 |

首期可以使用文件系统存储配置、订阅和修订记录。待查询需求和数据量明确后，再评估 SQLite。

敏感数据（订阅 URL、节点凭据、令牌）应限制读取权限；日志和 API 响应应脱敏；备份与导出文件也应受访问控制。

---

## 7. API 草案

统一使用 `/api/v1` 前缀。

| 模块 | 示例接口 | 用途 |
|---|---|---|
| 系统与环境 | `GET /system/info` | 系统和平台信息 |
| 能力检测 | `GET /system/capabilities` | 内核与运行能力报告 |
| 核心管理 | `GET /core/status` | 核心运行状态 |
| 核心升级 | `POST /core/upgrade` | 受控升级任务 |
| 订阅管理 | `GET /subscriptions` | 订阅来源列表 |
| 订阅刷新 | `POST /subscriptions/{id}/refresh` | 刷新指定来源 |
| 节点管理 | `GET /nodes` | 标准化节点列表 |
| 分流规则 | `GET /routing/rules` | 获取规则模型 |
| 分流规则保存 | `PUT /routing/rules` | 保存结构化规则 |
| 配置预览 | `POST /config/preview` | 生成候选配置和差异 |
| 配置校验 | `POST /config/validate` | 校验候选配置 |
| 配置应用 | `POST /config/apply` | 创建应用任务 |
| 修订列表 | `GET /revisions` | 获取修订历史 |
| 修订恢复 | `POST /revisions/{id}/restore` | 恢复指定修订 |
| 网络接口 | `GET /network/interfaces` | 获取接口与拓扑信息 |
| 诊断报告 | `GET /diagnostics/report` | 获取诊断结果 |
| 任务状态 | `GET /jobs/{id}` | 查询后台任务 |

订阅刷新、核心升级和配置应用等耗时操作可以返回任务 ID，由前端轮询结果。所有 API 都应校验输入、执行鉴权和权限检查；禁止暴露任意 Shell 执行入口。

---

## 8. 开发路线图

### P0：核心管理与运行环境

首先在现有 Armbian ARM 设备上验证。

- 检测现有 dae 官方二进制、配置路径和服务方式。
- 完成内核与运行环境检测。
- 实现配置读取、校验、备份、应用和回滚。
- 建立核心管理接口与 Linux 适配器。
- 建立模拟平台和失败路径测试。

**交付标准：** 可以安全管理现有 dae，变更失败时能够恢复旧配置并检查服务状态。

### P1：聚合订阅中心

- 多来源订阅与手动节点管理。
- 标准化、去重、重命名、标签和过滤。
- 基础策略组编排。
- 订阅刷新前后差异预览。
- 生成配置预览与校验。

**交付标准：** 完成节点与订阅的统一管理闭环。

### P2：精准分流与 Web 面板

- 结构化规则模型与 dae 配置编译器。
- 规则优先级、模板和冲突检查。
- 完成仪表盘、订阅中心和分流编辑器。
- 统一配置应用与回滚入口。
- 保留高级原始配置的兼容路径。

**交付标准：** 用户可以主要通过 Web UI 完成日常订阅和分流配置。

### P3：网络适配与 OpenWrt

- 展示网卡、bridge、VLAN 和路由关系。
- 实现 OpenWrt procd、UCI、ubus 适配。
- 检查 dae 处理范围和网络隔离边界。
- 在独立测试设备上验证安装、升级和恢复。

**交付标准：** 具备 Linux / Armbian 与 OpenWrt 两类平台适配能力，不未经确认修改网络拓扑。

### P4：扩展能力

- Tailscale 系统客户端状态与连通性诊断。
- 更完善的节点连通性和运行诊断。
- 根据实际需求评估可选 tsnet 后端。
- 让私网扩展不影响基础 dae 管理功能。
- 模式 B 自建聚合订阅中心（多格式解析含 Clash YAML、跨来源去重/重命名）：E9 已实证 dae 原生不支持 Clash YAML，本项正式立项（见 §4.2）。

**交付标准：** 未启用 Tailscale 或 tsnet 时，基础功能仍可独立运行。

### goose→面板 迁移里程碑

依据 §4.7 五步落地路径拆为 M1–M5，与已有实机定论咬合。每个里程碑给出目标、交付物、可勾选验收标准、失败回退与依赖/风险；里程碑间的 gating 与回滚标注见本节末「里程碑门禁与回滚」。

**M1：导出 / 接管（全量快照）** — ✅ 已完成（v0.7 实机）
- 目标：完整快照现有 goose 生成链与配置，作为后续可回滚基线。
- 交付物：`/etc/goose/config.yaml` 与 `/etc/dae/config.dae` 只读备份（如 `config.dae.goose.bak`）、现有 subs 与节点导入面板模型、`dae export outline` JSON 存档。
- 验收标准：
  - [x] 原 `config.dae` 与 `config.yaml` 均已只读备份
  - [x] 订阅与节点已全量导入面板模型，无静默丢失
  - [x] `dae export outline` 已存档（供 M2 diff）
- 失败回退：本步不改动运行配置，失败仅清理导入数据，**可回滚**。
- 依赖/风险：R2 本地订阅为相对路径，需一并记录目录布局；风险—导入解析遗漏。
- **实机完成证据（v0.7）**：`/root/hnp/migration-m1/` 下 `config.dae.orig`（md5 `8486e911bf0c94d6e38b65aaf19ba1bb`）、`goose-config.yaml.bak`、`subs/`、`goose.service.bak`、`outline.before.json`（20066B）。

**M2：校验等价** — ✅ 已完成（v0.7 实机）
- 目标：证明面板生成的首版配置与 goose 现网等价。
- 交付物：面板生成首版 `config.d/panel-*.dae` + `include` 段；差异/语义比对报告。
- 验收标准：
  - [x] `dae export outline` 与 M1 存档 diff 通过（字段存在性 + 版本兼容，对应 §4.3 outline 校验关卡）
  - [x] `dae validate -c` 通过，且配置权限为 `0600`（dae 拒绝 `0644`）
  - [x] 订阅预检：URL 可达 + 可解析 + 节点数 > 0，并对齐 reload 的 0 节点拒绝（R1）
  - [x] 用真实 subs 成功执行一次 `dae reload`（**非仅 `dae run`**），`/var/run/dae.progress` 为 ReloadDone
  - [x] routing / group / dns 关键行为与 goose 现网等价
- 失败回退：仍使用 goose 现网配置，丢弃候选配置，**可回滚**。
- 依赖/风险：依赖 M1 全量导出；风险—0 节点导致 reload 拒绝（R1 / R3，见 §4.5）。
- **实机完成证据（v0.7）**：面板生成 `include` 结构后 `dae validate` 通过；`outline.before == outline.after` 逐字节一致（均 20066B）；真实 `dae reload`（非 run）成功，`/var/run/dae.progress` = ReloadDone（码点 `'2'`）。

**M3：停 goose** — ✅ 已完成（v0.7 实机）
- 目标：令面板成为唯一生成者前，先让旧生成器退役。
- 交付物：goose 停用记录 + 只读备份 + 双写检测结论。
- 验收标准：
  - [x] 实测存在 `/etc/systemd/system/goose.service`（`Type=simple`，`ExecStart=/usr/local/bin/goose serve --port 5889`），当前 **disabled + inactive**：`systemctl disable --now goose` 已确认生效 + 移除其它入口（shell 别名、脚本、GUI / 手动触发点）；无 timer、无 cron、无其它进程
  - [x] 保留 goose `config.yaml` 与历史 `config.dae` 只读备份
  - [x] 以 mtime / 内容式双写检测确认观察期内无外部写入者（见 §4.8）
- 失败回退：删除旧生成链属**不可逆**，须在 M1 备份完成后执行；若有 unit 可先 `systemctl enable` 恢复入口。
- 依赖/风险：**gating—M2 必须通过才可进 M3**；风险—遗漏隐藏触发点导致后续被覆盖。
- **实机完成证据（v0.7）**：确认 `goose.service` disabled+inactive、无进程、无 timer、无 cron；**未删除** unit / 二进制（可逆）。

**M4：面板接管** — ✅ 已完成（v0.7 实机）
- 目标：面板成为 `/etc/dae/config.dae` 唯一生成者并稳定写入。
- 交付物：面板写入的 `config.d/panel-*.dae` + 面板维护的 `include` 段。
- 验收标准：
  - [x] 仅写 `panel-*.dae`，不覆盖、不重排用户手写内容
  - [x] 入口 `include` 段由面板维护，本地订阅用相对路径 `file://`（R2）
  - [x] 写入后 `chmod 0600`
  - [x] 应用走 `dae reload` 成功，回滚语义成立（见 §4.5）
- 失败回退：写回 M1 备份配置 + 再 reload，**可回滚**（前提 M3 已完成、无外部写入者）。
- 依赖/风险：**gating—M1 全量导出完成方可回滚；M3 未完成不得进 M4**；风险—双写冲突。
- **实机完成证据（v0.7）**：面板成为 `/etc/dae/config.dae` 唯一生成者；preflight 对新配置 `blocking=false`；不带 `--allow-goose` 再次 apply 成功（reload ReloadDone、服务 active、pid 不变）。

**M5：验证无回退（观察期）**
- 目标：确认迁移后长期稳定、无回退。
- 交付物：观察期记录（mtime / 内容、reload 结果、连通性）。
- 验收标准：
  - [ ] 观察期内 `config.dae` mtime / 内容无外部改动（无外部写入者）
  - [ ] 多次 `dae reload` 后行为等价
  - [ ] 断网 / 直连行为正常，按 **netkit**（`dae0` ↔ `dae0peer`、`daens` netns）评估（E21）
  - [ ] 应急 suspend 可用带有效节点的配置恢复（见 §11）
- 失败回退：回写 M1 备份 + reload，**可回滚**。
- 依赖/风险：依赖 M4 完成；风险—netkit 钩子异常致断网。

**里程碑门禁与回滚**
- 顺序强约束：M1 → M2 → M3 → M4 → M5；**M3 未完成不得进 M4**；**M4 前必须完成 M1 全量导出以可回滚**。
- 可回滚：M1、M2、M4、M5（写回备份 + 再 reload，前提无外部写入者）；**不可逆：M3 停用 / 删除旧生成链**——须先完成 M1 备份、且 M2 通过后才执行。
- 交叉引用：回滚细节见 §4.5；双写检测见 §4.8；对应硬性验收见 §11（禁止双写、启动/重载 0 节点保护、应急 suspend 可恢复性）。

---

## 9. 测试与发布

### 测试门槛

- **单元测试**：规则模型、配置编译、差异计算、版本冲突和回滚决策。
- **模拟测试**：校验失败、服务启动失败、命令超时和权限不足。
- **Linux 集成测试**：Armbian ARM 实机、服务状态、文件权限。
- **OpenWrt 集成测试**：procd、UCI、接口发现与目标设备架构。
- **安全测试**：命令注入、未授权 API、敏感日志和路径穿越。
- **发布测试**：针对目标设备构建并验证安装、升级、卸载与恢复。

### 发布包建议

- Go 后端可执行文件。
- 前端静态资源。
- 安装、升级和卸载脚本。
- 默认配置及备份恢复说明。
- 支持平台、架构与已知限制。
- 版本号、构建信息与校验和。

不能只凭成功编译就宣称支持某个 OpenWrt 机型；应记录测试设备的架构、内核、闪存、内存和服务环境。

---

## 10. 安全原则与主要风险

| 风险 | 对策 |
|---|---|
| dae 配置语法随版本变化 | 按目标版本校验，不硬编码未经验证的语法 |
| OpenWrt 网络修改造成断网 | 首期只读展示拓扑；网络修改单独设计 |
| ARM 设备资源有限 | 单 Go 服务、静态前端，不引入重型服务 |
| 订阅与凭据泄露 | 敏感字段脱敏、严格文件权限、控制导出 |
| 管理接口暴露到公网 | 默认限制监听地址，启用鉴权，避免直接暴露 WAN |
| 平台行为不一致 | 使用平台适配器并进行实机集成测试 |
| 自动回滚仍无法恢复连接 | 提供本地恢复方式，网络拓扑修改另设恢复保护 |
| 并发修改造成覆盖 | 使用版本摘要、互斥锁和变更任务 |
| goose 残留可被手动启动覆盖（v0.7） | 面板为唯一生成者，但 goose 二进制 / `goose.service` 仍在（disabled）；M5 观察期停用并按需移除二进制（不可逆，需确认），双写检测识别 goose 头 / 进程 / 其它写者（见 §4.8、§11） |

---

## 11. 首版验收清单

- [ ] 能在 Armbian ARM 上发现并管理现有 dae 官方二进制。
- [ ] 能检测关键内核能力并解释失败原因。
- [ ] 能管理多个订阅来源并完成基础节点聚合。
- [ ] 能通过结构化规则生成 dae 配置。
- [ ] 配置应用前可以查看差异并完成校验。
- [ ] 配置失败后可以恢复旧配置并检查核心状态。
- [ ] 能展示网络接口及已知 bridge / VLAN 关系。
- [ ] 不会未经确认修改 OpenWrt 的 DHCP、VLAN 或防火墙配置。
- [ ] Web API 有鉴权，敏感信息不会泄露到普通日志。
- [ ] 核心业务逻辑不依赖 Tailscale 或 tsnet。
- [ ] **禁止双写检测（硬性）**：启动时若检测到 goose 或其它进程 / timer 仍在写 `/etc/dae/config.dae` 或 `config.d/`，则拒绝进入可编辑态并告警。（v0.7 现状：迁移后 preflight 实测已能识别 goose 头 / goose 进程 / 其它写者；当前新配置无 goose 头 → 不阻断；但 goose 二进制仍在 = 潜在风险已登记，见 §4.8、§10。）
- [ ] 已探测是否存在 daed 等第三方 Web UI 与面板写同一份 `config.dae` / `config.d`，并纳入双写检测。
- [ ] **启动 / 重载 0 节点保护（硬性）**：订阅预检须拦截“节点数为 0”；并对 `dae reload` 的 0 节点拒绝语义（R1）做处理——启动（run）容忍 0 节点但重载严格，面板不得因启动成功误判“可用”。
- [ ] **应急 suspend 可恢复性（硬性）**：suspend 后能用带 ≥1 有效节点的配置 `dae reload` 恢复；若配置解析为 0 节点则拒绝恢复（卡 no-load），面板须检测并提示。
- [ ] 具备自动化测试、安装文档、升级和恢复说明。

---

## 12. 最终开发优先级

**核心与驱动管理 → 聚合订阅 → 精准分流 → 网络适配 → Tailscale / tsnet 扩展**

项目首个可用版本应优先在现有 Armbian ARM 旁路由上完成 dae 管理闭环。订阅聚合与精准分流是主要产品价值；跨平台适配与安全配置事务则为长期维护提供基础。

---

## 13. 已确认的能力边界与坑（dae v2.1.1 源码核对）

> 本节把 `dae-cli-capability-checklist.md` D 节 12 条逐条落地。原为**源码核对**结论；v0.3 起分批在 192.168.1.85（dae v2.1.1）实机复核，**v0.5 起 E1–E26 全部跑完**，相关行以“实测”标注并见本节末「实机验证补充 / v0.5 定论补充」。每条都注明对架构的影响。

| # | 已确认事实（dae v2.1.1 源码核对） | 对架构的影响 |
|---|---|---|
| D1 | `dae validate` **不解析订阅、不联网取订阅**；订阅解析发生在运行期（构建控制面时） | §4.5「校验」对订阅类错误是盲区 → 面板必须自建订阅预取 + 解析预检（落地为 §4.5 第 2 步） |
| D2 | `dae validate` 覆盖 routing 规则（复用运行期同一套 optimizer 链：geoip / geosite 展开、别名解析、规则合并排序、outbound 存在性检查）+ dns routing + `fixed_domain_ttl` | 校验质量高于预期，可放心作为事务第 6 步；但不覆盖 BPF 层与全局参数的语义正确性 |
| D3 | （实测修正）`validate` 需读 geo 数据（`geoip.dat` / `geosite.dat`），按**搜索路径列表**定位：配置目录 → `~/.local/share/dae` → `/usr/local/share/dae` → `/usr/share/dae`；缺 geo 报 `geosite.dat: file does not exist in [...]` | 首次离线校验可能因缺 geo 失败 → 面板按路径逐项核对，并把该报错归入“缺 geo 资源”分类，提示先下载 geo（见 §4.1） |
| D4 | （E18 实测坐实）守护进程**忽略 SIGHUP** | 不能用 `systemctl reload`（默认 SIGHUP）或 `kill -HUP`，必须调用 `dae reload`（SIGUSR1）（见 §4.5、§2.4） |
| D5 | `reload` / `suspend` / `trace` 内部会 `AutoSu()` | 面板非 root 运行时可能卡在 sudo 密码提示 → 显式加 `--disable-sudo`，并保证自身已具备权限（见 §4.1、§4.5） |
| D6 | `suspend` 是**真正的无负载态**，`dae reload` 恢复 | 可做成面板应急开关，比“停服务”更平滑（不动服务、不做 teardown）（见 §4.1） |
| D7 | `sysdump` 输出到**当前工作目录** | 面板调用时必须把 cwd 指定为临时目录，否则污染 `/` 或 `/etc/dae`（见 §2.4、§4.1） |
| D8 | （实测修正）`dae` 顶层**确有** `trace` 与 `completion` 子命令（源码核对“默认没有”不成立）；源码里 `trace` 以 build tag `trace` 条件编译，本机 v2.1.1 构建**已包含且可用** | 诊断模块可直接包装 `dae trace` 做单域名/进程链路追踪，无需自研（见 §4.6） |
| D9 | `dae validate` **不需要 root**（实测以 nobody 运行合法配置退出 0），但受**文件读权限**约束；且主动**拒绝 0644** 配置，要求 0600/0640 | 面板写盘后必须先 `chmod 0600` 再校验，否则校验必失败（见 §4.5，已落地于 configtx.writeAtomic） |
| D10 | 退出码：合法 0 / 非法 1 / 缺 `-c` 也是 1（stdout `Argument "--config" or "-c" is required`） | 不能只靠退出码判定，需同时读 stderr/stdout 文本分类（见 §4.5 第 3 步） |
| D11 | 坏语法报错带**行列号 + 插入符**（例 `line 53:21 ... mismatched input '{' expecting '}'`） | 可直接把原始报错回显给用户，无需二次解释（见 §4.5） |
| D12 | 不存在的出站报 `outbound (group) not found`；routing 出现未知键报 `failed to parse routing: unexpected key: p` | 面板在渲染阶段就做 outbound 名存在性预检（前端已对非法出站标红，见 §14.11） |

### 13.1 实机验证补充 / v0.5 定论补充（dae v2.1.1 @192.168.1.85）

E1–E26 已全部在实机跑完，逐项结果见 `dae-cli-capability-checklist.md` 的「E 节实测附录」。影响架构的关键定论：

- **E9（Clash YAML 不支持）**：配置里写 Clash 风格订阅时，dae 只打印 WARN `subscription resolved to 0 nodes` 并继续以空组建控制面 → 面板的订阅模式必须二选一：**模式A**（托管 dae 原生订阅格式，首发默认）/ **模式B**（面板自建聚合，只产出 dae 原生 node 列表）。
- **R1（启动宽松 / 重载严格，非对称）**：`dae run` 容忍 0 节点继续跑，但 `dae reload` 会**拒绝应用** 0 节点配置（`refusing reload with 0 nodes: all subscription resolving failed; keeping the current generation`，progress=3）。面板不得因“启动成功”就判定配置可用。
- **R2（`file://` 只支持相对路径）**：绝对路径报 `not support absolute path`（WARN、0 节点）。相对路径的基准是**入口配置文件所在目录**（`/etc/dae`），因此 subscription 块必须写在入口文件里、不能下沉到 `config.d/`（详见 §14.3 与 §8 迁移里程碑）。
- **R3**：订阅不可达 / 格式不支持只是 WARN，不是致命错误，dae 会以空组继续 → 面板必须自己预检（落地为 preflight 的 CheckSubscriptions）。
- **R4**：`/var/run/dae.progress` 首字节是 **ASCII 码点**（`'2'`=ReloadDone=50、`'3'`=ReloadError=51、`'4'`=SuspendDone=52），换行之后才是消息文本。
- **R5**：suspend 之后若配置解析为 0 节点，`dae reload` 同样拒绝恢复 → 会卡在 no-load，面板须检测并提示。
- **E15**：3000 条规则 + 真实节点 reload 仅 0.52s，dae 级无超时；真正的硬超时是 systemd 的 `TimeoutStartUSec=2min`。
- **E17**：`--disable-pidfile` 下没有 `/var/run/dae.pid`，此时 `dae reload`（不带参数）报 `failed to read pid file`，必须显式 `dae reload <pid>`。
- **E20**：memlock soft=hard=8388608 字节（8192KB，非 unlimited）、`unprivileged_bpf_disabled=2`、内核 6.12.113+deb13-amd64、BTF=true。
- **E21**：拦截走 **netkit 设备对**（`dae0` ↔ `dae0peer`，netns `daens`），不是 tc-bpf；只有**停服务**才清钩子（suspend 不摘钩子，只转 no-load）。
- **E25 / v0.7 更正**：goose 0.5.18 曾在 `/usr/local/bin/goose` + `/etc/goose/config.yaml`，且**确实存在** `goose.service`（Type=simple，`goose serve --port 5889`，实测 disabled+inactive）；本机无 systemd timer / 无 cron，goose 为事件驱动式重写 `config.dae` → 双写风险是“事件驱动”而非“定时”。M3/M4 已把 goose 停用并移走二进制（可逆，见 §8）。
- **E26**：unit 为 `Type=notify`、`ExecStartPre=dae validate`、`ExecReload=dae reload MAINPID`、`Restart=on-abnormal`、StartLimit 5 次/5min（反复 stop/start 会触发限流，需 `systemctl reset-failed dae.service` 复位）。

---

## 14. 实现现状与 API

> 本节记录**已经跑通并部署到 192.168.1.85** 的实现，与前面“设计”章节区分：设计在 §1–§13，这里是“现在实际有什么”。

### 14.1 能力清单（实现状态）

| 模块 | 已实现能力 | 状态 |
|---|---|---|
| 核心与驱动管理 | dae 探测（路径/版本/服务状态/PID）、runtimecheck（内核/BTF/memlock/unprivileged_bpf）、status、outline 导出 | ✅ 实机验证 |
| 配置事务与安全 | 预检（goose 头/进程/其它写者/订阅）、写 0600、validate、reload、**失败自动回滚（写回 + 再 reload）**、备份与手动 rollback、变更串行化 | ✅ 实机验证（含坏配置回滚 MD5 还原） |
| 内容层（结构化模型） | live 配置 ⇄ JSON 模型双向（Parse/Render）、订阅/分组/规则的增删改、preview 预览、事务式 apply（多文件一次 reload） | ✅ 实机验证（幂等 MD5 不变） |
| 聚合订阅中心 | 订阅解析（base64/明文、vless/vmess/trojan/ss）、节点列表、分组 filter 本地求值、TCP 延迟测速、模式B 自建聚合并**一键安全切换**（可 dryRun、可回滚） | ✅ 实机验证 |
| 精准分流 | geosite.dat 解码 + 规则本地求值（三态：命中/不命中/未知）、`/api/route/test` 分流命中调试器、规则可视化编辑（排序/出站联表/未保存模型试算） | ✅ 实机验证 |
| 网络适配 | 流量/命中统计：③ DNS 旁路查询频度（AF_PACKET 只读）+ ② eBPF 源预留接口 | ✅ ③ 实机验证；② 预留 |
| 诊断扩展 | 前端 7 个标签页单页控制台、HTTPS、账号密码登录与会话 | ✅ 实机验证 |

Go 代码规模：约 4833 行（含测试），纯标准库；前端为内嵌单文件 Vue 3（`internal/api/index.html`，约 32KB）。

### 14.2 进程部署（实机现状）

- 二进制：`/usr/local/bin/homenet-panel`（755），unit 文件 `/etc/systemd/system/homenet-panel.service`。
- ExecStart：`homenet-panel serve --addr 0.0.0.0:8787 --backup-dir /var/lib/homenet-panel/backups --tls-cert /etc/homenet-panel/tls.crt --tls-key /etc/homenet-panel/tls.key --traffic-iface ens2`；`After=network-online.target dae.service`、`Restart=on-failure`、`RestartSec=3`、`WantedBy=multi-user.target`。
- 凭据：`/etc/homenet-panel.auth.json`（0600，加盐迭代 SHA-256，120000 轮）；改密码 `homenet-panel setpass --user admin --password <新> --auth-path /etc/homenet-panel.auth.json`（读时校验，无需重启）。
- TLS：自签证书 `/etc/homenet-panel/tls.crt` / `tls.key`（SAN: IP 192.168.1.85 / 127.0.0.1 / DNS localhost），首次访问需手动信任；8787 端口不再提供明文 HTTP。
- dae 侧写权限范围：仅 `/etc/dae/config.dae` 与 `/etc/dae/config.d/panel-*.dae`；备份落在 `/var/lib/homenet-panel/backups/`；结构化模型快照在 `/var/lib/homenet-panel/model.json`。
- CLI 子命令：`detect / runtimecheck / status / preflight / validate / apply / rollback / outline / suspend / setpass / serve`。

### 14.3 内容层数据流与所有权

1. `GET /api/model`：优先读 `/var/lib/homenet-panel/model.json`（source=file），否则从 live 配置 `Parse` 出来（source=live）。
2. 前端编辑 → `POST /api/model/preview` 得到渲染后的 entry/core 文本（只预览、不写盘）。
3. `POST /api/model/apply` → `Render` → `configtx.ApplyMulti`：逐份备份 → 写 0600 → 校验入口 → **只 reload 一次** → 任一步失败则全部回滚。成功后保存 `model.json`。
4. 落盘布局（迁移后，M4 定稿）：
   - 入口 `/etc/dae/config.dae`（0600，约 181B）= panel 头注释 + `include { config.d/*.dae }` + `subscription { CF: 'file://subs/CF.sub', dual: 'file://subs/dual.sub' }`；
   - 主体 `/etc/dae/config.d/panel-core.dae`（0600）= global + group + dns + routing。
   - **subscription 必须在入口文件**：`file://` 的相对基准是入口配置所在目录（`/etc/dae`），下沉到 `config.d/` 会改变基准 → 0 节点 → reload 被拒（R2/R1）。
5. 所有权：面板只写 `panel-*` 命名空间；文件头带 `# 由 HomeNet Panel 生成并托管（panel-managed），请勿手改` 标记，作为识别依据。

### 14.4 REST API（除标注“公共”外全部需登录）

鉴权：会话 Cookie `hnp_session`（HttpOnly，TLS 下带 Secure，SameSite=Lax，24h）；也支持静态 Bearer（`--token`，留空则纯账号登录）。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/` | 公共：单页控制台 |
| GET | `/assets/vue.js` | 公共：本地化 Vue 运行时 |
| POST | `/api/login` | 公共：`{username,password}` → Set-Cookie |
| POST | `/api/logout` | 注销 |
| GET | `/api/me` | 当前用户 |
| GET | `/api/health` `/api/status` `/api/detect` `/api/runtimecheck` `/api/outline` | 只读探测 |
| GET | `/api/preflight?path=` | 双写/订阅预检 |
| POST | `/api/validate` | `{path}` → 校验结果 |
| POST | `/api/apply` | `{target,content,allowGoose}` → 事务应用（失败自动回滚） |
| POST | `/api/rollback` | `{target}` → 用最近备份回滚 |
| POST | `/api/suspend` | 应急 suspend（no-load） |
| GET | `/api/model` | 结构化模型（source=file 或 live） |
| POST | `/api/model/preview` `/api/model/apply` | 预览渲染 / 事务应用模型 |
| GET | `/api/subs/nodes` `/api/subs/groups` | 订阅节点、分组命中计数 |
| POST | `/api/subs/speedtest` | `{nodes,timeoutMs}` TCP 建连延迟（默认 3000ms、并发 30） |
| GET | `/api/aggregate/preview` `/api/aggregate/build` | 模式B 聚合预览 / 生成 merged.sub |
| POST | `/api/aggregate/switch` | 模式B 一键安全切换（`dryRun` / `confirm`） |
| POST | `/api/route/test` | `{domain, model?}` 分流命中调试 |
| GET | `/api/traffic/hits` `/api/traffic/sources` | 流量/命中统计与采集源状态（§14.12） |

### 14.5 运行时订阅格式事实（实机）

- dae **只吃原生格式**：`subscription { name: 'file://相对路径' }` 或 http(s) URL，内容为 **base64 编码的节点链接列表**（或明文链接，逐行）；支持 vless / vmess / trojan / ss。
- **不支持 Clash / Clash.Meta YAML**（E9：静默 0 节点）。
- `file://` 仅支持**相对入口配置目录**的路径（R2）。
- 面板的节点解析（`internal/subs`）自行实现：明文与 base64 自动识别（试 Std/RawStd/URL/RawURL），链接手动切分 `#tag` 与 `?query`，vmess 走 base64 JSON。
- 实机样本：`subs/CF.sub` = 1 节点，`subs/dual.sub` = 29 节点（28 vless + 1 vmess），tag 含国旗 emoji。

### 14.6 前端本地化 / 测速 / 分组命中 / 模式B 约束

- 前端**完全本地化**：Vue 3.4.21 `vue.global.prod.js`（147534B）以 `//go:embed` 打进二进制，由 `/assets/vue.js` 提供，不依赖 CDN（断网也能开控制台——这对“代理机被墙”场景是刚需）。
- 分组命中：`MatchGroup` 在本地复刻 dae group filter 语义（多行 OR、行内 `&&` / `||`、括号；谓词 `subtag(x)`、`name(keyword|regex|full|prefix|suffix:...)`），因此可离线算出“每个分组命中几个节点”。
- 测速：TCP 建连 RTT（不是真实下载速度），默认 3s 超时、并发上限 30。
- 模式B 约束：分组 filter 若用 `subtag(dual)` / `subtag(CF)`，把订阅整体换成单一 `panel-merged.sub` 会令这些分组命中 0 → 因此“生成 merged”与“切换订阅”必须分离；切换时用 `RewriteSubtags` 把 filter 里的 subtag 名一并改写（见 §14.8）。

### 14.7 前端本地化方案取舍分析（v0.10）

**优势**：① 断网可用（核心，代理机被墙时仍能管理）；② 部署极简（一个二进制 = 后端 + 前端 + 静态资源，回滚=换二进制）；③ 版本钉死可复现，免受 CDN 投毒；④ 隐私（无外部请求）；⑤ 体积代价可忽略（+147KB）。

**劣势**：① 失去工程化（无 SFC / ESM / HMR / TS / tree-shaking / 代码分割）；② 全量加载；③ 手工模板转义坑（HTML 内单引号需写 `&#39;`，Go 字符串转义易踩）；④ 升级靠手动；⑤ 改一行 UI 需重编译 + 重启（运行中二进制会被 `text file busy` 挡住，必须先 stop）；⑥ 单文件模板到 3000~5000 行是维护天花板。

**结论**：当前形态是正确的（核心矛盾是“断网也能管代理”）。到单文件 3000~5000 行规模时，迁移到 **Vite + Vue SFC 构建出 dist，再 `go:embed` 打进二进制**，既恢复工程化又保留“单二进制”优势。

### 14.8 模式B 一键安全切换（v0.10）

- 请求：`POST /api/aggregate/switch`，body `{name?, path?, dryRun?, confirm?}`。
- 步骤：① 解析所有订阅并 `Aggregate` 去重（0 节点 → 422）；② 校验 merged 文件必须落在配置目录内，渲染为 `file://<相对路径>`（R2）；③ 取旧订阅名，对每个 group 的 filter 做 `model.RewriteSubtags`（`subtag(old...)` → `subtag(merged)`），记录 groupChanges；④ 渲染新 entry/core；⑤ 未 confirm 只返回预览；confirm 则写 merged.sub（base64，0600）→ `ApplyMulti` 事务应用 → 成功保存模型。
- 实机验证：切换后 applied=true、reload OK（progress=ReloadDone）、分组命中仍 hk=7 / proxy=30 / sg=2（subtag 已改写为 merged）；随后用原模型 `/api/model/apply` 精确回退（MD5 恢复）。**切换原子、可回滚。**
- 运维提示：`panel-merged.sub` 是**快照**，上游订阅更新后需重新生成并重切。

### 14.9 HTTPS / TLS（v0.11）

- `serve` 支持 `--tls-cert` / `--tls-key`，两者都给才启用；启用后 8787 端口**不再提供明文 HTTP**（发明文请求 → 400）。
- 登录 Cookie 在 TLS 下带 `Secure`；自签证书首次访问需手动信任（浏览器警告属预期）。
- 实机：`openssl req -x509 ... -addext "subjectAltName=IP:192.168.1.85,IP:127.0.0.1,DNS:localhost"`，825 天，0600。

### 14.10 分流命中调试器（v0.12）

- `internal/geo`：手写最小 protobuf 解码 `/usr/share/dae/geosite.dat`（GeoSiteList → GeoSite{code,domain[]} → Domain{type,value}；type 0=Plain 子串 / 1=Regex / 2=Domain 后缀 / 3=Full 精确）。
- `internal/route`：三态求值 —— 命中 / 不命中 / **未知**。地址类谓词（`ip` / `sip` / `dip` / `ipset`）标“未知”，上下文谓词（`pname` / `pid` / `uid` / `cgroup` / `mac` / `l4proto`…）标“上下文”，避免把“算不出来”谎报成“不命中”。`domain(...)` 支持逗号多参数 OR。
- 端点 `POST /api/route/test {domain, model?}`：带 `model` 时对“正在编辑、未保存”的规则试算；返回逐条结果、fallback、chosen、是否已决定及理由。
- 实机：www.youtube.com→hk、www.baidu.com→direct、api.anthropic.com→sg、github.com→proxy（fallback，因存在 ip 规则而为“未决”）。

### 14.11 分流规则可视化编辑增强（v0.12）

- 规则表加序号列与 ↑/↓ 排序（routing **首条命中即生效**，顺序敏感）；出站用 `<datalist>` 联表（groups + `direct` + `block`），非法出站标红；「用当前未保存规则试算」并高亮命中的那一行。
- 已清理 live 中迁移残留的 `pname(goose) -> direct` 规则。清理后 live 规则 4 条：`domain(geosite:youtube)->hk`、`domain(suffix: claude.ai, suffix: anthropic.com)->sg`、`domain(geosite:cn, geolocation-cn)->direct`、`ip(geoip:cn)->direct`；fallback = proxy。

### 14.12 流量 / 命中统计（DNS 旁路，v0.13 新增）

**目标与取舍**：先做 ③ **DNS 层查询频度统计**（只读旁路、不碰数据面、不改 dae 配置），同时预留 ② **eBPF 直读 dae map** 的可插拔接口。字节级计量（真实吞吐）属于数据面插桩，本期不做。

**实现（`internal/traffic`）**：
- 可插拔接口：`type Source interface { Name() string; Snapshot(limit int) Snapshot; Close() error }`；`Sources(dnsReady, dnsDetail)` 返回采集源清单（`dns-capture` ready/disabled、`ebpf-dae-maps` **reserved**）。
- ③ DNS 源（linux）：`AF_PACKET + SOCK_RAW + htons(ETH_P_ALL)` 绑定业务网卡（`ens2`），内核挂 10 条 cBPF 指令只放行「以太网 IPv4 + UDP + src/dst 端口 53」，数据面流量不进用户态；用户态批量 drain 后睡 100ms。
  - 踩坑记录：`AF_PACKET + SOCK_DGRAM + ETH_P_IP` 只能看到 DNS **响应**看不到查询（不可用）；必须 `SOCK_RAW + ETH_P_ALL`（查询表现为 `PACKET_OUTGOING`）。go1.27 无 `SetsockoptSockFprog`，用 `syscall.AttachLsf` + `LsfStmt/LsfJump`。`EAGAIN` 与 `EWOULDBLOCK` 同值，switch 双 case 会编译报错。
- 非 linux 平台提供 stub，返回“仅支持 linux”。
- 归因：每个域名用 live 模型跑 `route.Test` 补上 `out` / `decided`，再按出口汇总（未决的归并到 `<出口>(含未决)`）。

**端点**：`GET /api/traffic/hits?limit=100` → `{available,source,iface,startedAt,updatedAt,queries,domains,hits[{domain,count,lastSeen,out,decided}],groups,geoLoaded,sources}`；`GET /api/traffic/sources` → `{iface,active,detail,sources}`。启用方式：`serve --traffic-iface ens2`（unit 已加）。前端新增「流量/命中」标签页（刷新 / 每 10s 自动刷新 / 按出口汇总 / 域名明细 / 采集源状态）。

**实机验证（192.168.1.85）**：`/api/traffic/sources` 返回 `dns-capture: ready` + `ebpf-dae-maps: reserved`；`/api/traffic/hits` queries=68 / domains=8，groups={direct:2, hk:2, “proxy(含未决)”:62, sg:2}，明细 www.youtube.com→hk、www.baidu.com→direct、api.anthropic.com→sg；无 Cookie → 401。

**开销（实测）**：初版 20ms 忙轮询 idle CPU≈1.10%（单核）→ 改批量 drain + 100ms 睡眠后 idle CPU≈**0.27%**（RSS≈90MB，Go 运行时）；cBPF 把数据面流量挡在用户态外，稳态开销可忽略。

**口径**：统计的是「DNS **查询频度**」而非字节量；A/AAAA 各计一次（`getaddrinfo` 常见 ×2），DNS 缓存命中不重复计。真实字节吞吐留给预留的 ② eBPF 源。

**旁路观察（v0.15 更正）**：v0.13 曾把 `e9220e31-…bbtechconsultants.com` / `fbd56476-…bbtechconsultants.com` 当成「可疑 DNS 心跳」。**这是误判**：它们是订阅节点自己的主机名（订阅镜像 29 个节点里大量形如 `<uuid>.bbtechconsultants.com` 的 vmess/vless 节点，dae 需要解析它们才能连节点）。无需排查第三方软件。

---

### 14.13 订阅变更提醒 + 审计日志导出与保留（v0.14 新增）

**目标**：① 让「订阅有变化」在界面上显眼（原来只在后台登记表里）；② 审计日志可从 Web 导出、并可配置保留策略，免得只能 ssh 翻文件。

**① 订阅变更提醒**
- 后端：`/api/status` 增加 `subsrcChanged` / `subsrcNames` / `subsrcLastRun`（只读登记表 `subsrc.json`，不触发拉取）。
- `internal/subsrc`：`Entry` 增加 `AddedKeys` / `RemovedKeys`（节点键 `scheme|host|port|tag`，**不含凭据**），`capKeys` 上限 30 条（超出截断加省略号）；新增 `Summary(st)` 汇总「几条有变化 + 名字」。
- 前端：顶部状态栏在 `subsrcChanged>0` 时显示红色 pill「订阅 N 条有变化」，点击跳到「订阅源更新」并加载；订阅卡片内显示「新增节点 / 消失节点」明细。
- 语义边界：**只提醒、不自动应用**。变化必须人工点「拉取并应用」才落 live 配置（沿用 §4.5 事务与回滚）。

**② 审计日志导出 + 保留策略（`internal/audit`）**
- 轮转：默认单文件 5 MB、保留 3 份历史（`audit.log`、`.1`…`.N`），`--audit-max-mb` / `--audit-keep` 可调（`keep=0` 表示不留历史）。`rotate()` 从最高代往前删/改名，避免 `.N` 泄漏。
- 读取：`Tail` 跨轮转取最新 N 条（当前文件优先）；`ReadAll` 从最高代到当前正序拼接（导出用）。
- 导出：`GET /api/audit/export`（默认 CSV，带 UTF-8 BOM 便于 Excel；`?format=jsonl` 原样 JSONL），`Content-Disposition: attachment`；导出动作本身也写审计（`audit.export`）。
- `GET /api/audit` 响应附 `retention{maxMB,keep,desc}` / `files` / `bytes`，前端显示「单文件上限 N MB，保留 N 份历史」+「N 个文件 / KB」+ 导出按钮。
- **缺陷修复**：`handleAudit` 早已实现却从未注册路由（死代码），前端审计页此前一直是 404；本版补注册 `/api/audit` 与 `/api/audit/export`。

**磁盘口径（给路由器/NAS 场景）**：上限 = `maxMB × (keep+1)`。路由器（OpenWrt，flash 小、`/var` 多为 tmpfs）建议 `--audit-max-mb 2 --audit-keep 2`（最多约 6 MB），并把 `--subsrc-path` 放到持久分区；日志放 tmpfs 则接受重启丢失。**按大小轮转，无需 cron 定时清理。**

**实机验证（192.168.1.85）**：journal 打印「审计日志: 单文件上限 5MB，保留 3 份历史（最多约 20MB）」；`/api/audit` 返回 retention/files/bytes；CSV 导出 200、`text/csv; charset=utf-8`、attachment、BOM 正确、表头「时间,操作者,来源IP,动作,对象,结果,详情」；JSONL 导出 200。浏览器端（CDP）注入假变化后顶部 pill 显示「订阅 1 条有变化」、点击可跳转并展示新增/消失节点，验证后已恢复登记表原状。

### 14.14 安全直连规则（本机/内网流量被代理的坑；v0.15 新增）

**现场问题（2026-10-23 @192.168.1.85 实测）**：dae 的 `fallback: proxy` 加上 live 规则里**没有内网/私有网段直连**，导致**本机自身的出站流量也被丢进代理组**。journal 直接证据：

```
WARN UdpEndpoint read loop exited with error dialer=comet95.cf.090227.xyz
  error=websocket: close 1005 (no status) lAddr=192.168.1.85:68 nat_timeout=5m0s
  proxy_addr=192.168.1.1:67 sniffed=
```

即本机 dhcpcd 的 DHCP(68→67)、以及发往路由器 192.168.1.1 的 DNS，都被经香港节点代理出去了。连带影响：**面板自己的订阅拉取也被代理**（check 时好时坏、apply 反复 30s 超时——之前误判为「上游限流」，真实原因至少叠加了本地代理路径不稳）。

**诊断脚本（只读，可随时跑）**：`tools/netdiag/`（从 /root 收回仓库）
- `flowprobe.py [秒]`：采样 ens2 / dae0 计数增量 + `ss -tnip` 解析 dae 出站连接按节点归并（注意：`ss` 必须带 `-p` 才看得到 `users:(("dae"...))`；dae0 是 netkit 拦截侧设备，**dae0 有流量=有东西被代理**）。
- `flowsniff.py [秒] [网卡]`：AF_PACKET SOCK_RAW ETH_P_ALL 被动抓包，按 (客户端,目的,端口) 聚合字节，并标注目的地是否订阅节点。
- `flowwho.py [秒]`：按客户端 IP 汇总 Top 目的（跳过面板/SSH 端口）。

**修复（面板保证注入）**：`internal/model` 新增 `EssentialRules` 与 `EnsureEssentials`，`Render` 时**前置强制插入**（界面上删不掉、但不显示在规则表里，界面有黄字说明）：
```
routing {
    dip(192.168.0.0/16) -> direct
    dip(127.0.0.0/8, 169.254.0.0/16, 224.0.0.0/4) -> direct
    pname(homenet-panel, dae, dhcpcd, systemd-resolved, sshd, systemd-timesyncd) -> direct
    ...用户规则...
    fallback: proxy
}
```
注意：dae 对 `pname` 有 16 字符上限，`systemd-timesyncd` 会被自动裁剪成 `systemd-timesync`（validate 输出里会提示，属正常）。

**验证（应用后实测）**：reload code=0 / progress=ReloadDone；`journalctl --since` 中 `proxy_addr=192.168.1.1` 出现次数 = **0**；`dae0` 计数 10s 增量 **0B**（面板拉取不再被拦截）；面板 `/api/subsrc/check dual` 由 1.7–30s 降到 **0.8s**；普通进程访问境外站点仍走代理（97–2020ms 波动），符合预期；dae 分组探活正常（61 条到节点连接、持续 re-select）。

**重要现场结论（产品层面）**：被动抓包 45s 显示**没有任何 LAN 客户端真的经这台 dae 上网**——客户端→外网字节只有 KB 级（其余是本机 dae 自己的流量与探活），且同一网段单播交换使本机网卡根本看不到别的设备的流量。Mac(192.168.1.101) 的默认网关是 192.168.1.11，公网 IP 163.53.18.110（已被别的路径代理），**流量并不经过 .85**。也就是说：面板现在能管 dae、能拉订阅、能生成配置，但「让 LAN 设备走 dae」这一环还没接上（需要把 .85 变成网关/DHCP 下发网关或下一跳，见 §4.4 OpenWrt 适配）。

---

### 14.15 goose 残留归档 + 字节级流量统计（v0.16 新增）

#### (1) goose 存量归档（退役收尾，实机已完成）

迁移到面板后 goose 早已不是运行态（无进程、无 unit、crontab 无条目、`command -v goose` 为空），但磁盘上还散着一堆历史备份。核对与处置：

| 位置 | 内容 | 大小 | 处置 |
|---|---|---|---|
| `/usr/local/bin/goose-0.5.*.bak` 等 | 16 个历史二进制备份（0.5.0/.1/.3/.4/.5/.6/.9/.10/.11/.13/.14/.15/.16/.17 等） | 约 140MB | 移动 |
| `/etc/systemd/system/goose.service.disabled` | 已 disabled 的旧 unit | 小 | 移动 |
| `/etc/goose/` | `config.yaml`(1592B) + `auth.json`(0600) + `.session-key`(0600) + `backups/` 24 份 yaml | 120KB | 移动 |
| `/root/goose-deploy-bundle-20261020-v0.5.0(`.tar.gz`)` | 部署包 + 压缩包 | 123M + 52M | 移动 |

- 动作：建 `/root/goose-retired/{bin,etc,unit,deploy}`（0700），全部 **`mv`（不删除）**，合计 **315MB**；`systemctl daemon-reload`；写 `/root/goose-retired/README.txt`（背景、**含订阅 token 勿外传**、恢复方法=把对应文件拷回原位）。
- 归档后核对：`/usr/local/bin` 无 `goose*`、`/etc/goose` 不存在、`/etc/dae` 内无 goose 引用；`homenet-panel` 与 `dae` 均 active；配置基线 md5 未变（`config.dae` 7de9781a…、`panel-core.dae` 4ef4d2a0…）。
- 原则：**归档而非删除**——这些打包里有历史配置与订阅 token，留一份可回退证据；但不再参与运行。

#### (2) 字节级流量统计（`/api/traffic/bytes`）

**取舍：不用 eBPF 直读 dae 的 map。** 理由：(a) dae 的 map 名/键/值布局随版本变化（v2.1.1 → 未来版本易碎），面板强耦合会在升级时直接崩；(b) 本项目 go.mod **零第三方依赖**，引入 ebpf/cilium 库会显著扩大内核兼容面与构建复杂度；(c) 只读诊断的目标（“哪个节点用了多少字节”）用内核已有的 socket 计数器就能拿到，没有必要下到字节级抓包。

**实现（方案 B：轮询内核计数器）**，采集器 `internal/traffic.ConnCollector`：

| 数据源 | 取法 | 口径 |
|---|---|---|
| 逐节点字节 | `ss -tunip` 解析 dae 进程每条 socket 的累计 `bytes_sent`/`bytes_received`（只认进程名 `"dae"`） | 精确（TCP 稳定；UDP/QUIC 多数内核不计数） |
| 总量兜底 | `/proc/net/dev` 读 `dae0`（netkit 拦截侧设备）rx/tx | 含 UDP，但只覆盖“被拦截”的流量 |

- 归并：socket 键 = `proto|本地|对端`；对端是**私有地址**→ 计入 `clients`（即经本机代理的 LAN 设备），否则计入 `nodes`（`节点IP:端口`）。节点名由 `Server.nodeIndex()` 用订阅节点 `host:port` + `net.LookupHost` 解析出的 `ip:port` 反查（10 分钟缓存），查不到就显示原始对端地址。
- 速率：`本次累计 − 上次累计` / 实际间隔；**首见 socket 只记基线**（面板重启后不会把历史累计当成一次暴涨）。
- 端点：`GET /api/traffic/bytes`（需登录）→ `BytesSnapshot`（Enabled/Source/IntervalMs/Sockets/TotalTx,TotalRx/RateTx,RateRx/IfaceName,IfaceRx,IfaceTx,IfaceRateRx,IfaceRateTx/Nodes[]/Clients[]/Error）。启动参数 `--bytes-interval`（默认 10s，**0=关闭**）、`--bytes-iface`（默认 `dae0`）。
- 前端：「字节统计」tab（累计上下行、当前速率、活跃 socket、dae0 收发总量与速率、按节点表、按客户端表，10s 自动刷新开关）。

**已写进代码注释的局限（口径必须诚实）**：UDP/QUIC 多数无字节计数；两次采样之间的短连接会漏计（故数值是**下限**）；采集器重启后首见连接只作基线；`ss` 有解析开销（约 60 个 socket 量级可忽略）。

**实机实测（2026-10-23，dae v2.1.1 @192.168.1.85）**：`sockets 60–65`、逐节点 44 行（其中 2 条对端端口已不在当前订阅内，显示为未匹配）、常有 13–20 个节点有活动速率、Top 节点约 170–180 B/s 下行（health check + 小流量）；`dae0` 累计 rx≈726KB / tx≈375KB，只有真实代理流量时才增长（面板自身出站已直连，见 §14.14）；`clients` 为空——与 §14.14「当前没有 LAN 客户端经本机上网」吻合。


### 14.16 接入向导 P-A：只读能力探测 + 选档推荐（v0.19 新增，已完成并实机验证）

落实 §4.4.7 的 **P-A 阶段**：把"这台机器适合哪一档接入"变成可解释的探测结果，**只读、不改任何配置**。

#### (1) 代码结构

| 文件 | 内容 |
|---|---|
| `internal/access/probe.go` | `Runner` 接口（默认 `execRunner`）+ `Collect(r Runner, rootDir, daeConfigPath string) *Probe`；`Probe` 含 Platform/OSName/Arch/Kernel/ServiceManager/HasBPF/HasBTF/MemlockKB/Nft/DefaultGw/DefaultIface/IPForward/Ifaces/BusyPorts/DaeInstalled/DaeVersion/DaeRunning/DaeLanIfaces/StaleTables/Notes/Rec |
| `internal/access/recommend.go` | `AccessMode` 常量（gateway / bypass-manual / bypass-router-dhcp / bypass-self-dhcp / bypass-pbr / observe）、`Label()`、`Answers{DHCPGateway,DHCPDNS,CanDisableDHCP,PBR *bool}`（nil=不知道）、`Questions()`、`Recommend(p *Probe, a Answers) Rec`、`Guide(m AccessMode) []string`（每档四段：前提/步骤/自检/回滚） |
| `internal/api/access.go` | `GET /api/access/probe` → `{probe, guide}`；`POST /api/access/recommend`（body=Answers）→ `{rec, guide, label, question}`；均需登录 |
| `internal/api/index.html` | 新增「接入向导」tab：本机能力表、网卡表、推荐档位卡（blockers + reasons + guide）、向导四问下拉（不知道/能/不能 +「按回答重新推荐」）、遗留 `panel_*` 表清单 |

探测实现要点（**全部只读**）：

- 平台：有 `/etc/openwrt_release` → openwrt；有 `proc/sys` → linux；否则按 `runtime.GOOS`。
- 服务管理器：openwrt → procd；有 `run/systemd/system` → systemd；否则 other。
- 能力：BPF 看 `/sys/fs/bpf`、BTF 看 `/sys/kernel/btf/vmlinux`；`memlock` 解析 `/proc/self/limits` 的 *Max locked memory*（unlimited 记 −1）；`nft list tables` 同时识别遗留 `panel_tproxy/panel_snat/panel_dns`。
- 网卡/路由：`ip -j addr` 取地址、`/sys/class/net/<n>/` 取 MAC/operstate/bridge/device 判虚拟；`ip -4 route show default` 取默认网关与出口网卡。
- 端口占用：读 `/proc/net/{udp,tcp,udp6,tcp6}`（十六进制端口），关注 53/67/68/12345 —— 用来判断"本机是否已经有人占着 DNS/DHCP/tproxy 端口"。
- dae：`dae --version` → `dae version` → `dae -v` 依次尝试（见下 bug ①），失败用 `exec.LookPath("dae")` 兜底；`pgrep -x dae` 判运行。
- dae 配置：读入口配置里的 `include { … }` 块并 `filepath.Glob` 展开（最多两跳），跨文件汇总 `lan_interface`。
- **fail-open**：任何探测失败都只是少一条事实/多一条 note，推荐退回 `observe` 或基线档，绝不因此阻塞。

#### (2) 选档逻辑（与厂商无关）

阻塞项：非 linux/openwrt、缺 BPF 或 BTF → 只能 `observe`。基线：openwrt → `gateway`，否则 `bypass-manual`。升档只由回答驱动：`DHCPGateway=true → bypass-router-dhcp`；`PBR=true` 只追加理由（给出 `bypass-pbr` 片段）但不改默认档；`CanDisableDHCP=true` 只提示"可选 B3（不建议）"。以下事实只作**理由**输出、不改变档位：DNS 不能改、无默认路由、memlock < 64MB、`ip_forward=0`、存在遗留 `panel_*` 表、dae 未装/未运行、dae 未声明 `lan_interface`。

#### (3) 实机结果（2026-10-23，Debian 13 @192.168.1.85）

`platform=linux`、`osName=Debian GNU/Linux 13 (trixie)`、`arch=amd64`、`kernel=6.12.113+deb13-amd64`、`serviceManager=systemd`、`hasBpf=true`、`hasBtf=true`、**`memlockKB=8192`**（再次印证 §13.1 E20 隐患）、`nft=true`、`defaultGw=192.168.1.1 / ens2`、`ipForward=false`、`daeInstalled=true`、`daeVersion="dae version v2.1.1"`、`daeRunning=true`、**`daeLanIfaces=null`**（live 配置确实没声明 `lan_interface` → 当前只有本机流量被代理，与 §14.14 一致）、`busyPorts={"68":"udp"}`（dhcpcd）、`staleTables=null`。推荐 = **B1 bypass-manual**；回答"能改 DHCP 网关"后 → **B2 bypass-router-dhcp**，并给出"不能改 DNS → 域名分流可能失效"的理由。前端「接入向导」tab 用 CDP 无头浏览器实测：渲染正常、`{{` 泄漏 0、点「按回答重新推荐」后档位卡从 B1 切到 B2。

#### (4) 实机暴露并修掉的 3 个 bug（都是"只在真机上才会犯"的）

1. **`dae version` 子命令在 v2.1.1 不存在**（`Error: unknown command "version" for "dae"`，rc=1）→ 版本探测改按 `--version` / `version` / `-v` 顺序尝试，最后 LookPath 兜底。教训：**能力探测要按"从最可能成功到最保守"排序，并且永远留兜底**。
2. **只读入口配置忽略 `include`**：真实配置是 `config.dae` 里 `include { config.d/*.dae }`，`lan_interface` 写在 include 的文件里 → 新增 `daeConfigFiles` 做 glob 展开。教训：**dae 的"配置"是文件集合，不是单文件**，任何读配置的逻辑都必须展开 include。
3. **`nft list tables` 解析要求 4 个字段**：实际输出 `table inet panel_tproxy` 只有 3 个 token → 遗留表识别不到。改为 `len(f) >= 3 && f[2]`。教训：**解析命令行输出要按"最少必需字段"写，不要按样例行长度写**。

#### (5) 工程约束（写进开发习惯）

- 前端脚本块里**不得出现 HTML 实体**（`&#39;` 等会直接让 Vue 挂载失败），由 `TestIndexHTMLScriptHasNoHTMLEntities` 守住。
- 交叉编译必须显式 `GOOS=linux GOARCH=amd64 CGO_ENABLED=0`（本机 go 在 `/opt/homebrew/bin/go`，且需 `HOME`）。
- 用脚本批量改 Go 源码时，**反斜杠转义极易被上层字符串处理吃掉**（`"\n"` 变成真换行 → `string literal not terminated`）：改完必须 `gofmt -l` 复核再编译。

---

## 14.17 接入向导 P-B：计划生成 + 只读自检（v0.21）

### 14.17.1 产品口径（用户定稿）

- **默认档 = B1 旁路由 · 单机手改**：用户在**设备端**把网关与 DNS 指向旁路由，面板只出片段与自检清单。
- 其余档位（A 网关 / B2 主路由 DHCP / C 策略路由 / B3 面板托管 DHCP）**只保留「生成计划」的入口**，默认不启用、不自动执行；B3 仅出说明文本。
- 计划一律 dry-run：`dryRun: true`，面板不执行任何命令、不写系统状态；apply 阶段（真正写盘与建 nft 表）留待用户确认后实现。

### 14.17.2 代码结构

| 文件 | 内容 |
| --- | --- |
| `internal/access/plan.go` | `PlanOptions`（Mode/LanIfaces/TProxyPort/DNSMode/SNAT/ProxyIP/TestDevice）、`Step`、`Check`、`Plan`、`PlanFor(p,opt)`（纯函数）、渲染器 `RenderDaeGlobal` / `RenderNftDNS` / `RenderNftSNAT` / `RenderNftTProxyPlaceholder` / `RenderPBR`、`manualStepsFor`、`rollbackStepsFor`、`ChecksFor`、`modeWarnings`、`fingerprintOf`（sha256 前 8 字节） |
| `internal/access/verify.go` | `VerifyInput{Clients,DNSQueries,IfaceRx,IfaceTx}` + `Verify(in) []CheckResult`（6 项，Level = ok/warn/fail/info） |
| `internal/api/access.go` | `GET|POST /api/access/plan`（GET 走 query、POST 走 `PlanOptions` JSON）、`GET /api/access/verify`（自检数据来自 `access.Collect` + `s.Bytes.Snapshot()` + `s.Traffic.Snapshot(1).Queries`） |
| `internal/api/index.html` | 「接入向导」tab 增加：档位/LAN 网卡/旁路由 IP/tproxy 端口/DNS 方式/SNAT/测试设备 表单 + 「生成接入计划」「运行自检（只读）」+ 计划分区（警告 / 面板侧步骤、可复制片段 / 人工步骤 / 回滚 / 自检清单）+ 自检结果表 |

### 14.17.3 渲染口径（与 §4.4.5 一致）

- dae 侧只写**自有片段** `config.d/panel-access.dae` 的 `global`：`tproxy_port` / `wan_interface: auto` / `auto_config_kernel_parameter: true` / `lan_interface: <逗号分隔>`；lan_interface 未确定时输出注释行而不是乱填。
- `panel_dns`：把 LAN 网卡上 53/tcp、53/udp `dnat` 到 `<旁路由IP>:53`（DNS 必须经过本机，否则域名规则失效）；`DNSMode=none` 时仅输出关闭说明。
- `panel_snat`：`auto`（默认）**不生成**——被 tproxy/netkit 劫持的 TCP/UDP 在旁路由本地终结、回程天然对称；只有"未被劫持却被转发"的流量跨网段或主路由开 rp_filter 时才需要，届时 `snat=on` 生成 `masquerade`。
- `panel_tproxy`：**默认不建**（dae 会为 `lan_interface` 自行安装拦截规则），仅保留"自定义放行例外"的注释片段，避免规则叠加。
- 回滚统一为 `nft delete table inet panel_dns|panel_snat|panel_tproxy` + 各档位恢复步骤 + 删 `panel-access.dae` 后 `dae reload`。
- C 档（策略路由）**只渲染主路由侧 `ip rule`/`nftset` 片段**，面板不远程登录主路由。

### 14.17.4 实机验证（192.168.1.85，dae v2.1.1 / Debian 13）

- `POST /api/access/plan {mode:"",...}` → 200，`mode=bypass-manual`（默认 B1）、`lanIfaces=["ens2"]`（自动取内网网卡）、`proxyIp=192.168.1.85`、`tproxyPort=12345`、`fingerprint=a074ea50e148d531`；steps 4 / manual 3 / rollback 3 / checks 6；片段含 `lan_interface: ens2`。
- `POST /api/access/plan {mode:"bypass-router-dhcp",lanIfaces:["ens2"],...}` → 200，B2 计划、2 条风险警告（含"改错会断网""先记录原值"）。
- `GET /api/access/verify` → `4 项通过 / 1 项待确认`：dae 运行 ok、dae0 累计 861KB/453KB ok、DNS 已采集 220 次 ok、遗留表 ok；`lan_clients` 为 warn——**这台机上确实还没有 LAN 设备走 dae**，自检如实报出而不是假装成功。
- 浏览器（headless Chrome + 真实会话 cookie）：点「接入向导」→「生成接入计划」→「运行自检」，DOM 27,337 字节、`{{` 泄漏 0，计划卡渲染出 `计划：B1 旁路由 · 单机手改 DRY-RUN 指纹 A7392D4E19E0340C` 与自检表。

### 14.17.5 明确边界（本版没做）

- **apply 未实现**：真正写 `panel-access.dae`、建 nft 表、`dae reload` 需要用户在场确认（会在回家网络的 B2 真机场景做）。
- B3（面板托管 DHCP）只出说明，不提供执行路径。
- 计划不持久化：每次生成都重新探测，指纹用于将来 apply 时校验"用户看到的就是要执行的"。

### 14.17.6 工程教训

- 生成 Go 代码时，中文字符串里**不能夹 ASCII 双引号**当引号用（会把字面量提前闭合，报 `missing 
 in composite literal` / `string literal not terminated`）；统一用「」。
- 通过模板字符串拼接写 Go 源码时，`"\n"` 可能被还原成真实换行，写完必须 `gofmt -l` + `go vet` 复核。
- `traffic.Snapshot().Queries` 是 `int64`，跨包传值注意类型。

## 15. 本版（v0.21）变更清单

1. **修复文档截断**：v0.12 文件尾部丢失（§13 的 D8–D12、§14 全节、变更清单），本版按实机实测记录与现有代码重建。
2. 补全 §13 的 D8–D12（trace/completion 存在、validate 不需 root 但拒绝 0644、退出码语义、报错含行列号、outbound 名预检）。
3. 新增 §13.1「实机验证补充」：E9 / R1–R5 / E15 / E17 / E20 / E21 / E25 / E26 的架构级结论。
4. 新增 §14（14.1–14.11）：实现现状、部署形态、内容层数据流、完整 REST API 表、订阅格式事实、前端本地化取舍、模式B 切换、TLS、分流调试器、规则编辑增强。
5. **新增 §14.12**：③ DNS 旁路流量/命中统计 + ② eBPF 源预留接口（实现、端点、开销实测、统计口径）。
6. **新增 §14.13（v0.14）**：订阅变更提醒（状态栏红点 + 差异明细，只报告不自动应用）、审计日志导出（CSV/JSONL）与可配置保留策略（`--audit-max-mb` / `--audit-keep`，按大小轮转）、以及 `/api/audit` 死路由缺陷修复。
7. **新增 §14.14（v0.15）**：安全直连规则——实测发现本机 DHCP/DNS 与面板自身出站被 `fallback: proxy` 代理（journal 有 lAddr=…:68 proxy_addr=192.168.1.1:67 铁证），面板改为 `Render` 时强制前置注入 `dip(内网/回环/组播) -> direct` 与 `pname(面板/dae/dhcpcd/…)-> direct`，应用后内网代理提及归零、面板拉取 0.8s。同时更正 §14.12 把订阅节点域名误判为「可疑 DNS 心跳」，并记录「LAN 客户端目前并不经本机 dae」这一前置事实。
8. **新增 §14.15（v0.16）**：goose 残留归档（315MB 全部 mv 到 `/root/goose-retired/`，不删除、附恢复说明）；字节级流量统计（`GET /api/traffic/bytes`，方案 B 轮询内核 socket 计数器 + `dae0` 网卡总量，`--bytes-interval`/`--bytes-iface`，前端「字节统计」tab），并记录 eBPF 取舍理由与统计口径下限说明。
9. **§4.4 重写（v0.17）**：接入分档设计——三张流程图 + 五种接入档位（含 `bypass-self-dhcp` 默认关闭/30 分钟自动失效）+ 自动选档与人工开关 + `AccessProfile`/`Plan`/`PBRSpec` + 自检矩阵 + 阶段与回滚；并明确"不碰传统路由功能、只写自有 nft 表、策略路由只生成片段"三条红线。
10. **§4.4 去厂商耦合（v0.18）**：厂商画像改为 preset（只预填导航）+ 默认档统一 B1 + 向导三问通用化；新增 §4.4.9 开源通用性约束表（探测代替硬编码、service 抽象、fail-open、degraded 只读、Plan 先行）。
11. **新增 §14.16（v0.19）**：接入向导 P-A 落地——`internal/access` 只读探测（平台/服务管理器/BPF-BTF/memlock/nft/默认路由/网卡/端口占用/dae 版本与运行/include 展开后的 `lan_interface`/遗留 `panel_*` 表）+ 与厂商无关的选档推荐 + `/api/access/{probe,recommend}` + 前端「接入向导」tab；记录实机暴露的 3 个 bug 与对应开发习惯。
12. **§4.4.3 补实测样本（v0.20）**：中兴家用固件「地址管理」页可编辑「自定义网关地址」（DHCP Option 3）→ B2 在部分家用固件可行，纠正"家用路由普遍不能改 DHCP 网关"的统计假设；同时记录该字段可能影响路由器自身出口（填错整段断网）的风险与"记原值→改→验证→失败改回"的操作顺序。
13. **接入向导 P-B（v0.21）**：新增 §14.17——`internal/access/plan.go`（计划生成：dae global 片段、`panel_dns`/`panel_snat`/`panel_tproxy` 渲染、人工步骤、回滚、6 项自检清单、指纹）+ `verify.go`（只读自检）+ `/api/access/plan`（GET/POST，dry-run）+ `/api/access/verify` + 前端「接入向导」计划与自检 UI；产品口径定稿为**默认 B1（设备端手改网关/DNS）**，其余档位只留生成计划入口；实机 4 通过 1 待确认（LAN 客户端尚未接入）。
