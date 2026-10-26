# HomeNet Panel：总体架构与开发结构设计

> **文档版本：** v0.47  
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
> **修订（v0.47）：** 新增 §14.38「构建矩阵扩到 linux/armv7 与 linux/386」：CI 用 `产物名:GOARCH:GOARM` 四元组交叉编译，`online-install.sh` 的 `uname -m` 映射补 armv7/386、armv6 明确拒绝，并说明为什么不做 armv6 与 mips（dae 官方 deb 其实有 x86_32/armv7 包）。
> **修订（v0.46）：** 新增 §14.37「结构化 DNS 编辑器」：`internal/model/dns.go` 的结构化视图（Parse/Render/Lint）+ `/api/dns/parse`、`/api/dns/render` + 前端「DNS」标签页；校验规则来自 .85 上逐条 `dae validate` 实测（dns 的 qname 不支持 `geolocation:`；动作只有 `reject`，`accept` 仅限响应规则）。`Model.DNS` 仍为原样文本，保存后走同一 apply 事务。
> **修订（v0.12）：** 新增分流命中调试器（geosite 解码 + 规则本地求值，§14.10）；分流规则可视化编辑增强（排序、出站联表、未保存规则试算，§14.11）。
> **修订（v0.13）：** 修复 v0.12 尾部截断（§13 的 D8–D12 与整节 §14 丢失，已按实机实测与现有代码重建）；并新增 §14.12「流量 / 命中统计（DNS 旁路）」记录 ③ DNS 查询频度统计与 ② eBPF 预留接口的实现、口径与开销实测。
> **修订（v0.14）：** 新增 §14.13「订阅变更提醒 + 审计日志导出与保留」：状态栏订阅变化红点与差异明细（只提醒、人工应用）、CSV/JSONL 审计导出、按大小轮转与可配置保留份数（含路由器/NAS 磁盘口径建议），并修复 `/api/audit` 死路由（此前前端审计页 404）。  
> **修订（v0.15）：** 流量路径实测（脚本见 `tools/netdiag/`）发现两大事实：① dae 的 `fallback: proxy` 把**本机 DHCP/DNS 与面板自身出站**也代理了（`lAddr=192.168.1.85:68 … proxy_addr=192.168.1.1:67`），已由面板强制注入「安全直连规则」（§14.14）；② 当前**没有任何 LAN 客户端经本机上网**，Mac 网关是 192.168.1.11。并更正 §14.12 的「可疑 DNS 心跳」误判。  
> **修订（v0.16）：** ① goose 存量归档到 `/root/goose-retired/`（315MB，mv 不删，含恢复说明）；② 新增 §14.15 字节级流量统计（方案 B：`ss` socket 计数器 + `/proc/net/dev` 的 `dae0` 总量；明确不用 eBPF 的理由）与前端「字节统计」tab，实机验证通过。  
> **修订（v0.17）：** §4.4 重写为「网络接入与 LAN 接管」：两类流量命运、三张流程图（网关/旁路由/旁路由+策略路由）、**五种接入档位**（gateway / bypass-manual / bypass-router-dhcp / bypass-self-dhcp / bypass-pbr）+ observe、自动选档算法（含中兴光猫与中兴路由的默认假设）、`AccessProfile`/`Plan`/`PBRSpec` 数据模型（策略路由作为保留接口，只出片段不远程执行）、"只写自有 nft 表"的边界、七项自检矩阵、P-A…P-E 实施阶段与风险红线。  
> **修订（v0.18）：** §4.4 去厂商耦合（面向开源）：
> **修订（v0.19）：** 实现 §4.4.7 的 P-A 阶段——新增 `internal/access`（只读能力探测 + 选档推荐）与 `/api/access/probe`、`/api/access/recommend`、前端「接入向导」tab；实机（192.168.1.85）跑通并修掉 3 个真机 bug（`dae version` 子命令不存在 / 未展开 `include` / `nft list tables` 字段数假设），详见 §14.16。
> **修订（v0.20）：** 补入 §4.4.3 的现场实测样本：中兴家用固件「地址管理」页提供可编辑的**「自定义网关地址」**（= DHCP Option 3），说明"家用路由普遍改不了 DHCP 网关"不成立 → B2 在部分家用固件上可行；同时记录"该值可能影响路由器自身出口、填错会整段断网"的风险与操作顺序。详见 §4.4.3。  
> **修订（v0.21）：** 落地接入向导 P-B（只出计划与只读自检，不执行）：新增 §14.17，定稿"默认 B1、其余档位仅留生成入口"，并新增 README/LICENSE（MIT）用于开源。
> **修订（v0.22）：** 接入 apply/rollback 落地（写自有片段 + 建自有 nft 表 + reload + 失败回滚 + 幂等 + 指纹校验），并在 192.168.1.85 完成段1「本机事务测试」：发现并修掉 4 个真 bug（inet 表需 `dnat ip to`、重复 apply 会叠加规则、`ip_forward` 被 dae 置 1 后回滚不还原、回滚把「表不存在」当失败）；段2「B2 端到端测试」写成 `homenet-panel/docs/access-test-b2.md`。详见 §14.18。
> **修订（v0.23）：** 登录体验与 HTTPS：会话持久化（重启不掉登录）+ 「记住我 30 天」+ cookie `Secure` 按请求协议判定 + `/panel.crt` 证书下载 + 同端口协议分流（明文 `http://ip:8787` 自动 301 跳到 https，不再报 400）。详见 §14.19。  
> **修订（v0.24）：** 新增 §14.20「首次登录强制改密」：凭据加 `MustChange` 标记、auth 中间件 403 门禁（白名单 /api/me、/api/passwd、/api/logout、/api/health）、`POST /api/passwd` 与口令强度规则、`setpass --must-change`、前端改密卡片；实机 403 与浏览器渲染均验证通过。
> **修订（v0.25）：** 登录/改密表单补 form + name + autocomplete（让浏览器弹出「保存密码」并支持自动填充），补充 §14.20 说明与初始密码 10086 的例外口径。
> **修订（v0.26）：** 登录/改密改为**原生表单提交**（`POST /login`、`POST /passwd` + 303 跳转），解决 fetch 登录拿不到「保存密码」提示；补充证书未受信任时 Chrome 禁用密码管理器的说明（§14.20）。
> **修订（v0.27）：** 14.21 转发开关改由面板持久化（internal/access/sysctl.go，应用时打开并写 /etc/sysctl.d，回滚按原值还原）+ clashconv 支持 Clash 行内映射与 ssr 链接转换（实测 61/61）。
> **修订（v0.28）：** 转发改「有条件才开」（配置声明 lan_interface / dae reload 报 ipforward off 两条触发，自动重试一次），实机验证死结解除。
> **修订（v0.29）：** 修面板两处「点了没反应」：`--c` 未传入 `api.Config.EntryPath`（真 bug，本地/换路径场景会读错配置）；内容层加顶部「校验+备份+reload 应用」条并写明没有单独保存按钮；订阅源更新页新增「新增订阅源」+ 后端为新登记上游自动分配默认镜像 `subs/<name>.sub`。详见 §14.22。
> **修订（v0.30）：** 一次性订阅 token 的正确姿势（镜像而非 dae 直连）+ 上游原始 YAML 留档（`backups/subs-raw/`）+ 订阅源登记项删除接口与按钮（模型引用中的拒绝删除）。详见 §14.23。
> **修订（v0.31）：** 修两个订阅转换缺陷（alpn 嵌套列表被当作节点 → 40 个幽灵；anytls 协议未转换 → 20 个真节点丢失，已按 dae 的 outbound 解析实现补齐）；内容层订阅来源改为下拉选择。详见 §14.24。
> **修订（v0.32）：** 组策略受控 A/B（同机场同机、只改 proxy 组、结束已恢复）：baseline(CF+dual 全节点) L3 成功 31%，只用香港 89%，香港固定节点 93%，`!日本` 8%；并确认 dae 的 `name(keyword: …)` 是**子串**不是正则（`'香港|新加坡'` 能过 validate 但匹配不到任何节点）。结论：延迟最小策略会被「端口开但数据面丢包」的远端节点骗到，组内混入远端节点是 69% 丢包的主因。详见 §14.25。  
> **修订（v0.33）：** 按 A/B 结论把 `.85` 的 `proxy` 组正式改为「只用香港」（备份 `/root/ab/panel-core.dae.before-hkonly-20261025-152457`，md5 `393cc4f6db0abad99671522148b36d96`）；补测「reload 冷却期」——刚 reload 后立刻测是 0%，等 90 秒再测 baseline 28% / hk_only 92%，说明**应用配置后前几分钟的失败不能算进配置质量**。详见 §14.25.7。  
> **修订（v0.34）：** v2rayN 规则集实测迁移（UDP443/广告拦截/AI 分组，见 §14.26）；发现并修复 model.json 与线上配置漂移，新增漂移检测与「从线上重新解析」；AI 规则实测由 sg（0/6）改为 hk（6/6）。
> **修订（v0.35）：** 新增一条命令部署（`deploy/deploy.sh` 开发机侧 + `deploy/install.sh` 目标机侧，幂等、支持 --dry-run、systemd/procd 自动识别），实机验证见 §14.27。
> **修订（v0.36）：** 安装器补依赖预检：`--install-dae` 按 APT 主版本加 daeuniverse 官方源装 dae，`--retire-goose` 把 goose 移到 `/root/goose-retired`（移动不删除）+ systemd unit 加 `LimitMEMLOCK=infinity`；新增 `deploy/verify-install.sh` 只读自检。  
> **修订（v0.37）：** 修复单文件布局解析（goose 生成的 `/etc/dae/config.dae` 无 `include` 时读不到主体配置，界面 0 规则、点「应用」会静默清空线上分流规则）+ 空规则应用安全闸；`/api/model` 增加 `layout`/`layoutNote`，新增 §14.29 与两条回归测试；发布 v0.35.4。  
> **修订（v0.38）：** 补齐 goose 的完整残留面（`goose-update-geo.{service,timer}` 这类额外 unit 也会被检测并退役）+ 新增 daed（第三个写 dae 配置的面板）告警；n6 的 `/root/goose-retired` 已在留档后整目录删除，详见 §14.28.1。  
> **修订（v0.45）：** 接入计划的四块面板侧步骤新增**白话说明**（`Step.Note`：门岗比喻、门牌号 vs 收件人姓名、为什么通常不需要 SNAT、为什么不自建 tproxy），前端渲染为提示框（§14.36）。  
> **修订（v0.44）：** 应用配置时若 dae 没在运行，面板不再把 `dae reload` 的 CLI 用法当错误抛出，而是明确提示「dae 没有在运行」并回滚 + 指路「安装与初始化」页（`ErrDaeNotRunning` / `DaeDownHint` / `explainDaeOut`，见 §14.35）。  
> **修订（v0.43）：** 在线安装改为 **CDN 优先**：CI 在发布 Release 后把产物推到 `dist`（可变，含 `VERSION`）与 `<tag>-dist`（不可变）两个普通分支供 jsDelivr 转售；`deploy/online-install.sh` 重写为「版本解析（@dist/VERSION → GitHub API）+ 多源顺序回退（jsDelivr/fastly/gcore/raw/releases）+ 带 --http1.1 与重试的下载器 + sha256 校验」。实测补充：jsDelivr **只索引 `dist` 分支**，版本号样式的 `<tag>-dist` 分支名会被当成版本解析而 404，故钉版本走 `raw.githubusercontent.com/<tag>-dist`（见 §14.34.2/14.34.4）。起因是用户在 FNos 上遭遇 `curl (18)/(52)/(28)` 且旧脚本无重试。详见 §14.34。  
> **整理日期：** 2026-10-23  
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

## 14.18 （v0.22）接入 apply / rollback 落地与段1 事务测试

### 14.18.1 产品口径（用户 m02947）

- **段1（本轮完成）**：本机事务测试——只验证「改得对、回滚得掉」，不验证「有没有人走它」；不需要任何 LAN 客户端，也不改变别人的网络。
- **段2（已写成手册，待回家执行）**：B2 端到端——主路由 DHCP 网关/DNS 指向旁路由，看 `lan_clients` 从 warn 变 ok。手册：`homenet-panel/docs/access-test-b2.md`（前置条件/基线记录/单设备先行/改 DHCP/六项验证/回滚/排查/验收/记录表）。
- **最小权限原则**：apply 只做三件事——写 `config.d/panel-access.dae`（自有片段）、建/删自有 nft 表 `panel_dns`/`panel_snat`/`panel_tproxy`、`dae reload`。不碰 DHCP/PPPoE/NAT，不改 `/etc/config/*`，不改整段 `config.dae`。

### 14.18.2 代码结构

| 文件 | 职责 |
| --- | --- |
| `internal/access/apply.go` | `Applier{Runner,Dae,EntryPath,AccessPath,BackupDir,Probe}`；`Apply(opt, expectFingerprint, confirm)`：observe 拒绝 → 渲染 → dry-run 直返 → 指纹校验 → `configtx.ApplyMulti` 写片段+validate+reload → 清理自有表（幂等）→ 逐条执行 nft → 记录基线状态；`DeleteTables`（「表不存在」视为成功，detail=表不存在，跳过）；`RestoreAccess`（备份→删除片段→reload）；`Rollback(confirm)`（删表→还原片段→还原 `ip_forward`→删状态文件） |
| 状态文件 | `<BackupDir>/access-state.json`（0600）：`appliedAt`/`fingerprint`/`originalIpForward`。**基线只在第一次记录**，重复 apply 不覆盖（否则会把 dae 改过的 `ip_forward=1` 当成基线） |
| `internal/api/access.go` | `POST /api/access/apply`（body: `{opt, fingerprint, confirm}`；confirm 时持 `s.mu`；错误 422 + 审计 `access.apply`）、`POST /api/access/rollback`（audit `access.rollback`） |
| 前端「接入向导」 | 计划卡底部两个按钮「应用（写入 + validate + reload + 建自有表）」「回滚接入变更」，均带 `window.confirm` 二次确认；新增「执行结果」卡（applied pill + message + backupPath + 步骤表） |

### 14.18.3 段1 实测结果（192.168.1.85，panel + dae 均 active，dae pid 3489 全程未变）

| 用例 | 期望 | 实测 |
| --- | --- | --- |
| dry-run（confirm=false） | 不写文件、不跑 nft、返回渲染结果 | ✅ `panel-access.dae` 不存在、`nft list tables` 为空、返回 steps 0 条 |
| 指纹不一致 | 422 拒绝执行 | ✅ `计划已变化（当前 a7392d4e19e0340c ≠ 提交 0000000000000000）：请重新生成计划再应用` |
| 真正 apply | 片段 0600 + reload OK + 自有表建立 | ✅ 片段 183B/0600，含 `tproxy_port: 12345`、`wan_interface: auto`、`auto_config_kernel_parameter: true`、`lan_interface: ens2`；`table inet panel_dns` + 3 条规则；`/etc/dae/config.dae` md5 `7de9781a…`、`panel-core.dae` md5 `4ef4d2a0…` **均未变**；`dae.progress=2 OK` |
| 幂等（连续 apply 两次） | 规则集合不叠加 | ✅ 两次 apply 后 `dnat` 规则数仍为 2（udp+tcp 各 1），未翻倍 |
| 回滚 | 删表 + 删片段（先备份）+ reload + 还原内核参数 | ✅ 表清空、片段移除、备份 `backups/access-20261023-224441.844.dae`、`ip_forward` 1→0、md5 未变、dae active |
| 自检 | 6 项按预期分级 | ✅ `dae_running` ok / `lan_clients` warn（无 LAN 客户端，符合预期）/ `dae0_counters` ok（累计 ~900KB）/ `dns_hits` ok（已采集 ~900 次）/ `config_untouched` info / `stale_tables` ok |

### 14.18.4 实机暴露并修掉的 4 个真 bug

1. **inet 家族 nat 必须写族名**：`dnat to 192.168.1.85:53` 在 `table inet` 下报 `specify `dnat ip' or 'dnat ip6' in inet table to disambiguate`，首轮 apply 因此失败并正确回滚；改为 `dnat ip to <ip>:53` 后通过（`nftables v1.1.3` 实测）。
2. **重复 apply 会叠加规则**：原实现只增不删，第二次 apply 会让 `panel_dns` 规则翻倍。修法：建表前先 `DeleteTables()`（幂等清理），并在步骤里显式记录 `nft-cleanup`。
3. **`ip_forward` 被 dae 置 1 后回滚不还原**：`auto_config_kernel_parameter: true` 会把内核 `ip_forward` 从 0 改成 1，回滚 dae 不会改回来（实测回滚后仍为 1）。修法：apply 前记录基线到 `access-state.json`，rollback 时 `sysctl -w net.ipv4.ip_forward=0` 并在消息里注明「内核 ip_forward 已还原为 0」。**注意要读基线在读片段之前**（否则读到的是 dae 已经改过的值，等于没记）；且**基线只在第一次记录**，重复 apply 不得覆盖。
4. **「表不存在」被当成失败**：`nft delete table inet panel_snat` 在表不存在时返回 `No such file or directory`，`exec.Command(...).Output()` 只返回 stdout 拿不到 stderr。修法：`execRunner` 改用 `CombinedOutput()`，`DeleteTables` 把该错误归一化为 `ok=true`（detail=表不存在，跳过）。

### 14.18.5 边界与后续

- **B3（旁路由自当 DHCP）仍不实现**：违反「不碰 DHCP」底线，代码里只有计划与警告文案。
- **apply 不做网络连通性兜底**：如果 `lan_interface` 写错（例如写成 WAN 口），面板不会阻止，回滚路径仍是「点一下回滚」；因此前端强制二次确认并展示指纹。
- **计划不持久化**：指纹随计划实时计算，改配置/换网卡后旧指纹自然失效（这正是设计意图）。
- **段2 未执行**：`ip_forward=1` 与 `lan_interface` 生效后的真实 LAN 收流，必须回家在 B2 场景验证（手册第 6 节六项判定）。

### 14.19 登录体验与 HTTPS（v0.23：会话持久化 + 记住我 + 同端口明文跳转）

#### 14.19.1 用户反馈（现场）

- 「每次登录都要输密码，也记不住密码」；「浏览器提示不安全」；「直接输 `192.168.1.85:8787` 打不开，必须手写 `https://`」。

#### 14.19.2 根因

1. **会话是纯内存**（`auth.Sessions` 只有 `map`），面板每次重启（今天为部署重启了十余次）会话全丢；cookie 虽带 `MaxAge`（12h），但进程一重启就失效 → 表现为「老要重新登录」。
2. **没有「记住我」**：TTL 固定 12 小时，且 `Secure` 恒等于 `s.TLS`（配置值），拿不到请求自身协议。
3. **「不安全」来自自签证书**：证书确实是自签（`CN=homenet-panel`，SAN 含 `IP:192.168.1.85, IP:127.0.0.1, DNS:localhost`，有效期至 2029-01-25），加密强度没问题，只是没进设备信任库。
4. **明文打不开**：TLS 模式下端口只接受 TLS 握手，浏览器发明文会被 Go 回 400（`Client sent an HTTP request to an HTTPS server`）。

#### 14.19.3 改动

- `internal/auth`：`Sessions` 会话表可持久化——`SetPath()` + `Load()`，每次 `Create/Delete/过期清理` 后原子落盘（0600）；**键存 token 的 SHA-256 摘要**，文件泄露也不能直接拿来登录；新增 `CreateTTL(user, ttl)`、`Count()`；常量 `DefaultSessionTTL = 12h`、`RememberSessionTTL = 30d`。
- `internal/api`：`/api/login` 接受 `remember` 字段（true → 30 天，否则 12 小时）；cookie 的 `Secure` 改为**按请求实际协议** `r.TLS != nil`（否则内网明文访问时浏览器拒绝保存）；新增公共端点 `GET /panel.crt` 下载服务器证书（`application/x-x509-ca-cert` + attachment）；`Config` 增加 `SessionPath` / `CertPath`，默认 `SessionPath=/var/lib/homenet-panel/sessions.json`（启动时 `Load()`）。
- `cmd/homenet-panel`：新增 `--session-path`（空=仅内存）、`--allow-plain-http`（默认 false）；`serve` 在 TLS 模式下改用**同端口协议分流**：读连接首字节，`0x16` 当 TLS，否则当明文 HTTP → 明文默认 **301 跳 `https://同主机:同端口/原路径`**，加 `--allow-plain-http` 则明文直接服务（并在启动日志警告密码会明文过网）。
- 前端登录卡片：新增「记住我（30 天内免登录）」勾选（默认勾上）+ 「下载证书」链接与一句解释「提示不安全是因为自签证书，装一次即可消除」。

#### 14.19.4 协议分流实现要点

- `connChan`：把 `chan net.Conn` 包成 `net.Listener`，供 `http.Server.Serve` 消费；
- `splitConns`：`Accept` 后用 `bufio.Reader.Peek(1)` 判首字节（带 10s 读超时），**必须用 `bufferedConn` 把 Peek 过的字节还给下游**，否则 TLS 握手会丢第一个字节；
- 两个 `http.Server`：TLS 侧 `tls.NewListener(tlsL, cfg)`（`MinVersion TLS1.2`）；明文侧 handler 只在 `allowPlain` 时直通 `mux`，否则只做 301。

#### 14.19.5 实机验证（192.168.1.85）

| 项目 | 结果 |
| --- | --- |
| 明文 `http://127.0.0.1:8787/` | **301 → `https://127.0.0.1:8787/`**（同端口） |
| `remember=false` | `Set-Cookie: … Max-Age=43200; HttpOnly; Secure; SameSite=Lax`（12h） |
| `remember=true` | `Max-Age=2592000`（30 天） |
| 重启后面板 | 旧 cookie 仍有效（`GET /api/me` 200，`sessions.json` 397B / 0600） |
| `GET /panel.crt` | 200、`application/x-x509-ca-cert`、`attachment; filename="homenet-panel.crt"`、1172B、含 BEGIN CERTIFICATE |
| 登录页 DOM（headless） | 18540B、`{{` 泄漏 0、含「记住我」「下载证书」「不安全…」说明 |

#### 14.19.6 「不安全」怎么彻底消掉（用户可选，非必须）

自签证书的三个选择：①**下载证书并安装为受信任根证书**（iOS：设置→通用→关于本机→证书信任设置；Android：安装 CA 证书；macOS：钥匙串→系统→始终信任；Windows：受信任的根证书颁发机构）→ 之后是绿锁，且不影响加密；②**内网纯 HTTP**（`--allow-plain-http`，或干脆不配 `--tls-cert`）：密码与 cookie 明文过网，只建议在完全可控的内网；③**保持现状**（点「继续访问」）：加密照旧，只是每次新开浏览器要点一下。开源项目默认走 ①。
### 14.20 首次登录强制改密（v0.24）

**问题**：面板以固定初始密码分发（`setpass` 写入 `/etc/homenet-panel.auth.json`），安装后若忘了改，等于长期用公开口令。要求"第一次登录强制改密码"。

**实现（三道闸）**

| 层 | 位置 | 行为 |
| --- | --- | --- |
| 凭据标记 | `internal/auth` `credFile.MustChange` | `Set()` 委托 `SetWithFlag(path,user,pw,must)`；`NeedsChange(path,user)` 读文件判断该用户是否仍为初始密码 |
| 服务端门禁 | `internal/api/server.go` auth 中间件 | 已登录但 `NeedsChange` 为真时，除白名单外一律 `403 {"code":"must_change_password","mustChange":true}`。白名单：`/api/me`、`/api/passwd`、`/api/logout`、`/api/health` |
| 修改入口 | `handlePasswd` `POST /api/passwd` | `{oldPassword,newPassword}`；旧密码错 → 401；弱密码 → 400；新旧相同 → 400；成功写新凭据（**同时清空 MustChange**）并写审计 |

**口令强度规则**（`passwordWeakness`，返回空串表示合格）

- 长度 < 8 → "新密码至少 8 位"；> 128 → 过长；
- 必须同时含"字母"和"数字或符号"；
- 黑名单（小写比较）：`password`、`admin`、`12345678`、`homenet`。

**命令行**：`homenet-panel setpass --user admin --password '...' --must-change` 把账号标记为"初始密码"；首次启动自动建号也走 `--must-change`（提示"登录后必须修改"）。

**前端**：登录卡片与主控台之间插入 `v-else-if="mustChange"` 分支（三输入框 + "保存并进入"），`changePass()` 成功后清空字段、`mustChange=false`、直接刷新主控台。注意 Vue 的 `v-if/v-else` 链顺序：新分支必须插在登录卡（`v-if="!authed"`）与主控台（`v-else`）之间，否则主控台会先命中。

**实机验证（192.168.1.85）**

| 用例 | 结果 |
| --- | --- |
| `setpass --must-change` 后 `/etc/homenet-panel.auth.json` | `mustChange: true` |
| login | 200 |
| `GET /api/me` | `{"mustChange":true,"user":"admin"}` |
| `GET /api/status` | 403 `{"code":"must_change_password",...}` |
| `POST /api/passwd` 弱口令 `abc` | 400 "新密码至少 8 位" |
| `POST /api/passwd` 正常口令 | 200，之后 `/api/status` 200 |
| headless Chrome + 会话 cookie 渲染 | DOM 19,184B、`{{` 泄漏 0、"请先修改初始密码"出现、主控台标题不出现 |

**浏览器「保存密码 / 自动填充」为什么之前不弹（v0.25）**

原登录区是三个裸 `<input>` + `@click` 按钮，没有 `<form>`、没有 `name`、没有 `autocomplete` —— Chrome/Safari 的密码管理器只在**表单提交**（submit 事件）时才会提示保存，且靠 `autocomplete="username"` / `"current-password"` 判定字段语义，所以既不弹保存框也不会自动填充。改法：

- 登录卡包一层 `<form name="hnp-login" @submit.prevent="doLogin">`，账号 `name="username" autocomplete="username"`、密码 `name="password" autocomplete="current-password"`，按钮 `type="submit"`；
- 改密卡同样包 `<form @submit.prevent="changePass">`，当前密码 `autocomplete="current-password"`、两个新密码 `autocomplete="new-password"`（避免被旧密码自动填）；
- 回车提交由表单原生完成，去掉原来的 `@keyup.enter`。

前端技术栈本身：**Vue 3（内嵌 `vue.global.prod.js`，无构建步骤）+ 手写 CSS**，未使用 Element Plus / Ant Design 等 UI 库——所以表单语义属性必须自己写。浏览器是否真的弹「保存密码」还取决于证书：https + 自签证书时，Safari 对不受信任的站点会禁用自动填充，把 `/panel.crt` 装成受信任根证书即可。

**改用原生表单提交（v0.26）——Chrome 保存密码的可靠性问题**

即使在登录区补了 `<form>` 与 `autocomplete`，用 `@submit.prevent` + `fetch /api/login` 仍经常拿不到「保存密码」提示：Chrome/Safari 的密码管理器把「**表单提交并发生导航**」当作保存时机，纯 XHR 登录不产生导航。另一个更硬的原因：**页面证书未受信任时，Chrome 会直接禁用密码管理器**（既不提示保存也不自动填充）。

改法（服务端出真表单提交）：

| 端点 | 方法 | 入参 | 行为 |
| --- | --- | --- | --- |
| `POST /login` | form-urlencoded | `username`、`password`、`remember` | 成功 → `303 /` 并下发会话 cookie；失败 → `303 /?e=bad`；凭证文件异常 → `/?e=cred` |
| `POST /passwd` | form-urlencoded | `oldPassword`、`newPassword`、`newPassword2` | `303 /?pe=<code>`（`mismatch/old/weak/same/write`），成功 `303 /`；`/passwd` 已加入 mustChange 白名单 |

前端：登录表单 `method="post" action="/login"`、改密表单 `action="/passwd"`，去掉 `preventDefault`，`readFormMsg()` 解析 `?e=`/`?pe=` 显示中文错误后用 `history.replaceState` 清掉查询串。JSON 接口 `/api/login`、`/api/passwd` 仍保留（测试/CLI 用途）。

**实测**（临时实例 `127.0.0.1:8899` + 临时 auth 文件，不动生产凭证）：`bad → 303 /?e=bad`；`good → 303 / + 会话 cookie`；`mismatch → /?pe=mismatch`、`weak → /?pe=weak`、`ok → /`；改密后 `/api/me` 的 `mustChange` 变 false。

**给用户的判定顺序**（Chrome 仍不弹时）：① 打开 `chrome://password-manager/settings` 确认「提示保存密码」开启（无痕窗口一律不保存）；② 把 `/panel.crt` 装成受信任根证书——**这是自签 https 下最常见的原因**；③ 若不装证书，可临时用 `--allow-plain-http` 让内网走 http（Chrome 在 http 站也会保存密码，只是标注"不安全"）。

**注意**：`setpass` 走命令行，不做强度校验（便于设 `10086` 这种临时初始密码）；强度规则只作用于 `POST /api/passwd` 的改密环节。

**坑**：通过 `/api/passwd` 改密成功会清除 `MustChange`，做回归测试把密码改回原值时也会顺手清掉标记——验证完要重新跑一次 `setpass --must-change`。



## 14.21 转发的条件触发、归属与 Clash 行内映射 / ssr 支持（v0.27/v0.28）

### 14.21.1 现象与根因（2026-10-24）

用户在把网关切到 192.168.1.85 后反馈卡顿。排查发现一个更硬的故障：**任何 `dae reload` 都失败**，

```text
reload failed: publish staged reload candidate: bind LAN interface ens2:
attach interface ens2: ipforward on ens2 is off: /proc/sys/net/ipv4/conf/ens2/forwarding
```

根因链：

| 环节 | 事实 |
| --- | --- |
| 内核默认 | `net.ipv4.ip_forward = 0`；本机 `/etc/sysctl.conf` 为空、`/etc/sysctl.d/` 只有 README.sysctl，从未持久化 |
| 谁开过 | dae 仅在配置含 `auto_config_kernel_parameter: true` 时自己写 `/proc/sys` |
| 10-23 22:33 | 接入向导 apply 写出的片段含该选项 + `lan_interface: ens2`（备份 `/var/lib/homenet-panel/backups/access-20261023-223350.756.dae`），转发被 dae 打开 |
| 之后 | 该片段被整份核心配置覆盖/回滚，`config.d/` 只剩 `panel-core.dae`（global 无该选项） |
| 10-24 01:41 | dae 重启读到新配置 → 转发退回 0；journal 里 `ipforward on ens2 is off` 仅 7 次，全在 11:33（即首次 reload 尝试时） |

为什么转发关着 LAN 仍能上网：本机只有一块物理网卡 `ens2`，既当 LAN 又当 WAN（`wan_interface: auto` 命中的就是它），dae 在 ens2 上做 tproxy 拦截，被接管的 TCP/UDP 本地终结，不需要转发；但**未被接管的包**（ICMP、部分 UDP/QUIC、客户端 IPv6）需要转发才能过，关着就被静默丢弃——这是「有的网站能开、有的转圈」的一个真实来源。

### 14.21.2 面板侧实现：转发由面板负责到底（internal/access/sysctl.go）

用户口径：转发是必须的，那就要在面板或 dae 层面持久化，只有关闭服务/回滚时才回到默认关闭。因此面板不再依赖 dae 的 `auto_config_kernel_parameter`：

| 位置 | 行为 |
| --- | --- |
| `EnableForwarding(ifaces)` | `sysctl -w net.ipv4.ip_forward=1`、`conf.all.forwarding=1`、逐接口 `conf.<iface>.forwarding=1`，并写 `/etc/sysctl.d/99-homenet-panel.conf`（0644，含生成说明与撤回方法） |
| `DisableForwarding(st)` | 按应用前基线还原：原来有文件就逐字写回，没有就删除自有文件；只有应用前是 0 才把转发关回去（原值 1 说明系统本来开着，不动） |
| `Applier.Apply` | 顺序改为 **0) 记基线 → 0.5) 开转发并持久化 → 1) 写片段+validate+reload → 2/3) nft**；reload 或 nft 失败时一并回滚转发 |
| `accessState` | 新增 `hadSysctlFile` / `originalSysctl`，与既有 `originalIpForward` 一起构成回滚依据 |
| `Applier.Rollback` | 删表 → 移除片段 → `DisableForwarding` 还原转发与持久化文件 |

要注意的点：sysctl 文件路径与 `/proc/sys` 根都是包级变量，测试里注入临时目录，避免在开发机写 `/etc`。

### 14.21.3 clashconv：行内映射 + ssr（61/61 实测）

用户决定「把 OpenWrt 那边的机场拿到 dae 来测」，而那条机场是 SSR（61 个节点，`aes-256-cfb`），且 Clash YAML 用的是**行内映射**写法：

```yaml
proxies:
    - {name: 🇨🇳 【隧道】台湾 P1, server: so260520-cn-27v3.sonlodd365.com, port: 42345, type: ssr, cipher: aes-256-cfb, password: H6JmjL, protocol: auth_aes128_sha1, obfs: tls1.2_ticket_auth, protocol-param: 25164:hvF4PIxq, obfs-param: 2fc6925164.microsoft.com, udp: true}
```

改动：

1. `ParseProxies` 支持 `- {k: v, k2: {k3: v3}}`：`parseFlowItem` 递归展开成 `a.b` 扁平键，`splitTopLevel` 按顶层逗号切分（忽略引号与花括号内的分隔符），与块状写法共用 `Proxy.Get`。
2. 新增 `ssrLink`：`ssr://base64url(host:port:protocol:method:obfs:base64url(password)/?obfsparam=&protoparam=&remarks=)`，缺 `protocol` 补 `origin`、缺 `obfs` 补 `plain`，缺 cipher/password 进 Skipped（不静默丢弃）。
3. dae 侧验证：把 ssr 链接塞进 `file://` 订阅 + `dae validate` 得 EXIT=0（把订阅内容换成垃圾串则 EXIT=1，证明订阅确实被解析）。
4. 回归：`SUMMER_SAMPLE=/tmp/summer.yaml go test ./internal/clashconv -run Summer` → **61 个代理，转出 61 条，跳过 0 个，类型 map[ssr:61]**。

### 14.21.4 产品口径（v0.28 定稿）：转发必须「有条件才开」

用户口径（原话）：「应该是满足一个条件，设置为旁路由或者分配了 lan 输出的时候，打开转发」。

面板据此改成两条触发路径，**两条都以 dae 真的需要为前提**，只代理本机时绝不碰内核参数：

| 触发 | 条件 | 动作 |
| --- | --- | --- |
| ① 配置声明 | 渲染后的 live 配置（含 include 展开）里有 `lan_interface: <ifaces>` | 面板启动时与**每次 reload 前**（`dae.Adapter.BeforeReload`）检查该接口与 `ip_forward`，缺什么补什么，并写 `/etc/sysctl.d/99-homenet-panel.conf` |
| ② 运行时要求 | reload 报 `ipforward on <iface> is off` | `dae.Adapter.OnReloadError` 用正则取出接口名，只对该接口开转发并持久化，然后**自动重试一次 reload** |

②这条是实机逼出来的：live 配置里**没有** `lan_interface`（`wan_interface: auto` 把 ens2 当 WAN，dae 仍绑定它并要求转发），所以只看配置会漏——「条件」应当以 dae 的实际要求为准，而不是只看有没有那行声明。

配套实现：
- `internal/access/ensure.go`：`EnsureForwardingForConfig(configPath)`（路径①）、`EnsureForwardingForInterfaces(ifaces)`（路径②），核心 `ensureForwardingFor` 先读当前值、只补缺失项；无改动时不写持久化文件（幂等）。顺带修掉旧 `forwardingNow` 少一层 `net/` 的路径 bug。
- `internal/dae/dae.go`：`Adapter.BeforeReload func()` 与 `Adapter.OnReloadError func(code int, out string) bool`（返回 true 才重试一次），`Reload` 拆出 `reloadOnce`。
- `internal/api/forward.go`：`ensureForwarding` / `healForwarding` / `forwardIfaceFromError`；在 `api.New` 里挂到 `s.Dae` 上，并在启动时先自检一次。

## 14.21.5 实机验证（2026-10-24 16:02，192.168.1.85）

| 项 | 结果 |
| --- | --- |
| 部署 | 二进制 12,924,989 B（16:02）→ stop/mv/chmod755/start；panel 与 dae 均 `active` |
| 启动自检 | 日志无「转发自检」行 —— live 配置没声明 `lan_interface`，路径①按设计不动内核（`ip_forward` 仍是 0） |
| 触发路径② | 用临时实例（127.0.0.1:8899，`/tmp` 下的 auth/session/audit）做一次「内容未改」的 `/api/model/apply`：先失败于 `ipforward on ens2 is off` → 面板日志「转发自检: dae 要求接口 ens2 的转发…已打开并持久化，重试 reload」→ **apply 返回 200**，`/var/run/dae.progress` = `2\nOK` |
| 内核结果 | `net.ipv4.ip_forward = 1`、`net.ipv4.conf.ens2.forwarding = 1`；`/etc/sysctl.d/99-homenet-panel.conf`（302B）已写入 |
| 只读不变量 | `/etc/dae/config.dae` md5 `7de9781a07f51041bb9ce866228baf2e`、`config.d/panel-core.dae` md5 `4ef4d2a0b73b325882c7044ea53da6d7` 均未变；改动前已自动备份 `/var/lib/homenet-panel/backups/config-20261024-160236.549.dae` |
| 撤回 | `sysctl -w net.ipv4.ip_forward=0 net.ipv4.conf.ens2.forwarding=0` 并删 `/etc/sysctl.d/99-homenet-panel.conf`；或走面板「接入向导 → 回滚」 |

结论：**「改不动配置」的死结已解开** —— 之后任何模型/订阅/子订阅变更都能 reload 成功；且内核只在 dae 真的要求时才被改动。

14.21.6 待办
- `summer`（SSR，61 节点）尚未接入 dae 配置：转换能力已具备，等用户确认后再写 `/etc/dae/subs/summer.sub` 并在模型里加 `subscription summer`。
- OpenWrt 侧当前仍跑旧机场 `summer.yaml`（uci 里该订阅 `enabled=0`，未参与下载），与 .85 不构成同机场对比。


## 14.22 面板 UI 两处「看起来没反应」的真因（v0.29）

用户反馈截图（自家面板，不是 OpenClash）：「内容层」点了「+ 添加订阅」像没生效，「订阅源更新」页没有新增入口。

### 14.22.1 真因一：`--c` 没有传进 Server（真 bug，已修）

`cmd/homenet-panel/main.go` 里 `--c` 只被 preflight / validate 用到，构造 `api.Config` 时**漏了 EntryPath**：

```go
srv := api.New(api.Config{
    Dae:       a,
    EntryPath: *cfg,   // v0.29 补上
    ...
})
```

生产上 `--c /etc/dae/config.dae` 与默认值相同，所以一直没暴露；一旦指向别的路径（本地联调、OpenWrt 上换路径），面板读的还是 `/etc/dae/config.dae`，模型会 fallback 成空（`parseError: 未解析出任何配置块`）。本地复现：`/api/model` 返回 `entryPath=/etc/dae/config.dae` 而不是 `--c` 的值；修复后返回正确路径且订阅被解析出来。

### 14.22.2 真因二：「添加订阅」其实生效了，只是没有保存入口提示

`model.subscriptions.push(...)` 会立刻多出一行空行，但页面只有**最底部**一个「校验+备份+reload 应用」按钮，用户以为没有保存按钮。改动：

- 内容层顶部新增操作条：**「校验+备份+reload 应用」+「预览生成结果（不写盘）」**，与底部按钮等价；
- info 文案明写「本页没有单独的『保存』按钮：改完必须点应用才写盘生效（输入框里的改动只是暂存在浏览器里）」。

CDP 实测：点「+ 添加订阅」表格行 2 → 3；填入 `summer` / `file://subs/summer.sub` 后点「预览生成结果」，渲染出的`subscription {}` 包含 summer。

### 14.22.3 真因三：订阅源登记表只能改不能加

`syncEntries()` 只把**模型里已有的订阅**合并进登记表，所以「还没进模型」的上游（例如想先接 summer）无处登记。改动：

- 前端「订阅源更新」页新增「新增订阅源」卡片（名称 / 上游 URL / UA + 新增按钮），名称限 `^[A-Za-z0-9._-]+$`、长度 ≤ 64（`internal/api/subsrc.go` 的 `validSubName`，防路径穿越）；
- 后端 `handleSubsrcApply` 在 `entry.File == ""`（模型尚未引用）时自动分配默认镜像 `filepath.Join(baseDir, "subs", name+".sub")`、`Source = "file://subs/<name>.sub"`，并 `os.MkdirAll(subDir, 0o755)`；
- 前端 `saveSrc` 修竞态：原先不 `await this.loadSubsrc()`，导致「已新增」提示被随后刷新列表清空。

### 14.22.4 接一个新机场（summer）的三步

1. 「订阅源更新 → 新增订阅源」填名字 `summer` + 上游 URL + UA（按机场要求，例如 `clash-verge/v1.7.7`）；
2. 点「拉取并检查」看节点数（上游若是 Clash/Mihomo YAML 会自动转成节点链接，`format=clash`）；
3. 点「拉取并应用」（写 `<baseDir>/subs/summer.sub`）→「内容层 → 订阅」把 `summer` 的来源选成 `file://subs/summer.sub` → 顶部「校验+备份+reload 应用」。

## 14.23 一次性订阅 token、跳过节点留档与登记项删除（v0.30）

### 14.23.1 现场反馈

用户原话（要点）：「是 dae 自己无法加载这个订阅，且这个订阅被一个设备读取后 token 就会失效；我把 dae 的删除了以后，重新在服务端生成一个订阅并在订阅源导入了；还有订阅源更新那里没有删除的按钮。」

### 14.23.2 一次性 token 改变了「订阅源」的默认姿势

- **不要在「内容层 → 订阅」里填 http(s) 上游**：dae 每次 reload 都会自己拉一次，token 一次性 ⇒ 第二次就是 404/空订阅，且 dae 对拉不到的订阅**不报错**（静默 0 节点），面板也看不到原因。
- **正确姿势 = 镜像**：面板「订阅源更新 → 拉取并应用」只在人工点击时拉一次 → 解析校验（0 节点拒绝覆盖）→ 备份旧镜像 → 0600 写 `/etc/dae/subs/<name>.sub` → reload；dae 只读 `file://` 本地文件，reload 多少次都不会消耗 token。
- 所以 `--subsrc-check-hours` 自动巡检对一次性 token 的上游也应谨慎：巡检会再拉一次并可能把 token 烧掉。当前默认 6 小时（只报告差异），对一次性 token 上游建议在面板把该订阅的巡检关掉（后续可做 per-entry 开关）。

### 14.23.3 跳过节点的留档（新增）

Clash 转换只覆盖 `vmess/vless/ss/ssr/trojan/hysteria2/tuic`，其余类型进 `Skipped`。现场数据：summer 上游 121 个代理 → 转出 **61 个 ssr**、**跳过 60 个**（类型当时未留档，无法回溯）。

改动：`internal/subsrc` 增 `FetchPayloadForceRaw` / `FetchPayloadRaw`（缓存里一并保存原始内容）；`handleSubsrcApply` 在 `format=clash` 时把上游原始 YAML 落到 `<backup-dir>/subs-raw/<name>.<时间戳>.yaml`（0600），响应里带 `rawPath`。这样即使 token 一次性，也能事后按真实字段补转换（例如 anytls）。

### 14.23.4 登记项删除

「订阅源更新」页此前只能新增/修改，不能删除。新增 `POST /api/subsrc/remove`：

- 只删**登记**（上游 URL/UA/检查结果），**不删磁盘镜像**，也不碰 dae 配置；
- 仍被「内容层 → 订阅」引用的登记项拒绝删除（409，提示先去内容层删除并应用），避免出现「模型引用一个没有上游的订阅」；
- `syncEntries` 给模型里的订阅打 `inModel:true`，前端据此把「删除」按钮置灰并显示「内容层已引用」。

实机验证（临时实例，独立 subsrc 副本，未动生产登记表）：列表 `[('CF',True),('dual',True),('summer',False,skipped=60)]`；删除 summer → 200；删除 CF → 409；删除后列表只剩 CF/dual。

## 14.24 订阅再拉取后的两个转换缺陷 + 内容层订阅选择器（v0.31）

### 14.24.1 现场数据（summer 上游，2026-10-24 23:03 留档）

`/var/lib/homenet-panel/backups/subs-raw/summer.20261024-230349.yaml`（1,105,734 B / 21,360 行）实测：`type: anytls` 20 个、`type: ssr` 61 个、`type: select` 12 个（分组）。而上游返回后，面板记录的 `skipped=60`、镜像只有 **61** 个 ssr。

### 14.24.2 缺陷一：alpn 列表项被当成新节点（真 bug）

anytls 节点带 `alpn:` 嵌套列表：

```yaml
  - name: "【专线】台湾F3"
    type: anytls
    alpn:
      - h2
      - http/1.1
```

`ParseProxies` 原先对任何 `- ` 开头的行都开一个新 `Proxy`，于是每个 `- h2`/`- http/1.1` 都变成 type 为空、无 server 的「幽灵节点」。20 个 anytls × 2 行 = **40 个幽灵**，加上 20 个真 anytls，正好 60 = `skipped` 的数字。

修复：记录**节点列表项自身的缩进**（`itemIndent`，取 proxies 段里第一个 `- ` 的缩进），只有缩进等于 `itemIndent` 的 `- ` 才开新节点，更深的 `- ` 视为节点内部嵌套列表跳过。实测 121 → 81 个代理，幽灵归零。

### 14.24.3 缺陷二：anytls 不被转换（dae 其实支持）

- 依据 dae 依赖的 `github.com/olicesx/outbound` 的 `dialer/anytls/anytls.go` `parseAnytlsURL`：链接形如 `anytls://<password>@host:port?sni=...&insecure=1#name`；`Auth=userinfo 用户名`、`Sni` 依次取 query 的 `peer`→`sni`→hostname、`Insecure` 只认 `insecure=1`。
- 新增 `anytlsLink(p Proxy)`，字段映射：`password`→userinfo、`sni/servername/server-name`→`sni`、`skip-cert-verify|insecure|allowInsecure`→`insecure=1`、`name`→fragment。
- 真实样本回归：`proxies=81 types=map[anytls:20 ssr:61] links=81 skipped=0`（`/tmp/summer81.sub` 20,125 B，20 条 anytls + 61 条 ssr）。
- 单测 `TestConvertAnytlsAndNestedLists` 同时锁住两件事：alpn 不产生幽灵节点、anytls 链接字段与「只能有一个 #」。

### 14.24.4 内容层订阅：从手输入改成下拉选择

用户原话（要点）：「这里我没加，这里应该做成选项，勾取或者拖动，不然要手动输入实在太糟糕了。」

「内容层 → 订阅」的来源栏新增下拉：列出「订阅源更新」里已登记的镜像（`file://subs/<name>.sub`，带节点数），选中后自动填 `source` 并在名称为空时按文件名补 `name`；仍保留文本框以支持自定义/直连 URL。标题下加说明：推荐先用镜像，dae 只读本地文件、不会重复消耗上游 token。

### 14.24.5 待办

- 服务器上的 `/etc/dae/subs/summer.sub` 仍是旧的 61 节点版本（新代码只影响「下一次 apply」）；上游 token 一次性，重拉可能 404，可用留档的原始 YAML 离线重转（`/tmp/summer81.sub` 已生成待用户批准写入）。
- `summer` 尚未被 `/etc/dae/config.dae` 引用，dae 实际上还没用到它。

## 14.25 组策略受控 A/B：远端节点是丢包主因（v0.32）

### 14.25.1 背景与方法

S1 双路对比先给出了不对称（2026-10-25 01:00–01:35 CST，同一时间窗、同一批目标）：

| 视角 | 境外 L3(HTTPS) 成功率 | 境外 TLS p50 / p95 |
|---|---|---|
| `.85`（本机 dae，机场 A） | 10/32 → 42/101 ≈ **31–42%** | 570–860ms / 2.0–2.8s |
| Mac 经 `.11`（OpenClash，机场 B） | 264/281 = **94.0%** | 503–537ms / 780–873ms |

失败形态不是"慢"：探针记录 `connMs≈780–1216ms`（**TLS 握手成功**）、`firstByteMs=0`、`err=EOFNoResponse`；串行 20 次测试是精确 **15.5±0.2s** 断开（成功 0.9–1.2s），双峰分布 = 被定时掐断。

两台机器用的**不是同一家机场**，所以不能直接归因。于是在 `.85` 上做受控 A/B：`tools/netdiag/stability/ab_group.py` 只替换 `config.d/panel-core.dae` 里的 `proxy` 组块 → `dae validate` → `systemctl reload dae` → 跑 150 秒 l3 探针 → 记录；`finally` 无条件恢复备份并再 reload。跑完核对 md5 与原始一致（`4ef4d2a0b73b325882c7044ea53da6d7`）。

### 14.25.2 结果（同一机场、同一台机、同一批目标）

| 变体 | proxy 组 | L3 成功 | 成功率 |
|---|---|---|---|
| baseline | `subtag(CF)` + `subtag(dual)`（CF 1 个 + dual 29 个，含 日/新/港/印/英/德/加/法/阿/韩/澳） | 10/32 | **31%** |
| hk_only | `subtag(dual) && name(keyword: '香港')`，`min_moving_avg` | 50/56 | **89%** |
| hk_fixed | 同上但 `policy: fixed(0)` | 41/44 | **93%** |
| baseline3（回归） | 恢复 baseline | 13/36 | **36%** |
| hk_sg_regex | `name(keyword: '香港|新加坡')` | 4/36 | **11%** |
| not_jp | `subtag(dual) && !name(keyword: '日本')` | 2/24 | **8%** |

### 14.25.3 顺带查清的 filter 语义（dae v2.1.1）

- `name(keyword: 'x')` 是**子串匹配，不是正则**：`keyword: '香港|新加坡'` 能通过 `dae validate`（rc=0），但运行时一个节点都匹配不到（所以 hk_sg_regex 掉到 11%）——**validate 通过 ≠ 语义正确**，这类"空过滤器"只能靠运行时数据发现。
- 多行 `filter:` 是**AND**（原配置 `filter: subtag(CF)` + `filter: subtag(dual)` 就是取交集）。
- 带括号的 `(a || b)` 直接被解析器拒：`extraneous input '(' expecting {'!', ID}`。
- `!name(keyword: …)` 取反可用（not_jp 通过 validate 并能跑）。

### 14.25.4 结论与改法

1. **组内混入远端节点是主因**：只保留香港节点就从 31% 提到 89–93%。`min_moving_avg` 按**测得的延迟**选节点，而大量远端节点"端口开、握手快、数据面丢包/被掐"，延迟指标看不出坏，于是被反复选中。
2. **按地区分组而不是大杂烩**：`proxy` 只放低延迟同区域节点；新加坡等远端节点留给**显式规则**（现有 `sg` 组服务 `claude.ai` / `anthropic.com`），不要塞进 fallback 组。
3. **不要指望正则/复合关键词**：需要"香港+新加坡"时只能拆成两个组，或用路由规则分别指向。
4. 候选后续：面板侧把「组里混入了多少地区/多少节点」做成配置体检（Lint），并在组编辑处给出建议语句。

### 14.25.5 教训

- **节点归因（`attribute.py`）不可靠**：`min_moving_avg` 平均每 12s 重选一次，而失败在 15s 之后才落地，"失败时刻最近一次选中"与真实承载体不一致；当时按它得出的「香港 HKBN 73% 失败」与 A/B 结果（香港整体 89% 成功）矛盾，以 A/B 为准。正确的归因方式是**固定单节点 + 对照**（`policy: fixed(0)` + 单节点过滤）。
- **`dae validate` 只保证语法**：空过滤器、无节点组这类语义错误必须靠运行态验证（探针/日志/字节统计）。

### 14.25.6 复现

```bash
# 本机（Mac）侧：客户端经软路由
python3 tools/netdiag/stability/probe.py --tag via11 --duration 3600 --interval 10 --layers l1,l2,l3,l4 --out probe-via11.jsonl
# 服务器侧：只改 proxy 组做 A/B，结束自动恢复
scp tools/netdiag/stability/ab_group.py root@<旁路由>:/root/ab_group.py && ssh root@<旁路由> 'python3 /root/ab_group.py'
```

### 14.25.7 冷却期与应用落地（v0.33）

**踩到的坑：reload 之后的前几分钟全红。** 按 A/B 结论把 `proxy` 改成只用香港后，**立刻**跑 240 秒探针得到 `0/28 = 0%`（`EOFNoResponse` 21 次 + 超时 7 次），差点被误判成"改坏了"。原因：`min_moving_avg` 需要重新测速建立排序，刚 reload 时组内还没有可用测量值，连接全被掐。

**公平复测（每次 reload 后先静置 90 秒，再跑 240 秒 l3 探针）**：

| 变体 | 境外 L3 | 成功率 |
|---|---|---|
| baseline（CF+dual 全节点） | 10/36 | **28%** |
| hk_only（`subtag(dual) && name(keyword:'香港')`） | 85/92 | **92%** |

与第一轮 A/B（31% vs 89%）一致，结论稳定。**运维含义：应用配置后至少等 1–3 分钟再判断好坏；面板的「应用」成功提示应当附带这句话。**

**已落地**（2026-10-25 15:24 CST，`.85`）：

- 备份：`/root/ab/panel-core.dae.before-hkonly-20261025-152457`
- 现配置 `/etc/dae/config.d/panel-core.dae` md5 `393cc4f6db0abad99671522148b36d96`，`proxy` 组只有一行过滤：`filter: subtag(dual) && name(keyword: '香港')`
- `dae validate` rc=0 → `systemctl reload dae` → `/var/run/dae.progress` = `2|OK`，`dae` 与 `homenet-panel` 均 active
- 回滚一条命令：`cp /root/ab/panel-core.dae.before-hkonly-20261025-152457 /etc/dae/config.d/panel-core.dae && chmod 600 … && systemctl reload dae`
- 注意：面板下次「应用」会用**模型里解析到的同一份 filters** 重新渲染，所以这次改动会自然保留；若在面板 UI 里改回去，同样生效。

## 14.26 v2rayN 规则迁移实测 + 模型漂移防护（v0.34）

### 14.26.1 背景

用户提供了一份 v2rayN 规则集（`ruu.txt`，7 条，60 行）：UDP 443 阻断、private 直连、广告拦截、AI 专用代理、YouTube 代理、国内直连、兜底代理。问题：**这些规则能不能在 HomeNet Panel 里表达，怎么表达。**

### 14.26.2 语义映射表

| v2rayN | 面板 / dae | 说明 |
|---|---|---|
| `port: 443, network: udp` → block | `l4proto(udp) && dport(443) -> block` | UDP 443 是 QUIC/HTTP3 与部分 UDP 系业务 |
| `geoip:private` + `geosite:private` → direct | 面板已自动注入 `dip(192.168.0.0/16)`、`dip(127.0.0.0/8, 169.254.0.0/16, 224.0.0.0/4)`、`dip(255.255.255.255/32)`、`pname(...)` | 属 EssentialRules，不在规则表内、删不掉 |
| `domain:geosite:category-ads-all` → block | `domain(geosite:category-ads-all) -> block` | 语义一致 |
| `domain:geosite:category-ai-!cn` → proxy | `domain(geosite:category-ai-!cn) -> <组>` | 组名由用户决定 |
| `domain:geosite:youtube` → proxy | `domain(geosite:youtube) -> <组>` | 同上 |
| `ip:geoip:cn` + `domain:geosite:cn` → direct（或关系） | 拆两行：`domain(geosite:cn, geosite:geolocation-cn) -> direct` 与 `ip(geoip:cn) -> direct` | **同一行谓词是「与」不是「或」**，必须拆 |
| `port: 0-65535` → proxy | 写在 routing 末尾 `fallback: proxy` | dae 不支持端口区间写法 |

### 14.26.3 能力核实（实机，非推测）

- `/usr/share/dae/geosite.dat` 是 v2fly `GeoSiteList` protobuf，**tag 以大写存储**，共 1554 个；`CATEGORY-ADS-ALL`、`CATEGORY-AI-!CN`、`YOUTUBE`、`PRIVATE`、`CN`、`GEOLOCATION-CN` 均存在，引用时用小写可直接解析。
- 翻译后的规则在临时配置上 `dae validate` **rc=0**（含 `domain(geosite:category-ai-!cn)` 这种带 `!` 的 tag）。
- 面板规则模型是**自由文本**：`internal/model/model.go:26-27` 的 `Matcher`/`Out` 两个字符串，parseRouting 只按 `" -> "` 切分；`model.go:296` 渲染为 `    <matcher> -> <out>`。前端 datalist `outopts` 已含 `block`，`outInvalid()` 接受 `direct/block/reject/组名`。所以**不需要为这些规则加任何新代码**，直接在「内容层 → 分流规则」加行即可。
- 模型往返验证：7 条规则 → Render → Parse 得 11 条（4 条 essential + 7 条自填），零丢失，fallback 保留。

### 14.26.4 上线与命中实测

应用后 `/etc/dae/config.d/panel-core.dae`（md5 `e924706ac24905778f0471c3c8f11a7f`）的 routing 顺序：4 条 essential → `l4proto(udp) && dport(443) -> block` → `domain(geosite:category-ads-all) -> block` → `domain(geosite:category-ai-!cn) -> hk` → `domain(geosite:youtube) -> hk` → `domain(suffix: claude.ai, suffix: anthropic.com) -> sg` → `domain(geosite:cn, geosite:geolocation-cn) -> direct` → `ip(geoip:cn) -> direct` → `fallback: proxy`。

面板 `/api/route/test`（走真实 geosite/geoip）命中结果：www.youtube.com → hk（geosite:youtube）；claude.ai / chatgpt.com → category-ai-!cn；doubleclick.net → block（category-ads-all）；baidu / qq → direct；github.com / wikipedia → proxy（fallback）。

连通性实测（.85 本机）：

| 目标 | 组 | 结果 |
|---|---|---|
| doubleclick.net / googleadservices.com | block | TCP 6.1–6.2s 超时（确被丢弃） |
| www.baidu.com | direct | 6/6，18–55ms |
| youtubei.googleapis.com | hk | 6/6，avg 1.8s |
| www.google.com | proxy | 6/6，avg 1.3s |
| www.youtube.com | hk | 3/6（首屏域名本身不稳，子资源域名 6/6） |
| www.reddit.com | proxy | 4/6 |
| **claude.ai / chatgpt.com** | **sg** | **0/6** |
| `api.openai.com` | sg | 同样失败 |

**受控对照**：把 AI 规则临时改到 `-> hk`，validate 0、reload、**静置 90 秒**再测 → claude.ai 6/6 avg 1.2s、chatgpt.com 6/6 avg 2.3s、api.openai.com 6/6 avg 2.7s；测试脚本结束后自动恢复成 sg。

**结论**：`sg` 组（新加坡 2 个节点，journal `alive_dialers=2 group=sg`）当时完全不可用，`hk`/`proxy` 组（alive_dialers 6–7）可用。AI 规则最终落为 `-> hk`，`sg` 组保留备用。这也再次印证 §14.25 的「reload 后有冷却期」：组策略是 `min_moving_avg`，刚 reload 完立刻测会得到一堆假失败。

### 14.26.5 模型漂移（真风险）与防护

**发现**：面板 UI 的真源是 `/var/lib/homenet-panel/model.json`（`internal/api/server.go` 的 loadModel 先读它，读不到才 Parse 线上文件）。实机检查发现该文件（1394B，10-24 23:47）里 `proxy` 组仍是旧的 `["subtag(CF)","subtag(dual)"]`、只有 4 条规则 —— 而线上配置已经被手工改过（HK-only 分组 + v2rayN 规则）。**此时用户在面板点一次「应用」，就会把线上的改动全部覆盖回旧状态。**

处置：
1. 立即把 `model.json` 手工同步成与线上一致（HK-only 分组、sg 组、11 条规则、fallback proxy），原文件备份为 `/root/ab/model.json.before-v2rayn-*`；
2. 补代码能力（不依赖人肉同步）。

新增 `homenet-panel/internal/api/model_drift.go`：

| 函数 / 方法 | 作用 |
|---|---|
| `normalizeConfig(s string) string` | 去行尾空白、去首尾空行（避免脏格式误报） |
| `(s *Server) modelDrift(m *model.Model) []string` | 把模型 Render 后与线上 `EntryPath`/`CorePath` 逐块比对，返回中文差异原因 |
| `(s *Server) handleResyncModel` | `POST /api/model/resync`：Parse 线上 entry+core → saveModel → 审计 `model.resync` → 200。**只覆盖面板模型，不改线上** |

配套改动：`handleGetModel` 响应新增 `diverged`/`drift`；前端「内容层」顶部在 `diverged` 时显示黄条「面板模型与线上配置不一致」+ 列出原因 + 按钮「从线上重新解析（覆盖面板模型，不改线上）」；新增测试 `model_drift_test.go` 的 `TestModelDriftDetectsDifference`（一致 / 行尾空行 / 多一条规则 / nil 模型 四例）。

实机验证：临时实例 `/api/model` → `diverged:false, drift:null, rules:11`；`POST /api/model/resync` → 200 `ok=true rules=11`，之后仍 `diverged:false`。

### 14.26.6 教训

1. **有第二真源就有漂移**。文件是 dae 的真源，model.json 是 UI 的真源，两者一旦分叉，「应用」就是从 UI 覆盖文件的动作，会静默吃掉手工改动。防护只能靠每次读模型时做一致性检查 + 一键反向同步。
2. 规则语义迁移不能只看名字：`domain`+`ip` 在 v2rayN 是「或」、在 dae 同一行是「与」，拆行与否直接改变命中集合。
3. 验证代理质量必须**等冷却期**（§14.25），否则会把「组还没测完速」误判成「节点坏了」。

## 14.27 一条命令部署（deploy/install.sh + deploy/deploy.sh，v0.35）

### 14.27.1 目标

把「交叉编译 → 上传 → 目录/证书/账号 → 服务定义 → 启动 → 自检」压成两条命令，且**幂等**：重复执行不覆盖已有证书、凭证与业务配置。

### 14.27.2 两个脚本的分工

| 脚本 | 运行位置 | 职责 |
|---|---|---|
| `deploy/deploy.sh` | 开发机 | 探测目标 `uname -m` → 交叉编译（`GOOS=linux GOARCH=<auto> CGO_ENABLED=0`，`-trimpath -ldflags "-s -w"`）→ scp 二进制与 install.sh 到 `/tmp/hnp-deploy` → 远程执行 → 清理 |
| `deploy/install.sh` | 目标机（root） | 8 步安装：能力探测 / 目录 / 二进制 / 证书 / 账号 / 服务定义 / 启动 / 自检；`--dry-run` 只打印不写盘 |

### 14.27.3 install.sh 的关键取舍

1. **服务管理器自动识别**：`/etc/openwrt_release` 或 `/sbin/procd` → procd 的 `/etc/init.d/homenet-panel`；`/run/systemd/system` → systemd unit；都识别不到则不启动、只打印手动启动命令（fail-open，不猜）。
2. **已有资源不覆盖**：证书存在则保留（`--force-cert` 才重签）；`/etc/homenet-panel.auth.json` 存在且没传 `--password` 则保留；旧二进制备份成 `homenet-panel.bak`，旧 unit/init 备份到 `/root/homenet-panel.{service,init}.bak.<时间戳>`。
3. **密码只显示一次**：首次安装用 `openssl rand -base64 12` 生成 16 字符随机密码，经 `setpass` 写入后仅在末尾摘要里打印。
4. **对外网卡自动识别**：取默认路由的 `dev`，识别不到再退到首个全局 IPv4 网卡；也可 `--traffic-iface` 指定。
5. **EnvironmentFile 保留**：目标机存在 `/etc/homenet-panel.env` 时自动写入 `EnvironmentFile=-`（前导 `-` 保证文件缺失也不报错）。
6. **自检走系统工具**：`ss -ltn` 看端口（服务器上没有 curl 也成立）、`pgrep -x dae`/`systemctl is-active dae` 看内核守护。

### 14.27.4 实机验证（192.168.1.85，Debian 13 trixie）

```
[1/4] 目标 root@192.168.1.85 (x86_64) → 构建 linux/amd64
[2/4] 交叉编译 → bin/homenet-panel-linux-amd64 （8.9MB，-s -w 后）
[4/4] 远程安装
  1/8 systemd / BTF 可用 / memlock 足够 / dae version v2.1.1 / 对外网卡 ens2
  4/8 已有证书，保留        5/8 已有凭证，保留
  6/8 旧 unit 备份 → /root/homenet-panel.service.bak.20261025-220402，写入新 unit
  7/8 homenet-panel 运行中    8/8 端口 8787 已监听 / dae 运行中
部署完成 → https://192.168.1.85:8787  admin
```

安装后 unit 实际内容（保留 `EnvironmentFile=-`，ExecStart 调成与原手工版等价的参数）：

```ini
ExecStart=/usr/local/bin/homenet-panel serve --addr 0.0.0.0:8787 \
  --backup-dir /var/lib/homenet-panel/backups --audit-path /var/lib/homenet-panel/audit.log \
  --audit-max-mb 5 --audit-keep 3 --traffic-iface ens2 \
  --tls-cert /etc/homenet-panel/tls.crt --tls-key /etc/homenet-panel/tls.key
```

副作用说明：安装过程**只**替换面板自身二进制与 unit，不动 `/etc/dae/**`；实测安装前后 `panel-core.dae` 与 `config.dae` 的内容未变、dae 未重启。

### 14.27.6 在线安装（v0.35 追加）

`deploy/online-install.sh`（目标机侧）+ `.github/workflows/release.yml`（CI 侧）：

- workflow 打 `v*` tag 触发：`go vet` + `go test` → 交叉编译 amd64/arm64（v0.47 起再加 armv7/386，见 §14.38；`-trimpath -ldflags "-s -w"`，8.9MB）→ 拷两个安装脚本 → `sha256sum` → `gh release create`（已存在则 `gh release upload --clobber`）。
- 在线脚本识别 `uname -m`，取 `homenet-panel-linux-<arch>` + `install.sh` + `sha256sums.txt`（校验后才继续），再以 `--binary` 调 `install.sh`，结束清理临时目录。
- **私有仓库必须走 API**：`github.com/.../releases/latest/download/<asset>` 对私有仓库恒返回 404（即使带 token，因为那是网页通道）；正确姿势是 `GET /repos/{owner}/{repo}/releases/{latest|tags/TAG}` 找 `asset.id`，再 `GET /repos/.../releases/assets/{id}` 带 `Accept: application/octet-stream`。脚本里 `asset_url()` 就做这件事（优先 python3，其次 OpenWrt 的 `jsonfilter`，最后 grep 兜底）。
- 下载器优先级：`curl` → `wget` → `python3 urllib`（.85 上没有 curl，实测走 wget）。
- CI 首次失败原因：`cd homenet-panel && go build -o dist/...` 把产物写进了 `homenet-panel/dist`，随后在仓库根 `cp deploy/* dist/` 报 `cp: target 'dist/': No such file or directory`；改为先在根建 `dist/`、build 输出 `../dist/` 后通过（run 40200858051 failure → 修复后 success）。

### 14.27.5 坑

1. 早期版本用 `"$BIN" --print-version` 判架构，二进制没有该 flag 会误报「架构不匹配」；改为「执行它、只看 126/127 退出码（exec format error / not found）」。
2. `ssh host 'cmd'` 加 `-t` 会打印 `Pseudo-terminal will not be allocated`，无实际影响。
3. 部署脚本在 macOS 上要显式给 `HOME` 与 `GOCACHE`，否则 Go 报 `GOCACHE is not defined and $HOME is not defined`。

### 14.28 安装器补依赖预检：dae 一键装 + goose 退役（v0.36）

目标机第一次装面板时，最常见的两个「装完不能用」来源是**没有 dae** 与**还留着 goose**（goose 是面板的前身，会和面板抢同一份 dae 配置）。`deploy/install.sh` 新增 0/8 步统一处理：

| 检测项 | 判据 | 处置 |
| --- | --- | --- |
| dae 已装 | `command -v dae` | 打印版本；`pgrep -x dae` 不在则提示「应用配置会失败」 |
| dae 未装 | 同上为空 | 提示 degraded；`--install-dae` 时自动加 daeuniverse APT 源并安装 |
| goose 存在 | `command -v goose` / `/etc/goose` / goose.service / `pgrep -x goose` | 默认只告警；`--retire-goose` 退役到 `/root/goose-retired` |

- **dae 自动安装**只走官方 APT 仓库（`https://daeuniverse.pages.dev`），按 APT 主版本选 `daeuniverse.sources`（APT ≥ 3）或 `daeuniverse.list`；同时下载 `daeuniverse-archive-goose.gpg` 到 `/usr/share/keyrings/`。三个 URL 实测 200（142B / 111B / 2275B）。非 APT 系统直接 `die` 并给文档链接，绝不瞎猜包管理器。
- **goose 退役是「移动」不是删除**：先 `systemctl disable --now goose.service`，再把二进制、`/etc/goose`、unit 分别移到 `/root/goose-retired/{bin,etc,unit}`，写 `README.txt`（含「config.yaml 里有订阅 token，别外传」与恢复步骤），最后 `daemon-reload`。默认不执行，必须显式 `--retire-goose`。
- 实机干跑（192.168.1.85，临时造 `/etc/goose` + 假 `/usr/local/bin/goose`）输出：默认 `[!] 检测到 goose … [=] 一并退役：加 --retire-goose`；加 `--retire-goose` 后打印 4 条「将执行/将移动」且**未真正移动**（dry-run 会跳过所有 mv），清理后 `/etc/goose` 与 `goose` 均不存在。

### 14.28.1 goose 的「完整残留面」（v0.38 补）

n6 上退役 goose 后复查发现两个首次退役没覆盖到的点，已补进 `install.sh` 的 0/8 依赖预检：

| 残留形态 | 实际情况（n6） | 处置 |
| --- | --- | --- |
| 额外 systemd unit | `/lib/systemd/system/goose-update-geo.{service,timer}`（`ExecStart=/usr/local/bin/goose update-geo`，Weekly Sun 04:00，`disabled` 但 `preset: enabled`，不属于任何 dpkg 包） | 检测范围从「只看 `/etc/systemd/system/goose.service`」扩到 `/etc/systemd/system/goose*.{service,timer}` + `/lib/systemd/system/goose*.{service,timer}`；`--retire-goose` 时逐个 `systemctl disable --now` 再移到归档 `unit/` |
| 另一个写 dae 配置的面板 | `dpkg` 里装着 **daed 2.1.1**（`/usr/bin/daed`、`/etc/daed/wing.db`），服务 `disabled`+最后一次运行 2026-10-20 3h14m 后 failed，当前未运行 | 只告警不自动处置：打印「daed 与本面板都会写 dae 配置，二者只能留一个」，并按 `is-active` 给出 `systemctl mask daed` / `apt remove daed` 建议 |

- goose 归档已于 2026-10-25 在 n6 **整目录删除**（7.6MB）：删前写 `/root/goose-retired.inventory.txt`（2046B，含目录内每个文件的路径/字节/权限 + 两个 unit 的原文 + 恢复方法 + 「config.yaml 含订阅 token」提示），删后核对 goose 不在 PATH、systemd 无 goose 条目、面板与 dae 仍 active、`/etc/dae/config.dae` md5 `6d46aa9b1a7f021accd7499fdbb04ad0` 未变。
- 实机验证（dry-run，未写盘）：.85 → 「未发现 goose」无告警；.85 造一个假 `goose-fake.timer` → 「检测到 goose」+「将执行: systemctl disable --now goose-fake.timer」+「将移动 …/goose-fake.timer」；n6 → 「未发现 goose」+ daed 告警（`daed 当前未运行（failed）`）。

## 14.29 单文件布局解析与空规则安全闸（v0.37）

### 14.29.1 现象

在 n6（Armbian bookworm / aarch64）用在线安装装完后第一次打开页面：内容层 **0 条规则**，红字 `parseError=未解析出任何配置块（可能格式不匹配）`，黄条提示模型漂移「读不到线上主体配置 /etc/dae/config.d/panel-core.dae」；而线上实际有 13 条规则、1 个分组、2 个订阅。

### 14.29.2 根因

`internal/api/server.go` 的 `loadModel()` 固定读 `s.CorePath`（默认 `/etc/dae/config.d/panel-core.dae`）。.85 是面板接管后的 include 双文件布局，所以一直没暴露；n6 是 goose 留下的**单文件布局**（`/etc/dae/config.dae` 2046B，首行「# 由 Goose 生成 …」，无 `config.d/`）→ coreText 为空 → `model.Parse("")` 什么都不产出。

危险方向是不对称的：界面显示 0 规则 → 用户点「校验+备份+reload 应用」→ 写进去的是「空的 panel-core.dae + include」→ reload 成功、dae 继续运行，但**分流规则被静默清空**。这类 bug 不能靠用户眼力兜底，必须在写入侧阻断。

### 14.29.3 代码改动

| 位置 | 改动 |
| --- | --- |
| `internal/api/server.go` | 新增 `liveTexts() (entryText, coreText string, single bool)`：CorePath 缺失/为空时把入口文本当主体返回 `single=true`；新增 `coreExists() bool` |
| `loadModel()` / `handleResyncModel` | 改用 `liveTexts()`（`model.json` 仍优先） |
| `internal/api/model_drift.go` | 单文件布局直接返回 nil 漂移（没有第二份文件可比） |
| `handleGetModel` | 响应增加 `layout`（`split`/`single`）与 `layoutNote`（说明单文件布局下「应用」会改写成 include + `config.d/panel-core.dae`，自动备份可回滚） |
| `handleApplyModel` | 安全闸：提交的 `rules` 为空 **且** 线上主体含 `routing {` → 422「拒绝应用：提交的模型没有任何分流规则…若确实要清空，请先手工改配置再点「从线上重新解析」」 |
| `internal/api/index.html` | 蓝框 banner「线上是单文件布局：{layoutNote}」，位置在黄框漂移提示之前 |
| `internal/api/model_layout_test.go`（新） | `TestLiveTextsSingleFile`（single=true、rules=2、fallback=proxy、groups=1、subs=1、drift=nil）、`TestApplyModelRejectsEmptyRules`（空 rules + 线上有 routing → 422） |

### 14.29.4 实机验证

- **n6（单文件）**：`/api/model` → `layout=single`、`parseError=None`、`rules=13`（`pname(NetworkManager, systemd-resolved, dnsmasq)`、`dip(224.0.0.0/3, 'ff00::/8')`、`dip(geoip:private)`、`dip(geoip:cn)`、`dip(198.41.192.0/19, …)`、`domain(geosite:cn)`…、`l4proto(udp) && dport(443)`、`dport(853)`）、`groups=1`（proxy / min_moving_avg）、`subs=2`（cf、dual，均 `file://subs/*.sub`）、`fallback=proxy`、`diverged=false`；`/api/subs/nodes` `total=30`。
- **.85（include 双文件）**：`layout=split`、`rules=11`、`groups=3`、`subs=3`、`diverged=false`，未受影响。
- `go test ./... -count=1` 全绿；发布 **v0.35.4**（workflow run 40205550549 success，资产 6 个，含 `verify-install.sh`）。
- `install.sh` 幂等复装：保留原凭证与证书（输出「密码: (沿用原有凭证)」），`/etc/dae/**` 未变（.85 的 `config.dae` 218B/mtime 10-24 23:47、`panel-core.dae` 1228B/mtime 10-25 21:45）。

### 14.29.5 教训

1. 「只认自己那套布局」是典型的环境绑定 bug：任何「读线上配置」的入口都必须**先判布局再决定读哪个文件**（面板接管前的老机器、goose 遗留机都可能是单文件）。
2. 写侧必须比读侧更保守：解析失败/模型为空时无条件拒绝写入，而不是让用户看出「怎么是空的」。
3. 安装脚本的幂等性要用「**再装一次**」验证，不能只看首次成功。
4. 排查时注意 `/api/model` 里分组字段名是 `filters`（复数，`model.Group{Name,Policy,Filters}`），不是 `filter`——用错字段名会把正常数据误判为 None。

## 14.30 Geo 数据更新、出站白名单与「未找到该出站」（v0.39）

### 14.30.1 现场问题的三个答案

| 用户问题 | 结论 | 证据 |
| --- | --- | --- |
| 规则第 1 条的出站 `must_direct` 显示「未找到该出站」 | `must_direct`/`must_block` 是 **dae 内置出站**，面板前端白名单漏了；反过来 `reject` 面板当成合法、dae 却不认 | 逐个 `dae validate`：`-> direct/block/must_direct/must_block` 全部 rc=0；`-> reject` rc=1 `outbound (group) "reject" not found`；`grep -ac must_direct /usr/bin/dae` = 1 |
| 这些规则是谁写的、用的哪个 geo 库 | 规则来自 goose/daed 时代；数据源是 **Loyalsoldier v2ray-rules-dat**（不是 v2fly 官方） | `/usr/share/dae/geoip.dat` 16,570,425B 与 Loyalsoldier 发布资产**逐字节同大小**；v2fly 官方 geoip.dat = 23,258,601B |
| `geosite:steam@cn`、`geosite:microsoft@cn` 会不会命中 | 不会。geosite.dat 里没有这两个 tag ⇒ **死规则**（dae 不报错，静默不匹配） | 扫描 geosite.dat 可打印串：有 CN/APPLE-CN/GOOGLE-CN/GEOLOCATION-CN/TLD-CN/CATEGORY-ADS-ALL/CATEGORY-AI-!CN/YOUTUBE/PRIVATE/STEAM，无 STEAM@CN、MICROSOFT@CN；Loyalsoldier 最新 release 同样缺 |

另：`dae --help` 的子命令只有 export / honk / reload / run / suspend / sysdump / trace / validate，**没有 update-geo**（那是 goose 自己的命令）⇒ geo 更新必须由面板提供。

### 14.30.2 新包 internal/geodata（下载 → 校验 → 备份 → 原子替换）

| 项 | 说明 |
| --- | --- |
| 默认源 | `DefaultURLs`：Loyalsoldier `releases/latest/download/{geoip,geosite}.dat`（tag 比 v2fly 全） |
| 校验 | 小于 1MiB 直接判失败；geosite 必须能扫到 `cn` tag，否则视为「不是 geosite.dat」 |
| 备份 | 替换前复制到 `<backupDir>/geo/<name>.<20060102-150405>`，按 `--geo-keep`（默认 3）轮转 |
| 写入 | `writeAtomic`：同目录临时文件 + 0644 + rename |
| 符号链接保护 | 目标若是符号链接（apt 装的 `v2ray-rules-dat` 会把 `/usr/share/dae/*.dat` 链到 `/usr/share/v2ray-rules-dat/*`）→ **下载前**就报错并提示用 apt 更新，绝不覆盖链接 |
| dry-run | 只下载+校验，不写盘、不 reload |
| reload | 真实更新成功后 `Dae.RunningPID()` + `Reload`，回填 `reloadCode`/`reloadOut`/`progress`；失败不静默 |

`/api/geo`（GET，返回 `data` = 目录、文件表（含 symlink 指向、可识别字符串数）、备份表、默认 URL、`keep`）、`/api/geo/update`（POST `{targets,urls,dryRun}`；`urls` 支持换镜像）；`--geo-dir`（默认 `/usr/share/dae`）、`--geo-keep`（默认 3）。

前端新增「Geo 数据」tab：文件表 + 可编辑数据源 + 历史备份 + 「刷新 / 检查更新（dry-run）/ 下载并更新（会 reload dae）」。

### 14.30.3 出站白名单与模型检查（internal/model/lint.go）

- `BuiltinOutbounds = direct / block / must_direct / must_block`；前端 `outInvalid()` 白名单与之对齐并**移除 reject**，datalist 补两个内置项。
- `Lint(m)` 检查：出站未定义、`reject` 用法、出站为空、`fallback` 未定义；`LintGeo(m, tags)` 用 geosite 的真实 tag 集合判断 `geosite:xxx` 是否会命中，命中不了就提示「这条规则永远不会命中」。
- `/api/model` 增 `issues` 与 `builtinOutbounds`；内容层顶部出现红框「模型检查发现 N 个问题」。

### 14.30.4 模型漂移改为「语义比较」

上一版 `normalizeConfig` 只去行尾空白/空行，导致手写的 `panel-core.dae`（换行、缩进、`tproxy_port: 12345` 的空格不同）被判成「模型与线上不一致」，n6 全新配置一上线就报 `diverged=true`。现改为：去空行 → 丢弃空 `subscription {}`/`group {}` → 折叠连续空白 → 去掉标点两侧空白 → 再逐字比较；`TestNormalizeIgnoresFormattingOnly` 覆盖「排版不同=一致、数值不同=不一致」。

### 14.30.5 n6（Armbian aarch64）彻底重装 dae

| 步骤 | 结果 |
| --- | --- |
| 备份 | `/root/dae-etc-backup-20261026-000532.tar.gz`（41,522B，含 `/etc/dae` 与 `/etc/daed`） |
| 卸载 daed | `apt-get purge -y daed`；核对 `/usr/bin/daed`、`/etc/daed`、`daed.service` 均不存在 |
| 加官方源 | `/etc/apt/sources.list.d/daeuniverse.sources` + `/usr/share/keyrings/daeuniverse-archive-goose.gpg` |
| 装 dae | `apt-get install -y dae` → dae 2.1.1/arm64 由 dpkg 管理，附带 `v2ray-rules-dat`（该机带宽约 32kB/s，下载 7 分 58 秒） |
| 副作用 | `/usr/share/dae/{geoip,geosite}.dat` 变成指向 `/usr/share/v2ray-rules-dat/` 的符号链接 ⇒ 面板 geo 更新按上表拒写并提示 apt 更新 |
| 从头配置 | 旧 goose 配置存 `/root/config.dae.goose-backup-20261026-001713`；新写 split 布局（入口只 include `config.d/*.dae`，主体 4 条 essential 直连 + `fallback: direct`），`chmod 600`，`dae validate` rc=0，删除旧 `model.json`，`systemctl start dae` → active |
| 面板 | arm64 二进制部署后 active；`/api/model` → `layout=split`、4 规则、`diverged=false`；`/api/geo/update` dry-run **0.0s** 返回符号链接说明 |

#### n6 的 apt 数据源其实就是 Loyalsoldier，只是版本日期不同

`dpkg -s v2ray-rules-dat` → `Version: 202610222206.0.0`、`Maintainer: dae@daeuniverse`、`Homepage: https://github.com/Loyalsoldier/v2ray-rules-dat` —— apt 包只是把 Loyalsoldier 的 dat 按时间戳重新打包发布，**与面板默认源同一家**。所以两机的差别只在「哪一天的构建」：

| 机器 | 安装方式 | geosite.dat | geoip.dat | 关键 tag |
| --- | --- | --- | --- | --- |
| 192.168.1.85 | 手工放的实文件（goose 时代下载） | 11,225,745B | 16,587,463B | CN / STEAM / APPLE-CN / CATEGORY-AI-!CN / GEOLOCATION-CN 均有；STEAM@CN、MICROSOFT@CN 均无 |
| n6 | apt 包 `v2ray-rules-dat`（2026-10-22 构建）经符号链接提供 | 11,381,894B | 16,569,543B | 同上（两机一致） |

结论：tag 集合一致，规则写法可互换；差异仅是数据构建日期带来的条目增减。

### 14.30.6 .85 上的真实更新验证

GitHub 直连在该网络下不稳（`unexpected EOF` / `dial tcp ... i/o timeout`，一次 dry-run 用掉 120s），因此用本机 HTTP 镜像做端到端验证：`/api/geo/update {urls:{geoip:http://127.0.0.1:8123/geoip.dat, ...}}` → 200、两文件替换成功、备份落到 `backups/geo/*`、`reloadCode=0`、`progress=2/OK`、dae 仍 active。失败态也验证过：dry-run 报错时返回 200 + `result.errors`，真实更新报错时 422 且不动现有文件。

### 14.30.7 教训

1. 新建 Go 包目录要先 `mkdir -p` 再写文件，否则 `fs.writeFile` 直接 `ENOENT`。
2. `File.Tags` 带 `omitempty`，探针脚本用 `f["tags"]` 会 `KeyError`，要 `.get()`。
3. 网络不稳的机器上「先下载再判符号链接」会让 dry-run 卡 2 分钟；判断顺序要按代价从低到高排。
4. 漂移检测只比语义，不比排版，否则全新配置也会报「被手工改过」。

## 14.31 Geo 版本检测（v0.40）

### 14.31.1 为什么需要「检测版本」

`geoip.dat` / `geosite.dat` 是 protobuf，**文件里没有版本字段**，所以「本地是哪个版本」只能靠两条线索：

1. **apt 托管**（本项目在 n6 上就是这种）：`/usr/share/dae/*.dat` 是指向 `/usr/share/v2ray-rules-dat/*` 的符号链接，版本就是 apt 包版本（`dpkg-query -W -f=${Version} v2ray-rules-dat` → `202610222206.0.0`，折成上游 tag 即 `202610222206`）。
2. **面板/手工文件**：面板每次成功更新后把「来源 URL、字节数、sha256、上游构建号、时间」写进 `<备份目录>/geo-version.json`；没有记录就只能拿本地大小与上游资产大小对比。

上游版本信息有两个来源，二者都不下载正文：GitHub release API（`/repos/Loyalsoldier/v2ray-rules-dat/releases/latest` → `tag_name` + `published_at` + 资产大小）与文件 `HEAD`（`Content-Length` + `Last-Modified`，需跟随 302 到 `release-assets.githubusercontent.com`）。

### 14.31.2 实现（internal/geodata/version.go）

| 组件 | 作用 |
| --- | --- |
| `DetectVersions(VersionOptions) VersionReport` | 汇总本地状态 + 上游构建号 + 逐文件结论；默认超时 15s |
| `fetchLatestRelease` | 一次 GitHub API 请求拿 tag / 发布时间 / **每个资产的大小** |
| `headSize` | 仅当数据源被改成镜像（非默认 GitHub 源）时才 HEAD，取大小与 Last-Modified |
| `aptVersion` | `dpkg-query` 读已装版本、`apt-cache policy` 读候选版本 |
| `pkgVersionTag` | `202610222206.0.0` → `202610222206` |
| `LoadState` / `SaveState` | `<备份目录>/geo-version.json`（0600→0644 原子写）；`Update` 成功后回填 |
| `verdictFor` | 依次判断：本地 tag == 上游 tag → 一致；apt 托管且 tag 不同 → 看候选版本给「apt 可升级」或「apt 源尚未提供」；否则比大小（相同=一致，不同=可能有新版本） |

接口：`GET /api/geo/versions`（可带 `?geoip=&geosite=` 覆盖源），返回 `{report:{remoteTag, remotePublished, files[], hint, errors[]}}`；`GET /api/geo` 增 `state`（上次更新记录）。前端「Geo 数据」页顶部新增「检测版本（只发 HEAD，不下载）」按钮 + 版本对照表 + 复制 apt 命令。

### 14.31.3 两次提速（实机逼出来的）

| 版本 | 问题 | 处理 | n6 实测 |
| --- | --- | --- | --- |
| 初版 | apt 托管也去 HEAD GitHub，n6 上两条 HEAD 各 45s 超时 → 接口 91 秒 | apt 托管跳过 HEAD；HEAD/API 并发；单次超时 15s | 1.0s |
| 二版 | .85 上 GitHub HEAD 全超时（`context deadline exceeded`），只能显示「无法探测上游」 | 默认源改用 **release API 的资产大小**（一次请求），只有自定义镜像才 HEAD | 0.7s，两个文件都拿到上游大小 |

结论：**能被卡住的是 HEAD，不是 API**；能一次拿到的信息就不要逐文件请求。

### 14.31.4 实机结果

| 机器 | remoteTag | 本地 | 结论 |
| --- | --- | --- | --- |
| n6（apt 托管，Linux arm64） | 202610242205 | 202610222206（apt 202610222206.0.0） | 「本机 202610222206，上游已发布 202610242205（apt 源尚未提供；可先 apt update 再看）」+ hint |
| .85（面板文件，Linux amd64） | 202610242205 | geoip 16,587,463B / geosite 11,225,745B（goose 时代） | 「上游构建大小不同（本地 16,587,463 / 上游 16,570,425 字节），可能已有新版本」 |

浏览器端（CDP + n6 真实会话）：点「Geo 数据」→「检测版本」后表格渲染出「apt（符号链接）/ 15.80 MB / 202610222206 / apt 202610222206.0.0 / 15.80 MB / 2026-10-24 22:06:35 / 结论」，DOM 26,710 字节、`{{` 泄漏 0。

### 14.31.5 边界（刻意不做的事）

1. **不自动升级 apt 包**：检测只给结论与命令，`apt install --only-upgrade v2ray-rules-dat` 由用户执行（面板不替用户操作包管理器）。
2. **不以「大小相同」当作内容一致**：只在没有 tag 可比时作为启发式；面板自己更新过的文件用 sha256 + tag 判定。
3. **符号链接永不覆盖**：apt 托管的文件，面板即使点了「下载并更新」也只返回 apt 指引。
## 14.32 订阅 UA：为什么留空会 403（v0.41）

现象（用户截图）：订阅源页 `dual` 一行，User-Agent 留空 → 「拉取上游失败: 订阅返回 HTTP 403 Forbidden」，本地 0 节点 / 上游 0 节点。

根因：`internal/subs/subs.go` 的 `fetchWithUA` 在 UA 为空时不设置 `User-Agent` 头，Go 会补默认值 `Go-http-client/1.1`；`s3.dlpublicsub.com`（Cloudflare 后）按 UA 判客户端，脚本式 UA 直接 403，而 `clash-verge/v1.7.7` 等常见客户端 UA 返回 200（同一 URL、同一时刻实测：Go 默认 → 403，clash-verge → 200/38655B）。所以**不是链接失效，也不是协议不支持**。

修法（v0.41）：新增 `var DefaultUA = "clash-verge/v1.7.7"`，UA 解析顺序改为「单条订阅的 ua → 进程级 `--sub-ua` → `DefaultUA`」，即**留空也能拉**；前端占位文案改成「User-Agent（留空=clash-verge/v1.7.7）」。仍保留 per-source UA：少数机场只认特定客户端的 UA（如 `mihomo/…`、`v2rayN/…`），在那一行填即可覆盖。

测试：`internal/subs/subs_test.go` 的 `TestFetchRawUA` 改为断言「留空 → 发出 DefaultUA 且成功」「显式 UA 优先」「`SetUserAgent` 优先于 DefaultUA」，另加 `TestFetchRawNoUAFallsBackToGoDefault`（把 `DefaultUA` 置空时确实回退到 `Go-http-client`）。

实机验证（n6，v0.37.1）：`/api/subsrc` 里 `dual.ua = None`，`POST /api/subsrc/check {name:"dual"}` → `format=clash`、`remoteCount=29`、`localCount=29`、`changed=false`，不再 403。
## 14.33 面板内「安装与初始化」：把 install.sh 的缺口做成按钮（v0.42）

### 14.33.1 起因（两个真缺口）

用户问「现在在线部署会帮我安装 dae 吗」，核实后暴露两件事：

1. `deploy/install.sh --install-dae` 只装二进制与服务，**不写** `/etc/dae/config.dae`；而 `dae.service` 的 `ExecStartPre=dae validate` 会让服务直接启动失败。
2. install.sh 里那句「dae 还没配置：首次打开面板可生成初始配置」在当时是空头承诺——`internal/`、`cmd/` 里没有任何初始配置生成逻辑。

于是用户提出：「可以把安装做成按钮写到面板上吗」。答案是做成「安装与初始化」页：状态只读 + 三个动作（安装 dae / 生成初始配置 / 启动 dae），每个动作都能 dry-run。

### 14.33.2 实现

| 位置 | 内容 |
| --- | --- |
| `homenet-panel/internal/bootstrap/bootstrap.go` | `Options{Runner,Fetch,EtcDir,EntryPath,CorePath,DryRun,IsRoot}`；`Detect(o) Status`（root / 服务管理器 / APT / dae 版本与运行 / 配置大小与是否面板托管 / BTF / memlock）；`InstallDae`（APT 源 + GPG key + `apt-get update` + `install -y dae`，非 APT 系直接给文档链接）；`WriteInitConfig`（`model.Render(DefaultModel())` 写入口 + panel-core.dae，0600）；`StartDae`（systemd enable+restart 或 procd restart，轮询 is-active 最多 5s）；`DefaultModel()` = tproxy 12345 + wan_interface auto + DNS udp://223.5.5.5:53 + `fallback: direct`（EssentialRules 由 `model.Render` 自动前置注入） |
| `homenet-panel/internal/api/bootstrap.go` | `GET /api/setup/status`、`POST /api/setup/install-dae`、`POST /api/setup/init-config`、`POST /api/setup/start-dae`（后三者带 `{dryRun}`，init-config 另带 `{force}`）；全部 `s.auth` 保护 + `setupMu` 串行 + 审计动作 `setup.installDae` / `setup.initConfig` / `setup.startDae` |
| `homenet-panel/internal/api/index.html` | 新增 tab「安装与初始化」：现场状态表（运行身份 / 服务管理器 / APT / dae / 配置文件 / 主体配置 / BTF 与 memlock）+ 一键操作（安装 dae、生成初始配置、启动 dae、两个 dry-run）+ 执行记录表（步骤 / 命令 / 结果 / 输出） |
| `deploy/install.sh` | 那句不实提示改为「装完面板后打开『安装与初始化』页可一键生成初始配置」 |

### 14.33.3 安全边界

- **必须是 root**：`Detect` 报 `root:false` 时三个动作都返回「需要 root 权限」，页面只读仍可用。
- **串行**：`setupMu` 保证同一时刻只跑一个动作，避免「安装 dae」与「生成配置」交叉写系统。
- **已有配置必须显式 force**：`WriteInitConfig` 只要入口或主体任一文件非空就拒绝（含面板自己托管的配置）——初版只拦「非面板托管」，实机复核时发现那会把线上 11 条规则冲成默认值，已收紧并补 `TestWriteInitConfigRefusesManagedConfigToo`。
- **写前备份**：`<path>.bak-<20060102-150405>`，模式沿用原文件权限。
- **validate 失败自动回滚**：`dae validate -c <entry>` 失败 → 用最新备份恢复两个文件并记录「已回滚到写入前的配置」步骤。
- **dry-run**：只打印命令与目标路径，不写盘、不调 apt、不重启服务（`Step.Skipped=true`）。
- **不做面板自更新**：避免面板在运行中替换自己的二进制。

### 14.33.4 实机验证（192.168.1.85，v0.42）

- `GET /api/setup/status` → `root:true, serviceManager:"systemd", hasApt:true, daeInstalled:true, daeVersion:"dae version v2.1.1", daeRunning:true, configExists:true, configBytes:218, configManaged:true, coreExists:true, btf:true, memlockKB:8192`。
- `install-dae {dryRun:true}` → 200，步骤「dae 已安装，跳过」（不重复装）。
- `init-config {dryRun:true}` → **422 拒绝**：`已存在配置（面板托管）：/etc/dae/config.dae；拒绝覆盖（生成初始配置只用于空白设备）。确认要替换请带 force=true（会先备份）` —— 这正是想要的护栏。
- `start-dae {dryRun:true}` → 200，三步 `systemctl enable dae` / `systemctl restart dae` / 检查运行状态（全部跳过）。
- 浏览器（headless Chrome + CDP，登录后点「安装与初始化」）：DOM 26,330 字节，`{{` 泄漏 0，渲染出状态表（root（可安装/初始化）/ systemd / APT 可用 / dae version v2.1.1 · 运行中 / 218 字节 · 面板托管）与三个动作按钮。

### 14.33.5 刻意不做

- 面板自更新、daed 式「面板当 daemon 管家」：面板只保证 dae 能跑起来，生命周期仍交给 systemd / procd。
- apt 卸载/降级 dae：破坏性太强，留给用户命令行。
- 非 APT 系自动安装（如 OpenWrt opkg）：只给官方文档链接，避免写错源把系统搞脏。

## 14.34 CDN 安装链路：dist / <tag>-dist 分支 + 多源回退（v0.43）

### 14.34.1 起因：初始环境连不上 GitHub

用户在一台 FNos（x86_64）上跑在线安装，两次都停在第 2 步：

```
curl: (18) HTTP/2 stream 1 was not closed cleanly ...
curl: (52) Empty reply from server
curl: (28) Failed to connect to github.com port 443 after 133331 ms
[x] 下载 online-install.sh 失败
```

三个问题叠在一起：

1. 初始环境的出网路径不稳定（HTTP/2 被中间设备打断、连接被静默丢弃）——这是用户侧事实，面板改不了；
2. 旧 `online-install.sh` 的 `fetch()` 是裸 `curl -fsSL`：**没有重试、没有 --http1.1**，一次抖动就整条安装失败；
3. 所有资产只有一个来源（GitHub Releases），一旦 github.com 不可达就没有退路。

用户结论（逐字）："我觉得用cdn会更好，因为一个初始环境可能不一定连得上github"。

### 14.34.2 方案：发布到普通分支，让 CDN 转售

GitHub Releases 的资产只能从 github.com 取；jsDelivr 这类 CDN 只转售**仓库文件**。所以 CI 在发布 Release 之后，把同一批产物再推进两个普通分支：

| ref | 内容 | 特性 |
|---|---|---|
| 分支 `dist` | 最近一次发布的全部产物 + `VERSION`（内容就是 tag 字符串） | 可变，指向最新；**CDN 上有分钟级缓存滞后** |
| 标签 `<tag>-dist`（如 `v0.38.2-dist`） | 该次发布的全部产物 | 不可变快照，CDN 解析正常、永久可缓存 |

对应 URL：

```
https://cdn.jsdelivr.net/gh/leoz26/dae-panel@dist/online-install.sh
https://raw.githubusercontent.com/leoz26/dae-panel/v0.38.1-dist/homenet-panel-linux-amd64
```

**实测结论（重要，走了两次弯路）**：

1. **分支名里带版本号会被 CDN 当成「版本」解析**：`@v0.38.1-dist`（分支）在 jsDelivr 上恒 404（`Couldn't find the requested file /VERSION in leoz26/dae-panel.`），`cdn.statically.io` 与 purge 后重试同样 404，但同一条分支在 `raw.githubusercontent.com` 上 200、文件完整。
2. **同名 git 标签则完全正常**：在 `dist` 分支的提交上打标签 `v0.38.2-dist` 后，`@v0.38.2-dist/online-install.sh` 立即 200 / 6635B（正确内容）。
3. **分支内容有缓存滞后**：`dist` 分支已更新到 6635B 的新脚本后，`@dist/online-install.sh` 在 cdn/fastly/gcore 三个入口仍返回旧的 6540B，purge 返回 `finished`（CF/FY 都刷了）后依旧；同一时刻 `@v0.38.2-dist` 已是新的。

结论：**最新版靠 `@dist`（可能滞后几分钟到几小时），钉版本靠 `@<tag>-dist` 标签（立即生效、永久缓存）**。安装脚本因此把标签源放在第一位，`@dist` 只作兜底。

工作流步骤（`.github/workflows/release.yml`，仅在 `refs/tags/v*` 时执行）：

1. `echo "$TAG" > dist/VERSION`，并把 `VERSION` 一起挂到 Release；
2. 在 `dist/` 里 `git init -b dist` → commit → `git push -f <token>@github.com/<repo>.git dist` 与 `dist:<tag>-dist`（每次都是新提交、强推覆盖，历史不膨胀）；
3. 对 7 个文件逐个请求 `https://purge.jsdelivr.net/gh/<repo>@dist/<文件>` 刷新 CDN 缓存（失败 `continue-on-error`）。

### 14.34.3 安装脚本：多源 + 重试 + 校验

`deploy/online-install.sh` 重写后的下载策略：

| 环节 | 行为 |
|---|---|
| 版本解析 | 先问 `api.github.com/repos/<repo>/releases/latest` 的 `tag_name`（权威）；不可达才读 `@dist/VERSION` 并打印「可能滞后」告警；都没有则退 `latest` |
| 源顺序 | `--base-url` 优先；否则 jsDelivr `@<tag>-dist` → jsDelivr `@dist` → fastly.jsdelivr.net → gcore.jsdelivr.net → raw.githubusercontent.com/`<tag>-dist` → github.com/releases/download/`<tag>` |
| 单次下载 | `curl -fsSL --http1.1 --retry 4 --retry-all-errors --connect-timeout 15 --max-time 600`；无 curl 退 `wget -q --tries=4 --timeout=20`；再无则 `python3 urllib` |
| 逐源回退 | **按源整体试**：`try_source()` 用同一个源取齐 `install.sh` + 二进制 + `sha256sums.txt`，任一失败或校验和不匹配就整源放弃；全部失败才 die |
| 完整性 | 拿到 `sha256sums.txt` 就比对；不一致时**换源重试**（而不是直接失败）——见下面的「串味」教训 |
| 私有仓库 | 匿名全失败且给了 `GH_TOKEN` 时，走 API 两步（`releases/latest|tags/<tag>` 取 asset.id → `releases/assets/<id>` 带 `Accept: application/octet-stream`） |
| 透传参数 | 未知参数收集进 `PASSTHRU` 交给 `install.sh`，因此 `--port`/`--traffic-iface`/`--install-dae` 照旧可用 |

### 14.34.4 CDN 可达性实测（开发机，2026-10-26）

对 `leoz26/dae-panel@main/README.md` 逐个发 GET：

| URL | 结果 |
|---|---|
| `https://cdn.jsdelivr.net/gh/...` | 200 / 8914B |
| `https://fastly.jsdelivr.net/gh/...` | 200 / 8914B |
| `https://gcore.jsdelivr.net/gh/...` | 200 / 8914B |
| `https://raw.githubusercontent.com/...` | 200 / 8914B |
| `https://cdn.statically.io/gh/...` | 200 / 8914B（脚本未列入首选，保留作手工镜像） |

发布后再验 `dist` 分支的资产：

| URL | 结果 |
|---|---|
| `@dist/VERSION` | 200，内容 `v0.38.1` |
| `@dist/online-install.sh` | 200 / 6635B（CDN 首字节后可缓存） |
| `@dist/sha256sums.txt` | 200，两个二进制的 sha256 |
| `fastly.jsdelivr.net/@dist/online-install.sh` | 200 / 6540B |
| `gcore.jsdelivr.net/@dist/online-install.sh` | 200 / 6540B |
| `raw.githubusercontent.com/<repo>/v0.38.1-dist/homenet-panel-linux-arm64` | 200 / 7,602,360B |
| `cdn.jsdelivr.net/@v0.38.1-dist/...`（分支） | **404**（版本号样式的分支名不被索引） |
| `cdn.jsdelivr.net/@v0.38.2-dist/online-install.sh`（标签） | 200 / 6635B（新内容，未滞后） |
| `cdn.jsdelivr.net/@dist/online-install.sh` | 200 / 6540B（**旧内容**，purge 后仍滞后） |

**端到端实测（两台真机，`--dry-run` 不写盘）**：

- n6（Armbian bookworm / aarch64，**无 curl 只有 wget**）：`wget -qO /tmp/oi.sh https://cdn.jsdelivr.net/gh/leoz26/dae-panel@main/deploy/online-install.sh && bash /tmp/oi.sh --dry-run` → 下载 6635B 脚本 → 8 步预检全过（dae v2.1.1 在跑、BTF 可用、对外网卡 eth0、已有证书与凭证保留、旧 unit 备份）→ `[+] 在线安装完成`。
- .85（Debian 13 / amd64）：同一命令，前几行输出证明链路正确 ——
  ```
  [=] 版本：v0.38.1
  [=] 来源：jsdelivr@dist fastly@dist gcore@dist raw/v0.38.1-dist raw/dist releases/download/v0.38.1
  [+] install.sh
  [+] homenet-panel-linux-amd64（8118456 字节）
  [+] 校验和一致
  [=] DRY-RUN：只检查，不写任何文件
  ```

### 14.34.6 两个真踩到的坑

**坑 1：校验和「串味」**。最初是逐文件独立回退：二进制可能来自 `@v0.38.2-dist`，而 `sha256sums.txt` 该源一次抖动后回退到了 `@dist`（已经是 v0.38.3）→ 校验必然不匹配，用户看到「下载损坏或被篡改」这种吓人的提示，实际只是源混用。改成 `try_source()`：**一个源要么全部成功，要么整体放弃**，校验不匹配也当作该源失败继续换源。

**坑 2：`@dist/VERSION` 缓存滞后导致版本漂移**。`dist` 分支已发到 v0.38.3，CDN 仍返回 `v0.38.2`，于是脚本按旧版本取资产。改成先问 GitHub API（权威），只有当 API 不可达时（例如用户那台连不上 github.com 的机器）才用 CDN 的 `VERSION`，并明确打印「可能滞后」告警；要绝对确定就用 `--tag vX.Y.Z`。

顺带记录两条环境事实：

- `.85` 这台机器上 `api.github.com` **不可达**（脚本打印了「GitHub API 不可达」），而 `cdn.jsdelivr.net` 可达——这正是用户要求走 CDN 的现实依据。
- CDN 的**多个入口之间可达性不同**：同一时刻在 n6（移动宽带）上 `cdn.jsdelivr.net` `rc=4` 网络失败、`fastly.jsdelivr.net` 也失败，而 `gcore.jsdelivr.net` 200 / 7866B。所以脚本里的 fastly/gcore 回退不是装饰，是实测必需。

### 14.34.5 边界与代价

- **jsDelivr 有分支缓存**：同一个 `@dist` 路径可能短时间内仍返回旧文件。缓解措施是 CI 主动 purge + 提供 `--tag vX.Y.Z`（走不可变的 `<tag>-dist`）。对「钉版本」场景缓存是好事，对「永远最新」场景偶发滞后。
- **jsDelivr 单文件上限 20MB**：当前二进制 8–9MB，够用；若将来膨胀要换 R2/OSS。
- **jsDelivr 不支持「版本号样式」的分支名，但支持同名标签**：所以钉版本一律用 `vX.Y.Z-dist` 标签，不要用分支。
- **分支缓存无法可靠 purge**：实测 purge 报 `finished` 后 `@dist` 仍是旧文件（`@main` 同样滞后）。因此「一条命令装最新」有几分钟到几小时的滞后风险；需要确定版本用 `--tag vX.Y.Z`，要拿最新脚本本身就用 `@<tag>-dist` 标签 URL。
- **CDN 只是传输层**：面板不因此信任任何内容——仍然验 `sha256sums.txt`，`install.sh` 仍然 `--dry-run` 可用。
- **不做**：不把二进制提交进 `main`（历史会膨胀）、不依赖第三方上传服务（用户没有 R2/OSS 账号）、不在面板里做「自更新」（仍是 apt/脚本的职责）。

## 14.35 「应用失败：failed to read pid file /var/run/dae.pid」是什么（v0.44）

### 14.35.1 现象

面板点「应用」后报：

```
应用失败: reload 失败(exit 1): Error: failed to read pid file: open /var/run/dae.pid: no such file or directory
Usage: dae reload [pid] [flags]
Flags:
  -a, --abort   Abort established connections.
  -h, --help    help for reload
```

### 14.35.2 根因：dae 没在运行，不是配置写错

- `dae reload` 的 pid 来源只有两个：命令行参数 `dae reload <pid>`，或默认 pid 文件 `/var/run/dae.pid`（`internal/dae/dae.go` 的 `DefaultPidFile`）。
- 面板调用时先 `RunningPID()` 读 pid 文件并确认进程存活；**读不到就传 0**，于是 dae CLI 回落到自己读 `/var/run/dae.pid`，也读不到 → 报上面这段并顺带打印用法。
- 也就是说：**这一行报错等价于「dae 进程当前没在跑」**。文件其实已经写盘并 0600、`dae validate` 也已通过，只是没有任何进程能加载它。
- 反过来也成立：只要 `/var/run/dae.pid` 存在且进程活着（本例两台机器 `.85` pid 3489、`n6` pid 1933），同样的 apply 会返回 `reloadCode: 0`、`progress 2/OK`。

常见触发场景（按概率排序）：

| 场景 | 判据 | 处理 |
| --- | --- | --- |
| dae 装了但从未启动/没有配置 | `ls /var/run/dae.pid` 不存在，`systemctl status dae` 为 inactive/failed | 「安装与初始化」页生成初始配置 → 启动 dae |
| 用 `--install-dae` 装完 dae，配置仍是空的 | `journalctl -u dae -n 20` 里 `ExecStartPre=dae validate` 失败 | 同上（生成初始配置） |
| dae 崩溃后被 systemd 启动限流挡住 | `journalctl` 含 `start request repeated too quickly` | `systemctl reset-failed dae && systemctl start dae` |
| dae 手动跑在前台或换了 pid 路径 | `pgrep -x dae` 有进程但无 pid 文件 | 用 systemd 托管，或 `dae reload <pid>` |

### 14.35.3 改法：reload 前先判「dae 在不在」

`internal/configtx/configtx.go` 新增：

```go
var ErrDaeNotRunning = errors.New("dae 没有在运行")

const DaeDownHint = "读不到 " + dae.DefaultPidFile + "。" +
	"先在「安装与初始化」页点『生成初始配置』和『启动 dae』（或用 systemctl start dae；OpenWrt 用 /etc/init.d/dae start），" +
	"确认 dae 在运行后再点应用。"

func explainDaeOut(out string) string // 把 failed to read pid file / no such file 翻译成上面那句，只带原始输出首行
```

- `Apply` / `ApplyMulti`：validate 通过、准备 reload 时若 `RunningPID() <= 0` → **把刚写入的文件回滚**、`RolledBack=true`、返回 `ErrDaeNotRunning` + `DaeDownHint`。不再把 dae 的用法说明当错误正文抛给用户。
- `Rollback`：同样先判，文件已恢复但明确提示「未重载」。
- 失败分支的 `msg` 统一过 `explainDaeOut`：即使 pid 在调用瞬间消失（竞态），报错也仍是人话。
- 订阅 apply（`internal/api/subsrc.go`）本来就有 `pid <= 0` 分支（"镜像已更新，但 dae 未运行，未执行 reload"），行为不变。

### 14.35.4 测试

`internal/configtx/configtx_test.go` 新增：

- `TestApplyDaeNotRunning`：`runningPID: 0` → 返回 `ErrDaeNotRunning`、`RolledBack=true`、目标文件回到原文、**reload 调用 0 次**、错误文案含「安装与初始化」。
- `TestApplyMultiDaeNotRunning`：多文件事务同样回滚。
- `TestExplainDaeOutPidFile`：pid 文件报错被翻译、**不回显 `Usage:`**、未知错误原样透传。

### 14.35.5 为什么不在 apply 里顺手把 dae 拉起来

- 面板的职责是「写配置 + 让 dae 重载」，**启动/重启进程属于「安装与初始化」页**（显式按钮 + 审计记录），自动拉起会让一次误操作变成「服务被面板悄悄重启」。
- 唯一例外是既有的「转发自检」：那不是重启 dae，只是在 reload 报 `ipforward on <iface> is off` 时补 sysctl 再重试一次（§14.21.4）。

## 14.36 计划步骤的白话说明（Note 字段，v0.45）

用户反馈「第②块『旁路由看不到域名只能看到 IP』看不懂」。截图里的四块面板侧步骤此前只有技术化 Detail，本次给每个 Step 加了一个 `Note` 字段（`internal/access/plan.go` 的 `Step.Note`，JSON `note`），前端渲染成详情下方的蓝色提示框（`.note`，`white-space: pre-line` 让 `\n` 自然分段）。

四条 note 的要点：

| 步骤 | 白话解释的核心比喻 |
| --- | --- |
| ① `panel-access.dae` / lan_interface | 「dae 在小区哪个门岗值班」：不填只管自己家（本机），填 `ens2`/`br-lan` 则从这个门进来的住户（LAN 设备）都归它管；它决定「谁来处理」，不决定「怎么处理」 |
| ② `panel_dns` / DNS 重定向 | 网络包里只有**门牌号（目标 IP）**，没有**收件人姓名（域名）**；「youtube.com = 142.250.72.14」的对照只在查 DNS 的一瞬间出现，设备把 DNS 发给主路由，dae 就没听见 → 只能看到发往该 IP 的连接，认不出是谁，所有 `domain(...)` 规则失效（只剩 `ip/geoip` 有效）。这块就是把「查号码的过程」也交给 dae 听一遍。附带说明 dae 会从 HTTPS SNI 嗅探域名补漏，但加密/纯 IP 连接看不到 |
| ③ `panel_snat` | 被 dae 接管的连接旁路由「本地收下再自己发出去」，回程天然对称，所以通常不需要 SNAT；只有没被接管、只是被转发的流量（ping/ICMP、内网互访、部分 IPv6）才可能回程绕道，症状是「能连上但网页打不开 / ping 不通」，那时再改 `on` |
| ④ `panel_tproxy` | tproxy 是「把连接半路改道到本地代理端口」的手艺，dae 已经自己做（netkit 设备对 + 独立 netns），面板再叠一层只会打架；等需要「某些目标明确不走代理」的白名单例外时才建 |

验证：`TestPlanStepsHavePlainLanguageNotes`（每个面板侧 Step 必须有非空 Note，且 DNS 那步必须含「门牌号」）；实机 `POST /api/access/plan` 的四条 step 都带 `note`；CDP 浏览器实测 DOM 35,286 字节、模板占位符泄漏 0、`class="note"` 出现 4 次、正文含「门牌号」。部署到 `.85`(amd64) 与 `n6`(arm64)。

## 14.37 结构化 DNS 编辑器（v0.46）

### 14.37.1 起因

面板的 `dns { }` 段一直是**原样文本**（`internal/model/model.go:73` 的 `DNS string`，Render 时 `dns {` + `ensureNL(m.DNS)` + `}`）。用户看到 daed 那类面板的 DNS 页有「上游服务器 / 请求规则 / 响应规则 / 各自兜底」，要求把等价能力做进本面板；同时明确**不要**做「无 IPv6 就自动 reject AAAA」的一键开关（见 §14.37.6）。

### 14.37.2 设计：结构化视图只是「一层皮」，真源仍是文本

为不破坏既有 apply 事务、漂移检测（`modelDrift`）与单文件布局兼容，**没有**把 `Model.DNS` 改成结构体，而是新增一个纯函数的结构化视图：

```
dns 块正文（Model.DNS，原样文本）
        ↓ ParseDNS（宽松，看不懂的行忽略）
DNSSpec{Upstreams, Request, RequestFallback, Response, ResponseFallback}
        ↓ 前端表格编辑
        ↓ RenderDNS（规范化 4 空格缩进）
新的 dns 块正文 → 写回 Model.DNS → 走原来的 /api/model/apply（校验+备份+reload+失败回滚）
```

好处：写配置的路径仍然只有一条（模型 → Render → 事务），DNS 编辑与订阅/分组/规则共享同一套备份、指纹、回滚与审计；dns 解析失败也不会让整体模型解析崩掉。

### 14.37.3 代码

| 位置 | 内容 |
| --- | --- |
| `internal/model/dns.go` | `DNSUpstream{Name,Addr}`、`DNSRule{Matcher,Out}`、`DNSSpec{Upstreams,Request,RequestFallback,Response,ResponseFallback}`、`DefaultDNSSpec()`、`ParseDNS(body) DNSSpec`、`RenderDNS(spec) string`、`LintDNS(spec) []string`；辅助 `blockOpen/splitKV/quoteAddr/stripLineComment`（行内 `#` 注释按引号状态剥离） |
| `internal/api/dns.go` | `POST /api/dns/parse {text}` → `{spec, warnings, defaulted}`（线上没有 dns 块或解析不出内容时返回 `DefaultDNSSpec()` 并置 `defaulted=true`）；`POST /api/dns/render {spec}` → `{text, warnings}`；两个端点只读、不写盘，均在 `s.auth` 下 |
| `internal/model/dns_test.go`、`internal/api/dns_test.go` | 线上真实文本解析、带规则与行内注释的解析、`Render→Parse` 往返深比较、默认值 lint 干净、坏输入告警文案、两个 handler 的往返与「空文本给默认值」 |
| `internal/api/index.html` | 新标签页「DNS」：上游表（名字/地址/删除）、请求规则表、响应规则表、两个兜底下拉，按钮「从当前配置重新载入 / 填回默认值 / 渲染预览（不写盘）/ 保存到模型并应用」，告警黄条 + 渲染结果 `<pre>` 预览 |

### 14.37.4 dae 真正接受什么（.85 上逐条 `dae validate` 实测）

这一节是编辑器的**校验依据**，不是抄文档：

| 写法 | 结果 |
| --- | --- |
| `qname(geosite:cn) -> cn` | rc=0 |
| `qname(suffix: apple.com.cn, suffix: icloud.com.cn) -> cn` | rc=0 |
| `qname(keyword: lucky) -> cn` | rc=0 |
| `qname(regex: .*\.cn$) -> cn` | rc=0 |
| `qname(geolocation-cn) -> cn` | **rc=1** `addQName: unsupported key:` —— dns 的 qname 不支持 geolocation（分流 routing 里可以） |
| `qtype(aaaa) -> reject` | rc=0 |
| `qtype(aaaa) -> drop` | **rc=1** `upstream "drop" not found` —— dns 的动作只有 `reject` |
| 请求规则里 `-> accept` | **rc=1** `upstream "accept" not found` —— `accept` 只能用在响应规则 |
| `response { ip(geoip:cn) -> reject }` | rc=0（可用于挡污染解析） |
| 上游 `udp://` / `tcp://` / `tls://` / `https://` | 均 rc=0 |
| 只有 `upstream { }`、没有 `routing { }` | rc=0 |

据此 `LintDNS` 输出中文告警：上游缺协议前缀、上游重名/缺地址、规则指向不存在的上游、`drop` 提示改 `reject`、请求规则里误用 `accept`、`qname` 里写了 `geolocation`。前端下拉也按上下文给（请求规则 = 上游名 + `reject`；响应规则 = 上游名 + `reject` + `accept`）。

### 14.37.5 实机与浏览器验证

| 项目 | 结果 |
| --- | --- |
| `.85` 临时实例（127.0.0.1:8899，独立 auth/session/audit/backup）`/api/dns/parse` 线上 dns 文本 | 200：`{cn: udp://223.5.5.5:53}` + `requestFallback=cn` + `responseFallback=accept`，告警为空 |
| `/api/dns/render` 原样渲染回去 | 与线上 `panel-core.dae` 的 dns 段**逐字节一致** |
| `/api/model/preview`（dns 不改） | 生成的 `panel-core.dae` 与线上文件 `strip()` 后**完全相同**（`identical_to_live: True`） |
| 加一条 `qname(geosite:cn) -> cn` 后再 preview | 只多这一行（`diff lines: ['qname(geosite:cn) -> cn']`） |
| CDP 浏览器（headless Chrome + 真实登录 cookie） | 点「DNS」标签：3 张卡片（上游/请求/响应）渲染，输入框预填 `cn` / `udp://223.5.5.5:53` / `cn` / `accept`，提示「已从当前配置载入」，模板占位符泄漏 0，两个 datalist（`dnso-req`/`dnso-res`）存在 |
| 浏览器点「填回默认值」+「渲染预览（不写盘）」 | 提示「已渲染（还没写盘）」，`<pre>` 出完整 dns 文本；点「+ 添加请求规则」表格增行 |

部署：交叉编译 amd64/arm64 → `.85` 与 `n6` 的 homenet-panel 与 dae 均 active；`gofmt -l` 干净、`go vet ./...` 与 `go test ./... -count=1` 全绿。

### 14.37.6 边界（刻意不做）

- **不做「无 IPv6 就 reject AAAA」开关**：用户明确不要。要挡 AAAA 就在请求规则里加 `qtype(aaaa) -> reject`（编辑器支持，并提示代价：IPv6-only 服务会解析失败）。
- **不校验 DNS 里的 geosite tag 是否存在**：`internal/model/lint.go` 的 `GeoRefs/LintGeo` 目前只覆盖分流规则；`qname(geosite:xxx)` 写错只是永不命中，不会让 validate 失败。
- **不改 apply 管线**：DNS 编辑保存后仍走 `POST /api/model/apply`（校验 → 备份 → 写盘 → reload → 失败回滚），没有第二条写配置通道。
- **不覆盖 dns 块的其他字段**（例如未经验证的 `bind` 等键）：当前只做 upstream + routing.request/response + 兜底，其他写法请用「整段配置(高级)」。
## 14.38 构建矩阵扩到 linux/armv7 与 linux/386（v0.47）

### 14.38.1 起因

此前 CI 只发 `linux/amd64` 与 `linux/arm64`，`deploy/online-install.sh` 遇到其它架构直接 die（`只发布 linux/amd64 与 linux/arm64`）。用户要求加 armv7 与 386：树莓派 2/3/4 跑 32 位系统、老 x86 软路由/工控机、部分 OpenWrt x86 镜像都是这两类。

### 14.38.2 改动

| 位置 | 改动 |
| --- | --- |
| `.github/workflows/release.yml` | 编译循环从 `for arch in amd64 arm64` 改为 `for spec in amd64:amd64:0 arm64:arm64:0 armv7:arm:7 386:386:0`，`spec` 是 `产物名:GOARCH:GOARM`；只有 `GOARM≠0` 时 `export GOARM`（`unset GOARM` 开头，避免污染下一个架构）；产物名仍是 `homenet-panel-linux-<名字>`，`sha256sum homenet-panel-linux-*` 自动带上新文件 |
| 同上 | jsDelivr purge 列表补 `homenet-panel-linux-armv7`、`homenet-panel-linux-386`；release notes 文案改为「amd64、arm64、armv7、386」 |
| `deploy/online-install.sh` | `uname -m` 映射扩容：`armv7l|armv7 → armv7`、`i386|i486|i586|i686|x86 → 386`；**`armv6l` 明确 die**：`GOARM=7` 的二进制在 ARMv6 CPU 上会 `Illegal instruction`，报错里直接说明「暂不提供 armv6 构建，请自行交叉编译 GOARCH=arm GOARM=6 或换设备」；未知架构的提示同步改成「已发布 amd64、arm64、armv7、386」 |

### 14.38.3 为什么没有 armv6 / mips

- **armv6**（树莓派 Zero、初代 A/B）不是「少一个产物」的问题：`GOARM=7` 编出来的指令 ARMv6 不认，装上就是 `Illegal instruction`，比明确拒绝更难排查。要支持得单独加 `GOARM=6` 产物，收益（这些板子的内存/CPU 也基本带不动 dae）不划算，故先拒绝并给出手工编译命令。
- **mips/mipsle**：面板本身能编（纯 Go、`CGO_ENABLED=0`），但 dae 需要内核 eBPF + BTF，绝大多数 MIPS 家用路由器内核没有 BTF，装了也跑不起来（§4.4.8 平台限制表）。所以不发布，避免给出「能装不能用」的产物。
- 反过来说，**dae 官方 deb 是覆盖这两类的**：daeuniverse/dae 最新 release 的资产里有 `dae-linux-x86_32.deb`、`dae-linux-armv7.deb`（还有 loongarch64 / mips32 / mips64 / powerpc64 / riscv64 / s390x），所以「面板 386 + dae x86_32」「面板 armv7 + dae armv7」这两条组合在包层面是齐的。

### 14.38.4 验证

| 项目 | 结果 |
| --- | --- |
| 本机交叉编译（Go 1.24，`CGO_ENABLED=0 -trimpath -ldflags '-s -w'`） | `/tmp/hnp-armv7` = `ELF 32-bit LSB executable, ARM, EABI5, statically linked, stripped`；`/tmp/hnp-386` = `ELF 32-bit LSB executable, Intel 80386, statically linked, stripped` |
| 编译循环的 shell 逻辑 | 单独跑 `sh` 干跑：`name=amd64 goarch=amd64 goarm=unset` / `arm64` / `armv7 goarm=7` / `386 goarm=unset` |
| 脚本语法 | `bash -n` 三个脚本全通过 |
| 端到端 | 见 §14.38.5：tag `v0.39.0` 的 CI 产物与 CDN 上的四个二进制 |

### 14.38.5 发布与实测

（发布后回填：CI 产物清单、CDN 上 armv7/386 的取回与 sha256 校验结果。）
## 15. 本版（v0.47）变更清单

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
14. **新增 §14.18（v0.22）**：接入 apply/rollback 落地（写自有片段 + 建自有 nft 表 + reload + 失败整体回滚 + 幂等清理 + 指纹校验 + 内核参数还原），段1 本机事务测试 6 个用例全通过（干跑/指纹拒绝/apply/幂等/回滚/自检），修掉 4 个真 bug（`dnat ip to`、规则叠加、`ip_forward` 不还原、表不存在误判失败），段2 B2 端到端测试手册落地为 `homenet-panel/docs/access-test-b2.md`。
15. **新增 §14.19（v0.23）**：登录体验与 HTTPS——会话落盘（`/var/lib/homenet-panel/sessions.json`，只存 token 摘要）、「记住我 30 天」、cookie `Secure` 按请求协议判定、公共端点 `GET /panel.crt` 下载证书、同端口协议分流（明文请求 301 跳 https，`--allow-plain-http` 可放开）；顺带给出「消除不安全提示」的三个可选做法。
16. **首次登录强制改密**：凭据加 `MustChange` 标记、auth 中间件 403 门禁（白名单 /api/me、/api/passwd、/api/logout、/api/health）、`POST /api/passwd` + 口令强度规则、`setpass --must-change`、前端改密卡片；实机 403 与浏览器渲染均已验证（§14.20）。
17. 登录与改密表单语义化（form + name + autocomplete=username/current-password/new-password），恢复浏览器「保存密码/自动填充」；初始密码可设为 10086（setpass 不做强度校验）。
18. 登录/改密改原生表单提交（`/login`、`/passwd` + 303），并把 `/passwd` 加入 mustChange 白名单；临时实例实测四类跳转码。
19. **新增 §14.21（v0.27）**：转发开关改由面板持久化（`internal/access/sysctl.go`：应用时 `sysctl -w` 打开转发 + 写 `/etc/sysctl.d/99-homenet-panel.conf`，回滚按应用前基线还原或删除自有文件；`accessState` 增 `hadSysctlFile`/`originalSysctl`，apply/reload 失败路径一并回滚）+ clashconv 支持 Clash **行内映射** `- {k: v}` 与 **ssr** 链接转换（`parseFlowItem`/`splitTopLevel`/`ssrLink`），真实样本 61/61 转换、dae `validate` 接受 ssr 订阅。
20. **转发改「有条件才开」（v0.28）**：`internal/access/ensure.go`（`EnsureForwardingForConfig` / `EnsureForwardingForInterfaces`，幂等且只补缺失项）+ `internal/dae` 的 `BeforeReload` / `OnReloadError`（reload 报 `ipforward on <iface> is off` 时按接口开转发并自动重试一次）+ `internal/api/forward.go` 接线；实机验证「改不动配置」死结解除，内核仅在 dae 要求时被改动（§14.21.4/14.21.5）。
21. **面板 UI 两处「看起来没反应」（v0.29）**：补 `EntryPath` 传递（`--c` 真正生效）；内容层顶部新增应用条 + 明确「没有单独保存按钮」；「订阅源更新」页支持新增订阅源（名称白名单 `validSubName`），后端对尚未进入模型的上游自动分配默认镜像 `subs/<name>.sub` 并在 apply 时 0600 原子写入（§14.22）。
22. **一次性 token 与登记项管理（v0.30）**：明确「上游只由面板拉一次并镜像，dae 只读 file://」；apply 时把 Clash 原始 YAML 留档到 `backups/subs-raw/`（便于补齐被跳过的节点类型）；新增 `POST /api/subsrc/remove` 与「删除」按钮，仍被内容层引用的登记项 409 拒绝（§14.23）。
23. **订阅转换两项修复 + 订阅选择器（v0.31）**：`ParseProxies` 按 `itemIndent` 区分节点与节点内嵌套列表（修掉 alpn 产生的 40 个幽灵节点）；新增 anytls 链接转换（依据 outbound 的 `parseAnytlsURL`），真实样本 81/81 全转、skipped 0；「内容层 → 订阅」来源改为下拉选择已登记镜像（§14.24）。
24. **组策略受控 A/B（v0.32）**：同机场同机只改 `proxy` 组，实测 baseline 31% / 只用香港 89% / 香港固定节点 93% / `!日本` 8%；查清 `name(keyword:)` 是子串非正则（`'香港|新加坡'` 能过 validate 但零匹配）、多行 `filter` 是 AND、`(a || b)` 语法被拒；结论是把远端节点赶出 fallback 组，归因需固定单节点而不能靠「最近一次选择」（§14.25）。
25. **组策略落地（v0.33）**：`.85` 的 `proxy` 组改为只用香港并实机验证（28% → 92%）；补记「reload 冷却期」（刚 reload 立刻测是 0%，静置 90 秒后才有参考价值），据此要求面板在应用成功提示里写明「等 1–3 分钟再看效果」（§14.25.7）。
26. **v2rayN 规则迁移 + 模型漂移防护（v0.34）**：7 条 v2rayN 规则全部翻译为 dae 语法并上线（UDP443 阻断、广告拦截、YouTube/AI 分组、国内拆两行直连、端口区间改 fallback），`/api/route/test` 命中与连通性双向验证；受控对照证明 `sg` 组 0/6、`hk` 组 6/6，AI 规则最终改走香港；发现 `model.json` 与线上配置漂移（点「应用」会覆盖手工改动），新增漂移检测 `diverged`/`drift` 与 `POST /api/model/resync`（§14.26）。另新增 `docs/USAGE.md`（部署到新设备的完整说明）（§14.26）。
27. **一条命令部署（v0.35）**：`deploy/deploy.sh`（开发机）探测架构→交叉编译→上传→远程安装→自检；`deploy/install.sh`（目标机）8 步幂等安装，自动识别 systemd/procd、保留已有证书与凭证、首次安装打印一次性随机密码；`--dry-run` 全链路试运行；实机在 .85 验证通过且未触碰 `/etc/dae/**`（§14.27）。

28. **单文件布局解析 + 空规则安全闸（v0.37）**：goose 遗留机（`/etc/dae/config.dae` 单文件、无 `config.d/`）上 `loadModel` 读不到主体配置 → 界面 0 规则且「应用」会静默清空路由规则。新增 `liveTexts()`/`coreExists()` 布局判定、`/api/model` 的 `layout`/`layoutNote`、以及「提交模型无规则且线上有 routing 时拒绝写入」的 422 安全闸；`.85`（split）与 `n6`（single，13 规则）双布局实测通过，发布 v0.35.4（§14.29）。

29. **goose 完整残留面 + daed 告警（v0.38）**：`install.sh` 的 goose 检测扩到 `goose*.{service,timer}`（含 `/lib/systemd/system`，实测 n6 有 `goose-update-geo.timer` 指向已删二进制），退役时逐个 disable 并归档；新增 daed 检测（实测 n6 装着 daed 2.1.1，disabled/failed）只告警并给处置建议（§14.28.1）。

30. **Geo 数据更新 + 出站白名单（v0.39）**：新增 `internal/geodata`（下载/校验/备份轮转/原子替换/符号链接保护/dry-run）与 `/api/geo`、`/api/geo/update`、`--geo-dir`/`--geo-keep`、前端「Geo 数据」tab；`internal/model/lint.go` 给出站白名单（补 `must_direct`/`must_block`、去掉 dae 不认的 `reject`）与 `LintGeo` 死规则检查，`/api/model` 增 `issues`；模型漂移改为语义比较（排版差异不再误报）；n6 上 purge daed + 官方源重装 dae 2.1.1 + 从零配置（§14.30）。

31. **Geo 版本检测（v0.40）**：`internal/geodata/version.go` 给出本地版本（apt 包版本 / 面板更新记录）+ 上游构建号（GitHub release API）+ 逐文件结论；默认源用 release 资产大小而不是逐文件 HEAD（.85 上 HEAD 全超时、n6 上慢到 91s），apt 托管时跳过 HEAD；接口 `GET /api/geo/versions`、`/api/geo` 增 `state`，前端「Geo 数据」页加「检测版本」按钮与版本对照表；实测 n6 91s→1.0s、.85 0.7s（§14.31）。

32. **订阅 UA 兜底（v0.41）**：UA 留空时不再把 `Go-http-client/1.1` 暴露给机场，改用 `DefaultUA = "clash-verge/v1.7.7"`（顺序：单条 ua → `--sub-ua` → DefaultUA）；n6 上留空实测 29/29 通过（§14.32）。
33. **新增 §14.33（v0.42）**：面板内「安装与初始化」页——`GET /api/setup/status` + 三个动作（安装 dae / 生成初始配置 / 启动 dae，均可 dry-run），root 校验 + 串行锁 + 已有配置强制 force（避免把线上规则冲成默认值）+ 写前备份 + `dae validate` 失败自动回滚 + 审计 `setup.*`；同时修正 install.sh「首次打开面板可生成初始配置」的不实提示。

34. **CDN 安装链路（v0.43）**：新增 `dist` / `<tag>-dist` 分支发布（`.github/workflows/release.yml`）与 jsDelivr 缓存刷新；`deploy/online-install.sh` 重写为多源回退 + `--http1.1` 重试 + sha256 校验 + `--tag/--base-url/--cdn` 参数，推荐一行命令改为 `https://cdn.jsdelivr.net/gh/leoz26/dae-panel@dist/online-install.sh`（§14.34）。
35. **dae 未运行时的应用报错（v0.44）**：`dae reload` 在拿不到 pid 时会读 `/var/run/dae.pid`，读不到就打 `failed to read pid file … Usage: dae reload`——这实际是「dae 没在运行」。`internal/configtx/configtx.go` 新增 `ErrDaeNotRunning`/`DaeDownHint`/`explainDaeOut`，`Apply`/`ApplyMulti`/`Rollback` 在 reload 前判 `RunningPID()<=0` 并回滚 + 指路「安装与初始化」页，失败文案不再回显 CLI 用法（§14.35）。
36. **计划步骤白话说明（v0.45）**：`internal/access/plan.go` 的 `Step` 新增 `Note` 字段（4 条全部填写），前端在每条步骤详情下渲染蓝色提示框；新增 `TestPlanStepsHavePlainLanguageNotes`；实机 API 与 CDP 浏览器均验证（§14.36）。
37. **结构化 DNS 编辑器（v0.46）**：新增 `internal/model/dns.go`（`DNSSpec`/`ParseDNS`/`RenderDNS`/`LintDNS`/`DefaultDNSSpec`）、`internal/api/dns.go`（`/api/dns/parse`、`/api/dns/render`）与前端「DNS」标签页；`Model.DNS` 仍是原样文本，编辑结果渲染回文本后走同一 apply 事务。校验依据来自 .85 上逐条 `dae validate`（`qname(geolocation:)`、`-> drop`、请求规则 `-> accept` 都不被 dae 接受）。实机 preview 证明「dns 不改时生成结果与线上逐字节一致」，详见 §14.37。
38. **构建矩阵扩到 armv7 / 386（v0.47）**：CI 编译循环改为 `产物名:GOARCH:GOARM` 四元组（`amd64`/`arm64`/`armv7`(GOARM=7)/`386`），purge 与 release notes 同步；`deploy/online-install.sh` 的 `uname -m` 映射加 `armv7l→armv7`、`i686→386`，`armv6l` 明确拒绝（GOARM=7 二进制在 ARMv6 上 Illegal instruction）。dae 官方 deb 本身有 `x86_32` 与 `armv7` 包，组合可行。详见 §14.38。
