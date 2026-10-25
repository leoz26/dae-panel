# HomeNet Panel 使用文档

> 版本：v1.0（对应面板 v0.34 之后）｜适用：Debian/Ubuntu、OpenWrt（iStoreOS 等）、任意 Linux（x86_64 / arm64）
> 面板是 dae 的**管理层**：它不实现代理协议，只负责生成配置、做事务化应用与回滚、托管订阅、记录审计、检测接入条件。

---

## 1. 它做什么 / 不做什么

**做**：结构化编辑 dae 配置（订阅 / 分组 / 分流规则 / DNS）→ 校验 → 备份 → 原子写入 → `dae reload` → 失败自动回滚；订阅上游托管（含 Clash/Mihomo YAML → 链接转换）；审计日志；字节级流量统计；接入向导（旁路由/网关/观测的档位推荐、计划生成、只读自检）。

**不做**：不接管 DHCP、不改 `/etc/config/firewall`、不远程登录主路由改配置、不实现协议栈、不管 TLS 以外的网络接线。面板只写两类东西：`/etc/dae/config.d/panel-*.dae` 与自有 nft 表（`panel_dns` / `panel_snat` / `panel_tproxy`）。

---

## 2. 部署到新设备

### 2.1 先核对设备能不能跑 dae

dae 依赖内核 eBPF + BTF，**不是所有设备都能用**：

| 条件 | 怎么查 | 不满足会怎样 |
|---|---|---|
| 内核 ≥ 5.8 且有 BTF | `ls /sys/kernel/btf/vmlinux` | dae 起不来 |
| 架构 x86_64 / arm64 | `uname -m` | MIPS/ARMv7 路由器基本无望 |
| root 权限 | `id -u` = 0 | 无法挂载 eBPF |
| memlock ≥ 64MB | `ulimit -l` 或 `/proc/self/limits` | 加载 BPF map 失败 |

面板自带探测：部署后打开「接入向导」页，能力表会直接告诉你 BPF/BTF/memlock/nft 是否就绪。

### 2.2 编译（在开发机上交叉编译）

```bash
cd homenet-panel
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/homenet-panel-linux-amd64 ./cmd/homenet-panel
# arm64 软路由 / 树莓派：
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bin/homenet-panel-linux-arm64 ./cmd/homenet-panel
```
产物是静态单文件，目标机不需要 Go 运行时。

### 2.3 安装与目录

```bash
install -m 0755 bin/homenet-panel-linux-amd64 /usr/local/bin/homenet-panel
mkdir -p /etc/homenet-panel /var/lib/homenet-panel/backups
```

| 路径 | 用途 |
|---|---|
| `/etc/homenet-panel/tls.crt` `tls.key` | HTTPS 证书（自签即可） |
| `/etc/homenet-panel/auth.json` | 登录凭证（0600） |
| `/var/lib/homenet-panel/model.json` | 面板模型（**UI 真源**） |
| `/var/lib/homenet-panel/backups/` | 每次应用的配置备份 |
| `/var/lib/homenet-panel/audit.log` | 审计日志（JSONL，自动轮转） |
| `/var/lib/homenet-panel/subsrc.json` | 订阅源登记表 |
| `/var/lib/homenet-panel/sessions.json` | 登录会话（重启免登录） |

自签证书一条命令：
```bash
openssl req -x509 -newkey rsa:2048 -nodes -days 825 \
  -keyout /etc/homenet-panel/tls.key -out /etc/homenet-panel/tls.crt \
  -subj "/CN=homenet-panel" -addext "subjectAltName=IP:192.168.1.85,IP:127.0.0.1,DNS:localhost"
chmod 600 /etc/homenet-panel/tls.key
```

### 2.4 systemd（Debian/Ubuntu）

`/etc/systemd/system/homenet-panel.service`：
```ini
[Unit]
Description=HomeNet Panel (dae 配置管理 REST API + Web 控制台)
After=network-online.target dae.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/homenet-panel serve --addr 0.0.0.0:8787 \
  --backup-dir /var/lib/homenet-panel/backups \
  --tls-cert /etc/homenet-panel/tls.crt --tls-key /etc/homenet-panel/tls.key \
  --traffic-iface ens2
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
```
`--traffic-iface` 填**对外的物理网卡**（`ip route show default` 里的 dev），不是 `dae0`。

```bash
systemctl daemon-reload && systemctl enable --now homenet-panel
```

### 2.5 OpenWrt / iStoreOS（无 systemd）

用 procd 写 `/etc/init.d/homenet-panel`：
```sh
#!/bin/sh /etc/rc.common
USE_PROCD=1
START=99
start_service() {
    procd_open_instance
    procd_set_param command /usr/local/bin/homenet-panel serve \
        --addr 0.0.0.0:8787 --traffic-iface br-lan \
        --subsrc-path /etc/homenet-panel/subsrc.json \
        --audit-path /etc/homenet-panel/audit.log \
        --audit-max-mb 2 --audit-keep 2
    procd_set_param respawn
    procd_close_instance
}
```
小 flash 设备建议：审计 `--audit-max-mb 2 --audit-keep 2`（最多约 6MB）；`subsrc.json` 与 `model.json` 放持久分区；日志放 `/tmp` 可接受重启丢失。**不要改 `/etc/config/firewall` 与 `/etc/config/dhcp`**，那是 LuCI 的地盘。

### 2.6 首次初始化

```bash
homenet-panel setpass --user admin            # 不带 --password 会自动生成并打印一次
systemctl restart homenet-panel
```
浏览器打开 `https://<ip>:8787`，自签证书会提示风险，点继续即可（也可先访问 `/panel.crt` 下载证书导入信任库）。

### 2.7 上线自检（三条）

1. 状态栏应显示 `dae: active` + `ReloadDone`；
2. 「内容层」顶部若出现**面板模型与线上配置不一致**的黄条，说明线上被手工改过 → 点「从线上重新解析」同步面板模型（不改线上）；
3. 「接入向导 → 运行自检」应 4 项通过，剩下 1 项「还没有内网设备走本机」在旁路由接入完成前是正常的。

---

## 3. 界面速览

| 页签 | 干什么 |
|---|---|
| 内容层 | 结构化编辑订阅/分组/规则/DNS；**唯一写盘入口是「校验+备份+reload 应用」** |
| 订阅源更新 | 登记上游 URL/UA、拉取检查、转换并写镜像、查看新增/消失节点、删除登记项 |
| 自建聚合 | 把多个订阅合并成一个订阅文件（模式 B） |
| 分流调试 | 输入域名，看当前规则命中哪一条、出站是谁 |
| 流量·命中 | 最近 DNS 命中排行，标注归属出口 |
| 字节统计 | 按节点/客户端统计累计与速率（轮询内核计数器，不注入 eBPF） |
| 运行探测 | 环境体检、dae 运行态、日志摘要 |
| 接入向导 | 能力探测 → 推荐档位 → 生成接入计划（dry-run）→ 只读自检 |
| 审计日志 | 谁在什么时候改了什么；可导出 CSV / JSONL |

---

## 4. 分流规则语法速查

规则自上而下，**首条命中即生效**。面板会自动在最前面注入 4 条安全规则（内网/回环/组播/DHCP 广播直连 + 本机进程直连），不在表里也删不掉。

常用匹配器：

| 写法 | 含义 |
|---|---|
| `domain(geosite:youtube)` | 命中 geosite 分类 |
| `domain(geosite:cn, geosite:geolocation-cn)` | 多个分类（逗号 = 或） |
| `domain(suffix: claude.ai, suffix: anthropic.com)` | 域名后缀 |
| `ip(geoip:cn)` | 目标 IP 归属 |
| `dip(192.168.0.0/16)` | 目标网段 |
| `pname(homenet-panel, dhcpcd)` | 进程名（**最多 16 字符**，超长会被裁断） |
| `l4proto(udp) && dport(443)` | 协议 + 目标端口 |
| `fallback: proxy` | 兜底出站（写在 routing 末尾） |

**三个坑**：
1. `name(keyword: 'x')` 在分组 filter 里是**子串匹配**，不是正则——`'香港|新加坡'` 能过校验但零匹配；
2. 同一行多个谓词是**与**关系，想要「或」要拆成多行；
3. 括号 OR（`(a || b)`）不支持。

### 4.1 从 v2rayN 规则迁移

| v2rayN | 面板/dae |
|---|---|
| UDP 443 → block | `l4proto(udp) && dport(443)` → `block` |
| geoip:private + geosite:private → direct | 面板已自动注入，无需填 |
| geosite:category-ads-all → block | `domain(geosite:category-ads-all)` → `block` |
| geosite:category-ai-!cn → proxy | `domain(geosite:category-ai-!cn)` → 你的组 |
| geosite:youtube → proxy | `domain(geosite:youtube)` → 你的组 |
| ip:geoip:cn + domain:geosite:cn → direct | 拆两行：`domain(...)` 与 `ip(geoip:cn)` |
| port 0-65535 → proxy | 填在「兜底 fallback」 |

---

## 5. 订阅源管理

**推荐姿势**：面板「订阅源更新」填上游 URL（+ 必要时 UA）→ 拉取检查 → 拉取并应用（转换并写 `/etc/dae/subs/<name>.sub`）→ 到「内容层 → 订阅」用下拉选 `file://subs/<name>.sub` → 应用。

为什么必须镜像：① 一次性 token 每次拉取都可能变，dae 自己拉会静默 0 节点；② 上游若返回 Clash/Mihomo YAML，dae 不认，面板会先转成节点链接；③ 订阅变更由审计记录，便于回溯。

注意：
- 部分机场只认特定 UA（如 `clash-verge/v1.7.7`），否则 403；面板支持按订阅源单独填 UA。
- 上游返回网页（HTML）会被识别并报错，不会写坏配置。
- Clash YAML 里被跳过的节点类型会存到 `<backup>/subs-raw/<name>.<时间>.yaml` 留档。
- 上游限流时会有 2 分钟拉取缓存（检查后立刻应用不用再拉一次）。

---

## 6. 接入档位（旁路由/网关）

| 档位 | 前提 | 适用 |
|---|---|---|
| **B1 bypass-manual**（默认） | 无 | 单台设备手改网关/DNS 指向本机 |
| B2 bypass-router-dhcp | 主路由 DHCP 能改「网关」项 | 全屋生效 |
| C bypass-pbr | 主路由支持 ip rule/nftset | 只让部分设备/域名走本机 |
| B3 bypass-self-dhcp | 能关主路由 DHCP | 风险最高，默认不启用 |
| A gateway | 装在同一台路由器上 | 同机即网关 |
| observe | 无 | 只读观察，不接管 |

面板**永不接管 DHCP**。所有变更都先出「计划（dry-run）」：含面板侧步骤、需要人工在路由器上做的步骤、回滚命令、自检清单。默认档是 B1，需人工把设备网关/DNS 改成本机 IP。

自检矩阵里最关键的一条：**「字节统计」的客户端表出现内网地址 = LAN 流量真的来了**。

---

## 7. 运维须知

- **应用配置后等 1–3 分钟再判断好坏**：`min_moving_avg` 组策略需要重新测速排序，刚 reload 完的失败不算配置质量问题（实测刚 reload 立刻测 0%，静置 90 秒后 92%）。
- **ip_forward 不用手拨**：配置声明了 `lan_interface`，或 dae reload 报 `ipforward on <iface> is off` 时，面板会自动开转发并写 `/etc/sysctl.d/99-homenet-panel.conf`；回滚会撤销。
- **审计日志按大小轮转**，磁盘上界 = `maxMB × (keep+1)`，不需要定时任务。
- **回滚**：面板「回滚」按钮，或从 `/var/lib/homenet-panel/backups/` 里挑一份 `cp` 回去再 `dae reload`。
- **双写防护**：面板生成的配置带 `panel-managed` 头；若检测到非面板写入的痕迹会拒绝覆盖。

---

## 8. FAQ

**Q：页面提示证书不安全？** 自签证书的正常现象；导入 `/panel.crt` 或按提示继续。

**Q：改了规则没生效？** 检查是否点了「校验+备份+reload 应用」（页面没有单独保存按钮），以及是否等了 1–3 分钟。

**Q：dae reload 失败？** 面板会自动回滚到上一版并保留错误输出（含行列号）。常见原因：语法错误、缺少 `lan_interface` 对应转发、出站组名不存在。

**Q：为什么不能写 Clash YAML 进订阅？** dae 只吃节点链接列表；面板会在「拉取并应用」时转换。

**Q：能不能远程改我家路由器？** 不能，也不打算做。面板只输出可粘贴的片段和步骤。

**Q：`/api/traffic/bytes` 里客户端表一直空？** 说明还没有 LAN 设备把流量送过来（单机代理或同网段单播交换都会这样）。

---

## 9. 安全建议

1. 首次登录后立刻改密码；不要把面板暴露到公网，需要远程访问用 VPN/Tailscale。
2. 订阅 URL 里带 token，不要提交进 git、不要贴到公开渠道；仓库里也不应出现明文密码与 token。
3. 审计日志含操作者与来源 IP，敏感环境下自行控制导出范围。
