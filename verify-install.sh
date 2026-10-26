#!/bin/sh
# verify-install.sh —— 安装后只读自检（不改任何东西）
# 用法: sh verify-install.sh [--host 127.0.0.1] [--port 8787]
set -u

HOST=127.0.0.1
PORT=8787
while [ $# -gt 0 ]; do
  case "$1" in
    --host) HOST=$2; shift 2 ;;
    --port) PORT=$2; shift 2 ;;
    -h|--help) sed -n '2,4p' "$0"; exit 0 ;;
    *) echo "未知参数: $1" >&2; exit 2 ;;
  esac
done

if [ -t 1 ]; then C_OK=$(printf '\033[32m'); C_NOTE=$(printf '\033[33m'); C_BAD=$(printf '\033[31m'); C_Z=$(printf '\033[0m'); else C_OK=; C_NOTE=; C_BAD=; C_Z=; fi

ok=0; warn=0; bad=0
pass() { ok=$((ok+1));  printf '  [%sOK%s] %s\n' "$C_OK" "$C_Z" "$1"; }
note() { warn=$((warn+1)); printf '  [%s--%s] %s\n' "$C_NOTE" "$C_Z" "$1"; }
fail() { bad=$((bad+1));  printf '  [%sXX%s] %s\n' "$C_BAD" "$C_Z" "$1"; }

echo "1/6 运行时与能力"
pass "内核 $(uname -r) / 架构 $(uname -m)"
[ -e /sys/kernel/btf/vmlinux ] && pass "BTF 可用（dae 需要）" || fail "缺少 BTF（内核未开 CONFIG_DEBUG_INFO_BTF，dae 无法运行）"
MLOCK_B=$(awk '/^Max locked memory/{print $5}' /proc/self/limits 2>/dev/null)
case "$MLOCK_B" in
  ''|*[!0-9]*) MLOCK=unlimited ;;
  *) MLOCK=$((MLOCK_B / 1024)) ;;
esac
case "${MLOCK:-}" in
  unlimited) pass "memlock unlimited" ;;
  *) if [ "$MLOCK" -ge 65536 ]; then pass "memlock ${MLOCK}KB"; else note "memlock ${MLOCK}KB 偏小（建议 LimitMEMLOCK=infinity）"; fi ;;
esac
[ -r /proc/sys/net/ipv4/ip_forward ] && pass "ip_forward=$(cat /proc/sys/net/ipv4/ip_forward)" || note "读不到 ip_forward"

echo "2/6 安装产物"
for d in /var/lib/homenet-panel /etc/homenet-panel; do
  if [ -d "$d" ]; then pass "目录 $d（$(stat -c %a "$d" 2>/dev/null || echo ?)）"; else fail "缺少目录 $d"; fi
done
if [ -x /usr/local/bin/homenet-panel ]; then
  pass "二进制 /usr/local/bin/homenet-panel（$(stat -c %s /usr/local/bin/homenet-panel 2>/dev/null) 字节）"
else
  fail "缺少二进制 /usr/local/bin/homenet-panel"
fi
[ -f /etc/homenet-panel.auth.json ] && pass "凭证（$(stat -c %a /etc/homenet-panel.auth.json 2>/dev/null)）" || note "无凭证文件，需 setpass 设密码"
[ -f /etc/homenet-panel/tls.crt ] && pass "证书 /etc/homenet-panel/tls.crt" || note "无证书（自签可由 install.sh 生成）"

echo "3/6 服务定义"
if [ -f /etc/systemd/system/homenet-panel.service ]; then
  pass "systemd unit 存在"
  if command -v systemctl >/dev/null 2>&1; then
    systemctl is-enabled homenet-panel >/dev/null 2>&1 && pass "已 enable" || note "未 enable"
    systemctl is-active homenet-panel >/dev/null 2>&1 && pass "active" || fail "未运行"
  fi
elif [ -f /etc/init.d/homenet-panel ]; then
  pass "procd init 脚本存在"
  [ -x /etc/init.d/homenet-panel ] && pass "可执行" || fail "不可执行"
  pgrep -f 'homenet-panel serve' >/dev/null 2>&1 && pass "进程在跑" || note "进程未运行"
else
  fail "既无 systemd unit 也无 init.d 脚本"
fi

echo "4/6 端口与进程"
if command -v ss >/dev/null 2>&1; then
  ss -ltn 2>/dev/null | grep -q ":$PORT " && pass "监听 $PORT" || fail "端口 $PORT 未监听"
elif command -v netstat >/dev/null 2>&1; then
  netstat -ltn 2>/dev/null | grep -q ":$PORT " && pass "监听 $PORT" || fail "端口 $PORT 未监听"
else
  note "无 ss/netstat，跳过端口检查"
fi
pgrep -x dae >/dev/null 2>&1 && pass "dae 在跑（$(pgrep -x dae | tr '\n' ' ')）" || note "dae 未运行（面板仍可用，但无法应用配置）"

echo "5/6 HTTPS 可达性"
if command -v python3 >/dev/null 2>&1; then
  python3 - "$HOST" "$PORT" <<'PY'
import ssl, sys, urllib.error, urllib.request
host, port = sys.argv[1], sys.argv[2]
ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE
for path in ("/panel.crt", "/api/status"):
    url = "https://%s:%s%s" % (host, port, path)
    try:
        with urllib.request.urlopen(urllib.request.Request(url), timeout=8, context=ctx) as r:
            code = r.status
    except urllib.error.HTTPError as e:
        code = e.code
    except Exception as e:
        print("  [XX] %s -> %s" % (path, e))
        continue
    mark = "OK" if code in (200, 401, 403) else "--"
    print("  [%s] %s -> %d" % (mark, path, code))
PY
else
  note "无 python3，跳过 HTTPS 探测"
fi

echo "6/6 结论"
printf '  通过 %d 项 / 提示 %d 项 / 失败 %d 项\n' "$ok" "$warn" "$bad"
if [ "$bad" -eq 0 ]; then echo "  [+] 自检通过"; else echo "  [x] 有失败项，见上"; fi
[ "$bad" -eq 0 ] && exit 0 || exit 1
