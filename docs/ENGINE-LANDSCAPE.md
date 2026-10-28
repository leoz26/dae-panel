# 引擎版图观察：dae / honk / doona（2026-10-28 核对）

> 本文只记录事实与判断，不构成迁移决定。数据来自 GitHub REST API 与项目自带文档（行内标注来源文件）。
> 一句话结论：**honk 值得盯，但现在是 alpha，本项目不迁移**；doona 与我们定位不同（它需要 honk 的 native API；daed 与 doona 都不做「接入层」与「事务化文件真源」）。

## 1. 三个项目是什么

| 项目 | 角色 | 事实 |
| --- | --- | --- |
| [dae](https://github.com/daeuniverse/dae) | 引擎（Go，AGPL-3.0） | 6349★；最新稳定版仍是 **v2.1.1**（2026-09-18）；仓库最后 push 2026-09-30。本项目的目标引擎。 |
| [honk](https://github.com/daeuniverse/honk) | 新引擎（Rust，GPL-3.0） | 360★ / 35 fork；2026-07-20 建仓，最后 push 2026-10-26；最新 **v0.1.0-beta.3**（2026-10-27，prerelease）。自述「受 dae 与 sing-box 启发，独立实现，不是逐行移植」。 |
| [doona](https://github.com/Zakkaus/doona) | Web UI（TypeScript/React，GPL-3.0） | 128★；2026-09-15 建仓，最后 push 2026-10-28；官方 demo demo.daeuniverse.org。**必须连 honk 且 honk 开启 native API**；README（繁中）明写「目前不支援 dae」。 |

honk 与 doona 是**两个独立仓库**：honk 是引擎，doona 是它的前端；honk 可以 `--features native-ui` + `ui: embedded` 把 doona 打进去（honk README「Opt-in native observation API」段）。

## 2. honk 的硬要求（决定谁能用）

来源：`doc/en/how-to-start.md`、README。

- **Linux 6.12+ 硬门槛**：低于 6.12 在 attach 之前就被拒绝（“Older kernels are rejected before attachment”）。对比 dae：6.6 上能用（无 netkit 时回退 veth，已在 iStoreOS 6.6.144 验证）。
- 内核选项：`CONFIG_BPF`、`BPF_SYSCALL`、`BPF_JIT`、`CGROUP_BPF`、`NET_SCHED`、`NET_CLS_ACT`、`NET_XGRESS`、`NET_NS`；默认开启的 held-first-packet UDP 还要 `NFT_CT`、`NFT_QUEUE`、`NETFILTER_NETLINK_QUEUE`。
- **cgroup v2 + 根 cgroup 的 sock_ops 槽位**：若有别的程序（如 `bpftool cgroup attach` 装的 legacy 程序）占着，honk 直接拒绝启动。
- 架构：`x86_64` / `aarch64`，gnu 或 musl；**不支持 MIPS**，也没有 Windows/macOS datapath。
- 数据面：TC eBPF + `dae0`/`daens` + **NFQUEUE 320**；netkit 可选（不支持时回退 veth）。**没有 `tproxy_port`**（dae 是 tproxy 12345）。
- 明确行为：「honk does not insert local-interface routing rules」（routing 参考首段）——内网直连要用户/面板自己写，这一点与 dae 一致。

## 3. honk 的优势（对本项目有意义的）

1. **配置仍是 dae 语法**：`include {}`、`global`、`group`、`routing`、`dns`、`subscription`、`assets`、`experimental` 八段齐备（`doc/en/configuration.md` 总表）。我们的渲染器与模型大体可复用。
2. **匹配器几乎全覆盖**：`domain`（`suffix:`/`keyword:`/`full:`/`regex:`/`geosite:`）、`dip`（含 `geoip:`）、`sip`、`dport`、`sport`、`l4proto`、**`pname`**（argv[0] basename，15 字节归一；无 BTF/argv 能力时用线程 comm）、`mac`、`ipversion`、`dscp`。我们的 `EssentialRules`（`pname(homenet-panel, dae, …)`）在 honk 里语义仍成立。
3. **订阅原生支持 Clash YAML**：`sub_type: simple` 自动识别「share-link 列表 / Clash YAML·JSON / SIP008 / sing-box JSON」；`sub_type: clash` 直接按 `proxies` 序列解析；每条订阅可单独设 `ua`、`interval`、`cache`、`route`（`doc/en/reference/subscription.md`）。**这意味着 `internal/clashconv` 的差异化只对 dae 成立**。
4. **DNS 上游齐全**：UDP/TCP/DoT/DoH/DoQ/DoH3，可走节点或组、带路由与缓存。我们为 DoT 853 被掐折腾的那一整轮（改 UDP 53 / DoH）在 honk 里是一个配置项。
5. **有原生 API**：`/api/v1`（bearer/密码登录）、Clash 兼容 REST/WS API、按网络的 Selector 切换、精确关闭连接、flows/logs/DNS 历史、provider 刷新、geodata 校验后更新、可选「配置即源码」的 PATCH 事务、`--store db` 按修订存配置。
6. **组策略更多**：Selector / URLTest / LoadBalance / Fallback / **Score**（`policy: score`），且 `min_moving_avg`、`min_avg10` 也在（`doc/en/reference/groups.md`）。
7. **CLI 与我们现有调用面接近**：`honk-core` 有 `validate`、`reload`、`check`、`status`、`version`；另有 `honk-tool` 诊断工具箱。

## 4. honk 的劣势与风险（为什么现在不迁）

1. **官方自我定性是 alpha**：「Early alpha, not recommended for production. Expect breaking changes, incomplete features, and limited real-world validation.」
2. **已有破坏性变更先例**：README 明写 VLESS 升级时 `vless_mode` 被移除、**所有 VLESS 节点 ID 重新推导**，并要求迁移静态链接与缓存订阅内容。
3. **多数用户态子系统是 AI 编写、部分未经审阅**：README「Review status」勾选表里 `Configuration parser (dae extensions)`、`Trojan-GFW` 仍未勾；且写明在所有未审代码处理完之前**不会发 `test.1` 标签**。TODO 还有「评估 AF_XDP/XDP」「加 honk 专属 REST API」「加 inbound」。
4. **缺功能**：没有 FakeIP 引擎、没有完整 mihomo 兼容、没有 Windows/macOS 数据面。
5. **内核门槛把 OpenWrt 大部分设备挡在门外**：iStoreOS 24.10 是 6.6.x（我们那台 6.6.144），honk 直接拒绝；要 25.x 或自编 6.12+ 且开 `NET_XGRESS`/`NFT_QUEUE`。
6. **许可**：honk 是 GPL-3.0（dae 是 AGPL-3.0），本项目 MIT。只要**不链接**（走配置文件与 HTTP API），MIT 不受影响；一旦嵌入/链接就传染。
7. **生态位置在变**：dae 上游能量明显向 honk 转移（dae 稳定版停在 v2.1.1、honk 一周三个 beta），但 dae 仍有 6349★ 存量与稳定行为，短期不会消失。

## 5. 与 doona 的关系：它是另一种面板，但不是我们的替代品

| 维度 | doona | 本项目 dae-panel |
| --- | --- | --- |
| 后端依赖 | **必须** honk 开 native API（`experimental.native_api`），当前不支持 dae | 直接驱动 dae：写文件 + `dae validate` + `dae reload` |
| 真源 | 后端 API / honk 的 db 或源码事务 | **文件即真源**（`config.d/panel-*.dae`），可被任何编辑器接续 |
| 事务 | 依赖后端 API 的 PATCH/PUT 语义 | 自带五步事务：渲染 → validate → 备份 → 写盘 → reload → 失败回滚（含进度码判定） |
| 审计 | README 功能列表未提 | JSONL 审计（登录/应用/回滚/订阅/导出），按大小轮转 + CSV/JSONL 导出 |
| 接入层 | 不做（DHCP/转发/SNAT/DNS 劫持、档位推荐、计划与自检） | 做（`internal/access`：探测 → 推荐 → 计划 → 应用 → 回滚 → 自检） |
| Clash YAML 订阅 | 不需要（honk 自己会解析） | 需要且已实现（`internal/clashconv`，与 goose 镜像 29/29 一致） |
| 小 flash / 无 Node 构建 | 静态前端，但要 honk（6.12+） | 单二进制内嵌前端，`CGO_ENABLED=0` |
| 许可 | GPL-3.0 | MIT |

**判断**：doona 覆盖的是「节点/分组/规则/配置的可视化编辑」——正是 honk native API 已能做的那一半；我们真正独占的是「接入层 + 事务化文件真源 + 审计 + 能力探测降级 + 小设备适配」。doona 出现不构成「不用做了」，但**削弱了纯粹做配置编辑器的价值**。

## 6. 对我们的三条影响

1. **不改当前路线**：dae 仍是唯一「6.6 也能跑、存量最大」的引擎；把接入层与事务做扎实，收益大于追 alpha。
2. **保留可切换的缝**：`internal/model` 渲染的是 dae 语法，honk 也是 dae 方言，将来加 honk 后端的成本集中在**三处**——`global` 里的 `tproxy_port`（honk 无此键）、健康检查键的落点、以及「validate/reload/进度码」这套 dae 二进制耦合（honk 有 `validate`/`reload`，但没有 `/var/run/dae.progress` 语义）。
3. **盯三个信号**再谈迁移：honk 打 `v1.0`（或至少 `test.1` 正式标签）、Review status 全勾、dae 停止发版超过一个季度。

## 7. 核对来源与时间

- 核对时间：2026-10-28。
- 数据源：GitHub REST API（repos / releases）+ honk 仓库 `README.md`、`doc/en/how-to-start.md`、`doc/en/configuration.md`、`doc/en/reference/{routing,groups,subscription,cli,dialect}.md` + doona `README.md`。
- 相关章节：`architecture.md` §14.50（dae 健康检查与选路，含 daed/NaiLink 对照）、§14.53（本条摘要）。
