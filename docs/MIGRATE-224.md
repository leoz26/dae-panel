# 迁移到 192.168.1.224（从 .85 起）

> 状态：**已于 2026-10-28 18:33–18:36 执行完毕**（§6 是实执行记录）。原始前置条件记录：前置条件只差一条——.224 的 SSH 登录方式（22 端口开着，但现有三把密钥都被拒）。本文是执行手册，不是已完成记录。

## 0. 目标机现状（2026-10-28 只读探测）

- 存活：ping 2.8ms；22 / 80 / 443 / 8787 端口开放。
- 8787 上**已经跑着一份早期构建的面板**：首页返回 200 / 86,042 字节，标题「HomeNet Panel 控制台」，引用独立资源 `/assets/vue.js`（147,534 字节），`/panel.crt` 可下载（1,188 字节）。当前主线构建是单文件内嵌（约 115KB），两者**不是同一版本**。
- `/api/health`、`/api/status` 都返回 401 → 面板正常工作，只是要登录。

结论：.224 不是空白机，迁移必须在「先备份、可回滚」的前提下覆盖。

## 1. 迁移清单（搬什么）

| 项 | .85 上的位置 | 处理 |
| --- | --- | --- |
| 面板二进制 | `/usr/local/bin/homenet-panel` | 用仓库 `bin/homenet-panel-linux-amd64`（CGO_ENABLED=0 静态链接，约 13MB） |
| 面板账号与会话 | `/etc/homenet-panel/auth.json`、`/var/lib/homenet-panel/sessions.json` | 搬 → 不用重设密码（账号密码是你改过的，迁移中不重置） |
| 面板模型 | `/var/lib/homenet-panel/model.json` | 搬 → 配置结构现成 |
| 订阅登记与镜像 | `/var/lib/homenet-panel/subsrc.json`、`/etc/dae/subs/*.sub` | 搬 → 不用重新拉上游（文件含 token，注意别外传） |
| TLS 证书 | `/etc/homenet-panel/tls.crt`、`tls.key` | 搬（SAN 只有 .85，浏览器仍会提示不受信）；或按新 IP 重签 |
| 审计历史 | `/var/lib/homenet-panel/audit.log*` | 搬 → 记录连续 |
| dae 配置 | `/etc/dae/config.dae`、`/etc/dae/config.d/panel-core.dae` | 搬（两份都 0600） |
| dae 本体 | `/usr/bin/dae`（v2.1.1） | .224 上没有就装：`deploy/install.sh --install-dae`（Debian 系走 APT，无 APT 走官方静态包并写 procd） |

## 2. 执行顺序（每步可单独回滚）

1. **登录 .224 只读摸底**：`uname -a`、`/etc/os-release`、`systemctl is-active homenet-panel dae`、`ls /etc/dae /var/lib/homenet-panel`。
2. **备份 .224 现状**：`tar czf /root/backup-224-<时间>.tgz /etc/homenet-panel /etc/dae /var/lib/homenet-panel /usr/local/bin/homenet-panel`。
3. **从 .85 打包**：`tar czf /tmp/hnp-migrate.tgz /etc/homenet-panel /etc/dae /var/lib/homenet-panel`，scp 到 .224。
4. **在 .224 落盘**：先 `mv` 掉目标目录再解包（不要原地覆盖）；`chmod 600` 配置与私钥、`chmod 700` 目录。
5. **换二进制**：`install -m 755 homenet-panel-linux-amd64 /usr/local/bin/homenet-panel.new && mv …`，systemd unit 参数照 .85 的抄。
6. **先用临时端口验证**：以 `serve --addr 0.0.0.0:8899` + 临时 auth 文件起一份，确认能读旧 model/subsrc/audit，再换正式 8787。
7. **起服务并验收**：`/api/status`（dae active + pid）、`/api/model`（diverged=false）、`/api/subsrc`（两条订阅在）、`/api/audit`（历史在）、浏览器登录一次。
8. **切流量（最后做）**：把设备/主路由的网关与 DNS 指向 .224，在「接入向导 → 运行自检」看 `lan_clients` 是否出现内网地址。这一步之前 .224 纯观测，不影响任何人上网。
9. **确认无回退需求后**，再停 .85 的面板与 dae，最后删虚拟机。

## 3. 回滚

- 面板：`systemctl stop homenet-panel` → 把第 2 步的 tgz 解回原路径 → `systemctl start homenet-panel`。
- dae：拷回备份的 `config.dae` 后 **restart**（DNS/global 段改动 `reload` 不生效）。
- 网络：把设备网关/DNS 改回 192.168.1.1（或改回指向 .85）。

## 4. 已知风险（迁移时必看）

1. **证书 SAN 不含 192.168.1.224**：搬过去的证书 SAN 只有 .85，用新 IP 访问会提示名称不匹配 → 建议按新 IP 重签（`-addext subjectAltName=IP:192.168.1.224,IP:127.0.0.1,DNS:localhost`）。
2. **订阅里的 `file://` 是相对配置目录的路径**：`subs/` 必须跟着搬，否则订阅全部报「文件不存在」。
3. **两份 live 配置的 md5 应保持一致**（`/etc/dae/config.d/panel-core.dae`、`/etc/dae/config.dae`），可作为验收项。
4. **.224 上那份旧面板的数据结构可能更老**：`model.json` 缺新字段会按缺省补齐；但必须先换新二进制再启动，不要用旧二进制读新数据。
5. **迁移期间不要双写**：.85 仍持真源，.224 只读观察。

## 5. 待确认

- .224 的 SSH 登录方式（root 密码 / 允许贴公钥 / 非 root 账户）。
- .224 上那份旧面板是否还有用处（没用就整目录备份后覆盖）。
- 是否顺便把 .224 的 dae 也装成 v2.1.1。

## 6. 实执行记录（2026-10-28 18:33–18:36）

用户授权口径：「不用备份 .224，那个是测试用的，没有价值，直接删除 dae 和面板重新部署」「.85 的规则也要弄过去」。

### 6.1 进入方式
- 目标机是 **飞牛 OS（fnOS）**，root 的 sshd 配置为 `PermitRootLogin no`（所以 root 公钥即使被 sshd 接受也会被拒），只有普通用户 `sthho`（在 Administrators 组）能登录，且 sudo 需要密码。
- 做法：用 `sthho` + 密码装好专用密钥（`~/.ssh/hnp_224_ed25519`），root 操作由 `ssh sthho@… 'sudo -S -p "" bash -s'` 走（密码与脚本同走 stdin，不需要 tty）。

### 6.2 清空（无备份，按授权）
停并 disable `dae`、`homenet-panel` → 删 `/usr/bin/dae`、`/usr/local/bin/homenet-panel`、`/etc/dae`、`/etc/homenet-panel`、`/etc/homenet-panel.auth.json`、`/var/lib/homenet-panel`、两个 unit → `daemon-reload` → 清 `panel_*` nft 表与 `dae0`。

### 6.3 迁入（来自 .85）
`/etc/dae/config.dae`、`config.d/panel-core.dae`、`config.d/panel-access.dae`、`subs/*`、`/var/lib/homenet-panel/{model,subsrc}.json`、`/etc/homenet-panel.auth.json`、`/usr/bin/dae`（v2.1.1）；面板二进制用仓库 `bin/homenet-panel-linux-amd64`。

### 6.4 与现场不同的三处
1. 证书按新 IP 重签：SAN `IP:192.168.1.224,IP:127.0.0.1,DNS:localhost`。
2. `panel-access.dae` 的 `lan_interface: ens2` → **`enp2s0-ovs`**（fnOS 用 OVS 桥，LAN 地址在这个口上）。
3. 面板 unit 的 `--traffic-iface` 同样是 `enp2s0-ovs`；`dae.service` 补 `LimitMEMLOCK=infinity` 与 `ExecStartPre=dae validate`。
4. 登录密码用 `homenet-panel setpass` 重设（不沿用 .85 的哈希，因为 .85 密码已由用户自行修改）。

### 6.5 验收结果
- `dae validate` rc=0；dae active；日志 `Successfully created Netkit device pair (performance mode)` + `Safely enabled bpf_redirect_peer() (kernel v6.18.18, netkit+scrub=NONE)`；VmRSS 98MB；memlock unlimited。
- `https://192.168.1.224:8787/` 200 / 114,961 字节（与仓库当前构建一致）；登录 200；`/api/status` = `{service:active, progressCode:ReloadDone}`。
- `/api/model`：`diverged=false`、`drift=null`；`/api/subsrc`：`summer` 已登记、`format=clash`、81/81、`changed=false`。
- headless Chrome 渲染 40,117 字节 DOM、`{{` 泄漏 0。
- 收尾：两台机器 `/tmp` 里的迁移包（含订阅 token 与密码哈希）已删除。
