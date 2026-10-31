# MCP 接入说明

dockerCopilot 自 v2.2.0 起内置 MCP（Model Context Protocol）服务，Agent 可以直接调用
容器管理、镜像管理、备份恢复、进度查询、设置与自更新等能力，无需再经过网页前端。

HTTP API（`/api/*`）与 MCP（`/mcp`）**并存**，脚本和旧客户端不受影响。

## 1. 传输与地址

| 项 | 值 |
|---|---|
| 协议 | Streamable HTTP（MCP 官方传输，兼容 SSE） |
| 地址 | `http://<host>:12712/mcp` |
| 方法 | `POST`（JSON-RPC 消息）、`GET`（SSE 流）、`DELETE`（结束会话） |
| 版本 | `serverInfo.version` 与程序版本一致 |

## 2. 鉴权

`/mcp` 默认要求 token，取值优先级：

1. 环境变量 `MCP_TOKEN`（若设置）
2. 否则回退到 `secretKey`（与 `/api` 的 JWT 密钥同源）

三种携带方式都支持，便于不同客户端：

```bash
# 1) Bearer
curl -H "Authorization: Bearer <token>" ...

# 2) 自定义头（Claude Code / Cursor 等常用的 headers 配置）
curl -H "X-MCP-Token: <token>" ...

# 3) URL 参数（仅支持 URL 配置的客户端）
curl "http://127.0.0.1:12712/mcp?token=<token>"
```

显式关闭鉴权（**仅限完全隔离的内网**）：

```yaml
environment:
  - MCP_TOKEN=off
```

关闭时启动日志会打印一条 error 级别告警。

## 3. 客户端配置示例

### Claude Code / Cursor / Cline（Streamable HTTP）

```json
{
  "mcpServers": {
    "dockercopilot": {
      "type": "streamable-http",
      "url": "http://127.0.0.1:12712/mcp",
      "headers": {
        "Authorization": "Bearer <secretKey>"
      }
    }
  }
}
```

### 只支持 URL 的客户端

```json
{
  "mcpServers": {
    "dockercopilot": {
      "url": "http://127.0.0.1:12712/mcp?token=<secretKey>"
    }
  }
}
```

## 4. 工具清单

共 27 个工具，命名统一以 `dc_` 开头。

### 通用

| 工具 | 说明 |
|---|---|
| `dc_ping` | 健康检查 |
| `dc_info` | 版本、监听地址、Docker daemon 真实连通性、备份目录、MCP 鉴权状态 |

### 容器

| 工具 | 参数 | 说明 |
|---|---|---|
| `dc_list_containers` | - | 列出全部容器（含已停止），带 `haveUpdate` / `isCompose` 标记 |
| `dc_inspect_container` | `containerId` | 单容器完整 inspect |
| `dc_start_container` | `containerId` | 启动 |
| `dc_stop_container` | `containerId` | 停止（SIGTERM，10s 宽限） |
| `dc_restart_container` | `containerId` | 重启 |
| `dc_rename_container` | `containerId`, `newName` | 重命名 |
| `dc_update_container` | `containerId`, `containerName`, `image?`, `delOldContainer?` | 一键更新，全链路回滚；`image` 不传或留空时使用容器当前镜像 |
| `dc_autoupdate_run` | - | 手动触发一轮自动更新；**前置条件**：`AUTOUPDATE_CRON` 与 `AUTOUPDATE_WHITELIST` 均已配置，否则报错 |

### 镜像

| 工具 | 参数 | 说明 |
|---|---|---|
| `dc_list_images` | - | 本地镜像列表 |
| `dc_check_image_update` | - | 立即做一次远端 digest 检查并返回结果 |
| `dc_remove_image` | `imageId`, `force?` | 删除镜像 |
| `dc_clean_images` | `filter` | 批量清理：`untagged` 删除无 TAG 镜像，`unused` 删除未使用镜像；返回 `removed` / `failed` 明细 |

### 备份 / 恢复

| 工具 | 参数 | 说明 |
|---|---|---|
| `dc_backup_containers` | - | 备份为 JSON |
| `dc_backup_compose` | - | 导出 docker-compose YAML |
| `dc_list_backups` | - | 备份文件列表 |
| `dc_restore_container` | `filename`, `start?`, `force?` | 从 JSON 备份恢复；`force` 走“改名旧容器→新建→成功后删旧容器”，失败回滚 |
| `dc_delete_backup` | `filename` | 删除备份文件（不动容器） |

### 进度

| 工具 | 参数 | 说明 |
|---|---|---|
| `dc_list_progress` | - | 全部任务（含重启恢复的历史任务） |
| `dc_get_progress` | `taskId` | 单任务进度 |
| `dc_cancel_progress` | `taskId` | 取消任务（步骤边界生效） |

### 设置 / 版本

| 工具 | 参数 | 说明 |
|---|---|---|
| `dc_get_settings` | - | 当前与生效的备份目录 |
| `dc_update_settings` | `backupDir` | 修改备份目录（绝对路径 + 可写校验） |
| `dc_get_version` | - | 本地版本与构建时间 |
| `dc_check_program_update` | - | 检查程序新版本；FnOS 版本返回 `fnos=true` 与提示（不走通用更新链） |
| `dc_update_program` | - | 自更新并重启（会中断当前服务）；FnOS 版本会明确拒绝 |

## 5. 与 HTTP API 的对应关系

MCP 工具直接复用 `internal/logic` 与 `internal/utiles` 的实现，与 HTTP API 共享同一套
锁、进度、审计与回滚逻辑，因此两条通道行为一致。

- 更新容器：`dc_update_container` ↔ `POST /api/container/:id/update`
- 恢复容器：`dc_restore_container` ↔ `POST /api/container/backups/restore`
- 进度：`dc_list_progress` ↔ `GET /api/progress`

## 6. 自检

```bash
# 未带 token：应返回 401
curl -s -o /dev/null -w '%{http_code}\n' -X POST http://127.0.0.1:12712/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}'

# 带 token：应返回 serverInfo 与 Mcp-Session-Id
curl -s -D - -X POST http://127.0.0.1:12712/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'Authorization: Bearer <secretKey>' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}'
```

## 7. 安全建议

- `/mcp` 等同 root 级 Docker 控制面：能启停、删除容器与镜像。**不要直接暴露公网**。
- 建议只监听内网/回环：`ports: ["127.0.0.1:12712:12712"]`。
- 需要给不同 Agent 分配不同凭据时，用 `MCP_TOKEN` 单独设置，避免泄露 `secretKey`。
- 不需要 MCP 时可不配置任何客户端；端点存在但没有有效 token 无法调用。
