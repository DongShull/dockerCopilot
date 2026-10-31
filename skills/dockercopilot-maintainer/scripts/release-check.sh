#!/usr/bin/env bash
# 发布前检查：版本、测试、构建、镜像边界。
# 用法：
#   scripts/release-check.sh [version]
# 不传 version 时读取 ./version。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"
HERE="$(cd "$(dirname "$0")" && pwd)"

TARGET="${1:-$(tr -d '\r\n' < version 2>/dev/null || true)}"

echo "== 目标版本 =="
if [ -z "$TARGET" ]; then
  echo "版本为空" >&2
  exit 1
fi
echo "$TARGET"
echo

echo "== 1. 工作区 =="
if [ -n "$(git status --porcelain 2>/dev/null || true)" ]; then
  echo "警告: 工作区有未提交改动"
  git status --short
else
  echo "干净"
fi
echo

echo "== 2. 版本文件一致性 =="
for f in version fn_version ugreen_version; do
  if [ -f "$f" ]; then
    v="$(tr -d '\r\n' < "$f")"
    printf '%-15s %s\n' "$f" "$v"
  fi
done
echo "目标: $TARGET"
echo

echo "== 3. 开发检查 =="
"$HERE/dev-check.sh"
echo

echo "== 4. 镜像边界 =="
"$HERE/package-audit.sh"
echo

echo "== 5. MCP =="
if go test ./internal/mcp/ -count=1 >/dev/null 2>&1; then
  COUNT=$(grep -rho 'NewTool("dc_[a-z_]*"' internal/mcp/*.go | sort -u | wc -l | tr -d ' ')
  echo "MCP 测试通过，注册工具数: ${COUNT}"
else
  echo "失败: internal/mcp 测试未通过" >&2
  exit 1
fi
if grep -q "MCP_TOKEN" docs/MCP.md 2>/dev/null; then
  echo "docs/MCP.md 已说明鉴权"
else
  echo "警告: docs/MCP.md 缺少 MCP_TOKEN 说明"
fi
echo

echo "== 6. 敏感文件 =="
for p in .env .env.local etc/secret.yaml; do
  if [ -e "$p" ] && git ls-files --error-unmatch "$p" >/dev/null 2>&1; then
    echo "失败: $p 已被 git 跟踪"
    exit 1
  fi
done
echo "未发现被跟踪的 .env/secret 文件"
echo

echo "发布前检查通过"
