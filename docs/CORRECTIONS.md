# 修正记录（Errata / 行为修正）

> 维护约定：本项目的结论大多来自实机验证，结论被推翻时**不静默改掉旧文字**，而是登记在这里，并在 `architecture.md` 对应小节标上版本（v0.x）。每条都附证据位置，方便复核。
> 最近更新：2026-10-28（v0.63 之后）

## A. 文档勘误：被实测推翻的结论

| # | 原说法 | 实测事实 | 现在的说法 / 影响 |
| --- | --- | --- | --- |
| A1 | geo 数据"按配置目录推断" | `dae validate` 缺 geo 时报 `geosite.dat: file does not exist in [配置目录, ~/.local/share/dae, /usr/local/share/dae, /usr/share/dae]` | geo 按**搜索路径列表**定位；环境检测要逐路径核对，这个报错归入"缺 geo 资源" |
| A2 | "dae 默认没有 `trace` / `completion` 子命令"（源码核对结论） | v2.1.1 顶层确有这两个子命令，`trace` 走 build tag `trace` 编译，本机构建已包含且可用 | 诊断可直接包装 `dae trace`，不必自研（见 §4.6）。另注：`trace` 启动要 attach 上千 kprobe，几十秒才出结果，不适合快速排查 |
| A3 | 拦截形态是 tc-bpf | .85（内核 6.12）用 **netkit 设备对** `dae0 ↔ dae0peer`（netns `daens`），`tc filter` 全空 | 诊断/卸载检查针对 netkit 设备与 `daens` netns；6.6 内核没有 netkit，dae 自动回退 veth |
| A4 | "本机没装 daed / 没有 goose" | goose 0.5.18 曾装在 `/usr/local/bin/goose` + `/etc/goose`，且是 `config.dae` 当时的唯一生成者 | goose 是面板的**前身**，已被 M1–M4 迁移取代，残留归档后删除（见 `docs/NOTES.md`） |
| A5 | journal 里 `e9220e31-…bbtechconsultants.com` / `fbd56476-…` 是"可疑 DNS 心跳" | 它们是**订阅节点的主机名**（29 个节点里大量 `<uuid>.bbtechconsultants.com`） | 撤回该告警结论（§14.12 已更正） |
| A6 | 旁路由"必须 SNAT 才有回程" | 被 tproxy/netkit 劫持的 TCP/UDP 在旁路由本地终结，回程天然对称 | 旁路由真问题是**收流**（未开转发/未劫持）与**DNS 不经过本机**；只有未被劫持的纯转发流量在跨网段或主路由严格 zone/rp_filter 时才需要 SNAT（§4.4.1） |
| A7 | "家用路由普遍改不了 DHCP 下发的网关" | 中兴家用固件「地址管理」页把 DHCP Option 3 做成可编辑字段「自定义网关地址」 | 该统计假设至少对这批固件不成立 → B2 档在部分家用固件可行；但默认档仍 B1（§4.4.3）。风险：该值可能同时用于路由器自身出口，填错整段断网 |
| A8 | "dae 不解析 Clash YAML"（当时只是静态推断） | dual 上游确实只返回 Clash YAML（`&flag=clash` 唯一有效），dae v2.1.1 无法消费 | 面板新增 `internal/clashconv`：Clash/Mihomo YAML → 节点链接；与 goose 当年生成的镜像做集合比对 **29/29 一致** |
| A9 | 面板报 `diverged` 说明"线上被手工改过" | 真正原因：`parseGroups` 丢掉组级 `check_interval`/`check_tolerance`，且渲染顺序与现场不同 | v0.63 起 `Group.Extra` 往返保真，顺序统一为 policy → 组长参数 → filter；.85 resync 后 `diverged=false`、渲染结果与线上逐字节一致 |
| A10 | "面板点『应用』会抹掉现场调优" | 上述 A9 修好后，应用等于重放现状 | v0.63 起该风险消除；调优默认值只在**缺键**时补，不覆盖现场值 |
| A11 | procd 里 `procd_set_param limits nofile=65535` 能放开 fd | 硬上限来自 pid1（4096），日志刷 `socket: too many open files` | 必须用 `sh -c "ulimit -Hn 65535; ulimit -n 65535; exec dae run …"` 包装（`deploy/install.sh` 与 daed 静态包两处模板都已改） |
| A12 | `dae reload` 能应用 DNS / global 段改动 | reload 返回 OK 但 DNS 全灭（改动不生效） | 改 DNS/global **必须 restart**，且 restart 后有 45–90 秒预热期，其间 `Temporary failure in name resolution` 不算故障 |
| A13 | 双写风险是"常驻进程竞争" | goose.service 实测 disabled + inactive，无 timer/cron | 双写是**事件驱动**：谁手动起 goose 谁就覆盖入口配置 → 检测要盯生成物 mtime/内容变化，不能只查进程 |
| A14 | "LAN 设备都经 .85 上网" | 45s 被动抓包里客户端→外网只有 KB 级；Mac 默认网关是 192.168.1.11（走 Tailscale 出口） | 当时**没有任何 LAN 客户端经本机 dae**；后续应用「接入向导」时这项靠 `/api/traffic/bytes` 的 clients 表验证 |
| A15 | "装了 zram-swap 就会自动有 swap" | .11 上 `zram-swap`/`kmod-zram` 都在、`/etc/rc.d/S15zram` 也挂着，但 `SwapTotal: 0`、`/sys/block/zram0/disksize` 为 0 | Linux 只在有人 `swapon` 后才用；默认大小是 `ram_kb/2048` MB（2GB ≈ 990MB）。是否启用人决定（当时决定不启） |

## B. 行为 / 代码修正（bug 与修法）

| # | 现象 | 根因 | 修法 |
| --- | --- | --- | --- |
| B1 | 页面原样显示 `{{loginErr}}`，登录卡与主控台同时出现 | 内联 `<script>` 里混进 8 处 HTML 实体 `&#39;`（本该是 `'`），整段脚本 SyntaxError，Vue 从未 mount | 改回 `'`；新增回归测试 `TestIndexHTMLScriptHasNoHTMLEntities`（内联脚本块出现任何 HTML 实体即失败） |
| B2 | 审计页一直 404 | `handleAudit` 早已实现但**从未注册路由**（死代码） | `server.go` 注册 `/api/audit`、`/api/audit/export` |
| B3 | 订阅源 UA 填了不生效 | `subsrc.Upsert` 只复制 URL/Source/File，没带上 `UA` | 补 `cur.UA = e.UA`；dual 上游 403 的根因就是缺 UA（`clash-verge/v1.7.7` 才 200） |
| B4 | 「拉取并应用」长时间 502 超时 | 上游（Cloudflare 后）对短时间内重复请求会挂起，不是代码 bug | 加 120s 拉取缓存（检查→应用只拉一次）+ 限流友好文案 + 错误串里 token 打码 |
| B5 | 本机 dhcpcd 的 DHCP(68→67)、发往 192.168.1.1 的 DNS、面板自身出站都被丢进代理组 | live 规则只有 `geosite:cn`/`geoip:cn -> direct`，没有内网直连 | `model.EssentialRules` + `EnsureEssentials` 强制前置注入内网/回环/组播/CGNAT/本机进程直连（§14.14） |
| B6 | 接入向导在 .85 上一取版本就失败 | `dae version` 子命令在 v2.1.1 不存在（rc=1） | 按 `--version` → `version` → `-v` 顺序探测，最后 `LookPath` 兜底 |
| B7 | 接入向导报 `lan_interface: null`（明明写了） | 只读入口配置，没展开 `include { config.d/*.dae }` | 加 include glob 展开（最多两跳） |
| B8 | 遗留 `panel_*` nft 表识别不到 | `nft list tables` 输出 `table inet panel_tproxy` 只有 3 个字段，代码要求 4 个 | 改 `len(f) >= 3 && f[2]` |
| B9 | 健康诊断页只有「时间窗口」卡有内容 | 前端 `api()` 返回 `{code, data}`，页面写成 `r.report` | 改 `r.data.report` / `r.data.kinds`；教训：页面里 raw `fetch()` 能拿到的形状不代表 `api()` 一致 |
| B10 | 应用配置后 dae 报 `WARN No node found.`、所有组 `no dialer in this group` | 一次操作把整份 core 覆盖进了 `/etc/dae/config.dae`，入口 218B 的 `include` + `subscription` 段被冲掉 | 恢复入口（注释头 + `include { config.d/*.dae }` + `subscription`）；**教训：apply 失败回滚后必须检查入口首行** |
| B11 | 改完 group filter 后配置语法炸裂，出现 `filter: subtag(summer) filter: namefilter: name …` | 用 `sed`/`awk` 替换串时 `&` 被展开成整个匹配内容 | 替换串写 `\&`（awk 要双反斜杠）；改完先 `dae validate` |
| B12 | 走本地 DNS 的域名全返回 `198.18.x` 假 IP，随后 `curl: (35) TLS connect error … unexpected eof` | OpenClash 残留：`uci show dhcp` 里 `noresolv=1` + `server=127.0.0.1#7874`（fake-IP DNS），dae 把假 IP 丢给节点 | 停用 OpenClash；`uci set dhcp.@dnsmasq[0].noresolv=0` + `uci del dhcp.@dnsmasq[0].server` + dnsmasq restart；用 `curl --resolve` 交叉验证 |
| B13 | `architecture.md` 从 107,231 字节变成 4 字节 | 写成 `fs.writeFile(p, 'utf8')`（漏传 content，把编码当内容写进去） | 从 DSH 会话日志恢复（zstd 多帧逐帧解压 + 取最后一份全量快照 v0.12 + 按 seq 重放 11 次变更），恢复后逐字节一致；随后把项目 git 化 + 加 `.bak` 备份习惯 |

## C. 容易误判的默认值与环境事实（不是错，但坑）

| 项 | 事实 | 依据 |
| --- | --- | --- |
| 节点判死阈值 | TCP 探测**一次失败即判死**并立刻移出 alive 集合；DNS-UDP 3 次、真实 TCP 流量 10 次、UDP 数据 50 次 | `connectivity_check.go:1188-1198`、`alive_dialer_set.go:333-355` |
| `check_tolerance` | 默认 **0**（`config/config.go:31`，example.dae 里的 50ms 只是示例）→ 排序并列也会切，没有 sticky | 同上 |
| 拨号失败 | i/o timeout、TLS 失败**不直接判死**，只触发 2s 冷却的紧急探测；网络不可达/EADDRNOTAVAIL 才立刻判死 | `control/dial.go:53-72` |
| 重试 | 拨号重试**不换节点**（同一节点连拨两次，各 8s） | `control/dial.go:242,266` |
| 全组判死 | `ErrNoAliveDialer` 直接失败，兜底只对"组内仅 1 个 dialer"生效 | `dialer_group.go:352,380,405` |
| `min_moving_avg` | EMA `MA=(MA+latency)/2`，**只采样成功样本**；只在降延迟时切换 | `connectivity_check.go:1234` |
| 配置权限 | `dae validate` 主动拒绝 0644 配置，要求 0600/0640 | .85 实测 |
| `bootstrap_resolver` | 必须写 `ip:port`；写 `udp://223.5.5.5:53` 报 `each colon-separated field must have at least one digit`，写裸 IP 报 `not an ip:port` | `example.dae:148-150` + 实测 |
| memlock | .85 默认 soft=hard=8192KB（unit 未设 `LimitMEMLOCK`），大规则集会受限 | E20 实测 |
| netkit | 需内核 ≥ 6.7 且编译时 `CONFIG_NETKIT=y`；内核编进而非模块，`lsmod` 看不到。6.6 → dae 自动 veth 回退 | .85(6.12)/.11(6.6) 对照 |
| reload 边界 | 改 routing/group 用 `dae reload` 安全；改 DNS/global 必须 restart | .11 实测 |
| 面板本机 | 提权工具是 `run0` 不是 `sudo`；无 `curl`、有 `python3`；OpenWrt 侧无 python3、busybox shell | 各机实测 |
| 交叉编译 | 必须显式 `GOOS=linux GOARCH=amd64 CGO_ENABLED=0`，并设 `HOME`/`PATH`/`GOCACHE`（沙箱里 `HOME` 为空） | 构建记录 |

## D. 仍待验证 / 未解

| # | 问题 | 现状 |
| --- | --- | --- |
| D1 | .11 上剩余约 8% 数据面失败究竟归因于内核 6.6 的 veth 回退还是节点本身 | 绕过 DNS 后仍有失败（11/12 成功），但那台机器同时有 9 次 OOM | 
| D2 | 面板在 .11 的 `model.json` 与 live 曾经 `diverged`（已回滚到 OpenClash，暂不处理） | 若再部署，先 resync 再应用，v0.63 起应无漂移 |
| D3 | 192.168.1.224 上已跑着一份版本不同的面板 | 迁移前需核对版本/dae/`/var/lib/homenet-panel` 数据（用户指示"不着急"） |
| D4 | 机场节点 IP 被网站标黑（claude 地区限制） | 面板侧只能给"换节点/换地区"的动作，无法从根上解决 |
