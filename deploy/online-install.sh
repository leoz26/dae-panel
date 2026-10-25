#!/usr/bin/env bash
# HomeNet Panel 在线安装（目标设备上以 root 运行，从 GitHub Releases 下载并安装）
#   公开仓库： curl -fsSL https://github.com/leoz26/dae-panel/releases/latest/download/online-install.sh | bash
#   私有仓库： curl -fsSL -H "Authorization: Bearer $GH_TOKEN" -o /tmp/oi.sh <同一URL> && GH_TOKEN=... bash /tmp/oi.sh
#              （私有仓库的资产必须走 API + Accept: application/octet-stream，本脚本已处理）
#   离线/镜像： bash online-install.sh --base-url http://my-mirror/hnp --tag v0.35.0
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
    -h|--help) sed -n '2,6p' "$0"; exit 0;;
    *) PASSTHRU+=("$1"); shift;;
  esac
done

ok()   { printf '  \033[32m[+]\033[0m %s\n' "$1"; }
info() { printf '  \033[36m[=]\033[0m %s\n' "$1"; }
die()  { printf '  \033[31m[x]\033[0m %s\n' "$1"; exit 1; }
step() { printf '\n\033[1m%s\033[0m\n' "$1"; }

[ "$(id -u)" = 0 ] || die "请用 root 运行"

if [ -z "$BASE_URL" ]; then
  if [ "$TAG" = latest ]; then BASE_URL="https://github.com/$REPO_SLUG/releases/latest/download"
  else BASE_URL="https://github.com/$REPO_SLUG/releases/download/$TAG"; fi
fi

case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64;;
  aarch64|arm64) ARCH=arm64;;
  *) die "不支持的架构 $(uname -m)（只发布 linux/amd64 与 linux/arm64）";;
esac

TMP=$(mktemp -d /tmp/hnp-online.XXXXXX) || die "无法创建临时目录"
trap 'rm -rf "$TMP"' EXIT

# fetch <url> <out> [accept]      输出：成功 0 / 失败非 0
fetch() {
  local url="$1" out="$2" accept="${3:-}"
  if command -v curl >/dev/null 2>&1; then
    if [ -n "$TOKEN" ]; then
      if [ -n "$accept" ]; then curl -fsSL -H "Authorization: Bearer $TOKEN" -H "Accept: $accept" -o "$out" "$url"
      else curl -fsSL -H "Authorization: Bearer $TOKEN" -o "$out" "$url"; fi
    else
      if [ -n "$accept" ]; then curl -fsSL -H "Accept: $accept" -o "$out" "$url"; else curl -fsSL -o "$out" "$url"; fi
    fi
  elif command -v wget >/dev/null 2>&1; then
    local h=()
    [ -n "$TOKEN" ] && h+=(--header="Authorization: Bearer $TOKEN")
    [ -n "$accept" ] && h+=(--header="Accept: $accept")
    wget -q "${h[@]}" -O "$out" "$url"
  elif command -v python3 >/dev/null 2>&1; then
    python3 - "$url" "$out" "$TOKEN" "$accept" <<'PY'
import sys, urllib.request
url, out, token, accept = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
req = urllib.request.Request(url)
if token: req.add_header('Authorization', 'Bearer ' + token)
if accept: req.add_header('Accept', accept)
with urllib.request.urlopen(req, timeout=90) as r, open(out, 'wb') as f:
    f.write(r.read())
PY
  else
    return 127
  fi
}

# 私有仓库：通过 API 解析 asset id，再以 application/octet-stream 取流
asset_url() {
  local name="$1" rel json id
  if [ "$TAG" = latest ]; then rel="https://api.github.com/repos/$REPO_SLUG/releases/latest"
  else rel="https://api.github.com/repos/$REPO_SLUG/releases/tags/$TAG"; fi
  json=$(fetch "$rel" "$TMP/rel.json" "application/vnd.github+json" && cat "$TMP/rel.json") || return 1
  if command -v python3 >/dev/null 2>&1; then
    id=$(printf '%s' "$json" | python3 -c 'import json,sys;d=json.load(sys.stdin);print(next((a["id"] for a in d.get("assets",[]) if a["name"]==sys.argv[1]),""))' "$name" 2>/dev/null)
  elif command -v jsonfilter >/dev/null 2>&1; then
    id=$(printf '%s' "$json" | jsonfilter -e "@.assets[@.name=\"$name\"].id" 2>/dev/null | head -1)
  else
    id=$(printf '%s' "$json" | tr '{' '\n' | grep -F "\"name\":\"$name\"" | grep -o '"id":[0-9]*' | head -1 | cut -d: -f2)
  fi
  [ -n "$id" ] || return 1
  printf 'https://api.github.com/repos/%s/releases/assets/%s' "$REPO_SLUG" "$id"
}

download() {
  local name="$1" out="$2" url
  if [ -n "$TOKEN" ] && [ -z "${HNP_BASE_URL:-}" ]; then
    if url=$(asset_url "$name"); then
      fetch "$url" "$out" "application/octet-stream" && return 0
    fi
    info "API 取 $name 失败，回退网页地址"
  fi
  fetch "$BASE_URL/$name" "$out"
}

step "1/4 环境"
info "平台: $(uname -s) $(uname -m) → linux/$ARCH"
info "来源: $BASE_URL${TAG:+  (tag=$TAG)}"
if [ -n "$TOKEN" ]; then info "使用 GitHub token（走 API 下载，适用于私有仓库）"; fi
if [ -z "$TOKEN" ] && [ -z "${HNP_BASE_URL:-}" ]; then info "提示：仓库若为私有，匿名下载会 404 —— 加 GH_TOKEN=<只读 token> 再跑"; fi

step "2/4 下载"
download "online-install.sh" "$TMP/online-install.sh" || die "下载 online-install.sh 失败（网络 / token / 是否为私有仓库）"
ok "online-install.sh"
download "install.sh" "$TMP/install.sh" || die "下载 install.sh 失败"
ok "install.sh"
download "homenet-panel-linux-$ARCH" "$TMP/homenet-panel" || die "下载二进制失败（该架构可能没有发布产物）"
ok "homenet-panel-linux-$ARCH（$(wc -c < "$TMP/homenet-panel") 字节）"

if download "sha256sums.txt" "$TMP/sha256sums.txt" 2>/dev/null && command -v sha256sum >/dev/null 2>&1; then
  WANT=$(grep "homenet-panel-linux-$ARCH" "$TMP/sha256sums.txt" | awk '{print $1}' | head -1)
  if [ -n "$WANT" ]; then
    GOT=$(sha256sum "$TMP/homenet-panel" | awk '{print $1}')
    if [ "$WANT" = "$GOT" ]; then ok "校验和一致"; else die "校验和不匹配（下载损坏或被篡改）"; fi
  fi
fi

step "3/4 安装"
chmod 755 "$TMP/homenet-panel" "$TMP/install.sh"
if [ "${#PASSTHRU[@]}" -gt 0 ]; then bash "$TMP/install.sh" --binary "$TMP/homenet-panel" "${PASSTHRU[@]}";
else bash "$TMP/install.sh" --binary "$TMP/homenet-panel"; fi
RC=$?

step "4/4 完成"
[ "$RC" = 0 ] || die "安装脚本返回 $RC"
ok "在线安装完成"
