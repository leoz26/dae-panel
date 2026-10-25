#!/usr/bin/env bash
# HomeNet Panel 一条命令安装器（在目标设备上以 root 运行）
# 用法：bash install.sh --binary /tmp/homenet-panel [--port 8787] [--traffic-iface eth0] [--dry-run]
set -uo pipefail

BIN_SRC=""; PORT=8787; TRAFFIC_IFACE="auto"
STATE_DIR=/var/lib/homenet-panel; ETC_DIR=/etc/homenet-panel
PANEL_USER=admin; PANEL_PASS=""; AUDIT_MAX_MB=5; AUDIT_KEEP=3
DO_START=1; DRY=0; FORCE_CERT=0
BIN_DST=/usr/local/bin/homenet-panel
AUTH_FILE=/etc/homenet-panel.auth.json

ok()   { printf '  \033[32m[+]\033[0m %s\n' "$1"; }
info() { printf '  \033[36m[=]\033[0m %s\n' "$1"; }
warn() { printf '  \033[33m[!]\033[0m %s\n' "$1"; }
die()  { printf '  \033[31m[x]\033[0m %s\n' "$1"; exit 1; }
step() { printf '\n\033[1m%s\033[0m\n' "$1"; }

while [ $# -gt 0 ]; do
  case "$1" in
    --binary) BIN_SRC="$2"; shift 2;;
    --port) PORT="$2"; shift 2;;
    --traffic-iface) TRAFFIC_IFACE="$2"; shift 2;;
    --state-dir) STATE_DIR="$2"; shift 2;;
    --etc-dir) ETC_DIR="$2"; shift 2;;
    --user) PANEL_USER="$2"; shift 2;;
    --password) PANEL_PASS="$2"; shift 2;;
    --audit-max-mb) AUDIT_MAX_MB="$2"; shift 2;;
    --audit-keep) AUDIT_KEEP="$2"; shift 2;;
    --no-start) DO_START=0; shift;;
    --dry-run) DRY=1; shift;;
    --force-cert) FORCE_CERT=1; shift;;
    -h|--help) sed -n '2,3p' "$0"; exit 0;;
    *) die "未知参数：$1";;
  esac
done

[ "$(id -u)" = 0 ] || die "必须用 root 运行"
[ -n "$BIN_SRC" ] || die "缺少 --binary <二进制路径>"
[ -f "$BIN_SRC" ] || die "找不到二进制：$BIN_SRC"
[ "$DRY" = 1 ] && info "DRY-RUN：只检查，不写任何文件"

run() { if [ "$DRY" = 1 ]; then info "将执行: $*"; else "$@"; fi }

# ---------- 1. 平台 ----------
step "1/8 平台与能力探测"
ARCH=$(uname -m)
SVC=""; PLATFORM="linux"
if [ -f /etc/openwrt_release ]; then PLATFORM=openwrt; SVC=procd;
elif [ -d /run/systemd/system ]; then SVC=systemd; fi
if [ "$SVC" = procd ] && [ -x /sbin/procd ]; then :; elif [ "$SVC" = procd ]; then SVC=""; fi
info "系统: $PLATFORM  架构: $ARCH  服务管理: ${SVC:-未识别}"

if [ -f /sys/kernel/btf/vmlinux ]; then ok "内核 BTF 可用"; else warn "缺少 /sys/kernel/btf/vmlinux：dae 无法运行（面板界面仍可用）"; fi
MEMLOCK=$(awk '/Max locked memory/ {print $5}' /proc/self/limits 2>/dev/null || echo 0)
if [ "$MEMLOCK" = unlimited ] || { [ -n "$MEMLOCK" ] && [ "$MEMLOCK" -ge 65536 ] 2>/dev/null; }; then ok "memlock 足够"; else warn "memlock 偏小（${MEMLOCK}KB）：dae 大节点量时可能 map 分配失败"; fi
if command -v dae >/dev/null 2>&1; then info "dae: $(dae --version 2>/dev/null | head -1)"; else warn "未检测到 dae：面板可以装上，但应用配置会失败（先装 dae）"; fi

TRAFFIC_IFACE_AUTO=$(ip route show default 2>/dev/null | awk '/default/ {print $5; exit}')
[ -z "$TRAFFIC_IFACE_AUTO" ] && TRAFFIC_IFACE_AUTO=$(ip -o -4 addr show scope global 2>/dev/null | awk '{print $2; exit}')
if [ "$TRAFFIC_IFACE" = auto ]; then TRAFFIC_IFACE="$TRAFFIC_IFACE_AUTO"; fi
[ -n "$TRAFFIC_IFACE" ] || die "无法自动识别对外网卡，请用 --traffic-iface 指定"
info "对外网卡: $TRAFFIC_IFACE（用于字节统计）"

"$BIN_SRC" >/dev/null 2>&1; RCBIN=$?
if [ "$RCBIN" = 126 ] || [ "$RCBIN" = 127 ]; then
  die "二进制无法在本机执行（exec 失败，疑似架构不匹配：本机 $ARCH）"
fi

# ---------- 2. 目录 ----------
step "2/8 目录与权限"
run mkdir -p "$STATE_DIR/backups" "$ETC_DIR"
run chmod 700 "$STATE_DIR" "$ETC_DIR"
ok "$STATE_DIR（model.json / backups / audit.log / subsrc.json / sessions.json）"
ok "$ETC_DIR（TLS 证书）"

# ---------- 3. 二进制 ----------
step "3/8 安装二进制"
if [ -f "$BIN_DST" ]; then info "已存在旧版本，将被替换（旧文件备份到 $BIN_DST.bak）"; run cp -a "$BIN_DST" "$BIN_DST.bak"; fi
run install -m 0755 "$BIN_SRC" "$BIN_DST"
ok "$BIN_DST"

# ---------- 4. 证书 ----------
step "4/8 HTTPS 证书（自签）"
CERT=$ETC_DIR/tls.crt; KEY=$ETC_DIR/tls.key
if [ -f "$CERT" ] && [ -f "$KEY" ] && [ "$FORCE_CERT" = 0 ]; then
  info "已有证书，保留（要重签加 --force-cert）"
else
  IPS=$(ip -o -4 addr show scope global 2>/dev/null | awk '{print $4}' | cut -d/ -f1 | tr '\n' ',' | sed 's/,$//')
  SAN="IP:127.0.0.1,DNS:localhost"
  for i in $(echo "$IPS" | tr ',' ' '); do SAN="$SAN,IP:$i"; done
  info "SAN=$SAN"
  if command -v openssl >/dev/null 2>&1; then
    run openssl req -x509 -newkey rsa:2048 -nodes -days 825 -subj "/CN=homenet-panel" -addext "subjectAltName=$SAN" -keyout "$KEY" -out "$CERT"
    run chmod 600 "$KEY" "$CERT"
    ok "已生成自签证书"
  else
    warn "没有 openssl：跳过证书生成，面板将以明文 HTTP 启动（不推荐）"
  fi
fi

# ---------- 5. 账号 ----------
step "5/8 登录账号"
GENPASS=""
if [ -f "$AUTH_FILE" ] && [ -z "$PANEL_PASS" ]; then
  info "已有凭证 $AUTH_FILE，保留（要改密码用 --password 或 homenet-panel setpass）"
else
  if [ -z "$PANEL_PASS" ]; then PANEL_PASS=$(openssl rand -base64 12 2>/dev/null | tr -d '/+=' | cut -c1-16); GENPASS="$PANEL_PASS"; fi
  if [ "$DRY" = 1 ]; then info "将执行: $BIN_DST setpass --user $PANEL_USER --password *** --auth-path $AUTH_FILE";
  else "$BIN_DST" setpass --user "$PANEL_USER" --password "$PANEL_PASS" --auth-path "$AUTH_FILE" >/dev/null && ok "账号 $PANEL_USER 已设置（$AUTH_FILE，0600）"; fi
fi

# ---------- 6. 服务定义 ----------
step "6/8 服务定义（${SVC:-手动}）"
TS=$(date +%Y%m%d-%H%M%S)
SERVE_ARGS="serve --addr 0.0.0.0:$PORT --backup-dir $STATE_DIR/backups --audit-path $STATE_DIR/audit.log --audit-max-mb $AUDIT_MAX_MB --audit-keep $AUDIT_KEEP --traffic-iface $TRAFFIC_IFACE"
if [ -f "$CERT" ] && [ -f "$KEY" ]; then SERVE_ARGS="$SERVE_ARGS --tls-cert $CERT --tls-key $KEY"; fi

if [ "$SVC" = systemd ]; then
  UNIT=/etc/systemd/system/homenet-panel.service
  ENVLINE=""
  if [ -f /etc/homenet-panel.env ]; then ENVLINE="EnvironmentFile=-/etc/homenet-panel.env"; fi
  if [ -f "$UNIT" ]; then run cp -a "$UNIT" "/root/homenet-panel.service.bak.$TS"; info "旧 unit 备份到 /root/homenet-panel.service.bak.$TS"; fi
  if [ "$DRY" = 1 ]; then info "将写入 $UNIT"; else
  cat > "$UNIT" <<EOF
[Unit]
Description=HomeNet Panel (dae 配置管理 REST API + Web 控制台)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
$ENVLINE
ExecStart=$BIN_DST $SERVE_ARGS
# dae 要建大量 BPF map，默认 8MB memlock 会限制规模（E20）
LimitMEMLOCK=infinity
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
  ok "写入 $UNIT"; fi
elif [ "$SVC" = procd ]; then
  INIT=/etc/init.d/homenet-panel
  if [ -f "$INIT" ]; then run cp -a "$INIT" "/root/homenet-panel.init.bak.$TS"; fi
  if [ "$DRY" = 1 ]; then info "将写入 $INIT"; else
  cat > "$INIT" <<EOF
#!/bin/sh /etc/rc.common
USE_PROCD=1
START=99

start_service() {
	procd_open_instance
	procd_set_param command $BIN_DST $SERVE_ARGS
	procd_set_param limits memlock=unlimited 2>/dev/null || true
	procd_set_param respawn
	procd_close_instance
}
EOF
  chmod 755 "$INIT"; ok "写入 $INIT"; fi
else
  warn "未识别服务管理器：文件已就位，请手动启动"
  info "$BIN_DST $SERVE_ARGS"
  DO_START=0
fi

# ---------- 7. 启动 ----------
step "7/8 启动服务"
if [ "$DO_START" = 0 ]; then warn "跳过启动（--no-start 或平台不支持）"
elif [ "$DRY" = 1 ]; then info "将启用并启动服务";
else
  if [ "$SVC" = systemd ]; then
    systemctl daemon-reload && systemctl enable homenet-panel >/dev/null 2>&1 && systemctl restart homenet-panel
    sleep 2
    systemctl is-active homenet-panel >/dev/null 2>&1 && ok "homenet-panel 运行中" || die "启动失败：journalctl -u homenet-panel -n 40"
  elif [ "$SVC" = procd ]; then
    "$INIT" enable >/dev/null 2>&1; "$INIT" restart
    sleep 2
    pgrep -f "homenet-panel serve" >/dev/null 2>&1 && ok "homenet-panel 运行中" || die "启动失败：logread | tail -40"
  fi
fi

# ---------- 8. 自检 ----------
step "8/8 自检"
if [ "$DRY" = 1 ]; then info "DRY-RUN 结束，未做任何改动"; exit 0; fi
LISTEN=0
for i in 1 2 3 4 5 6 7 8 9 10; do
  if ss -ltn 2>/dev/null | grep -q ":$PORT "; then LISTEN=1; break; fi
  sleep 1
done
[ "$LISTEN" = 1 ] && ok "端口 $PORT 已监听" || warn "端口 $PORT 未监听，检查日志"
if command -v dae >/dev/null 2>&1; then
  if systemctl is-active dae >/dev/null 2>&1 || pgrep -x dae >/dev/null 2>&1; then ok "dae 运行中"; else warn "dae 未运行"; fi
fi
if [ -f "$STATE_DIR/model.json" ]; then info "面板模型: $STATE_DIR/model.json"; else info "面板模型: 首次打开页面时会从 $ETC_DIR/../dae/config.dae 解析生成"; fi

IP=$(ip -o -4 addr show scope global 2>/dev/null | awk '{print $4}' | cut -d/ -f1 | grep -v '^127\.' | head -1)
SCHEME=https; [ -f "$CERT" ] || SCHEME=http
printf '\n\033[1m部署完成\033[0m\n'
printf '  访问:   %s://%s:%s\n' "$SCHEME" "${IP:-<本机IP>}" "$PORT"
printf '  账号:   %s\n' "$PANEL_USER"
if [ -n "$GENPASS" ]; then printf '  密码:   %s  \033[33m(仅显示这一次，请立刻保存)\033[0m\n' "$GENPASS"; else printf '  密码:   (沿用原有凭证，可用 setpass 修改)\n'; fi
printf '  状态:   %s\n' "$STATE_DIR"
printf '  证书:   %s（自签，浏览器需点继续，或导入信任库）\n' "$CERT"
printf '  下一步: 打开页面 →「接入向导」跑一次探测与自检\n'
