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

发布的静态二进制覆盖 **linux/amd64、arm64、armv7、386**（`uname -m` 为 `x86_64`/`aarch64`/`armv7l`/`i686` 都可直接在线安装）；`armv6l`（树莓派 Zero/初代）与 MIPS 不提供构建。

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

常用参数：`--sub-ua`（订阅拉取默认 UA）、`--audit-max-mb`/`--audit-keep`（审计轮转）、`--subsrc-check-hours`（订阅巡检间隔，0=关闭）、`--bytes-interval`（字节采样间隔，0=关闭）、`--session-path`（登录会话落盘位置，重启不掉登录；空=仅内存）、`--allow-plain-http`（TLS 开启时也接受明文访问，默认只 301 跳 https）。

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

## 登录与 HTTPS（常见疑问）

### 浏览器不弹「保存密码 / 自动填充」怎么办

登录/改密走的是**原生表单提交**（`POST /login`、`POST /passwd`），符合浏览器密码管理器的识别条件。仍不弹时按顺序排查：

1. `chrome://password-manager/settings` 里确认「提示保存密码 / 自动填充」已开启；无痕窗口一律不保存。
2. **自签证书未受信任时 Chrome 会禁用密码管理器** —— 先点登录页的「下载证书」把 `panel.crt` 装成受信任根证书（macOS：钥匙串访问 → 系统 → 始终信任）。
3. 不想装证书就让内网走明文：启动加 `--allow-plain-http`，直接访问 `http://<ip>:8787`（Chrome 在 http 站也会保存密码，只是标注"不安全"）。

### 初始密码（首次登录必须改）

首次部署执行 `homenet-panel setpass --user admin --password '你的初始密码' --must-change`。用初始密码登录后，面板会**强制要求先改密码**才能进入控制台（其余接口一律返回 403 `must_change_password`）。规则：至少 8 位、同时含字母与数字（或符号），且不能是 password/admin/12345678/homenet 之类的常见口令。忘了密码就重跑一次 `setpass`（不加 `--must-change` 就不再强制改）。

- **要不要 HTTPS？** 面板能改 dae 配置、能看审计日志，建议保留 TLS；内网也一样，防的是同网段嗅探与误操作。若不配 `--tls-cert/--tls-key` 则纯 HTTP，仅建议绑在 `127.0.0.1`。
- **浏览器提示「不安全」？** 那是自签证书没进设备信任库，加密本身没问题。登录页有「下载证书」链接（`GET /panel.crt`），装成受信任根证书即可（iOS：设置→通用→关于本机→证书信任设置；macOS：钥匙串→始终信任；Windows：受信任的根证书颁发机构；Android：安装 CA 证书）。
- **可以直接输 `ip:8787` 吗？** 可以。同端口会识别明文 HTTP 并 **301 跳到 `https://`**；不想跳可加 `--allow-plain-http`（密码会明文过网）。
- **每次都要重新登录？** 登录页勾「记住我」= 30 天免登录；会话写在 `--session-path`（默认 `/var/lib/homenet-panel/sessions.json`，0600，只存 token 的 SHA-256 摘要），面板重启也不会掉。

## 一条命令部署（见 `docs/USAGE.md` 2.0 / 2.0b，安装后用 `deploy/verify-install.sh` 只读自检）

走 CDN（初始环境连不上 github.com 也能装）：

```bash
curl -fsSL https://cdn.jsdelivr.net/gh/leoz26/dae-panel@dist/online-install.sh | sudo bash
```

GitHub 直连备用：`curl -fsSL https://github.com/leoz26/dae-panel/releases/latest/download/online-install.sh | sudo bash`

```bash
bash deploy/deploy.sh root@192.168.1.85 -i ~/.ssh/id_ed25519   # 自动交叉编译 + 上传 + 安装 + 起服务 + 自检
```

幂等可重复执行；已有证书、账号、配置不会被覆盖。详见 [docs/USAGE.md](docs/USAGE.md)。

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

- **`docs/USAGE.md`：使用与部署文档（部署到新设备、规则语法与 v2rayN 迁移、订阅源管理、接入档位、运维与 FAQ）**
- `architecture.md`：总体架构、配置事务、订阅与聚合、接入档位设计、实测结论与变更清单
- `docs/RULES.md`：分流规则写法详解（匹配器全表、面板注入的 4 条安全规则逐条拆解、常见坑、验证方法）
- `docs/access-test-b2.md`：B2（主路由 DHCP 指向旁路由）端到端测试手册——前置条件、改前记录、逐项自检、三级回滚
- `docs/NOTES.md`：运维笔记（按时间倒序）——每台机上装/卸 dae 与面板的实际步骤、东西挪到哪、怎么撤、顺带确认的事实
- `dae-cli-capability-checklist.md`：dae CLI 命令/信号/配置模型/坑点核对表

## 许可证

MIT，见 [LICENSE](LICENSE)。
