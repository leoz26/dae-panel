#!/usr/bin/env bash
# HomeNet Panel 一条命令安装器（在目标设备上以 root 运行）
# 用法：bash install.sh --binary /tmp/homenet-panel [--port 8787] [--traffic-iface eth0] [--dry-run]
set -uo pipefail

BIN_SRC=""; PORT=8787; TRAFFIC_IFACE="auto"
STATE_DIR=/var/lib/homenet-panel; ETC_DIR=/etc/homenet-panel
PANEL_USER=admin; PANEL_PASS=""; AUDIT_MAX_MB=5; AUDIT_KEEP=3
DO_START=1; DRY=0; FORCE_CERT=0
INSTALL_DAE=0; RETIRE_GOOSE=0; GOOSE_ARCHIVE=/root/goose-retired
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
    --install-dae) INSTALL_DAE=1; shift;;
    --retire-goose) RETIRE_GOOSE=1; shift;;
    --goose-archive) GOOSE_ARCHIVE="$2"; shift 2;;
    -h|--help) sed -n '2,3p' "$0"; exit 0;;
    *) die "未知参数：$1";;
  esac
done

[ "$(id -u)" = 0 ] || die "必须用 root 运行"
[ -n "$BIN_SRC" ] || die "缺少 --binary <二进制路径>"
[ -f "$BIN_SRC" ] || die "找不到二进制：$BIN_SRC"
[ "$DRY" = 1 ] && info "DRY-RUN：只检查，不写任何文件"

run() { if [ "$DRY" = 1 ]; then info "将执行: $*"; else "$@"; fi }


dae_running() { # pgrep -x 在 busybox/OpenWrt 上不可靠，退到 pid 文件
  if pgrep -x dae >/dev/null 2>&1; then return 0; fi
  if [ -r /var/run/dae.pid ]; then
    pid=$(cat /var/run/dae.pid 2>/dev/null)
    [ -n "$pid" ] && [ -d "/proc/$pid" ] && return 0
  fi
  return 1
}

fetch_to() { # fetch_to URL DEST —— curl → wget → python3 三级回退
  local url=$1 dest=$2
  if command -v curl >/dev/null 2>&1; then curl -fsSL -o "$dest" "$url" && return 0; fi
  if command -v wget >/dev/null 2>&1; then wget -qO "$dest" "$url" && return 0; fi
  if command -v python3 >/dev/null 2>&1; then python3 -c 'import sys,urllib.request;open(sys.argv[2],"wb").write(urllib.request.urlopen(sys.argv[1],timeout=60).read())' "$url" "$dest" && return 0; fi
  return 1
}

step "0/8 依赖预检（dae / goose）"

# --- dae ---
DAE_BIN=$(command -v dae 2>/dev/null || true)
if [ -n "$DAE_BIN" ]; then
  DAE_VER=$("$DAE_BIN" --version 2>/dev/null | head -1)
  ok "dae: ${DAE_VER:-$DAE_BIN}"
  if dae_running; then ok "dae 在运行"; else warn "dae 未运行：面板可打开，但应用配置会失败（reload 报错）"; fi
else
  warn "未安装 dae：面板将以 degraded 模式运行（只读，不能应用配置）"
  APT=0; command -v apt-get >/dev/null 2>&1 && APT=1
  if [ "$INSTALL_DAE" = 1 ]; then
    [ "$APT" = 1 ] || die "--install-dae 目前只支持 APT 系（Debian/Ubuntu/Armbian）；其它发行版请按 https://github.com/daeuniverse/dae 安装"
    APTMAJ=$(apt-get --version 2>/dev/null | head -1 | sed -n 's/.*apt \([0-9][0-9]*\).*/\1/p')
    if [ "${APTMAJ:-2}" -ge 3 ]; then SRCFILE=daeuniverse.sources; else SRCFILE=daeuniverse.list; fi
    info "添加 daeuniverse APT 仓库（$SRCFILE，APT 主版本 ${APTMAJ:-2}）"
    if [ "$DRY" = 1 ]; then
      info "将执行: 下载 https://daeuniverse.pages.dev/$SRCFILE → /etc/apt/sources.list.d/$SRCFILE"
      info "将执行: 下载 https://daeuniverse.pages.dev/daeuniverse-archive-goose.gpg → /usr/share/keyrings/"
      info "将执行: apt-get update && apt-get install -y dae"
    else
      fetch_to "https://daeuniverse.pages.dev/$SRCFILE" "/etc/apt/sources.list.d/$SRCFILE" || die "下载 APT 源失败（网络受限？）"
      fetch_to "https://daeuniverse.pages.dev/daeuniverse-archive-goose.gpg" "/usr/share/keyrings/daeuniverse-archive-goose.gpg" || die "下载 GPG key 失败"
      chmod 644 "/usr/share/keyrings/daeuniverse-archive-goose.gpg"
      DEBIAN_FRONTEND=noninteractive apt-get update -q >/dev/null || die "apt-get update 失败"
      DEBIAN_FRONTEND=noninteractive apt-get install -y -q dae >/dev/null || die "apt-get install dae 失败"
      DAE_BIN=$(command -v dae 2>/dev/null || true)
      if [ -n "$DAE_BIN" ]; then ok "dae 已安装：$("$DAE_BIN" --version 2>/dev/null | head -1)"; else die "安装后仍找不到 dae"; fi
      warn "dae 还没配置：装完面板后打开页面的「安装与初始化」可一键生成，或自行放 /etc/dae/config.dae"
    fi
  else
    info "要和面板一起装：加 --install-dae（APT 系自动添加官方仓库并安装）"
  fi
fi

# --- goose（旧配置生成器，会与面板双写）---
GOOSE_FOUND=""
GOOSE_UNITS=""
for u in /etc/systemd/system/goose*.service /etc/systemd/system/goose*.timer \
         /lib/systemd/system/goose*.service /lib/systemd/system/goose*.timer; do
  [ -f "$u" ] && GOOSE_UNITS="$GOOSE_UNITS $u"
done
[ -n "$(command -v goose 2>/dev/null || true)" ] && GOOSE_FOUND=1
[ -d /etc/goose ] && GOOSE_FOUND=1
[ -n "$GOOSE_UNITS" ] && GOOSE_FOUND=1
pgrep -x goose >/dev/null 2>&1 && GOOSE_FOUND=1
if [ -n "$GOOSE_FOUND" ]; then
  warn "检测到 goose（旧配置生成器）：它和本面板都会写 dae 配置，可能互相覆盖"
  if [ "$RETIRE_GOOSE" = 1 ]; then
    info "退役 goose → $GOOSE_ARCHIVE（移动，不删除）"
    run mkdir -p "$GOOSE_ARCHIVE/bin" "$GOOSE_ARCHIVE/etc" "$GOOSE_ARCHIVE/unit"
    if [ -n "$GOOSE_UNITS" ]; then
      for u in $GOOSE_UNITS; do
        n=$(basename "$u")
        case "$n" in
          *.timer|*.service)
            if [ "$DRY" = 1 ]; then info "将执行: systemctl disable --now $n"; else systemctl disable --now "$n" >/dev/null 2>&1 || true; fi;;
        esac
      done
    fi
    if [ "$DRY" = 1 ]; then info "将移动 /usr/local/bin/goose* → $GOOSE_ARCHIVE/bin/"; else
      for f in /usr/local/bin/goose /usr/local/bin/goose-0.5.*.bak; do
        [ -e "$f" ] && mv "$f" "$GOOSE_ARCHIVE/bin/" 2>/dev/null || true
      done
    fi
    if [ -d /etc/goose ]; then run mv /etc/goose "$GOOSE_ARCHIVE/etc/goose"; fi
    if [ -n "$GOOSE_UNITS" ]; then
      for u in $GOOSE_UNITS; do run mv "$u" "$GOOSE_ARCHIVE/unit/"; done
      if [ "$DRY" = 0 ]; then systemctl daemon-reload >/dev/null 2>&1 || true; fi
    fi
    if [ "$DRY" = 0 ]; then
      {
        echo "goose 已退役（$(date '+%Y-%m-%d %H:%M:%S')）——配置生成权已交给 HomeNet Panel。"
        echo "- 这些文件只是被移动，未删除；确认面板稳定后可整目录删除。"
        echo "- /etc/goose/config.yaml 含订阅 token，不要外传、不要提交到 git。"
        echo "- 恢复：把需要的 goose 版本从 bin/ 拷回 /usr/local/bin/goose，再 systemctl enable --now goose.service。"
      } > "$GOOSE_ARCHIVE/README.txt"
      ok "goose 已归档到 $GOOSE_ARCHIVE"
    fi
  else
    info "一并退役：加 --retire-goose（会停用并移动到 $GOOSE_ARCHIVE，不删除）"
  fi
else
  ok "未发现 goose"
fi

# --- daed（daeuniverse 的图形面板，同样会写 dae 配置）---
if command -v daed >/dev/null 2>&1 || dpkg -l 2>/dev/null | grep -q "^ii  *daed"; then
  warn "检测到 daed（daeuniverse 的图形面板）：它和本面板都会写 dae 配置，二者只能留一个"
  DAED_ACTIVE=$(systemctl is-active daed 2>/dev/null | head -1)
  [ -n "$DAED_ACTIVE" ] || DAED_ACTIVE=inactive
  if [ "$DAED_ACTIVE" = "active" ]; then
    warn "  daed 正在运行 → 建议先停用：systemctl disable --now daed（确定不用再 apt remove daed）"
  else
    info "  daed 当前未运行（$DAED_ACTIVE）；确定不再使用后可 systemctl mask daed 或 apt remove daed"
  fi
fi

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
# OpenWrt/iStoreOS 的 /usr/local/bin 可能不存在，装到 /usr/bin（同样在 overlay 上持久）
if [ "$PLATFORM" = openwrt ]; then BIN_DST=/usr/bin/homenet-panel; fi
if [ "$DRY" = 1 ]; then info "将确保 $(dirname "$BIN_DST") 存在"; else mkdir -p "$(dirname "$BIN_DST")"; fi
if [ -f "$BIN_DST" ]; then info "已存在旧版本，将被替换（旧文件备份到 $BIN_DST.bak）"; run cp -a "$BIN_DST" "$BIN_DST.bak"; fi
# 兼容没有 install(1) 的精简系统（OpenWrt/busybox）
if command -v install >/dev/null 2>&1; then
  run install -m 0755 "$BIN_SRC" "$BIN_DST"
else
  if [ "$DRY" = 1 ]; then info "将执行: cp $BIN_SRC $BIN_DST && chmod 755 $BIN_DST"; else
    cp -f "$BIN_SRC" "$BIN_DST" && chmod 755 "$BIN_DST" || die "复制二进制失败"
  fi
fi
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
  if systemctl is-active dae >/dev/null 2>&1 || dae_running; then ok "dae 运行中"; else warn "dae 未运行"; fi
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
