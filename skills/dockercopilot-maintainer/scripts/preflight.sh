#!/usr/bin/env bash
# dockerCopilot 预检：只读，不修改仓库。
set -u

# 常见 Go 安装路径兜底（NAS/非交互 shell 常缺 PATH）
for d in /usr/local/go/bin /tmp/go/bin "$HOME/go/bin"; do
  [ -x "$d/go" ] && PATH="$d:$PATH"
done
export PATH

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT" || exit 1

echo "== 仓库 =="
echo "路径: $ROOT"
echo "分支: $(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo '非 git 仓库')"
echo "提交: $(git rev-parse --short HEAD 2>/dev/null || echo '无')"
echo

echo "== 工作区 =="
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  if [ -n "$(git status --porcelain)" ]; then
    echo "有未提交改动："
    git status --short
  else
    echo "干净"
  fi
else
  echo "不是 git 仓库"
fi
echo

echo "== 工具 =="
for c in go git docker node npm; do
  if command -v "$c" >/dev/null 2>&1; then
    case "$c" in
      go)     v="$("$c" version 2>/dev/null | head -n 1)" ;;
      git)    v="$("$c" --version 2>/dev/null | head -n 1)" ;;
      docker) v="$("$c" --version 2>/dev/null | head -n 1)" ;;
      node)   v="$("$c" --version 2>/dev/null | head -n 1)" ;;
      npm)    v="$("$c" --version 2>/dev/null | head -n 1)" ;;
      *)      v="未知" ;;
    esac
    printf '%-7s %s\n' "$c" "$v"
  else
    printf '%-7s 未安装\n' "$c"
  fi
done
echo

echo "== 版本文件 =="
for f in version fn_version ugreen_version; do
  if [ -f "$f" ]; then
    printf '%-15s %s\n' "$f" "$(tr -d '\r\n' < "$f")"
  else
    printf '%-15s 缺失\n' "$f"
  fi
done
echo

echo "== MCP =="
if [ -d internal/mcp ]; then
  TOOL_COUNT=$(grep -rho 'NewTool("dc_[a-z_]*"' internal/mcp/*.go | sort -u | wc -l | tr -d ' ')
  echo "internal/mcp 工具数: ${TOOL_COUNT}（期望 26）"
  if [ "$TOOL_COUNT" -eq 0 ]; then
    echo "警告: 未发现 dc_* 工具注册"
  fi
  if grep -q "MCP_TOKEN" internal/mcp/server.go 2>/dev/null; then
    echo "MCP 鉴权: 已实现（MCP_TOKEN > secretKey）"
  else
    echo "警告: 未找到 MCP 鉴权逻辑"
  fi
else
  echo "警告: internal/mcp 不存在，/mcp 未接入"
fi
echo

echo "== Docker =="
if command -v docker >/dev/null 2>&1; then
  if docker version >/dev/null 2>&1; then
    echo "Docker 可用"
    docker version --format 'Server: {{.Server.Version}}' 2>/dev/null || true
  else
    echo "Docker 客户端存在，但无法连接守护进程"
    echo "DOCKER_HOST=${DOCKER_HOST:-未设置}"
  fi
else
  echo "未安装 docker"
fi
echo

echo "== 构建边界 =="
if [ -f .dockerignore ]; then
  if grep -qx 'skills' .dockerignore 2>/dev/null || grep -qx 'skills/' .dockerignore 2>/dev/null; then
    echo ".dockerignore 已排除 skills"
  else
    echo "警告: .dockerignore 未排除 skills"
  fi
else
  echo "警告: 没有 .dockerignore（Dockerfile 虽为白名单 COPY，仍建议补上）"
fi

if grep -qE '^[[:space:]]*(COPY|ADD)[[:space:]]+\. ' docker/Dockerfile 2>/dev/null; then
  echo "警告: Dockerfile 出现 COPY . / ADD .，可能把 skills 打进镜像"
else
  echo "Dockerfile 未使用 COPY . / ADD ."
fi
