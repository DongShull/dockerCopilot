#!/usr/bin/env bash
# 本地交叉编译 Linux 二进制。
# 用法：
#   scripts/build-linux.sh [amd64|arm64] [version-file]
# 默认：amd64 + version
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

ARCH="${1:-amd64}"
VERSION_FILE="${2:-version}"

case "$ARCH" in
  amd64|arm64) ;;
  *) echo "不支持的架构: $ARCH（只支持 amd64 / arm64）" >&2; exit 1 ;;
esac

if [ ! -f "$VERSION_FILE" ]; then
  echo "版本文件不存在: $VERSION_FILE" >&2
  exit 1
fi

VERSION="$(tr -d '\r\n' < "$VERSION_FILE")"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
OUT_DIR="dist/linux/$ARCH"
OUT="$OUT_DIR/dockerCopilot-new"

mkdir -p "$OUT_DIR"

echo "编译: $OUT"
echo "版本: $VERSION"
echo "日期: $BUILD_DATE"

CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" \
  go build -a --trimpath \
  -ldflags="-w -s -X 'github.com/onlyLTY/dockerCopilot/internal/config.Version=$VERSION' -X 'github.com/onlyLTY/dockerCopilot/internal/config.BuildDate=$BUILD_DATE'" \
  -o "$OUT" .

ls -lh "$OUT"
