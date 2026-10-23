# dae 实际 CLI 能力核对表

> **文档性质：** 核对表 / 事实底稿，不是设计方案
> **服务对象：** `architecture.md` 的 §4.1、§4.2、§4.5（核心管理、订阅、配置事务）
> **核查日期：** 2026-10-23
> **核查基准：** dae 官方仓库 `daeuniverse/dae` 的 `main` 分支源码 + 最新 release **v2.1.1**（2026-09-18 发布）
> **标注含义：** ✅ 源码核对 = 已在源码中确认；⬜ 待实机 = 必须在目标设备上跑一遍才能确定
> **重要提醒：** 本文所有 ✅ 项均来自源码阅读，**不等于已在 Armbian / OpenWrt 实机验证**。D 节和 E 节是必须补的实测部分。

---

## 0. 一句话结论

dae 的 CLI 能力比 `architecture.md` 假设的**更好**：它自带 `validate`（离线校验）、`reload`（不断连接重载）、`suspend`（降为无负载）、`export outline`（机器可读的配置结构）。

但同时有 **5 个坑会直接影响架构设计**，必须先知道：

| # | 坑 | 影响 |
|---|---|---|
| 1 | `dae validate` **不解析订阅**，订阅错误只在 `run` 阶段才暴露 | §4.5 的"先校验再应用"对订阅类错误无效 |
| 2 | 守护进程**忽略 SIGHUP**，重载必须用 `dae reload`（SIGUSR1） | 不能用 systemd 默认的 `ExecReload=kill -HUP` |
| 3 | dae 原生支持订阅，但只认 base64 / SIP008 / 明文节点行，源码中**未见 Clash YAML 解析** | 决定 §4.2 的订阅路径能否走 dae 原生 |
| 4 | `reload` / `suspend` 内部会 `AutoSu()`（可能触发 sudo 提示） | 面板调用若不在 root 下会**卡住等密码** |
| 5 | `reload` 靠 `/var/run/dae.pid` 找进程 | 开了 `--disable-pidfile` 就必须由面板显式传 pid |

---

## A. 命令与参数核对表

### A.1 命令清单（✅ 源码核对）

| 命令 | 关键参数 | 源码实际行为 | 面板用途 | root | 实测 |
|---|---|---|---|---|---|
| `dae`（无子命令） | — | 打印 help，退出 0 | 能力探测 | 否 | ⬜ |
| `dae --version` | — | 多行输出：版本 + `go runtime go1.x linux/arch` + 版权行 | 版本 / 架构检测，**注意是多行文本，需按行解析** | 否 | ⬜ |
| `dae run` | `-c/--config`（必填） | 前台运行，建控制面，启动 eBPF | 不建议面板直接调用，交给 systemd / procd | 是 | ⬜ |
| `dae validate` | `-c/--config`（必填） | 读配置 + 校验 routing 规则 + dns routing + `fixed_domain_ttl`，**不碰 BPF** | 配置事务的第 5 步（候选配置校验） | **否**（不调用 AutoSu） | ⬜ |
| `dae reload [pid]` | `-a/--abort` | 发 SIGUSR1 → 轮询 `/var/run/dae.progress` → 最多等 60s（首等 500ms，轮询 200ms） | 配置应用的"不断连接生效"入口 | 是（AutoSu） | ⬜ |
| `dae suspend [pid]` | `-a/--abort` | 发 SIGUSR2，进入无负载状态；`-a` 额外终止已建立连接（写 `/var/run/dae.abort`） | 故障应急"一键摘流量" | 是（AutoSu） | ⬜ |
| `dae export outline` | — | 打印 JSON：版本、字段路径列表（leaves）、结构树（字段名 / 类型 / 默认值 / 是否必填 / 描述） | **前端表单生成 + 版本兼容检测**，见 A.3 | 否 | ⬜ |
| `dae sysdump` | — | 打包 routing / interfaces / sysctl / nft / iptables 到**当前目录**的 `dae-sysdump.<unixts>.tar.gz` | 诊断中心"导出排障包" | 视情况（读 nft / iptables 通常需 root） | ⬜ |
| `dae trace` | `-4` / `-6`、`-p tcp/udp`、`-P port`、`-o 文件`、`--drop-only` | eBPF 抓包追踪 | 进阶诊断（v1 可不做） | 是（AutoSu） | ⬜ |
| `dae completion` | — | shell 补全脚本 | 无关 | 否 | ⬜ |
| `dae honk` | — | 彩蛋，响三声汽笛 | 无关（但可用来验证二进制可执行） | 否 | ⬜ |

### A.2 `dae run` 的运行期参数（✅ 源码核对）

| 参数 | 默认值 | 说明 | 面板需要注意 |
|---|---|---|---|
| `-c/--config` | 无（必填） | 入口配置文件 | 面板必须托管这个路径 |
| `--logfile` | 空 | 空 = 输出到 stdout / stderr | 走 journald / procd 时留空即可 |
| `--logfile-maxsize` | 30 | MB，超限轮转 | 若写文件，注意闪存写放大 |
| `--logfile-maxbackups` | 3 | 保留旧日志份数 | 同上 |
| `--disable-timestamp` | false | 关时间戳 | 交给 journald 时可开 |
| `--disable-pidfile` | false | 不生成 `/var/run/dae.pid` | **开了之后 `dae reload` 无法自动找 pid** |
| `--disable-sudo` | false | 关掉自动 sudo 提权 | 面板调用建议显式加上，避免交互式卡死 |

### A.3 `dae export outline` 的具体价值（✅ 源码核对，我认为被低估）

输出的 `leaves` 是全部配置字段路径的排序列表，`structure` 带每个字段的 `type` / `defaultValue` / `required` / `desc`。

面板可以直接拿它做三件事，比硬编码字段表可靠得多：

1. **前端表单生成**：从 outline 渲染字段，随 dae 版本自动跟进。
2. **版本兼容检测**：缓存每个 dae 版本的 outline，升级后 diff 出"字段被删除 / 默认值变更"，提前告警。
3. **配置预校验**：结构化模型产出的 key 是否在当前 dae 版本存在，写配置前就能查出来。

> 建议：`architecture.md` §4.3 的"规则模型 → 编译器"链路里，加一个"outline 校验"关卡。

---

## B. 文件、路径与信号契约（✅ 源码核对）

路径全部硬编码，面板不能假设可配置：

| 项 | 值 | 说明 |
|---|---|---|
| pid 文件 | `/var/run/dae.pid` | `reload` / `suspend` 无参时读它 |
| 重载进度文件 | `/var/run/dae.progress` | 首行是单字节状态码，后续行为文本消息 |
| 中止标记 | `/var/run/dae.abort` | `-a` 时创建；`dae run` 启动时**会先删除**它 |
| 配置入口 | 由 `-c` 指定 | 无默认值，无参即报错退出 1 |

**信号语义（这一条最容易踩坑）：**

| 信号 | dae 守护进程的反应 |
|---|---|
| `SIGUSR1` | 触发 reload（`dae reload` 用的就是它） |
| `SIGUSR2` | 触发 suspend（`dae suspend` 用的就是它） |
| **`SIGHUP`** | **显式忽略**（源码注释为 Ignore） |
| `SIGINT` / `SIGTERM` / `SIGQUIT` | 退出 |

> **直接结论：** 别写 `systemctl reload`（默认发 SIGHUP，等于什么也没做），也别 `kill -HUP <pid>`。必须调用 `dae reload` 二进制本身，因为它额外负责**等待并回报重载结果**。

**reload 的进度码：** ReloadSend / ReloadDone / ReloadError / ReloadBusy。其中 ReloadBusy 表示已有重载在跑——dae 自带互斥，正好可以满足 `architecture.md` §4.5「同一时间只允许一个配置应用任务」，面板不必自己造锁，但**要处理 Busy 的返回**。

---

## C. 配置模型核对（✅ 源码核对）

### C.1 顶层 section

| section | 是否必填 | 面板是否适合托管 |
|---|---|---|
| `global` | **必填** | 部分托管（只改界面暴露的字段） |
| `routing` | **必填** | 托管（结构化规则编译产物） |
| `dns` | 可选 | 部分托管 |
| `subscription` | 可选 | **决策点，见 F1** |
| `node` | 可选 | **决策点，见 F1** |
| `group` | 可选 | 部分托管（策略组编排） |
| `include` | 可选 | 见 C.2 |

### C.2 `include` 的语义（对"配置所有权"很关键）

支持拆分配置：

```text
include {
    config.d/*.dae          # 相对入口配置文件所在目录解析（不是 cwd）
    /etc/dae/config.d/*.dae # 绝对路径按原样使用
}
```

- 相对路径**相对入口配置文件所在目录**，不是当前工作目录。
- **出于安全，只允许包含入口配置目录下的文件。**
- 合并后按 section 名去重。

> 这是个重要机会：面板可以**只托管 ```config.d/``` 下的分组文件**（如 `panel-node.dae`、`panel-route.dae`），完全不碰用户手写的 `config.dae`。这样"面板托管 vs 用户自定义"的冲突从架构层面消除。建议把这条写入 §4.5 的配置所有权定义。

### C.3 订阅格式支持范围（⚠️ 关键，影响 §4.2）

`subscription` 是 `tag: 'url'` 形式的键值段，源码中可见的解析器只有：

| 格式 | 是否支持 | 依据 |
|---|---|---|
| Base64（std / url-safe）编码的节点行 | ✅ | `ResolveSubscriptionAsBase64` |
| SIP008 JSON | ✅ | `ResolveSubscriptionAsSIP008` |
| 明文节点行（协议 URL 逐行） | ✅ | Base64 分支内的行过滤逻辑 |
| 本地文件（`file://`） | ✅（存在 `ResolveFile`） | `ResolveFile` |
| **Clash / Clash.Meta YAML** | **源码中未见解析器** | 订阅相关代码无 YAML 处理 |

> ⚠️ 最后一行是**"未见"而非"不支持"**——我没有对完整代码库做穷尽搜索。E9 列了对应实测项。但若确认不支持 Clash YAML，则大量商业机场订阅**无法交给 dae 原生订阅托管**，§4.2 的结论要改。

---

## D. 已确认的能力边界与坑（每条都要写进架构文档）

| # | 事实（✅ 源码核对） | 对架构的影响 |
|---|---|---|
| D1 | `dae validate` **不解析订阅、不联网取订阅**；订阅解析发生在**运行期**（构建控制面时） | §4.5「校验」对订阅类错误是**盲区**。面板必须自己实现订阅预取 + 解析预检，否则"校验通过但应用失败"会成为常态 |
| D2 | `dae validate` 覆盖：routing 规则（复用运行期同一套 optimizer 链，含 geoip / geosite 展开、别名解析、规则合并排序、outbound 存在性检查）+ dns routing + `fixed_domain_ttl` | 校验质量**比预期高**，可以放心作为事务第 5 步。但它仍不覆盖 BPF 层与全局参数的语义正确性 |
| D3 | `validate` 需要读 geo 数据（`geoip.dat` / `geosite.dat`，按配置文件目录推断位置） | 首次离线校验可能因缺 geo 数据失败 → 面板要能识别并提示"需先下载 geo 资源" |
| D4 | 守护进程忽略 SIGHUP | 见 B 节，必须用 `dae reload` |
| D5 | `reload` / `suspend` / `trace` 内部 `AutoSu()` | 面板非 root 运行时可能卡在 sudo 密码提示 → 显式加 `--disable-sudo`，并保证自身已具备权限 |
| D6 | `suspend` 是**真正的无负载态**，`dae reload` 恢复 | 可以做成面板的应急开关，比"停服务"更平滑（不动服务、不做 teardown） |
| D7 | `sysdump` 输出到**当前工作目录** | 面板调用时必须把 cwd 指定为临时目录，否则污染 `/` 或 `/etc/dae` |
| D8 | `trace` 受 build tag `trace` 控制（`//go:build trace`） | **官方默认二进制里很可能没有 `dae trace`** → 探测时按"命令可能不存在"处理，不要写死 |
| D9 | `run` 启动时会**删除** `/var/run/dae.abort` | 回滚逻辑不要依赖该文件跨重启存活 |
| D10 | 重载失败会通过 `.progress` 文件回报错误文本 | 面板可以拿到**具体的重载失败原因**（不必只靠猜日志）；这是做"自动回滚"判定的可靠信号 |
| D11 | `global.auto_config_kernel_parameter: true` 时 dae 会**自动改内核参数** | 与面板"不擅自改系统"原则冲突，需在 UI 明示，或检测到开启时给出提示 |
| D12 | `wan_interface: auto` 会按默认路由自动探测网卡 | 面板展示接口时要把 auto 实际解析成了什么算出来再展示，否则用户看到的和实际不一致 |

---

## E. 待实机实测清单（⬜ 必须在 Armbian 上跑）

每项都写清「怎么测」和「判定标准」，跑完把结果回填到 A~D 表。

### E.1 基础探测

| # | 实测项 | 命令 / 做法 | 判定标准 |
|---|---|---|---|
| E1 | 实际版本与架构 | `dae --version` | 记录完整多行输出，确认 GOOS / GOARCH |
| E2 | 子命令全集 | `dae --help`、`dae help <cmd>` | 确认 `trace` 是否存在（build tag） |
| E3 | `run --help` 是否与源码一致 | `dae run --help` | 逐项对照 A.2 表 |
| E4 | 非 root 下 `validate` 是否可用 | 以普通用户跑 `dae validate -c ...` | 记录真实报错（源码看不需要 root，但可能受配置文件权限限制） |
| E5 | 非 root 下 `reload` 行为 | 普通用户跑 `dae reload` | 是否卡在 sudo 提示；加 `--disable-sudo` 后是否直接报错 |

### E.2 校验能力边界（最重要）

| # | 实测项 | 做法 | 判定标准 |
|---|---|---|---|
| E6 | 非法语法能否被 validate 抓出 | 故意写坏一行 routing | 退出码非 0，stdout 有可读错误 |
| E7 | 引用不存在的 group | `-> notexist_group` | 应报 outbound (group) 未找到 |
| E8 | **订阅不可达**能否被 validate 抓出 | 配一个 404 的订阅 URL 后 validate | 预期**发现不了**（验证 D1），这是关键结论 |
| E9 | **订阅内容是 Clash YAML** 会怎样 | 用真实 Clash 订阅跑 `dae run` | 确认是否报错、报错文本形态 → 决定 §4.2 路线 |
| E10 | 缺 geo 数据时的报错 | 清空 geo 目录后 validate | 记录错误文本，用于面板的错误分类 |
| E11 | validate 退出码全谱 | 合法 / 非法 / 缺 `-c` 三种 | 确认 `-c` 缺失时退出 1 且提示打到 stdout |

### E.3 reload / suspend 契约

| # | 实测项 | 做法 | 判定标准 |
|---|---|---|---|
| E12 | `dae reload` 成功路径 | 改一处无害配置后 reload | 退出 0，已建立连接不中断 |
| E13 | reload 失败路径 | 写坏配置后 reload | 退出非 0，`.progress` 有错误文本，且**旧配置下 dae 是否仍在运行**（关键：决定回滚策略） |
| E14 | 并发 reload | 同时起两个 reload | 第二个返回 Busy 而非排队执行 |
| E15 | reload 超时行为 | 构造大配置拖慢重载 | 记录 60s 超时的实际表现 |
| E16 | `suspend` 后网络表现 | `dae suspend` 后访问外网 | 确认是"直连 / 无负载"而非"断网" |
| E17 | `--disable-pidfile` 下的 reload | 加该参数启动后 reload | 确认必须显式传 pid |
| E18 | **`kill -HUP` 是否真的无效** | 对 pid 发 SIGHUP | 预期无任何反应（验证 D4） |

### E.4 权限与运行环境

| # | 实测项 | 判定标准 |
|---|---|---|
| E19 | 内核版本 / 必要内核配置 | 记录 `uname -a` 与 BPF 相关内核选项 |
| E20 | eBPF 相关系统限制 | 记录 memlock 限制、`/proc/sys/kernel/unprivileged_bpf_disabled` 等实际值 |
| E21 | tc 钩子的挂载与卸载 | **dae 启动后 `tc filter show dev <iface>` ingress / egress 的实际内容；停服务后是否干净卸载**（断网风险最高的一环） |
| E22 | `/var/run` 是否可写 | 容器或只读 rootfs 环境下 pid / progress 文件能否落地 |
| E23 | 闪存写入量 | 观察日志与配置写入频率，给出日志轮转参数建议 |

### E.5 既有安装与共存

| # | 实测项 | 判定标准 |
|---|---|---|
| E24 | 现有 dae 安装形态 | 二进制路径、由谁安装（安装脚本 / opkg / 手动）、服务定义内容 |
| E25 | 是否已装第三方 Web UI（daed 等） | 若存在，确认它是否也写同一份配置 → 决定是否需要"禁止双写"检测 |
| E26 | 服务管理器实际类型 | systemd unit 内容 / procd init 脚本内容，确认 Type、ExecStart、权限 |

---

## F. 对 `architecture.md` 的直接影响（需要改的地方）

### F1. §4.2 订阅策略可以定论了（原文自相矛盾处）

事实是：**dae 原生支持订阅，但只支持 base64 / SIP008 / 明文**，且原生 `group.filter` 支持 `subtag(...)`、`name(keyword: ...)` 这类过滤。

所以推荐改成两种模式，明确二选一：

- **模式 A（推荐首发）：托管原生订阅。**
  面板只写 `subscription { tag: 'url' }` 与 `group { filter: subtag(tag) && ... }`，
  节点拉取与解析交给 dae。面板的"去重 / 重命名"能力**降级**为：跨来源去重不可做，只能做组级过滤与命名规则。
  优点：实现量小、与官方行为一致、升级风险低。
- **模式 B（后续可选）：自建聚合。**
  面板自己抓取解析（需支持 Clash YAML 等更多格式）→ 只产出 `node { }` 段。
  优点：真正的跨来源去重 / 重命名 / 过滤。
  代价：要自己维护多格式解析器（等于重做一套 subconverter 能力），且同一批节点不能同时走 dae 原生订阅。

**不要在文档里继续写"优先利用 dae 原生能力 + 同时提供聚合编排"这种并列表述**，那是两条不同的数据通路。

### F2. §4.5 配置事务流程需要补两步

现有 8 步里缺：

- **新增"订阅预检"步。** 因为 `validate` 不覆盖订阅（D1），面板必须自己做：URL 可达性 + 内容能否解析 + 节点数非零。否则"校验通过 → 应用失败"会长期发生。
- **应用步改为 `dae reload` 而非重启。** 并利用 `.progress` 的 ReloadError 文本作为回滚判定依据（D10）——这比"看日志猜"可靠得多。只有 reload 不可用（如改了不支持热更的 `global` 字段）时才降级为 restart。
- **明确 ReloadBusy 的处理**：dae 自带互斥，面板不必造锁，但要把 Busy 映射成"已有变更进行中"的 API 错误。

### F3. §4.1 核心管理接口可以对齐真实命令

`CoreManager` 接口建议直接映射：

```go
Detect    → dae --version            (解析多行输出)
Validate  → dae validate -c <path>   (退出码 + stdout 错误文本)
Apply     → dae reload               (退出码 + .progress 错误文本)
Suspend   → dae suspend              (应急无负载)
Outline   → dae export outline       (缓存，用于表单与兼容检测)
Dump      → dae sysdump              (cwd 指定到临时目录)
```

注意 `Apply` 失败时**进程可能仍在跑旧配置**（需 E13 验证），所以回滚逻辑是"写回文件 + 再 reload"，不是"重启服务"。

### F4. 新增一节：配置所有权

基于 C.2 的 `include` 能力，建议明确：

> 面板默认只写 ```config.d/panel-*.dae```（node / group / route / dns 分组文件），
> 并在用户首次接管时**询问是否**在入口 `config.dae` 中加入 `include` 段。
> 面板不覆盖、不重排用户手写的既有内容。

这条一写，§4.5 的"原子写入 + 回滚"就有了清晰边界：面板只对自己的文件负责。

---

## G. 版本兼容性记录（持续追加）

| dae 版本 | 核查方式 | `validate` | `reload` | `suspend` | `export outline` | 订阅格式 | 备注 |
|---|---|---|---|---|---|---|---|
| v2.1.1 | 源码 | ✅ | ✅ | ✅ | ✅ | base64 / SIP008 / 明文 / file | 本文基准 |
| v2.1.1（实机实测） | 实机 @192.168.1.85 | ✅ | ✅ | ✅ | ✅ | base64/SIP008/明文/file(相对路径)；**Clash YAML 静默返回0节点=不支持**；订阅失败仅 WARN 非致命；**reload 拒绝0节点** | 见文末《实机验证结果 D.實測附录》 |

> 每次支持新版本时：先 `dae export outline > outline-<ver>.json` 存档，再与上一版 diff。
> 字段消失或默认值变化 → 在升级流程里提示，不要静默升级内核。

---

## H. 参考来源

| 内容 | 来源 |
|---|---|
| 仓库结构、命令源码 | `github.com/daeuniverse/dae` `main` 分支：`cmd/cmd.go`、`cmd/run.go`、`cmd/validate.go`、`cmd/reload.go`、`cmd/suspend.go`、`cmd/export.go`、`cmd/sysdump.go`、`cmd/trace.go`、`cmd/run_config.go` |
| 配置结构与订阅 | `config/config.go`、`config/outline.go`、`config/patch.go`、`common/subscription/subscription.go`、`cmd/run_controlplane.go` |
| 重载 / 挂起说明 | `docs/zh/user-guide/reload-and-suspend.md` |
| 拆分配置与 include 规则 | `docs/zh/configuration/separate-config.md` |
| 内核参数要求 | `docs/zh/user-guide/kernel-parameters.md` |
| 排障 | `docs/zh/troubleshooting.md` |
| 第三方 Web UI（共存检测对象） | `github.com/daeuniverse/daed`（A modern web dashboard for dae） |
| 最新 release | v2.1.1，2026-09-18 |

---

## I. 使用方式

1. **现在**：把 D 节 12 条坑与 F 节 4 条修改建议并入 `architecture.md`。
2. **装好设备后**：按 E 节 26 项逐条跑，把「实测」列从 ⬜ 改成实测结论（含退出码与报错原文）。
3. **E8 / E9 优先跑**：这两项决定 §4.2 走模式 A 还是模式 B，是唯一可能推翻现有架构表述的实测项。


---

## 附：实机验证结果（dae v2.1.1 @ Debian 13 / 192.168.1.85，2026-10-22）

> 环境：内核 6.12.113 x86_64；systemd dae.service（定制 unit）；机器在跑真实代理流量。
> 图例：✅ 通过/符合预期；⚠️ 有附加约束或与预期不同；— 说明性、未量化。

| # | 结论 | 关键证据 |
|---|---|---|
| E1 | ✅ v2.1.1，go1.26.8 linux/amd64，多行输出 | `dae --version` |
| E2 | ✅ `trace` 子命令存在（推翻 D8"默认没有"），`completion` 亦在 | `dae --help` |
| E3 | ✅ `run --help` 与 A.2 逐项一致 | `dae run --help` |
| E4 | ⚠️ validate 不需要 root（合法配置以 nobody 跑退出 0），但受**文件读权限**约束；本机无 sudo，提权工具是 run0/setpriv | runuser/setpriv 实测 |
| E5 | ⚠️ 非 root reload：run0 提权失败 `Failed to start transient service unit: Access denied` rc=1，**未卡住**；`--disable-sudo` 会直接因权限报错 | fork 子进程实测 |
| E6 | ✅ 坏语法 EXIT=1，stdout 带行:列号 + 脱字符 `^` | 追加坏行 validate |
| E7 | ✅ `-> notexist_group` → EXIT=1 `outbound (group) not found` | 实测 |
| E8 | ✅ **D1 盲区成立**：404 订阅配置 `dae validate` 退出 0，抓不到不可达订阅 | 实测 |
| E9 | ✅ **定论**：Clash YAML 订阅 → 静默返回 0 节点 `subscription resolved to 0 nodes`（WARN 非致命，dae 仍建控制面并加载 BPF）→ **不支持 Clash YAML，模式 A 胜出** | 受控 `dae run` 实测 |
| E10 | ✅ 缺 geo：EXIT=1，报搜索路径列表 `[配置目录, ~/.local/share/dae, /usr/local/share/dae, /usr/share/dae]` | 清空 geo 实测 |
| E11 | ✅ 退出码：合法 0 / 非法 1 / 缺 `-c` 1（stdout `Argument "--config" or "-c" is required`）；**附加：validate 主动拒绝 0644 配置，要求 0600/0640** | 实测 |
| E12 | ✅ reload 未改配置 rc=0 打印 `OK`，progress=ASCII '2'(ReloadDone)，已建立连接不中断 | 实测 |
| E13 | ✅ 坏配置 reload rc=1，progress=ASCII '3' + 行列号错误，且 **dae 仍 active 同 pid、旧配置继续跑** → 回滚=写回文件+再 reload | 实测 |
| E14 | ✅ 并发 reload：一个成功，另一个 rc=1 `another reload operation is in progress` → Busy 行为确认（dae 自带互斥） | 实测 |
| E15 | ✅ 3000 规则 + 真实节点：reload rc=0，耗时 **0.52s**；未见 60s 超时（dae 级无超时；systemd TimeoutStartUSec=2min 才生效） | 计时实测 |
| E16 | ⚠️ `dae suspend` rc=0，dae0 保持 UP（不摘钩子）；实测 1.1.1.1:443 在 suspend 后直连不通（推断 suspend→转直连，而该 IP 直连不可达）；`dae reload` 恢复 rc=0 成功——**恢复前提是配置有 ≥1 有效节点** | 实测+推断 |
| E17 | ✅ `--disable-pidfile` 下无 /var/run/dae.pid；`dae reload`(无 pid) rc=1 `failed to read pid file`；`dae reload <pid>` rc=0 → **必须显式传 pid** | 实测 |
| E18 | ✅ `kill -HUP <pid>` 无任何反应，pid 不变 → **D4 成立（SIGHUP 被忽略）** | 实测 |
| E19 | ✅ 内核 6.12.113，BPF + BTF 齐备 | 实测 |
| E20 | ⚠️ memlock soft=hard=**8192KB**（非 unlimited，unit 未设 LimitMEMLOCK）；`unprivileged_bpf_disabled=2` | 实测 |
| E21 | ✅ 拦截走 **netkit 设备对 dae0↔dae0peer（netns daens）**，非 tc-bpf；clsact qdisc 在但 tc filter 空；停服务后钩子随之清理（断网风险须按 netkit 重估） | 实测 |
| E22 | ✅ /var/run 可写；dae.pid(4B)、dae.progress 单字节码点+消息 | 实测 |
| E23 | — 默认 INFO 日志较频繁，建议 journald 轮转/限流(未精确量化写入量) | 观察 |
| E24 | ✅ 手动部署：从 /root/bundle-final 拷贝，非 apt；单文件 config | 实测 |
| E25 | ⚠️ 未装 daed；已装 **goose 0.5.18**（/usr/local/bin/goose、/etc/goose/config.yaml）。**更正（迁移实测）**：确实存在 systemd unit `/etc/systemd/system/goose.service`（`Type=simple`、`ExecStart=/usr/local/bin/goose serve --port 5889`，是个 Web 面板），当前 **disabled + inactive**（未运行）；无 timer、无 cron、无进程。退役前只是被禁用而非不存在；是 config.dae 唯一生成者 → 双写风险=若手动 `goose serve` 或 `systemctl start goose` 即从 config.yaml 覆盖 | 实测/更正 |
| E26 | ✅ 定制 unit：Type=notify、ExecStartPre=`dae validate`、ExecReload=`dae reload MAINPID`、Restart=on-abnormal、StartLimit 5次/5min（触发过启动限流，需 reset-failed 复位） | 实测 |

### 跨条目关键发现（比单条更有价值，须并入架构）

- **R1｜reload 拒绝 0 节点**：`dae reload` 对解析出 0 节点的配置会**拒绝应用**——`refusing reload with 0 nodes: all subscription resolving failed; keeping the current generation`（progress=3）。但**启动时（run）却容忍 0 节点继续运行**（仅 WARN）。这一「启动宽松 / 重载严格」的非对称必须写进 §4.5 事务流程与 §4.2——面板的订阅预检不能只看 validate（D1），还要看 reload 的 0 节点拒绝语义。
- **R2｜file:// 仅支持相对路径**：订阅用 `file://绝对路径` 会被拒（`not support absolute path`，WARN、0 节点）；路径相对**配置目录**解析（真实配置用 `file://subs/CF.sub`）。面板托管本地订阅文件时必须用相对路径 + 固定目录布局。
- **R3｜订阅失败非致命**：订阅不可达/格式不支持都只是 WARN，dae 以空组继续建控制面并加载 BPF → 单靠 dae 无法保证"可用"，面板必须在 apply 前自行做订阅可用性与节点数>0 预检（呼应 F2/D1）。
- **R4｜progress 文件格式**：首字节是 **ASCII 码点**（'2'=50=ReloadDone，'3'=51=ReloadError），换行后是消息文本。面板解析 ReloadError 文本做回滚判定时要按此格式读取。
- **R5｜suspend 恢复有前提**：suspend 后若配置解析出 0 节点，`dae reload` 同样拒绝 → 会**卡在 no-load**。应急 suspend 后必须用带有效节点的配置才能恢复；面板需对此做保护。

### 对 architecture.md 的落地映射
- §4.2：模式 A 定为首发（E9 实证 Clash YAML 不支持）；模式 B 可正式列入路线图。
- §4.5：新增"订阅预检须覆盖 dae reload 的 0 节点拒绝（R1）+ 订阅可达/节点数>0（R3）"；应用步按 reload 语义；回滚=写回+再 reload（E13）；progress 按 R4 解析。
- §4.1：Validate 增"Do not trust validate for subscription"；本地订阅路径用相对路径（R2）。
- §4.4/§13：E21 按 netkit 重估断网与诊断。
- §11 验收：新增"启动/重载 0 节点保护"与"应急 suspend 可恢复性"两项。
