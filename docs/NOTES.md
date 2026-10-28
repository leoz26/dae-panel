# 运维笔记（按时间倒序，一次操作一条）

> 每条记：**为什么动、动了什么、怎么撤、留下了什么**。命令都是现场跑过的原文，可直接照抄。

---

## 2026-10-28 .224（飞牛 OS）从 .85 整机接管

- **背景**：.85 与 .11 都是虚拟机，最终部署目标是物理机 192.168.1.224（飞牛 OS）。用户要求「不用备份 .224，直接删掉 dae 和面板重新部署」，并把 .85 的规则一起迁过去。
- **.224 能力实测（关键）**：内核 `6.18.18.c952-trim`、`CONFIG_NETKIT=y`（dae 起在 **netkit 性能模式**、`bpf_redirect_peer` 已启用）、BTF 完整、memlock 1.99GB、systemd 252、内存 15.9GB、LAN 在 OVS 桥 `enp2s0-ovs`。
- **清空 + 重装**：删掉旧 dae/面板与 unit，从 .85 迁入 `config.dae`/`config.d/*.dae`/`subs/*`/`model.json`/`subsrc.json`/`auth.json`/`dae` 二进制，面板用仓库构建；证书按 .224 重签。
- **踩到的两个坑**：①只搬配置不搬 `model.json` 会让面板 `diverged=true`，用户一点「应用」就覆盖现场规则；②`lan_interface`/`--traffic-iface` 是硬编码网卡名，.85 的 `ens2` 在 .224 不存在，必须改 `enp2s0-ovs`。
- **验收**：`diverged=false`、首页 114,961 字节与仓库一致、`summer` 订阅 81/81 未变、DOM 无 `{{` 泄漏。
- **.224 登录方式备忘**：root 的 sshd 是 `PermitRootLogin no`，只能用 `sthho` 登录（Administrators 组，sudo 需密码）；已为它装专用密钥 `~/.ssh/hnp_224_ed25519`，root 操作走 `ssh sthho@192.168.1.224 'sudo -S -p "" bash -s'`。面板地址 https://192.168.1.224:8787（账号 admin，密码已由 `setpass` 重设，见会话记录，不入库）。
### 18:55 复检 .224 时发现并修掉的两处配置问题

1. **`panel_dns` nft 表把 LAN DNS 变成黑洞**：表里有 `iifname enp2s0-ovs udp/tcp dport 53 dnat to 127.0.0.1:53`，但 .224 上**没有任何进程监听 :53**（systemd-resolved inactive、无 dnsmasq）→ 设备一旦把 DNS 指向 .224 就完全解析不了。已删除该表（`nft delete table inet panel_dns`）。要做 DNS 劫持必须先在本机跑一个解析器并让它监听 127.0.0.1:53。
2. **dae 的 `qtype(aaaa) -> reject` 把 IPv6-only 域名解析打死**：EasyTier 的接入点 `speedtest.leoz26.ccwu.cc` **只有 AAAA 记录**（DoH 实测：A 无答案、AAAA=2408:8256:5087:31e9:2dce:f0be:3c53:baf3），这台机器上 `getent hosts` 直接失败，easytier-core 报 `hickory dns lookup_ip failed` 连不上。修法是在 reject 之前加例外 `qname(suffix: ccwu.cc) -> googledns`；改完 `getent ahostsv6` 马上返回正确地址，easytier 也连上了对端。已 `POST /api/model/resync` 让面板模型跟上（`diverged=false`）。

### 18:52 EasyTier 迁移（.224 接管 .85 的组网）

- 从 .85 拷来：`/usr/local/bin/easytier-core`+`easytier-cli`（2.6.4-8428a89d）、`/etc/systemd/system/easytier.service`、`hnp-mesh-nat.service`、`/usr/local/sbin/hnp-mesh-nat.sh`（自有 nft 表 `hnp_mesh`，对 192.168.1.0/24 走 tun0 做 masquerade）。
- **连通性已实测**：用 `-i 10.144.0.3/24` 试跑 35 秒，peer 表同时看到 `10.144.0.1 armbian`（wss6，18ms）与 `10.144.0.2 Debian-te`（= .85，0.8ms）；试跑后已删除 tun0。
- 服务目前 **disabled + 未启动**：unit 里写的是 `-i 10.144.0.2/24`，与 .85 冲突；等 .85 虚拟机删掉后再 `systemctl enable --now easytier hnp-mesh-nat` 接管这个地址。
- 面板 https 证书重签后浏览器会报 `tls: unknown certificate`（.224 日志里来自 192.168.1.101），点「继续」或下载 `/panel.crt` 信任即可，不是故障。

- **.85 仍在跑**，待 .224 验收稳定后再停并删除虚拟机。

## 2026-10-28 · OpenWrt(.11) 卸载 dae + 面板，回到 OpenClash 单跑

### 背景
iStoreOS 24.10.8（x86_64 / 内核 6.6.144 / 2GB 内存无 swap）那台机上，dae + homenet-panel 是排查阶段装上去的。排查结束后决定**不再用 dae，默认回到 OpenClash**。同时确认 zram 不启用。

### 结果
- dae：进程、开机自启、init 脚本、二进制、`/etc/dae` 全部撤掉，无 `dae0` 网卡 / 无 `panel_*` nft 表 / 无残留 ip rule。
- homenet-panel：服务 stop + disable，二进制移走（没有 dae 它没有作用，还能省约 50MB）。
- OpenClash：恢复运行 + 开机自启；dnsmasq 由 OpenClash 自己写回 `noresolv=1` / `server=127.0.0.1#7874`（与安装本机前一致）。
- `/etc/hosts` 里为 dae 加的 `223.5.5.5 dns.alidns.com` 已删除。
- 内存：可用从 853MB 回到 **1.39GB**。

### 顺序（先让接班的跑起来，再停旧的，避免中途断网）
1. `/etc/init.d/openclash enable && /etc/init.d/openclash start`，等它就绪。
2. `/etc/init.d/dae stop` → 验证 OpenClash 能上网（`curl https://www.cloudflare.com/cdn-cgi/trace` 拿到 `ip=/loc=`）。
3. `/etc/init.d/dae disable`，删 `/etc/init.d/dae`。
4. `mv /etc/dae <保留目录>/etc-dae`。
5. 删 `/etc/hosts` 里那一行。

### 撤下来的东西放在哪（没删）
```
/root/dae-retired-20261028-132337/
├── dae-binary            约 35MB
├── homenet-panel-binary  约 9.2MB
├── init.d-dae            带 ulimit 包装的 procd 脚本
├── etc-dae/              整个 /etc/dae（config.d/panel-*.dae、subs/、backups/）
└── hosts.before          改前的 /etc/hosts
```

想恢复（按需，全部可逆）：

```sh
D=/root/dae-retired-20261028-132337
mv $D/etc-dae /etc/dae
cp $D/dae-binary /usr/bin/dae
cp $D/init.d-dae /etc/init.d/dae && chmod +x /etc/init.d/dae
/etc/init.d/dae enable && /etc/init.d/dae start
# 面板同理：cp $D/homenet-panel-binary /usr/bin/homenet-panel && /etc/init.d/homenet-panel start
```

### 顺带确认的事实
- **zram 未启用**：`SwapTotal: 0`、`/sys/block/zram0/disksize = 0`、logread 无 zram 记录。包（`zram-swap`/`kmod-zram`/`swap-utils`）和开机脚本 `/etc/rc.d/S15zram` 都在，但脚本从未真正生效。默认大小规则是 `ram_kb/2048` MB（2GB ≈ 990MB）。要开：`uci set system.@system[0].zram_size_mb=512 && uci commit system && /etc/init.d/zram start`；要撤：`/etc/init.d/zram stop`。本次**决定不开**。
- **DNS 实验没有波及 .85**：DoH（`https://dns.alidns.com/dns-query`）、`/etc/hosts` 那行、去掉 `tls://223.5.5.5:853` 这些只改在 .11 上；.85 至今仍是 `alidns: tls://223.5.5.5:853` + `googledns: udp://8.8.8.8:53`，dae 与面板均 active，解析正常。
- **剩下的不是回滚造成的**：OpenClash 现在 google/youtube 间歇 5s 超时，日志全部指向同一个中转 `so260520-cn-376n.sonlodd365.com:42336`（183.238.84.19）`i/o timeout`，另有「新加坡 Q2 健康检查超时」。属该中转节点/线路问题，需要在 LuCI 里换节点或更新订阅。

### 两条硬经验（下次别再踩）
- **换"谁在代理"必须先起来后停**：先停旧的再起新的，中间必然有一段全断，ssh 也跟着受影响。
- **OpenWrt 上没有 systemd，别照抄 `systemctl`**：这里是 `/etc/init.d/<svc> {start|stop|enable|disable|status}`，自启靠 `/etc/rc.d/S*` 符号链接。
