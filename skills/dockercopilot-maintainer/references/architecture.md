# 架构与数据流

## 1. 启动链路

`main()`：

1. `SetupLog("./logs")`：`logx.MustSetup` + 控制台 writer；当前使用 `Encoding: "plain"`，日志行只保留时间、级别、内容。
2. `conf.Load` 读取 `etc/dockerCopilot.yaml`，支持环境变量覆盖。
3. 校验 `Auth.AccessSecret`（非空、非纯数字、长度 > 8）。
4. `rest.MustNewServer` + CORS + 401 回调。
5. `svc.NewServiceContext(c)`：
   - 创建 Docker client（`DOCKER_HOST`，默认 `unix:///var/run/docker.sock`）；
   - 加载进度文件；
   - 初始化 Settings。
6. 注册路由；
7. 启动 cron：镜像更新检查、自动更新白名单；
8. `server.Start()`。

Docker 不可达时**不退出进程**，只记录错误，由定时任务稍后重试。

## 2. 包职责

```
internal/
├── config/       版本、构建时间
├── handler/      HTTP 参数解析、响应
├── logic/        业务编排
├── module/       认证、registry digest、镜像更新检查
├── svc/          ServiceContext、进度持久化、全局锁
├── types/        请求/响应结构
└── utiles/       Docker 原子操作、备份/恢复、更新、Compose、审计
```

### 2.1 `internal/utiles` 重点文件

| 文件 | 作用 |
|---|---|
| `getcontainerlist.go` | 容器列表 + 镜像信息 |
| `containerInspect.go` | 容器 inspect 封装 |
| `startcontainer.go` / `stopcontainer.go` / `restartcontainer.go` | 生命周期操作 |
| `renamecontainer.go` | 重命名 |
| `removeimage.go` / `image.go` | 镜像删除/列表 |
| `backupcontainer.go` | 导出容器 JSON 备份 |
| `backuppath.go` | 备份路径解析与文件名校验 |
| `backupslist.go` | 备份列表 |
| `backup2compose.go` | 导出 Compose |
| `backup_compose/config2yaml.go` | Docker inspect → compose-go 结构 |
| `restorecontainer.go` | 从 JSON 恢复，含 `force` 回滚 |
| `updatecontainer.go` | 更新容器，含回滚、健康检查、旧镜像清理 |
| `autoupdate.go` | 白名单自动更新 |
| `updateprogram.go` | 自更新下载、解压、SHA256 |
| `getRemoteVersion.go` | 远程 version 文件 |
| `audit.go` | JSONL 审计日志，10MB 轮转，0600 |
| `network.go` | 端点配置清洗 |

## 3. 容器更新流程

`UpdateContainer`：

1. `ContainerInspect` 旧容器，记录：
   - 旧镜像 ID；
   - 旧容器名；
   - 是否运行、是否 paused；
   - `Config` / `HostConfig` / `NetworkSettings`。
2. 拉取目标镜像，流式解码进度。
3. 如果原本运行：
   - paused 先 `ContainerUnpause`；
   - `ContainerStop`（SIGTERM，10s）。
4. 旧容器改名 `<name>-<timestamp>`。
5. `ContainerCreate` 新容器：
   - `Config.Image` 替换为目标镜像；
   - `Hostname` 清空；
   - 网络端点经 `sanitizeEndpointSettings`；
   - 名称使用旧名。
6. 如果原本运行：
   - `ContainerStart`；
   - 非 paused 时等待健康检查；
   - 原本 paused 时启动后 `ContainerPause`。
7. 删除旧容器（`DelOldContainer=false` 可保留）。
8. 清理旧镜像（旧容器已删除且无引用）。

失败回滚：

- 删除新容器（如果已创建）；
- 旧容器改回原名；
- 原本运行则启动；
- 原本 paused 则恢复 paused。

## 4. 容器恢复流程

`RestoreContainer`：

1. 解析文件名（只允许安全字符，防目录穿越）。
2. 读取 JSON → `[]ContainerBackupEntry`。
3. 串行化（`restoreMu`），避免并发恢复同名容器。
4. 逐条：
   - 校验 `Config.Image`；
   - 计算目标状态：
     - `startOverride != nil`：强制；
     - 否则 `stateKnown`：按 `wasRunning`；
     - 旧备份无状态：默认启动。
   - 同名容器存在：
     - 非 `force`：镜像一致则跳过/补启动，不一致报错；
     - `force`：进入替换流程。
5. `force` 替换流程：
   - 拉镜像；
   - 停止旧容器（paused 先 unpause）；
   - 改名旧容器；
   - 创建新容器；
   - 启动（按目标状态）；
   - 成功后删除旧容器。
6. 失败：`rollbackRestoredContainer` 清理新容器、恢复旧容器名与状态。
7. 汇总 `completed` / `failures` 到进度详情。

## 5. 备份格式

`ContainerBackupEntry` 大致结构：

```json
{
  "Name": "redis",
  "Config": { "...": "Docker container.Config" },
  "HostConfig": { "...": "Docker container.HostConfig" },
  "NetworkingConfig": {
    "EndpointsConfig": {
      "bridge": {
        "Aliases": ["redis"],
        "IPAMConfig": null
      }
    }
  },
  "stateKnown": true,
  "wasRunning": true,
  "wasPaused": false
}
```

兼容规则：

- 旧 JSON 没有 `StateKnown/WasRunning/WasPaused` 时按“未知”处理；
- 恢复默认启动；
- 新增字段必须可选，不能破坏旧文件反序列化。

## 6. 进度与 SSE

- `svc.TaskProgress`：`TaskID/Percentage/Message/Name/DetailMsg/IsDone`。
- 内存 map + 文件持久化（`PROGRESS_STORE`，默认 `/data/config/progress.json`）。
- 保存与读取在同一把锁内，临时文件 + rename 原子落盘。
- 已完成任务按 `PROGRESS_TTL_HOURS` 清理。
- SSE 端点：`GET /api/progress/:taskid/events`，15s 心跳。
- 取消：`POST /api/progress/:taskid/cancel`，通过 context cancel 传播。

## 7. 日志与审计

### 7.1 运行日志

`SetupLog`：

```go
logx.MustSetup(logx.LogConf{
    Path:     "./logs",
    Level:    "info",
    KeepDays: 7,
    Compress: true,
    Mode:     "file",
    Encoding: "plain",
})
logx.AddWriter(logx.NewWriter(os.Stdout))
```

- 文件：`logs/access.log`、`error.log` 等；
- 控制台同样输出；
- `plain` 格式：`时间 <TAB> 级别 <TAB> 内容`；
- 不再输出 `dockerCopilot:` 服务名前缀。

### 7.2 审计日志

`internal/utiles/audit.go`：

- 路径：`AUDIT_LOG`，默认 `/data/logs/audit.jsonl`；
- 每行一个 JSON：`time/action/target/detail`；
- 文件权限 `0600`；
- 超过 10MB 轮转为 `.1`。

## 8. 配置优先级

| 配置 | 优先级 |
|---|---|
| 备份目录 | `BACKUP_DIR` env > Settings API > `/data/backups` |
| 进度文件 | `PROGRESS_STORE` env > 默认 |
| 审计日志 | `AUDIT_LOG` env > 默认 |
| 更新仓库 | `UPDATE_REPO` env > 默认 |
| 更新分支 | `UPDATE_BRANCH` env > 默认 |
| 代理 | `githubProxy` env |
| registry TLS | `REGISTRY_INSECURE` env |

## 9. 镜像边界

Dockerfile 只 COPY：

- `dc-back/dist/$TARGETPLATFORM/dockerCopilot`
- `dc-back/etc/`
- `start.sh`

`skills/` 仅用于开发辅助，必须被 `.dockerignore` 排除，不进入镜像。
