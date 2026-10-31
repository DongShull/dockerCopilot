#!/usr/bin/env bash
# 镜像边界审计：确保 skills/ 不会进入 Docker 镜像。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

FAIL=0
warn() { echo "警告: $*"; }
fail() { echo "失败: $*"; FAIL=1; }

echo "== 1. Dockerfile COPY/ADD 白名单 =="
DOCKERFILE="docker/Dockerfile"
if [ ! -f "$DOCKERFILE" ]; then
  fail "找不到 $DOCKERFILE"
else
  # 禁止 COPY . / ADD .
  if grep -nE '^[[:space:]]*(COPY|ADD)[[:space:]]+\.([[:space:]]|$)' "$DOCKERFILE"; then
    fail "Dockerfile 使用了 COPY . / ADD .，会把 skills 等开发资料带进镜像"
  else
    echo "未发现 COPY . / ADD ."
  fi

  # 逐条检查 COPY/ADD：只允许 dc-back 与 start.sh
  while IFS= read -r line; do
    case "$line" in
      *dc-back*|*start.sh*)
        ;;
      *)
        fail "Dockerfile 出现非白名单 COPY/ADD: $line"
        ;;
    esac
  done < <(grep -E '^[[:space:]]*(COPY|ADD)[[:space:]]' "$DOCKERFILE" || true)

  if grep -q 'skills' "$DOCKERFILE"; then
    fail "Dockerfile 显式引用了 skills"
  fi
fi
echo

echo "== 2. .dockerignore =="
if [ ! -f .dockerignore ]; then
  fail "缺少 .dockerignore"
else
  if grep -qxE 'skills/?|/skills/?' .dockerignore; then
    echo ".dockerignore 已排除 skills"
  else
    fail ".dockerignore 未排除 skills"
  fi
fi
echo

echo "== 3. 发布包内容（如果已生成） =="
FOUND=0
for f in dockerCopilot-*.tar.gz; do
  [ -e "$f" ] || continue
  FOUND=1
  echo "检查 $f"
  if tar -tzf "$f" | grep -qE '(^|/)skills/'; then
    fail "$f 包含 skills/"
  else
    echo "  未包含 skills/"
  fi
done
if [ "$FOUND" = "0" ]; then
  echo "没有本地发布包，跳过"
fi
echo

echo "== 4. 源码泄漏检查（Dockerfile） =="
if grep -rqE 'go:embed[[:space:]]+front' --include='*.go' . 2>/dev/null; then
  fail "仍有 go:embed front/*；前端已移除，不应再嵌入"
else
  echo "未发现前端 go:embed"
fi
if grep -qE '^[[:space:]]*(COPY|ADD)[[:space:]]+(\./)?(internal|docs|static|assets)' "$DOCKERFILE" 2>/dev/null; then
  fail "Dockerfile 直接 COPY 源码/文档目录"
else
  echo "未直接 COPY 源码/文档目录"
fi
echo

if [ "$FAIL" != "0" ]; then
  echo "审计未通过"
  exit 1
fi

echo "审计通过：skills/ 不会进入镜像"
