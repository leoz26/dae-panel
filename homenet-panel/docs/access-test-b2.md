# 段2：B2（主路由 DHCP 下发网关）端到端测试手册

> 适用范围：家里网络（主路由可自定义 DHCP 下发的网关地址/DNS）。
> 目标：验证「LAN 设备真的经旁路由上的 dae 上网」这条链路，并把面板的六项自检从 warn 变成 ok。
> 本文只写步骤与判定标准，不会自动执行；所有改变都能按第 7 节回滚。

## 0. 一句话原理

B2 档 = 不改设备、不改网络拓扑，只让主路由的 DHCP Option 3（网关）和 Option 6（DNS）指向旁路由，
这样 LAN 设备**自己把包发给旁路由**。旁路由上的 dae 用 tproxy 把 TCP/UDP 劫持到本地处理，
再由 dae 按分流规则直连或走订阅节点出去。面板负责写 `config.d/panel-access.dae`（声明 `lan_interface`）
和自有 nft 表 `panel_dns`（把 LAN 的 53 端口劫持给 dae），其余一律不碰。

## 1. 前置条件

- [ ] 旁路由（dae + 面板所在机器）已装好，面板可访问，`dae` 与 `homenet-panel` 均 active
- [ ] 旁路由有固定 LAN IP，且**与要接管的 LAN 在同一二层网段**（例：旁路由 192.168.5.10、主路由 192.168.5.1）
- [ ] 面板「接入向导」页探测结果里：BPF/BTF 可用、nftables 可用、dae 运行中
- [ ] 主路由后台能找到「DHCP 网关 / 默认网关 / 路由器地址（Option 3）」字段并能保存
- [ ] 已确认主路由 LAN 网段（本手册示例统一用 `192.168.5.0/24`，请按实际替换）

> 注意：部分固件把「自定义网关地址」同时用于路由器自身的出口，填错会让**整台主路由断网**。
> 因此第 3 节要求先拍照记录原值，且用手机流量保持可回退的操作通道。

## 2. 记录基线（5 分钟，必做）

在旁路由上执行并保存输出：

```sh
date -Is
ip -4 addr show                # 确认 LAN IP
ip -4 route show default       # 默认路由
cat /proc/sys/net/ipv4/ip_forward
nft list tables                # 应无 panel_* 遗留表
systemctl is-active dae homenet-panel
md5sum /etc/dae/config.dae /etc/dae/config.d/*.dae
```

在主路由后台截图记录：

- [ ] DHCP 服务开关、地址池范围
- [ ] **自定义网关地址**（原值，例：`192.168.5.1`）
- [ ] DNS 服务器 / 备用 DNS（原值，例：`192.168.5.1` / `0.0.0.0`）
- [ ] 租期（例：86400 秒）

## 3. 先做单设备实验（推荐，风险最小）

不要一上来改全屋 DHCP，先拿一台设备手改（这一步等价 B1，能提前暴露 90% 的问题）：

1. 在该设备上把 **网关** 改成旁路由 IP（例 `192.168.5.10`），**DNS** 也改成旁路由 IP
   - macOS：系统设置 → 网络 → 详情 → TCP/IP（路由器）、DNS
   - Windows：网络适配器 → IPv4 属性
   - 手机：Wi-Fi → 详情 → IP 设置 = 静态，填路由器/DNS
2. 断开重连 Wi-Fi，确认能上网。
3. 立刻在面板「接入向导」点「运行自检（只读）」，逐项对照第 5 节。
4. 顺手跑一次流量走向脚本（可选）：`python3 tools/netdiag/whereami.py`，
   看出口 IP 是否变成订阅节点所在地区、境外 TLS 是否明显慢于境内。

若这一步就失败，**先别改 DHCP**，按第 8 节排查。

## 4. 面板侧生成计划（不改系统）

1. 打开面板 →「接入向导」→「接入计划」
2. 档位选 **B2 旁路由 · 主路由下发网关**；LAN 网卡选接内网的那块（例 `eth0`/`br-lan`）；旁路由 IP 填自己
3. 点「生成接入计划」，阅读面板侧步骤、人工步骤、警告与指纹
4. 点「应用（写入 + validate + reload + 建自有表）」，二次确认后执行

预期：片段 `config.d/panel-access.dae` 生成（0600）、`dae reload` 返回 OK、
nft 出现 `table inet panel_dns`、`/etc/dae/config.dae` 与 `panel-core.dae` 的 md5 **不变**。

## 5. 改主路由 DHCP（真正切换）

按顺序做，每步之后立刻验证：

1. 主路由后台 →「地址管理 / DHCP 服务」
2. **自定义网关地址**：`192.168.5.1` → `192.168.5.10`（旁路由 IP）
3. **DNS 服务器**：`192.168.5.1` → `192.168.5.10`；备用 DNS 可留空或填公共 DNS
4. 保存 → 观察主路由自身是否还能上网（若断了，立刻把网关改回原值，见第 7 节）
5. 让一台测试设备断网重连（或等租期过半后重新获取），`ip route`/`ipconfig` 里默认网关应变成旁路由 IP

## 6. 验证点（每条都要看）

在面板「接入向导」点「运行自检（只读）」，六项应满足：

| 自检项 | 期望 | 不达标的含义 |
| --- | --- | --- |
| `dae_running` | ok | dae 没跑，先 `systemctl status dae` |
| `lan_clients` | **ok**（关键） | 还没有内网设备的流量真的进到这台机：网段不对/DHCP 没生效/客户端仍在用旧网关 |
| `dae0_counters` | ok 且数值持续增长 | 计数不涨 = 没有流量被劫持 |
| `dns_hits` | ok 且持续增长 | DNS 没走旁路由 → 域名规则（geosite/关键词）会失效，只剩 IP 规则 |
| `config_untouched` | info | 恒为说明项，不需处理 |
| `stale_tables` | ok | 有遗留 `panel_*` 表，先回滚清理再重新接管 |

辅助验证：

```sh
# 1) 面板字节统计页「按客户端」表应出现 LAN 设备 IP
curl -s --insecure -b /tmp/hnp.cookie https://192.168.1.85:8787/api/traffic/bytes | head -c 400

# 2) 旁路由上看到 LAN 客户端（有客户端 IP/端口）
ss -tnp | grep -E ':(192\.168\.5\.(1[0-9]|[2-9][0-9]))' | head

# 3) DNS 确实被本机劫持（在测试设备上执行，查询会被转发到旁路由）
nslookup www.youtube.com 192.168.5.10
nslookup www.youtube.com            # 应也返回结果，说明 DHCP 下发的 DNS 已指向旁路由

# 4) dae 日志里出现 LAN 客户端地址
journalctl -u dae -n 50 --no-pager | grep -c 192.168.5. || true
```

再挑几个规则验证分流：

- 访问境外站点（如 youtube）→ 面板「流量·命中」应显示命中 `hk`/`sg` 分组
- 访问国内站点（如 baidu）→ 应命中 `direct`
- 手机上看直播/游戏，确认没有明显卡顿；若卡顿，检查是否订阅节点时延劣化（面板「运行探测」）

## 7. 回滚（任一步都可独立回滚）

**A. 只撤面板侧（保留 DHCP 指向）**

面板「接入向导」→「回滚接入变更」；或手工：

```sh
nft delete table inet panel_dns 2>/dev/null
nft delete table inet panel_snat 2>/dev/null
nft delete table inet panel_tproxy 2>/dev/null
rm -f /etc/dae/config.d/panel-access.dae
dae reload    # 或 systemctl reload dae
```

回滚会：删除自有 nft 表 → 备份并删除片段 → reload → 把 `ip_forward` 还原成应用前的值
（dae 的 `auto_config_kernel_parameter` 会把它置 1，回滚时面板负责改回 0）。

**B. 撤主路由 DHCP**

把「自定义网关地址」「DNS 服务器」改回第 2 节记录的原值 → 保存 → 测试设备断网重连。

**C. 全撤**

先 B 再 A；然后确认 `md5sum /etc/dae/config.dae` 与基线一致、`nft list tables` 无 `panel_*`。

## 8. 故障排查

| 现象 | 排查顺序 |
| --- | --- |
| 改完 DHCP，全屋断网 | 立刻把主路由网关改回原值；多半是固件把该字段也当自身出口用 |
| 设备能上网但 `lan_clients` 仍 warn | ① 设备是否真的拿到新网关（`ip route`）；② 旁路由是否与客户端同网段同 VLAN；③ 客户端 MAC 是否被主路由 ARP 绑定 |
| 能上网但访问境外很慢/不通 | ① 面板「运行探测」看节点时延；② `journalctl -u dae -f` 看是否有 dial 失败；③ 确认 `lan_interface` 与内网网卡一致 |
| 境外站点仍走直连 | DNS 没走旁路由（`dns_hits` 不涨）→ 域名规则失效；把 DHCP DNS 指向旁路由 |
| 部分 App 连不上 | 可能是 UDP/QUIC 被代理导致；先看 dae 日志里的 `UdpEndpoint` 报错，再考虑该 App 走直连规则 |
| 回滚后 `ip_forward` 还是 1 | 面板版本较旧；手工 `sysctl -w net.ipv4.ip_forward=0`，并升级到含 `sysctl-restore` 步骤的版本 |
| OpenWrt 上 dae 起不来 | 多为 MIPS/低版本内核缺 BPF/BTF；先用面板「接入向导」看能力探测结论 |

## 9. 验收标准

- [ ] 面板自检 6 项：4 项 ok + `lan_clients` ok（共 5 ok，1 info）
- [ ] 「字节统计→按客户端」出现真实 LAN 设备 IP，且速率随使用变化
- [ ] 一台设备访问境外命中代理分组、访问国内命中 `direct`
- [ ] `md5sum /etc/dae/config.dae` 与基线一致（面板没有改整段配置）
- [ ] 回滚后：`nft list tables` 无 `panel_*`、`panel-access.dae` 不存在、`ip_forward` 回到基线、dae 与面板 active
- [ ] 至少连续运行 30 分钟无断流；期间 `journalctl -u dae` 无持续 dial 失败

## 10. 记录表（测试时填写）

| 项目 | 值 |
| --- | --- |
| 测试日期 | |
| 旁路由 IP / 网卡 | |
| 主路由型号 / 固件版本 | |
| DHCP 原网关 / 原 DNS | |
| 计划指纹 | |
| 应用时间 / 备份路径 | |
| `lan_clients` 首次变 ok 的时间 | |
| 首个出现的 LAN 客户端 IP | |
| 回滚时间 / 结果 | |
| 备注（异常、时延、丢包） | |
