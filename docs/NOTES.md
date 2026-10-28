# 运维笔记（按时间倒序，一次操作一条）

> 每条记：**为什么动、动了什么、怎么撤、留下了什么**。命令都是现场跑过的原文，可直接照抄。

---

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
