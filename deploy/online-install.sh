#!/usr/bin/env bash
# HomeNet Panel 在线安装（在目标设备上以 root 运行，从 GitHub Releases 下载）
#   curl -fsSL https://github.com/leoz26/dae-panel/releases/latest/download/online-install.sh | bash
#   私有仓库需带 token（只读即可）：
#   curl -fsSL -H "Authorization: Bearer $GH_TOKEN" <同一URL> | GH_TOKEN=$GH_TOKEN bash
# 也可用 --base-url 指向自己的镜像目录。
set -uo pipefail

REPO_SLUG=${HNP_REPO:-leoz26/dae-panel}
TAG=${HNP_TAG:-latest}
BASE_URL=${HNP_BASE_URL:-}
TOKEN=${GH_TOKEN:-${GITHUB_TOKEN:-}}
PASSTHRU=()

while [ $# -gt 0 ]; do
  case "$1" in
    --base-url) BASE_URL="$2"; shift 2;;
    --tag) TAG="$2"; shift 2;;
    --repo) REPO_SLUG="$2"; shift 2;;
    -h|--help) sed -n '2,7p' "$0"; exit 0;;
    *) PASSTHRU+=("$1"); shift;;
  esac
done

ok()   { printf '  \033[32m[+]\033[0m %s\n' "$1"; }
info() { printf '  \033[36m[=]\033[0m %s\n' "$1"; }
die()  { printf '  \033[31m[x]\033[0m %s\n' "$1"; exit 1; }
step() { printf '\n\033[1m%s\033[0m\n' "$1"; }

[ "$(id -u)" = 0 ] || die "请用 root 运行（sudo bash 或 root shell）"

if [ -z "$BASE_URL" ]; then
  if [ "$TAG" = latest ]; then
    BASE_URL="https://github.com/$REPO_SLUG/releases/latest/download"
  else
    BASE_URL="https://github.com/$REPO_SLUG/releases/download/$TAG"
  fi
fi

case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64;;
  aarch64|arm64) ARCH=arm64;;
  *) die "不支持的架构 $(uname -m)：本项目只发布 linux/amd64 与 linux/arm64"
esac

TMP=$(mktemp -d /tmp/hnp-online.XXXXXX) || die "无法创建临时目录"
trap 'rm -rf "$TMP"' EXIT

fetch() { # fetch <url> <out>
  local url="$1" out="$2"
  if command -v curl >/dev/null 2>&1; then
    if [ -n "$TOKEN" ]; then curl -fsSL -H "Authorization: Bearer $TOKEN" -o "$out" "$url"; else curl -fsSL -o "$out" "$url"; fi
  elif command -v wget >/dev/null 2>&1; then
    if [ -n "$TOKEN" ]; then wget -q --header="Authorization: Bearer $TOKEN" -O "$out" "$url"; else wget -q -O "$out" "$url"; fi
  elif command -v python3 >/dev/null 2>&1; then
    python3 - "$url" "$out" "$TOKEN" <<'PY'
import sys, urllib.request
url, out, token = sys.argv[1], sys.argv[2], sys.argv[3]
req = urllib.request.Request(url)
if token:
    req.add_header('Authorization', 'Bearer ' + token)
with urllib.request.urlopen(req, timeout=60) as r, open(out, 'wb') as f:
    f.write(r.read())
PY
  else
    return 1
  fi
}

step "1/4 环境"
info "平台: $(uname -s) $(uname -m) → linux/$ARCH"
info "来源: $BASE_URL"
if [ -n "$TOKEN" ]; then info "使用 GitHub token（只读）"; fi
case "$BASE_URL" in
  https://github.com/*) if [ -z "$TOKEN" ]; then info "提示：仓库若为私有，匿名下载会 404，请用 GH_TOKEN=<只读token> 运行本脚本"; fi;;
esac

step "2/4 下载"
fetch "$BASE_URL/online-install.sh" "$TMP/online-install.sh" || die "下载 online-install.sh 失败（检查网络 / token / 仓库是否私有）"
ok "online-install.sh"
fetch "$BASE_URL/install.sh" "$TMP/install.sh" || die "下载 install.sh 失败"
ok "install.sh"
fetch "$BASE_URL/homenet-panel-linux-$ARCH" "$TMP/homenet-panel" || die "下载二进制失败（该架构可能没有发布产物）"
ok "homenet-panel-linux-$ARCH（$(wc -c < "$TMP/homenet-panel") 字节）"

if fetch "$BASE_URL/sha256sums.txt" "$TMP/sha256sums.txt" 2>/dev/null; then
  if command -v sha256sum >/dev/null 2>&1; then
    WANT=$(grep " homenet-panel-linux-$ARCH" "$TMP/sha256sums.txt" | awk '{print $1}' | head -1)
    if [ -n "$WANT" ]; then
      GOT=$(sha256sum "$TMP/homenet-panel" | awk '{print $1}')
      [ "$WANT" = "$GOT" ] && ok "校验和一致" || die "校验和不匹配（下载损坏或被篡改）"
    fi
  fi
fi

step "3/4 安装"
chmod 755 "$TMP/homenet-panel" "$TMP/install.sh"
if [ "${#PASSTHRU[@]}" -gt 0 ]; then
  bash "$TMP/install.sh" --binary "$TMP/homenet-panel" "${PASSTHRU[@]}"
else
  bash "$TMP/install.sh" --binary "$TMP/homenet-panel"
fi
RC=$?

step "4/4 完成"
[ "$RC" = 0 ] || die "安装脚本返回 $RC"
ok "在线安装完成"
exit 0
