#!/usr/bin/env bash
# HomeNet Panel 在线安装（目标设备上以 root 运行）
#
# 推荐（CDN，不依赖 GitHub 直连）：
#   curl -fsSL https://cdn.jsdelivr.net/gh/leoz26/dae-panel@dist/online-install.sh | sudo bash
# GitHub 直连（网络能到 github.com 时）：
#   curl -fsSL https://github.com/leoz26/dae-panel/releases/latest/download/online-install.sh | sudo bash
# 自建镜像： bash online-install.sh --base-url http://mirror/hnp --tag v0.38.1
# 钉版本：   bash online-install.sh --tag v0.38.1
# 私有仓库： GH_TOKEN=<只读 token> bash online-install.sh
set -uo pipefail

REPO_SLUG=${HNP_REPO:-leoz26/dae-panel}
TAG=${HNP_TAG:-}
BASE_URL=${HNP_BASE_URL:-}
CDN=${HNP_CDN:-https://cdn.jsdelivr.net/gh}
TOKEN=${GH_TOKEN:-${GITHUB_TOKEN:-}}
PASSTHRU=()

while [ $# -gt 0 ]; do
  case "$1" in
    --base-url) BASE_URL="$2"; shift 2;;
    --tag) TAG="$2"; shift 2;;
    --repo) REPO_SLUG="$2"; shift 2;;
    --cdn) CDN="$2"; shift 2;;
    -h|--help) sed -n '2,10p' "$0"; exit 0;;
    *) PASSTHRU+=("$1"); shift;;
  esac
done

ok()   { printf '  \033[32m[+]\033[0m %s\n' "$1"; }
info() { printf '  \033[36m[=]\033[0m %s\n' "$1"; }
warn() { printf '  \033[33m[!]\033[0m %s\n' "$1"; }
die()  { printf '  \033[31m[x]\033[0m %s\n' "$1"; exit 1; }
step() { printf '\n\033[1m%s\033[0m\n' "$1"; }

[ "$(id -u)" = 0 ] || die "请用 root 运行"

case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64;;
  aarch64|arm64) ARCH=arm64;;
  *) die "不支持的架构 $(uname -m)（只发布 linux/amd64 与 linux/arm64）";;
esac

TMP=$(mktemp -d /tmp/hnp-online.XXXXXX) || die "无法创建临时目录"
trap 'rm -rf "$TMP"' EXIT

# get <url> <out>：带重试与 HTTP/1.1，三级回退 curl → wget → python3
get() {
  local url="$1" out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --http1.1 --retry 4 --retry-all-errors --connect-timeout 15 --max-time 600 -o "$out" "$url"
  elif command -v wget >/dev/null 2>&1; then
    wget -q --tries=4 --timeout=20 -O "$out" "$url"
  else
    python3 - "$url" "$out" <<'PY'
import sys, urllib.request
url, out = sys.argv[1], sys.argv[2]
req = urllib.request.Request(url, headers={'User-Agent': 'homenet-panel-installer'})
with urllib.request.urlopen(req, timeout=60) as r, open(out, 'wb') as f:
    f.write(r.read())
PY
  fi
}

# api_asset <name> <out>：私有仓库走 API（Accept: application/octet-stream）
api_asset() {
  local name="$1" out="$2" id url
  local rel="https://api.github.com/repos/$REPO_SLUG/releases/"
  if [ -n "$TAG" ]; then rel="${rel}tags/$TAG"; else rel="${rel}latest"; fi
  id=$(curl -fsSL --http1.1 --retry 3 -H "Authorization: Bearer $TOKEN" -H 'Accept: application/vnd.github+json' "$rel" \
        | tr '{' '\n' | grep -F "\"name\":\"$name\"" | grep -o '"id":[0-9]*' | head -1 | cut -d: -f2)
  [ -n "$id" ] || return 1
  url="https://api.github.com/repos/$REPO_SLUG/releases/assets/$id"
  get_auth "$url" "$out" 'application/octet-stream'
}

get_auth() {
  local url="$1" out="$2" accept="${3:-}"
  if command -v curl >/dev/null 2>&1; then
    if [ -n "$accept" ]; then
      curl -fsSL --http1.1 --retry 4 -H "Authorization: Bearer $TOKEN" -H "Accept: $accept" -o "$out" "$url"
    else
      curl -fsSL --http1.1 --retry 4 -H "Authorization: Bearer $TOKEN" -o "$out" "$url"
    fi
  else
    return 1
  fi
}

step "1/4 环境"
info "平台：$(uname -s) $(uname -m) → linux/$ARCH"

# --- 选源 ---
SOURCES=()
if [ -n "$BASE_URL" ]; then
  SOURCES+=("$BASE_URL")
  info "来源：自定义镜像 $BASE_URL"
else
  # CDN 需要具体版本号：先读 dist 分支里的 VERSION
  if [ -z "$TAG" ]; then
    if get "$CDN/$REPO_SLUG@dist/VERSION" "$TMP/VERSION" 2>/dev/null; then
      TAG=$(tr -d ' \t\r\n' < "$TMP/VERSION")
    else
      TAG=$(curl -fsSL --http1.1 --retry 3 -H 'Accept: application/vnd.github+json' \
            "https://api.github.com/repos/$REPO_SLUG/releases/latest" 2>/dev/null \
            | grep -o '"tag_name"[^,]*' | head -1 | sed 's/.*"\(.*\)"/\1/')
    fi
    [ -n "$TAG" ] || TAG=latest
  fi
  info "版本：$TAG"
  if [ "$TAG" = latest ]; then
    SOURCES+=("https://github.com/$REPO_SLUG/releases/latest/download")
  else
    # 首选 CDN 上的 <tag>-dist **标签**（不可变、缓存友好）；
    # 分支 dist 只作为兜底（jsDelivr 对分支有分钟级缓存滞后）。
    SOURCES+=("$CDN/$REPO_SLUG@$TAG-dist")
    SOURCES+=("$CDN/$REPO_SLUG@dist")
    SOURCES+=("https://fastly.jsdelivr.net/gh/$REPO_SLUG@$TAG-dist")
    SOURCES+=("https://gcore.jsdelivr.net/gh/$REPO_SLUG@$TAG-dist")
    SOURCES+=("https://raw.githubusercontent.com/$REPO_SLUG/$TAG-dist")
    SOURCES+=("https://raw.githubusercontent.com/$REPO_SLUG/dist")
    SOURCES+=("https://github.com/$REPO_SLUG/releases/download/$TAG")
  fi
  info "来源：$(printf '%s ' "${SOURCES[@]}")"
  if [ -z "$TOKEN" ]; then
    info "提示：仓库若为私有，匿名下载会失败 —— 加 GH_TOKEN=<只读 token> 再跑"
  fi
fi

# download <name> <out>：按源顺序逐个尝试
download() {
  local name="$1" out="$2" src
  for src in "${SOURCES[@]}"; do
    if get "$src/$name" "$out" 2>/dev/null && [ -s "$out" ]; then
      return 0
    fi
    rm -f "$out"
  done
  if [ -n "$TOKEN" ]; then
    api_asset "$name" "$out" && [ -s "$out" ] && return 0
  fi
  return 1
}

step "2/4 下载"
download "online-install.sh" "$TMP/online-install.sh" || warn "未能下载 online-install.sh（不影响安装，仅用于版本自检）"
download "install.sh" "$TMP/install.sh" || die "下载 install.sh 失败（所有源都不可达：检查网络、代理、或 --base-url 镜像）"
ok "install.sh"
download "homenet-panel-linux-$ARCH" "$TMP/homenet-panel" || die "下载二进制失败（该架构可能没有发布产物，或所有源都不可达）"
ok "homenet-panel-linux-$ARCH（$(wc -c < "$TMP/homenet-panel") 字节）"

if download "sha256sums.txt" "$TMP/sha256sums.txt"; then
  if command -v sha256sum >/dev/null 2>&1; then
    WANT=$(grep "homenet-panel-linux-$ARCH" "$TMP/sha256sums.txt" | awk '{print $1}' | head -1)
    if [ -n "$WANT" ]; then
      GOT=$(sha256sum "$TMP/homenet-panel" | awk '{print $1}')
      if [ "$WANT" = "$GOT" ]; then ok "校验和一致"; else die "校验和不匹配（下载损坏或被篡改）"; fi
    fi
  else
    warn "本机没有 sha256sum，跳过校验"
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
