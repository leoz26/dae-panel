#!/usr/bin/env bash
# 一条命令部署：在开发机上执行，自动交叉编译 → 上传 → 目标机安装 → 自检
# 用法：bash deploy/deploy.sh root@192.168.1.85 [-i 私钥] [--arch auto|amd64|arm64] [--port 8787] [-- <install.sh 参数>]
# 例：  bash deploy/deploy.sh root@192.168.1.85 -i ~/.ssh/id_ed25519 --port 8787 -- --user admin
set -uo pipefail

REPO=$(cd "$(dirname "$0")/.." && pwd)
TARGET=""; IDENT=""; ARCH=auto; REMOTE_DIR=/tmp/hnp-deploy; PASSTHRU=()
while [ $# -gt 0 ]; do
  case "$1" in
    -i|--identity) IDENT="$2"; shift 2;;
    --arch) ARCH="$2"; shift 2;;
    --remote-dir) REMOTE_DIR="$2"; shift 2;;
    --) shift; PASSTHRU=("$@"); break;;
    -*) echo "未知参数 $1"; exit 2;;
    *) TARGET="$1"; shift;;
  esac
done
[ -n "$TARGET" ] || { echo "用法: $0 <user@host> [-i 私钥] [--arch amd64|arm64] [-- <install 参数>]"; exit 2; }

SSH=(ssh -o StrictHostKeyChecking=accept-new -o ConnectTimeout=10)
SCP=(scp -o StrictHostKeyChecking=accept-new)
[ -n "$IDENT" ] && SSH+=(-i "$IDENT") && SCP+=(-i "$IDENT")

echo "[1/4] 探测目标架构"
M=$("${SSH[@]}" "$TARGET" 'uname -m') || { echo "SSH 连接失败"; exit 1; }
if [ "$ARCH" = auto ]; then
  case "$M" in x86_64|amd64) ARCH=amd64;; aarch64|arm64) ARCH=arm64;; *) echo "不支持的架构 $M"; exit 1;; esac
fi
echo "      目标 $TARGET ($M) → 构建 linux/$ARCH"

echo "[2/4] 交叉编译"
OUT="$REPO/bin/homenet-panel-linux-$ARCH"
DEVLOCAL="${HOME:-$(cd ~ && pwd)}"
GOBIN="$(command -v go || echo /opt/homebrew/bin/go)"
( cd "$REPO/homenet-panel" && env HOME="$DEVLOCAL" GOCACHE="${GOCACHE:-$DEVLOCAL/Library/Caches/go-build}" \
    GOOS=linux GOARCH="$ARCH" CGO_ENABLED=0 "$GOBIN" build -trimpath -ldflags "-s -w" -o "$OUT" ./cmd/homenet-panel ) || { echo "编译失败"; exit 1; }
ls -l "$OUT"

echo "[3/4] 上传"
"${SSH[@]}" "$TARGET" "mkdir -p $REMOTE_DIR" || exit 1
"${SCP[@]}" "$OUT" "$TARGET:$REMOTE_DIR/homenet-panel" >/dev/null || exit 1
"${SCP[@]}" "$REPO/deploy/install.sh" "$TARGET:$REMOTE_DIR/install.sh" >/dev/null || exit 1

echo "[4/4] 远程安装"
CMD="bash $REMOTE_DIR/install.sh --binary $REMOTE_DIR/homenet-panel"
if [ "${#PASSTHRU[@]}" -gt 0 ]; then for a in "${PASSTHRU[@]}"; do CMD="$CMD $a"; done; fi
"${SSH[@]}" -t "$TARGET" "$CMD"
RC=$?
"${SSH[@]}" "$TARGET" "rm -rf $REMOTE_DIR" >/dev/null 2>&1
exit $RC
