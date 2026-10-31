# API 速查

> MCP（`/mcp`）与 HTTP API 并存。Agent 优先用 MCP：见 `references/mcp.md` 与 `docs/MCP.md`。

所有受保护接口需要 JWT：

```http
Authorization: Bearer <token>
```

登录（form）：

```http
POST /api/auth
Content-Type: application/x-www-form-urlencoded

secretKey=...
```

## 容器

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/containers` | 容器列表 |
| POST | `/api/container/:id/start` | 启动 |
| POST | `/api/container/:id/stop` | 停止 |
| POST | `/api/container/:id/restart` | 重启 |
| POST | `/api/container/:id/rename` | 重命名 |
| POST | `/api/container/:id/update` | 更新 |

更新（form）：

```http
POST /api/container/:id/update
Content-Type: application/x-www-form-urlencoded

imageNameAndTag=redis:7&containerName=redis
```

- `imageNameAndTag`：目标镜像，可带 tag；
- `containerName`：容器名，用于日志/回滚命名；
- 是否删除旧容器由环境变量 `DelOldContainer` 控制，不在请求体里。

## 备份 / 恢复

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/container/backup` | 备份全部容器为 JSON |
| GET | `/api/container/backup2compose` | 导出 Compose |
| GET | `/api/container/listBackups` | 备份文件列表 |
| DELETE | `/api/container/backups` | 删除备份 |
| POST | `/api/container/backups/restore` | 恢复 |

恢复请求：

```json
{
  "filename": "backup-2026-10-30.json",
  "start": true,
  "force": false
}
```

- `start` 省略：按备份中的容器状态恢复；
- `force=false`：同名容器存在时跳过/补启动，镜像不一致报错；
- `force=true`：先停止并临时改名旧容器，新建成功后才删除旧容器；失败自动回滚。

## 镜像

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/images` | 镜像列表 |
| DELETE | `/api/image/:id` | 删除镜像 |
| DELETE | `/api/images?filter=untagged\|unused` | 批量清理：无 TAG 镜像 / 未使用镜像，返回 removed/failed 明细 |

## 进度

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/progress` | 任务列表 |
| GET | `/api/progress/:taskid` | 单任务详情 |
| POST | `/api/progress/:taskid/cancel` | 取消任务 |
| GET | `/api/progress/:taskid/events` | SSE 事件流 |

SSE 事件示例：

```text
event: progress
data: {"taskid":"...","percentage":40,"message":"正在创建新容器","isDone":false}

event: ping
data: {}
```

## 设置

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/settings` | 读取设置 |
| PUT | `/api/settings` | 更新设置 |

设置项主要包含备份目录等。`BACKUP_DIR` 环境变量优先级最高。

## 图标

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/icons` | 读取自定义图标配置 |
| POST | `/api/icons` | 上传图标 |

## 版本 / 自更新

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/version` | 当前版本 + 远程版本 |
| PUT | `/api/program` | 触发自更新 |

自更新依赖：

- `UPDATE_REPO`
- `UPDATE_BRANCH`
- Release 资产 `dockerCopilot-<arch>.tar.gz` 与 `.sha256`
- 可选 `githubProxy`
- `UPDATE_SKIP_CHECKSUM=true` 仅用于旧版本过渡

## 错误格式

业务错误通常返回：

```json
{
  "code": 50000,
  "msg": "错误信息"
}
```

401 返回：

```json
{
  "code": 401,
  "msg": "未授权",
  "data": {}
}
```
