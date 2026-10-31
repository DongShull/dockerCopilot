# 排障手册

## 1. 启动

### 启动即退出

日志关键字：

```text
secretKey 配置不合法
Docker 客户端初始化失败
```

检查：

```bash
grep -n "secretKey" etc/dockerCopilot.yaml
ls -l /var/run/docker.sock
docker version
```

要求：

- `secretKey` 非空；
- 长度 > 8；
- 不能是纯数字；
- 不能包含未展开的 `${...}`。

### 打开首页没有界面

本项目已移除网页前端，`GET /` 只返回一段 JSON 状态（`API-only mode`），这是预期行为。
管理操作走 `/api/*` 或 `/mcp`，见 `docs/MCP.md`。

## 2. 容器更新

### 更新后容器没了 / 旧容器还在

检查进度详情：

```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://127.0.0.1:12712/api/progress/<taskid>
```

重点字段：

- `Percentage`
- `Message`
- `DetailMsg`
- `IsDone`

日志关键字：

```text
停止容器失败
重命名旧容器失败
创建新容器失败
启动新容器失败
新容器健康检查失败
回滚
```

### 更新成功但端口冲突

可能原因：

1. 旧容器仍在运行；
2. 旧容器 paused，端口仍被占用；
3. 新容器启动前旧容器未停止。

处理：

```bash
docker ps -a --filter name=<name>
docker inspect <name> --format '{{.State.Status}} {{.State.Paused}}'
```

`force` 恢复/更新会先停止并改名旧容器；若仍冲突，检查是否有其他容器占用同一端口。

### 健康检查超时

`waitForContainerHealthy` 默认最多 90 秒。

如果容器 `health-start-period` 很长，可能误判。临时处理：

- 去掉/缩短 healthcheck；
- 或调整 `waitForContainerHealthy` 的超时计算（建议按 `StartPeriod + Interval*Retries` 动态计算，并设上限）。

## 3. 备份 / 恢复

### 恢复后容器是停止的

检查备份 JSON 是否含：

```json
"stateKnown": true,
"wasRunning": true,
"wasPaused": false
```

旧备份没有这些字段时，`restoreShouldStart` 默认返回 `true`。如果仍然停止，检查请求里的 `start` 是否显式传了 `false`。

### 同名容器已存在

默认行为：

- 镜像一致：跳过创建，必要时补启动；
- 镜像不一致：报错，提示使用 `force=true`。

要覆盖：

```json
{"filename":"backup.json","force":true}
```

### `force=true` 失败后旧容器不见了

理论上不会：旧容器先改名，不删除；失败会改回原名。

检查：

```bash
docker ps -a --filter name=<name>-dc-old-
```

如果看到 `-dc-old-` 容器，说明回滚改名失败。日志会带：

```text
回滚异常: 旧容器改回原名失败
```

手动恢复：

```bash
docker rename <name>-dc-old-<timestamp> <name>
docker start <name>
```

### 备份文件读取失败

`ResolveBackupPath` 只允许安全文件名：

- 大小写字母；
- 数字；
- 短横线；
- `.json` 后缀。

不要传路径、`..`、斜杠。

## 4. 网络

### 重建后 IP / MAC 冲突

确认是否经过：

```go
sanitizeEndpointSettings(...)
```

应清空：

- `NetworkID`
- `EndpointID`
- `Gateway`
- `IPAddress`
- `MacAddress`
- `DNSNames`

保留：

- `Aliases`
- `Links`
- `DriverOpts`
- `IPAMConfig`

### 自定义网络丢失

Compose 导出只保留非 `bridge/host/none` 的网络；恢复 JSON 时 `NetworkingConfig` 必须包含 `EndpointsConfig`。

检查备份：

```bash
jq '.NetworkingConfig.EndpointsConfig' backup.json
```

## 5. 镜像更新检测

### 总是提示有更新

检查：

1. registry 是否需要认证；
2. `GetDigest` 是否拿到远程 digest；
3. 本地 `RepoDigests` 是否为空；
4. 是否配置了 `REGISTRY_INSECURE`；
5. 代理是否污染 registry 请求。

日志关键字：

```text
获取token失败
获取digest失败
未在本地获取到repoDigest
Digest为空
```

### 私有镜像无法检测

`GetBearerHeader` 会使用 `registryAuth`；若没有凭据，只能判断公开接口。

## 6. 自更新

### 404

检查：

- `UPDATE_REPO`
- `UPDATE_BRANCH`
- `version` 文件内容
- Release 资产名：`dockerCopilot-<arch>.tar.gz`
- `githubProxy` 是否拼接正确

日志/错误可能显示：

```text
获取版本信息失败：未找到
下载 ... 失败：未找到
```

### SHA256 失败

- 确认 `.sha256` 与 tar.gz 同批上传；
- 不要手动改包；
- 旧版本过渡可临时 `UPDATE_SKIP_CHECKSUM=true`，但不推荐长期使用。

### 解压后找不到 `dockerCopilot-new`

发布包结构应为：

```text
dockerCopilot-new
```

或旧结构：

```text
dist/linux/<arch>/dockerCopilot-new
```

## 7. 日志

### 日志中间出现 `dockerCopilot`

两种来源：

1. `docker compose logs` 的服务名前缀：

   ```bash
   docker compose logs --no-log-prefix -f
   ```

2. 旧版本 `logx` 配置带了服务名。当前 `SetupLog` 使用：

   ```go
   Encoding: "plain"
   ```

   输出应为：

   ```text
   2026-10-30 01:20:11  INFO  备份容器开始
   ```

### 日志是 JSON

检查 `SetupLog` 是否被改回默认 JSON。应保留：

```go
Encoding: "plain"
```

### 日志级别

`SetupLog` 当前固定 `Level: "info"`。需要 debug 时临时改为 `debug`，不要提交。

## 8. Docker / 权限

### `permission denied` 访问 docker.sock

```bash
ls -l /var/run/docker.sock
```

容器内运行需要挂载 socket；非 root 运行需要 `group_add` 对应 docker 组 GID。

### Docker 暂不可用

启动时 Docker 不可达不会退出；定时任务会重试。检查：

```bash
docker version
echo $DOCKER_HOST
```

## 9. MCP

### `/mcp` 返回 401

- 确认 token：`MCP_TOKEN` 优先，否则用 `secretKey`；
- 三种携带方式任选其一：`Authorization: Bearer`、`X-MCP-Token`、`?token=`；
- 若刻意关闭：`MCP_TOKEN=off`（仅隔离内网）。

### 客户端握手失败 / 工具为空

- 本项目只提供 Streamable HTTP，不支持 stdio；
- 先 `initialize` 拿 `Mcp-Session-Id`，后续请求必须带上；
- `Accept` 需包含 `application/json, text/event-stream`；
- 启动日志应出现 `MCP 已挂载: /mcp ... 共 26 个工具`，没有则检查 `mcp.Register` 是否被调用。

### 工具返回 `isError`

业务错误（如 Docker 不可达）会以 `isError: true` + 文本返回，读 `content[0].text`。
用 `dc_info` 看 `dockerReady` / `dockerError`。
