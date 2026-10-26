# 分流规则写法详解（dae routing）

> 本文里的每条语法都在真机（dae v2.1.1）上用 `dae validate` 跑过，标注为「实测」；语义部分对齐 dae 官方
> [docs/en/configuration/routing.md](https://github.com/daeuniverse/dae/blob/main/docs/en/configuration/routing.md)。

规则写在面板「内容层 → 分流规则」里，面板最终渲染成 dae 的：

```
routing {
    <匹配器> -> <出站>
    ...
    fallback: <出站>
}
```

## 1. 三条基本规则

| 规则 | 说明 |
|---|---|
| 顺序敏感 | **自上而下，首条命中即生效**，后面的规则不再看。所以「特例」必须写在「通用」前面。 |
| 一行一意图 | 一行里用 `&&` 组合多个匹配器是**与**关系；想要「或」，在**同一个匹配器**里用逗号，或拆成多行。 |
| 必须有兜底 | 没命中任何规则的流量走 `fallback`（面板里叫「兜底」）。填 `proxy` 组 = 默认走代理，填 `direct` = 默认直连。 |

## 2. 面板自动注入的 4 条安全规则

这 4 条由代码前置注入（`internal/model/model.go` 的 `EssentialRules` + `EnsureEssentials`），不在规则表里显示、删不掉，永远排在最前面：

```
dip(192.168.0.0/16) -> direct
dip(127.0.0.0/8, 169.254.0.0/16, 224.0.0.0/4) -> direct
dip(255.255.255.255/32) -> direct
pname(homenet-panel, dae, dhcpcd, systemd-resolved, sshd, systemd-timesyncd) -> direct
```

逐条拆开看：

### 2.1 `dip(192.168.0.0/16) -> direct`

- `dip` = **destination IP**，目标地址。括号里是 CIDR 网段，命中就直连。
- 这条覆盖 **RFC1918 私有网段里最常用的 192.168.x.x**：192.168.0.0/16 一个 /16 就包含 192.168.0.0–192.168.255.255 全部，所以家里/公司换网段（192.168.1.x、192.168.5.x、192.168.31.x…）都不用改。
- 为什么必须有：没有它，发往内网设备（NAS、路由器管理页、打印机）的流量会落到 `fallback`，被丢进代理组。历史上就踩过——本机 dhcpcd 的 DHCP 包（68→67）被打到香港节点，日志是
  `WARN UdpEndpoint read loop exited with error dialer=comet95.cf.090227.xyz lAddr=192.168.1.85:68 proxy_addr=192.168.1.1:67`。
- 等价写法：`dip(192.168.0.0/16) -> direct` / `dip(192.168.0.0/16) -> must_direct`（见 §4 的区别）。
- **如果你家的网段是 10.x 或 172.16–31.x**（常见于软路由、Docker、公司网），把这一条补成 `dip(10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)` 更稳妥——面板注入的那 4 条改不了，但你可以自己在规则表里**加一条更宽的**放在任意位置（它会排在那 4 条之后，但对 10.x 依然生效，因为那 4 条只命中 192.168/回环/组播）。

### 2.2 `dip(127.0.0.0/8, 169.254.0.0/16, 224.0.0.0/4) -> direct`

括号里三个网段，逗号 = **或**（任一命中即直连）：

| 网段 | 是什么 | 不直连会怎样 |
|---|---|---|
| `127.0.0.0/8` | 回环（localhost） | 本机访问自己的服务（面板 8787、dae 的 API）被扔进代理 → 直接连不上 |
| `169.254.0.0/16` | 链路本地（APIPA、mDNS/Bonjour、部分 Docker 网桥） | 局域网设备发现失效 |
| `224.0.0.0/4` | 组播（224.0.0.0–239.255.255.255，含 SSDP/UPnP、mDNS 224.0.0.251） | 智能家居发现、投屏、AirPlay 断掉 |

### 2.3 `dip(255.255.255.255/32) -> direct`

- 255.255.255.255 是 **受限广播（limited broadcast）**，不属于上面任何网段（它不在 224/4 里，也不在 192.168/16 里），所以必须单独写。
- 典型来源：DHCP 客户端在还没拿到地址时的 DHCPDISCOVER 广播、SMB 的 NetBIOS 名字广播。
- `/32` 表示「就这一个地址」。实测 `dip(255.255.255.255) -> direct`（不带 /32）dae 也接受，两种写法等价。

### 2.4 `pname(homenet-panel, dae, dhcpcd, systemd-resolved, sshd, systemd-timesyncd) -> direct`

`pname` = **process name**（发起连接的本机进程名）。逗号分隔多个，或关系。

| 进程 | 为什么必须直连 |
|---|---|
| `homenet-panel` | 面板自己要拉订阅、拉 GitHub release、探上游 geo 版本；被代理会拖慢或直接超时（实测被代理时 `/api/subsrc/apply` 30s 超时，直连后 0.8s） |
| `dae` | dae 自己出站（探活、下载 geo）不能再被自己代理，否则成环 |
| `dhcpcd` | 拿 IP；被代理就上不了网 |
| `systemd-resolved` | 本机 DNS 解析 |
| `sshd` | SSH 登录（否则远程管理可能被掐） |
| `systemd-timesyncd` | NTP 校时 |

**两个必须知道的限制（实测）**：

1. **进程名最多 16 个字符**，超了 dae 会静默裁断并打一行 info 日志。验证命令与输出：
   ```
   $ dae validate -c /tmp/x/config.dae
   level=info msg="pname routing: trim \"systemd-timesyncd\" to \"systemd-timesync\" because it is too long."
   ```
   `systemd-timesyncd` 是 17 个字符，所以只能匹配到内核里的 `systemd-timesync`。写超长不会报错，只是悄悄变短。
2. **`pname` 只对本机进程有效**（dae 文档原文：*only support localhost process when binding to WAN*）。旁路由转发别人的流量时拿不到对方的进程名，所以这条只解决「本机自己人」的问题；LAN 客户端的直连要靠 §2.1 那类目标网段规则。
3. **写错进程名不会报错**：`pname(nonexistent_proc_name_123)` 实测也 `validate` 通过（只 trim 到 16 字符），只是永远不命中——属于静默失效。

## 3. 匹配器全表

| 写法 | 含义 | 例 | 实测 |
|---|---|---|---|
| `domain(geosite:cn)` | geosite 分类 | `domain(geosite:youtube)` | ✅ |
| `domain(geosite:a, geosite:b)` | 多分类（逗号 = 或） | `domain(geosite:cn, geosite:geolocation-cn)` | ✅ |
| `domain(suffix: x.com)` | 后缀匹配（不带参数名时默认就是 suffix） | `domain(suffix: claude.ai)` | ✅ |
| `domain(full: x.com)` | 完全相等 | `domain(full: dns.google)` | 文档 |
| `domain(keyword: x)` | 子串包含 | `domain(keyword: facebook)` | 文档 |
| `domain(regex: '...')` | 正则（注意转义与引号） | `domain(regex: '\.goo.*\.com$')` | 文档 |
| `ip(geoip:cn)` / `dip(geoip:cn)` | 目标 IP 归属库 | `dip(geoip:cn)` | ✅ 两者等价 |
| `dip(1.2.3.4)` / `dip(10.0.0.0/8)` | 目标 IP / 网段 | `dip(192.168.0.0/16)` | ✅ |
| `sip(...)` | 源 IP（= 谁发起的，旁路由场景常用） | `sip(192.168.0.0/24) -> direct` | ✅ |
| `dport(80)` / `dport(10080-30000)` / `dport(1-1023, 8443)` | 目标端口（支持区间与逗号） | `dport(443)` | ✅ |
| `sport(...)` | 源端口 | `sport(38563)` | 文档 |
| `l4proto(tcp)` / `l4proto(udp)` | 四层协议（旧名 `proto` 亦可） | `l4proto(udp) && dport(443)` | ✅ |
| `ipversion(4)` / `ipversion(6)` | IP 版本 | `ipversion(6) -> ipv6组` | ✅ |
| `mac('aa:bb:cc:dd:ee:ff')` | 源 MAC（需引号） | `mac('02:42:ac:11:00:02')` | ✅ |
| `pname(curl, dae)` | 进程名（≤16 字符） | 见 §2.4 | ✅ |
| `dscp(0x4)` | DSCP 标记 | 文档 |
| `domain(ext:"a.dat:tag")` / `dip(ext:"a.dat:tag")` | 自定义 dat（放 `/usr/share/dae/`） | 文档 |

**没有的写法**（实测报错，别用）：

- `domain(a) || domain(b)` → `failed to parse config file … token recognition error at: '|'`
- 括号 OR `(a || b)` → 同上；用逗号放进同一个匹配器，或拆多行。

## 4. 出站写什么，以及 must 的区别

| 出站 | 含义 |
|---|---|
| `direct` | 直连（默认会接管并处理该连接的 DNS，用于域名分流） |
| `must_direct` | 直连但**不**接管 DNS。等价 `direct(must)`。DNS 解析成环时用（要写 `pname(mosdns) -> must_rules` 那类） |
| `block` | 丢弃（广告拦截靠这个） |
| `must_block` | 丢弃且不接管 DNS，等价 `block(must)` |
| `组名` | 例如 `hk` / `proxy` / `sg`，走该分组选出的节点 |
| `direct(mark: 0x800)` | 打 fwmark，配合 `ip rule` 把流量导到指定接口（如 wg0） |
| `fallback: 组名` | 兜底，只能写一条，且必须写在 routing 末尾 |

实测 `pname(dae) -> direct(must)` 与 `pname(dae) -> must_direct` 都能通过 `validate`。

## 5. 组合写法

```
# 与：一行内 &&
dip(8.8.8.8) && l4proto(tcp) && dport(1-1023, 8443) -> direct

# 非：! 前缀
!domain(geosite:cn) -> proxy

# 或：同一匹配器内逗号
domain(geosite:cn, geosite:geolocation-cn) -> direct

# 或（不同匹配器）：拆多行
domain(geosite:youtube) -> hk
domain(geosite:category-ai-!cn) -> hk
```

## 6. 常见坑

1. **顺序**：`domain(geosite:category-ai-!cn)` 写在 `domain(geosite:youtube)` 后面还是前面，会决定 youtube 走哪条——被前面命中的永远轮不到后面。
2. **域名规则必须让 DNS 经过 dae**（旁路由场景尤其注意）：客户端把 DNS 直接发给主路由，dae 只看到 IP 看不到域名，`domain(...)` 类规则就全不命中，只剩 `dip/ip` 生效。
3. **geosite tag 不存在不会报错，只是永不命中**（例如 `steam@cn`、`microsoft@cn` 在 Loyalsoldier 库里没有）。面板的「模型检查」会把这类规则标红。
4. **端口语法**：区间用 `-`（`1-1023`），多段用逗号，不要写成 `1:1023`。
5. **箭头两边要空格**：`->`；`fallback:` 用冒号且不带 `->`。
6. **pname 16 字符上限**，写超长静默裁断。
7. **同一个 domain 的 DNS 结果共享**：dae 是按「目标 IP → 域名位图」匹配的（见官方 *Device-scoped domain whitelist* 一节），一台设备解析过的域名，其 IP 对其他设备的域名规则也可能生效。

## 7. 怎么验证

三层，从快到慢：

1. **面板「用当前(未保存)规则试算」**：填域名，立刻给出命中第几条、出站是什么。它只评估域名类匹配器（`domain` 系列），`dip/sip/dport/pname/mac/ipversion` 属于连接上下文或需要 IP 数据，显示为「与域名无关」。
2. **面板「模型检查」**：应用前扫出「出站不存在 / geosite tag 不存在 / 规则永不命中」这类静态问题。
3. **`dae validate`**：语法级最终裁判。临时目录里试：
   ```bash
   cp -a /etc/dae /tmp/daetest && chmod 600 /tmp/daetest/config.dae
   # 编辑 /tmp/daetest/config.d/panel-core.dae
   dae validate -c /tmp/daetest/config.dae
   ```
   退出码 0 = 语法通过；报错会给出行列号与脱字符定位。
