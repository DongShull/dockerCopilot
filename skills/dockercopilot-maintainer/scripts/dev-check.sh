#!/usr/bin/env bash
# 开发检查：gofmt + build + vet + test，可选 race。
# 用法：
#   scripts/dev-check.sh            # 常规
#   scripts/dev-check.sh --race     # 追加 -race
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

RACE=0
if [ "${1:-}" = "--race" ] || [ "${RACE:-0}" = "1" ]; then
  RACE=1
fi

echo "== gofmt =="
UNFORMATTED="$(gofmt -l $(git ls-files '*.go' 2>/dev/null || find . -name '*.go' -not -path './.git/*'))"
if [ -n "$UNFORMATTED" ]; then
  echo "以下文件未格式化："
  echo "$UNFORMATTED"
  echo "执行: gofmt -w <file>"
  exit 1
fi
echo "通过"
echo

echo "== go build =="
go build ./...
echo "通过"
echo

echo "== go vet =="
go vet ./...
echo "通过"
echo

echo "== go test =="
go test ./... -count=1
echo "通过"
echo

if [ "$RACE" = "1" ]; then
  echo "== go test -race =="
  go test -race ./... -count=1
  echo "通过"
  echo
fi

echo "== git diff --check =="
git diff --check
echo "通过"
echo

echo "全部检查通过"
