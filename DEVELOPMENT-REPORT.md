# HomeNet Panel 开发报告与进度

> 整理日期：2026-10-24  
> 对应代码：`03fbaf2`（main，已推送 leoz26/dae-panel private 仓库）  
> 对应设计文档：`architecture.md` **v0.26**  
> 生产实例：`root@192.168.1.85`（Debian 13 / dae v2.1.1）

---

## 0. 一页速览

| 项 | 现状 |
| --- | --- |
| 项目定位 | dae 的**图形化管理面板**：配置事务化（预检→应用→回滚）、订阅源托管（含 Clash YAML 自动转节点链接）、审计、字节级流量观测、**接入层向导**（把流量送到 dae 面前这一层，daed 不管的部分） |
| 生产可用性 | 已在 192.168.1.85 长期运行（systemd 托管，panel + dae 双服务 active），本机流量已全部经 dae |
| 代码规模 | Go **11,160 行**（13 个包，其中 access 2,169 行）；前端单文件 `internal/api/index.html` **67,218 字节**（Vue 3 内嵌，无构建步骤） |
| 测试 | **99 个测试函数**，`go test ./...` 13 包全绿；`gofmt` 干净、`go vet` 通过 |
| API | 41 条路由（JSON API + 2 条原生表单端点 + 1 条静态资源） |
| 许可 | MIT（Copyright (c) 2026 leoz26），仓库 https://github.com/leoz26/dae-panel （private） |
| 明确未完成 | 家里网络 B2（主路由 DHCP 下发网关）真机验证、OpenWrt 适配验证、CI、密码管理器保存提示（用户暂缓） |

---

## 1. 目标演变（决策记录）

| 阶段 | 用户决策 / 目标 | 对实现的影响 |
| --- | --- | --- |
| 起点 | 按 `dae-cli-capability-checklist.md` 核对 dae v2.1.1 能力并于 192.168.1.85 实测 E1–E26 | 结论回填核对表「实测」列，并写进 `architecture.md` §13/§14 |
| 迁移 | 「goose 是前身」→ 面板**取代** goose，不共存 | 面板接管配置生成权；goose 已退役归档，最终整目录删除（/root/goose-retired 已不存在，仅留 inventory 清单） |
| 开源 | 「项目不是我一个人用，我打算开源到 GitHub」 | `architecture.md` §4.4.9 开源通用性约束：不写死网络参数/服务管理器/厂商；厂商画像只做 preset；探测失败 fail-open 到 observe |
| 接入 | 「不会考虑用 dae 当网关……openwrt 负责 DHCP，还有很多用作旁路由」 | 接入设计为**五档**（gateway / bypass-manual / bypass-router-dhcp / bypass-self-dhcp / bypass-pbr + observe），默认 **B1**；策略路由**只保留接口**（只生成片段，不远程登录主路由） |
| 对标 | 参考 daed（daeuniverse/daed） | 结论：面板+订阅+规则编辑已被 daed 做得很好；差异在它留白的一半——接线/事务回滚/审计/能力探测降级/小 flash 环境；另外 daed 不支持 YAML 规则，我们的 Clash 转换是真差异点 |
| 登录 | 「每次都要密码、提示不安全、直接输 ip:8787 打不开」 | 会话落盘 + 记住我 30 天 + 证书下载 + 同端口 301 跳 https + 首次登录强制改密 |

---

## 2. 交付物地图

| 路径 | 作用 | 规模 |
| --- | --- | --- |
| `architecture.md` | 总体架构设计（§1–§15，v0.26；含 §4.4 接入设计、§14 实现现状/踩坑记录、§15 本版变更清单） | 1,300+ 行 |
| `dae-cli-capability-checklist.md` | dae 能力核对表（A–I 节，E 节 26 项已回填实测） | 306 行 |
| `recon-probe-results.md` | 首轮只读实测报告 | — |
| `README.md` | 对外说明：快速开始、五档接入、常见疑问（HTTPS/初始密码/保存密码） | 4.8 KB |
| `homenet-panel/cmd/homenet-panel/main.go` | CLI：serve / setpass / 配置子命令 | 398 行 |
| `homenet-panel/internal/access/` | 接入向导：probe/recommend/plan/apply/verify | 2,169 行 |
| `homenet-panel/internal/api/` | HTTP 层 + 内嵌前端（server.go 1,171 行、index.html 67 KB） | 2,000+ 行 |
| `homenet-panel/internal/{model,configtx,preflight,dae,route,geo,subs,subsrc,clashconv,audit,auth,traffic,runtimecheck}` | 各功能内核 | 约 4,500 行 |
| `homenet-panel/docs/access-test-b2.md` | **B2 家里真机测试手册**（步骤/预期/回滚，待执行） | — |
| `homenet-panel/tools/netdiag/` | 流量走向诊断脚本（flowprobe/flowsniff/flowwho/pubip/whereami + README） | 6 个 py |
| `LICENSE` | MIT | — |

---

## 3. 功能进度表

图例：✅ 已实机验证上线 ｜ 🔧 已实现但有明确局限 ｜ ⬜ 未开始

### 3.1 配置与内容层

| 功能 | 状态 | 说明 / 证据 |
| --- | --- | --- |
| 运行态读取（service/pid/progress） | ✅ | `/api/status`；progress 码 2=ReloadDone 3=ReloadError |
| 配置事务（预检→备份→写入→dae reload→失败回滚） | ✅ | 写回文件 + 再 reload 即回滚（reload 失败不改变运行态，已实测） |
| 结构化模型编辑（分组/规则/DNS） | ✅ | `/api/model`、`/api/model/preview`、`/api/model/apply` |
| 安全直连规则（内网/回环/本机进程 → direct） | ✅ | `model.EssentialRules` 自动前置注入，修掉「本机 DHCP/DNS 被代理」的真 bug |
| 整段配置（高级）编辑 + 语法预检 | ✅ | `/api/outline` / `/api/preflight` / `/api/validate` |
| 接线校验（只写自有 config.d/panel-*.dae） | ✅ | 双写检测；`/etc/dae/config.dae` 只保留 include 行不变 |

### 3.2 订阅层

| 功能 | 状态 | 说明 |
| --- | --- | --- |
| 订阅节点解析（base64/SIP008/明文） | ✅ | `internal/subs` |
| Clash / Mihomo YAML → 节点链接转换 | ✅ | `internal/clashconv`；真实样本回归 **29/29 节点、0 跳过**，与 goose 当年生成的镜像一致（vmess 仅冗余键差异） |
| 订阅源登记（URL / per-source UA / 拉取格式） | ✅ | `/api/subsrc`、`/api/subsrc/url`；解决 dual 上游 **403（UA 限制）** |
| 新鲜度巡检（只报告差异，不自动改配置） | ✅ | 默认每 6 小时；`/api/subsrc/check`；拉取缓存 120s 避免上游限流 |
| 应用（备份→写入→reload→失败恢复） | ✅ | `/api/subsrc/apply`；用户已在 14:24–14:25 真实点击应用过 CF/dual |
| 分组（policy/filter）与自建聚合（模式 B） | ✅ | `/api/subs/groups`、`/api/aggregate/*` |
| 节点测速 | ✅ | `/api/subs/speedtest`（并发 TCP 建连延迟） |
| 订阅变更提醒（顶部 pill + 差异明细） | ✅ | `/api/status.subsrcChanged` |
| 订阅自动应用（无人值守） | ⬜ | 设计上刻意不做：只报告，人工确认后再应用 |

### 3.3 观测与审计

| 功能 | 状态 | 说明 |
| --- | --- | --- |
| DNS 命中 / 流量命中 | ✅ | `/api/traffic/hits`、`/api/traffic/sources`（Linux raw socket 旁路抓包 DNS） |
| 字节级流量统计（按节点/按客户端） | 🔧 | `/api/traffic/bytes`：轮询 ss -tunip 的 dae socket 计数 + /proc/net/dev 的 dae0 总量。**口径为下限值**（采样间隙会漏）；未用 eBPF 直读（map 布局随版本易碎 + 零第三方依赖） |
| 审计日志（JSONL、按大小轮转、CSV/JSONL 导出） | ✅ | 5 MB × 保留 3 份；`/api/audit`、`/api/audit/export`；含登录/改密/apply/rollback/订阅操作 |

### 3.4 接入向导（核心差异点）

| 功能 | 状态 | 说明 |
| --- | --- | --- |
| P-A 只读能力探测 | ✅ | `/api/access/probe`：平台/服务管理器/BPF/BTF/memlock/nft/默认路由/dae 版本与 lan_interface/端口占用/遗留 nft 表 |
| 选档推荐 + 向导三问 | ✅ | `/api/access/recommend`：默认 B1；唯一升档开关是「DHCP 能否下发网关」 |
| P-B 计划生成（dry-run） | ✅ | `/api/access/plan`：面板侧步骤 + 可粘贴片段 + 人工步骤 + 回滚步骤 + 自检清单 + 计划指纹 |
| 只读自检 6 项 | ✅ | `/api/access/verify`：dae 运行 / LAN 客户端 / dae0 计数 / DNS 命中 / /etc/config 未被改 / 遗留表 |
| P-C 策略路由片段生成 | ✅ | `RenderPBR`：只生成 ip rule/nftset 片段，**不远程登录主路由** |
| P-B 应用 / 回滚（本机侧） | ✅ | `/api/access/apply`、`/api/access/rollback`：写自有 config.d/panel-access.dae + 建自有 nft 表 + reload + 失败回滚 + 指纹校验 + 幂等；实测已跑通并回滚干净 |
| B2（主路由 DHCP 下发网关）真机验证 | ⬜ | 手册已写（`docs/access-test-b2.md`），**等回家网络执行**；公司网络只做 observe/dry-run |
| A 档（OpenWrt 同机即网关） | ⬜ | 需 procd 适配 + QEMU x86_64 验证，未开始 |
| B3（旁路由自建 DHCP） | ⬜ | 设计为「默认关闭 + 30 分钟自动失效 + 显式授权」，暂不实现 |

### 3.5 鉴权与前端

| 功能 | 状态 | 说明 |
| --- | --- | --- |
| 账号密码登录（scrypt 120k iters） | ✅ | 取代早期静态 Bearer token |
| 会话持久化（重启不掉登录） | ✅ | `/var/lib/homenet-panel/sessions.json`（0600，只存 token 摘要） |
| 记住我 30 天 | ✅ | DefaultSessionTTL 12h / RememberSessionTTL 30d |
| 首次登录强制改密 | ✅ | MustChange 门禁 + `/api/passwd` + 口令强度 + 前端改密卡片 |
| 原生表单登录（POST /login、POST /passwd） | ✅ | 为浏览器密码管理器做真导航提交；已在临时实例验证 4 类跳转码 |
| 同端口明文 301 跳 https + 明文可选放行 | ✅ | serveDual + connChan/splitConns；`--allow-plain-http` 默认关 |
| 证书下载（/panel.crt） | ✅ | 自签证书（CN=homenet-panel，SAN 含 192.168.1.85/127.0.0.1/localhost，有效期至 2029-01-25） |
| 浏览器「保存密码」提示 | 🔧 | 表单语义已修正；仍受**证书未受信任时 Chrome 禁用密码管理器**影响，用户暂缓处理 |
| 前端工程化（Vite/组件拆分） | ⬜ | 现在单文件 67 KB、Vue 3 内嵌、无构建步骤（刻意为之：零 CDN 依赖、离线可用） |

---

## 4. API 一览（41 条）

| 分组 | 路由 |
| --- | --- |
| 鉴权 | POST /api/login、POST /login(表单)、POST /api/logout、GET /api/me、POST /api/passwd、POST /passwd(表单)、GET /panel.crt |
| 运行态 | /api/health、/api/status、/api/detect、/api/runtimecheck |
| 配置事务 | /api/outline、/api/preflight、/api/validate、/api/apply、/api/rollback、/api/suspend |
| 模型 | /api/model、/api/model/preview、/api/model/apply |
| 订阅/分组 | /api/subs/nodes、/api/subs/groups、/api/subs/speedtest、/api/aggregate/preview、/api/aggregate/build、/api/aggregate/switch |
| 分流调试 | /api/route/test |
| 流量 | /api/traffic/hits、/api/traffic/sources、/api/traffic/bytes |
| 订阅源 | /api/subsrc、/api/subsrc/url、/api/subsrc/check、/api/subsrc/apply |
| 接入向导 | /api/access/probe、/api/access/recommend、/api/access/plan、/api/access/verify、/api/access/apply、/api/access/rollback |
| 审计 | /api/audit、/api/audit/export |

---

## 5. 生产运行态（192.168.1.85，采集于 2026-10-24 00:36）

| 项 | 值 |
| --- | --- |
| 系统 | Debian GNU/Linux 13 (trixie)、kernel 6.12.113+deb13-amd64、systemd |
| dae | v2.1.1（go1.26.8）、dae.service active，跑在 host netns，拦截侧 netkit 设备对 dae0↔dae0peer（netns daens） |
| 面板 | /usr/local/bin/homenet-panel（12,890,120 B，静态链接）、homenet-panel.service active |
| 启动参数 | serve --addr 0.0.0.0:8787 --backup-dir /var/lib/homenet-panel/backups --tls-cert /etc/homenet-panel/tls.crt --tls-key /etc/homenet-panel/tls.key --traffic-iface ens2 |
| 访问方式 | https://192.168.1.85:8787（自签证书；明文自动 301 跳转） |
| 配置不变量 | /etc/dae/config.dae md5 7de9781a07f51041bb9ce866228baf2e；/etc/dae/config.d/panel-core.dae md5 4ef4d2a0b73b325882c7044ea53da6d7（1062 B / 0600，仅此一个自有片段） |
| 内核状态 | nft list tables 为空（无残留自有表）；ip_forward = 0（未接管 LAN；apply 时会设置并在回滚时还原） |
| 账号 | admin，mustChange = False（用户已自行改密；密码由用户掌握，本报告不记录） |
| 订阅源 | CF → format=links；dual → format=clash, UA=clash-verge/v1.7.7；巡检 6 小时，lastRun 2026-10-23T15:55Z |
| 数据目录 | /var/lib/homenet-panel/{audit.log 18 KB, sessions.json 1,057 B, subsrc.json, backups/} |
| 磁盘 | 37 G 用 1.8 G（6%） |

**部署流程（当前手工）**：本机 GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/homenet-panel-linux-amd64 ./cmd/homenet-panel → scp 到 /usr/local/bin/homenet-panel.new → systemctl stop homenet-panel && mv && chmod 755 && systemctl start homenet-panel。

---

## 6. 关键实测记录（证据索引）

| 主题 | 结论 | 证据位置 |
| --- | --- | --- |
| dae CLI 能力 E1–E26 | v2.1.1 有 trace/completion；validate 抓不到 404 订阅（D1 盲区）；坏语法带行列号；缺 geo 报搜索路径列表 | 核对表 E 节 + architecture.md §13/§14 |
| reload 语义 | 失败不改运行态（旧配置继续跑）→ 回滚=写回+再 reload；并发 reload 有 Busy | §4.1/§4.5、§14 |
| 本机流量被代理（真 bug） | journal 出现 proxy_addr=192.168.1.1（DHCP 68→67 走香港节点）→ 加 EssentialRules 后该告警 0 次 | §14.14 |
| 订阅 UA 限制 | 缺 UA → HTTP 403；UA=clash-verge/v1.7.7 → 200；与 HTTP/2 无关 | §14.12 附近、subsrc 代码 |
| Clash YAML 转换 | 真实样本 29/29 一致，vless 参数逐字相同 | clashconv 回归测试（env 门控） |
| 上游限流 | 短时间重复拉取会被挂起 → 120 s 拉取缓存 + 友好提示 + token 打码 | subsrc 实现 |
| 字节统计 | 44 行节点统计、dae0 累计约 726 KB/375 KB、clients 为空（当时无 LAN 设备经本机） | §14.15 |
| 接入向导 P-A | memlock 8192 KB、ip_forward=0、daeLanIfaces=null、busyPorts={68:udp}、推荐 B1 | §14.16 |
| 接入 apply | 计划指纹 a074ea50e148d531；apply 实测「5 条自有 nft 规则 + reload 成功」，rollback 后表删净、ip_forward 归零 | §14.17、审计日志 |
| 登录体验 | 会话落盘后重启不掉登录；明文 301；证书可下载；强制改密 403 门禁生效 | §14.19、§14.20 |

---

## 7. 已知问题与风险

| # | 问题 | 影响 | 处置 |
| --- | --- | --- | --- |
| 1 | Chrome 仍不弹「保存密码」 | 体验 | **用户暂缓**；真因两条：fetch 登录无导航（已修）+ 自签证书未受信任时 Chrome 禁用密码管理器（需装证书或改走 http） |
| 2 | B2 未真机验证 | 接入闭环 | 手册 docs/access-test-b2.md 已就绪，等家里网络；公司网络只做 observe/dry-run |
| 3 | memlock 仅 8192 KB（非 unlimited） | dae BPF map 上限 | 已记录；建议 unit 加 LimitMEMLOCK=infinity（需单独评估） |
| 4 | 字节统计是**下限值** | 数据精度 | 口径已在文档标红；需要精确值再考虑 eBPF |
| 5 | 单用户模型、无 RBAC | 多用户场景 | 开源前需补（会影响审计 actor 语义） |
| 6 | 审计日志明文、仅本地 | 合规 | 可选：远端 syslog/对象存储推送 |
| 7 | 自签证书 2029-01-25 到期 | 长期运行 | 需加「证书生成/续期」命令与到期提醒 |
| 8 | 无 CI、无 release 产物 | 协作 | 建议 GitHub Actions：gofmt/vet/test + 交叉编译 amd64/arm64 |
| 9 | 前端单文件 67 KB | 维护性 | 现阶段刻意（零 CDN、离线可用）；后续可迁 Vite 但需保留单文件产物 |
| 10 | 订阅「只报告不自动应用」 | 自动化 | 设计选择（安全优先）；如需自动化应做成显式开关 + 审计 |

---

## 8. 下一步建议（按优先级）

1. **P0 — B2 家里真机验证**：按 homenet-panel/docs/access-test-b2.md 执行；重点验证「主路由 DHCP 下发网关＝旁路由」后：内网设备流量是否真的进入 dae（/api/traffic/bytes 的 clients 出现内网地址）、DNS 劫持是否命中、/etc/config/* 是否零改动、一键回滚是否干净。
2. **P1 — 工程化**：GitHub Actions（gofmt/vet/test + 交叉编译）、release 二进制与安装脚本、systemd unit 模板。
3. **P2 — OpenWrt 适配**：QEMU x86_64 起 OpenWrt 验证 procd/小 flash/无 systemd 路径；这决定 A 档能否落地。
4. **P2 — 多用户与权限**：至少区分「只读观察者 / 操作员」，否则公开仓库后不适合多人共用一台。
5. **P3 — 证书生命周期**：内置 cert 子命令（生成/续期/导出指纹）+ 到期提醒。
6. **P3 — 密码管理器体验收尾**：证书信任引导页（按平台给步骤）+ 可选的 --allow-plain-http 内网模式。

---

## 9. 提交历史（7 次）

| commit | 内容 |
| --- | --- |
| 24136ae | chore: 初始化仓库（基线快照） |
| fa826ac | feat(access): 接入计划生成与只读自检（P-B）+ README/LICENSE(MIT) |
| c7768d7 | feat(access): 接入 apply/rollback + B2 测试手册 + 文档 v0.22 |
| 1892ddf | feat(auth): 会话持久化 + 记住我30天 + 证书下载 + 同端口明文301跳https；文档 v0.23 |
| f0044c9 | feat(auth): 首次登录强制改密；文档 v0.24 |
| d3347ee | fix(ui): 登录/改密改用真实 form + autocomplete；文档 v0.25 |
| 03fbaf2 | feat(auth): 登录/改密改为原生表单提交（/login、/passwd + 303）；文档 v0.26 |

---

## 10. 文档索引

- architecture.md（v0.26）：§1 目标 / §2 总体架构 / §3 目录结构 / §4 模块设计（§4.4 接入设计、§4.5 事务、§4.7 迁移、§4.8 goose 共存风险）/ §5–§7 数据与接口 / §8 迁移里程碑 / §11 验收 / §13 dae 坑清单 / §14 实现现状与踩坑（§14.1–§14.20）/ §15 本版变更清单。
- dae-cli-capability-checklist.md：dae 能力与实测回填（A 命令 / B 路径契约 / C 配置模型 / D 12 条坑 / E 26 项实测 / F 建议）。
- recon-probe-results.md：首轮只读实测原始记录。
- homenet-panel/docs/access-test-b2.md：B2 真机测试手册（待执行）。
- homenet-panel/tools/netdiag/README.md：流量走向诊断脚本用法与判定依据。
- README.md：对外快速开始与常见疑问。
