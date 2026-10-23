# dae-panel（HomeNet Panel）

给 [dae](https://github.com/daeuniverse/dae) 做的一块「可运维」图形面板：把**配置变更做成事务**、把**订阅源托管起来（含 Clash/Mihomo YAML 自动转换）**、把**谁改了什么记成审计日志**，并额外提供 dae 官方面板不覆盖的那一层——**把流量送到 dae 面前（接入/接线）**。

- 单个 Go 二进制（零第三方依赖）+ 一个内嵌前端页面，不需要 Node 构建链
- 配置「文件即真源」：面板只写自有片段（`config.d/panel-*.dae`）与自有 nft 表（`panel_*`），不碰系统原有 DHCP/PPPoE/NAT 与 `/etc/config/*`
- 每次应用都是：渲染 → `dae validate` → 备份 → 写入 → `dae reload` → 校验 reload 结果；失败自动回滚

## 功能

| 模块 | 说明 |
| --- | --- |
| 结构化配置 | 订阅 / 分组 / 分流规则可视化编辑，global 与 DNS 保留高级原文编辑 |
| 配置事务 | 预览 → 校验 → 备份 → 写入 → reload → 失败回滚（reload 忙/失败都有明确映射） |
| 订阅源托管 | 记录上游 URL 与 UA；上游返回 Clash/Mihomo YAML 时自动转成节点链接；变更只报告差异，一键应用前先 dry-run |
| 自建聚合 | 把多个订阅合并成一份只含节点的镜像 |
| 流量观测 | DNS 命中排行、字节级统计（socket 计数器 + 拦截网卡总量，按节点/客户端归并） |
| 接入向导 | 只读探测本机能力 → 推荐接入档位（网关/旁路由/观测）→ 生成计划（配置片段、nft 片段、回滚、自检）→ **一键应用**（写自有片段 + 建自有 nft 表 + `dae reload`，失败整体回滚）与一键撤销；[段2 B2 端到端测试手册](docs/access-test-b2.md) |
| 审计日志 | 登录、应用、回滚、暂停、订阅更新等操作的 JSONL 记录，按大小轮转，可导出 CSV / JSONL |
| 安全直连 | 自动前置注入「内网/回环/组播 + 本机进程直连」规则，避免把本机与内网流量也代理掉 |

## 快速开始（Linux 旁路由/网关）

前提：已安装 dae 并能正常 `dae run`；内核具备 BPF/BTF。

```bash
go build -o bin/homenet-panel ./cmd/homenet-panel
sudo install -m 0755 bin/homenet-panel /usr/local/bin/homenet-panel

# 1) 设置登录账号密码（凭证存 --auth-path，默认 /var/lib/homenet-panel/auth.json）
sudo /usr/local/bin/homenet-panel setpass --user admin --password '<你的密码>'

# 2) 启动控制台（前端已内嵌，无需额外静态目录）
sudo /usr/local/bin/homenet-panel serve \
  --addr :8787 \
  --c /etc/dae/config.dae \
  --tls-cert /etc/dae/panel.crt --tls-key /etc/dae/panel.key \
  --backup-dir /var/lib/homenet-panel/backups \
  --audit-path /var/lib/homenet-panel/audit.log \
  --subsrc-path /var/lib/homenet-panel/subsrc.json \
  --bytes-interval 10 --bytes-iface dae0
```

浏览器访问 `https://<主机>:8787`（自签证书首次需手动确认）。不传 `--tls-cert/--tls-key` 则为明文 HTTP，仅建议在 `127.0.0.1` 上这样用。

常用参数：`--sub-ua`（订阅拉取默认 UA）、`--audit-max-mb`/`--audit-keep`（审计轮转）、`--subsrc-check-hours`（订阅巡检间隔，0=关闭）、`--bytes-interval`（字节采样间隔，0=关闭）。

## 接入档位（把流量送到 dae 面前）

| 档位 | 前提 | 影响面 | 面板做什么 |
| --- | --- | --- | --- |
| **B1 旁路由 · 单机手改（默认）** | 无 | 只有你改过的那台设备 | 生成 dae `lan_interface` 片段、DNS 重定向片段、自检清单；网关/DNS 由你在设备上改 |
| B2 旁路由 · 主路由 DHCP | 主路由能改「下发的网关/DNS」 | 全屋 | 同上 + 提示先记录原值、给回滚步骤 |
| A 网关（同机） | dae 与面板跑在同一台 OpenWrt/软路由，内核支持 BPF/BTF | 全屋 | 只写自有片段，DHCP/PPPoE/NAT 仍归路由器 |
| C 旁路由 · 策略路由 | 主路由能配 `ip rule`/`nftset` | 命中的设备/域名 | **只生成片段**，不远程登录主路由 |
| B3 面板托管 DHCP | 能关闭主路由 DHCP | 最大 | 风险最高，默认关闭，仅出说明 |
| observe | 无 | 无 | 只读观测，不改任何路径 |

原则：**面板永不接管 DHCP**；一切变更先出计划（dry-run），应用时备份并可一键回滚；探测失败一律降级到 observe。

## 目录结构

```
cmd/homenet-panel/        入口（serve / 参数）
internal/api/             HTTP 路由、内嵌前端 index.html
internal/model/           配置模型与 dae 配置渲染（含 EssentialRules 安全直连）
internal/configtx/        配置事务：校验、备份、写入、reload、回滚
internal/subs/            订阅解析（base64 / SIP008 / 明文链接），带 UA 拉取
internal/clashconv/       Clash/Mihomo YAML → 节点链接（手写 YAML 子集解析）
internal/subsrc/          订阅源登记表、差异计算、拉取缓存
internal/traffic/         DNS 命中聚合 + 字节级统计（ss / /proc/net/dev）
internal/access/          接入探测、档位推荐、计划生成、apply/rollback、只读自检
internal/audit/           审计日志（JSONL、按大小轮转、CSV/JSONL 导出）
tools/netdiag/            外部网络诊断脚本（流量走向、被动抓包、按客户端汇总）
architecture.md           详细架构设计与实测记录
dae-cli-capability-checklist.md  dae CLI 能力核对表
```

## 测试

```bash
go vet ./... && go test ./... -count=1
```

## 文档

- `architecture.md`：总体架构、配置事务、订阅与聚合、接入档位设计、实测结论与变更清单
- `docs/access-test-b2.md`：B2（主路由 DHCP 指向旁路由）端到端测试手册——前置条件、改前记录、逐项自检、三级回滚
- `dae-cli-capability-checklist.md`：dae CLI 命令/信号/配置模型/坑点核对表

## 许可证

MIT，见 [LICENSE](LICENSE)。
