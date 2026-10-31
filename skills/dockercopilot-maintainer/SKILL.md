---
name: dockercopilot-maintainer
description: |
  维护、二开、构建、发布和排障 dockerCopilot（DC）项目时使用。触发场景包括：
  修改 Go 后端、容器更新/回滚、备份与 force 恢复、Compose 导出、Docker 网络清洗、
  进度任务与 SSE、审计日志、自更新与 SHA256、MCP（/mcp）工具增删与鉴权、HTTP API、
  FNOS/UGREEN 构建、GitHub Actions、Docker 镜像打包、日志中文化，
  以及“DC 这个项目怎么改/怎么发版/为什么失败”。
  也适用于检查 skills/ 等开发资料是否被误打进镜像。
metadata:
  harness:
    emoji: "🧭"
  octop:
    label:
      zh: "dockerCopilot 维护"
      en: "dockerCopilot Maintainer"
    summary:
      zh: "DC 项目的开发、构建、发布与排障全流程技能。"
      en: "Full workflow skill for developing, building, releasing and troubleshooting DC."
---

# dockerCopilot 维护技能

dockerCopilot（简称 DC）是一个 Go 后端 + MCP/HTTP API 的 Docker 容器管理工具，
**已移除网页前端**：所有能力通过 `/api/*`（HTTP）与 `/mcp`（Streamable HTTP）暴露。
本技能覆盖：代码地图、开发检查、备份恢复、更新回滚、Compose 导出、MCP 工具适配、
FNOS/UGREEN 构建、CI/Release、镜像边界和日志排障。

## 0. 先确认边界

工作目录应为 DC 仓库根目录，特征文件：

```text
go.mod
dockercopilot.go
docker/Dockerfile
internal/
docs/MCP.md
version
fn_version
ugreen_version
```

如果不在仓库根目录，先 `cd` 到包含 `go.mod` 的目录。不要假设路径是
`/vol1/1000/Github/dockerCopilot`；以实际仓库为准。

## 1. 快速预检

```bash
skills/dockercopilot-maintainer/scripts/preflight.sh
```

它会检查：

- git 分支/工作区；
- Go、Docker、Node、npm；
- `version` / `fn_version` / `ugreen_version`；
- `internal/mcp` 工具是否可编译、`/mcp` 鉴权配置；
- Docker 守护进程是否可达；
- `.dockerignore` 是否排除 `skills`；
- Dockerfile 是否出现 `COPY .`。

## 2. 代码地图

| 路径 | 职责 |
|---|---|
| `dockercopilot.go` | 启动、配置、日志、路由注册、定时任务 |
| `internal/handler/` | HTTP handler，goctl 生成 |
| `internal/logic/` | 业务逻辑，参数校验后调用 `internal/utiles` |
| `internal/svc/` | ServiceContext：Docker 客户端、进度、设置、Hub 镜像信息 |
| `internal/utiles/` | Docker 操作、备份/恢复、更新、自更新、审计、网络清洗 |
| `internal/module/` | 镜像仓库鉴权、digest 检查 |
| `internal/types/` | 请求/响应类型 |
| `internal/mcp/` | MCP 工具层：`server.go` 注册与鉴权，`tools_*.go` 按域暴露能力 |
| `docker/Dockerfile` | 运行镜像，白名单 COPY |
| `.github/workflows/` | CI、FNOS、UGREEN、DockerHub |
| `skills/` | 开发资料，**禁止进入镜像** |

详细说明见 `references/architecture.md`。

## 3. 开发检查（每次改完必跑）

```bash
skills/dockercopilot-maintainer/scripts/dev-check.sh
# 需要 race：
skills/dockercopilot-maintainer/scripts/dev-check.sh --race
```

等价手动命令：

```bash
gofmt -l $(git ls-files '*.go')
go build ./...
go vet ./...
go test ./... -count=1
go test -race ./... -count=1
git diff --check
```

要求：

- 新增/修改 Go 文件必须 `gofmt`；
- 不允许 `go vet` 警告；
- 修 bug 先补回归测试；
- Docker 相关逻辑优先用接口/helper 做单元测试，不依赖真实 daemon；
- 提交前 `git diff --check`。

## 4. 高风险区域

### 4.1 更新容器（`internal/utiles/updatecontainer.go`）

必须保持：

1. inspect 旧容器，记录镜像、名称、运行/暂停状态；
2. 拉新镜像；
3. 停止旧容器（paused 先 unpause）；
4. 旧容器改名；
5. 创建/启动新容器；
6. 健康检查；
7. 成功后才删除旧容器；
8. 任一步失败回滚：移除新容器、旧容器改回原名、恢复运行/暂停；
9. 旧镜像只在无容器引用时清理；
10. 旧容器删除失败只告警，不算更新失败。

不要改成“先删旧容器再建新容器”。

### 4.2 恢复（`internal/utiles/restorecontainer.go`）

- `restoreMu` 串行化恢复；
- 默认幂等：同名容器存在则跳过，镜像不一致报错；
- `force=true`：停止 → 临时改名 → 创建/启动 → 成功才删旧；
- 失败必须 `rollbackRestoredContainer`；
- 旧备份没有 `StateKnown/WasRunning/WasPaused` 时，默认恢复后启动；
- 恢复网络配置必须经过 `restoreNetworkingConfig` / `sanitizeEndpointSettings`。

### 4.3 网络清洗（`internal/utiles/network.go`）

只保留：

- `Aliases`
- `Links`
- `DriverOpts`
- `IPAMConfig`

清空：

- `NetworkID`
- `EndpointID`
- `Gateway`
- `IPAddress`
- `IPPrefixLen`
- `IPv6Gateway`
- `GlobalIPv6Address`
- `MacAddress`
- `DNSNames`

备份、更新、恢复三条链必须一致。

### 4.4 备份与 Compose 导出

- JSON 备份要保留 `Config`、`HostConfig`、`NetworkingConfig`、状态；
- Compose 导出要保留 healthcheck、devices、cap_add/cap_drop、security_opt、
  ulimits、extra_hosts、dns、tmpfs、shm_size、user、group_add、logging、
  stop_signal、stop_grace_period、sysctls、资源限制；
- 命名卷用 `MountPoint.Name`，不要导宿主路径；
- 网络别名过滤容器 ID/名称。

### 4.5 自更新

- 下载限制 512MB；
- 非 200 必须报错；
- 默认校验 SHA256；
- 解压先到临时目录，成功后再替换；
- `FNOS` 版本禁止走通用版自更新；
- `UPDATE_SKIP_CHECKSUM=true` 只作为旧版本过渡。

### 4.6 日志

- 使用中文；
- `SetupLog` 使用 `Encoding: "plain"`，输出“时间 + 级别 + 内容”；
- 不要恢复 `dockerCopilot:` 前缀；
- HTTP 404/500 等错误要以中文描述输出；
- `docker compose logs` 的前缀用 `--no-log-prefix` 去掉，不要改代码硬扛。

## 5. MCP 层（`internal/mcp/`）

前端已移除，Agent 通过 `/mcp` 调用；`/api/*` 保留给脚本与旧客户端。

结构：

| 文件 | 职责 |
|---|---|
| `server.go` | 构建 MCPServer、注册 26 个工具、挂载 `/mcp`、token 鉴权中间件 |
| `utils.go` | `textResult` / `jsonResult` / 参数读取等公共辅助 |
| `tools_common.go` | `dc_ping` / `dc_info`（含 Docker daemon 真实 Ping） |
| `tools_container.go` | 列表、inspect、启停重启、重命名、更新、自动更新 |
| `tools_image.go` | 镜像列表、更新检查、删除 |
| `tools_backup.go` | JSON/Compose 备份、列表、恢复、删除 |
| `tools_progress.go` | 进度列表/详情/取消 |
| `tools_settings.go` | 备份目录读写 |
| `tools_version.go` | 版本、检查更新、自更新 |

新增工具必须：

1. 复用 `internal/utiles` / `internal/logic`，不要在 `internal/mcp` 里重写 Docker 逻辑；
2. 名称以 `dc_` 开头、描述用中文、必填参数用 `mcp.Required()`；
3. 破坏性操作补 `utiles.AuditLog(...)`；
4. 长任务用 `ctx.RegisterTask/CancelTask/FinishTask` 并返回最终进度；
5. 在 `internal/mcp/mcp_test.go` 的 `expectedTools` 里登记，保证工具清单可回归。

鉴权：`MCP_TOKEN` > `secretKey`；`MCP_TOKEN=off` 关闭（仅隔离内网）。支持
`Authorization: Bearer`、`X-MCP-Token`、`?token=` 三种方式。

详见 `docs/MCP.md` 与 `references/mcp.md`。

## 6. 本地构建

```bash
# amd64 + version
skills/dockercopilot-maintainer/scripts/build-linux.sh

# arm64 + fn_version
skills/dockercopilot-maintainer/scripts/build-linux.sh arm64 fn_version
```

产物：`dist/linux/<arch>/dockerCopilot-new`。

手动等价：

```bash
VERSION=$(cat version)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a --trimpath \
  -ldflags="-w -s -X 'github.com/onlyLTY/dockerCopilot/internal/config.Version=$VERSION' \
  -X 'github.com/onlyLTY/dockerCopilot/internal/config.BuildDate=$(date)'" \
  -o dist/linux/amd64/dockerCopilot-new .
```

## 7. 发布

```bash
skills/dockercopilot-maintainer/scripts/release-check.sh
```

流程：

1. 更新 `version` / `fn_version` / `ugreen_version`（按平台）；
2. 跑 `dev-check.sh --race`；
3. 跑 `package-audit.sh`；
4. 提交并推送对应分支；
5. GitHub Actions 构建后端 + Docker 镜像（前端步骤已移除）；
6. 创建/上传 Release 资产（tar.gz + sha256）。

镜像标签：

| 工作流 | 标签 |
|---|---|
| `docker-build-push-latest.yml` | `dockercopilot:latest-dev` |
| `docker-build-push-latest-stable.yml` | `dockercopilot:latest` |
| `docker-build-push-UGREEN.yml` | `dockercopilot:UGREEN-DEV` |
| `docker-build-push-UGREEN-stable.yml` | `dockercopilot:UGREEN` |

详细说明见 `references/release.md`。

## 8. 镜像边界（本技能的关键约束）

`skills/` 只用于开发，**不得进入镜像/发布包**。

三层保护：

1. `docker/Dockerfile` 只白名单 COPY：
   - `dc-back/dist/$TARGETPLATFORM/dockerCopilot`
   - `dc-back/etc/.`
   - `start.sh`
2. `.dockerignore` 排除 `skills/`；
3. `scripts/package-audit.sh` 检查：
   - Dockerfile 没有 `COPY .` / `ADD .`；
   - 没有引用 `skills`；
   - `.dockerignore` 含 `skills`；
   - 本地发布包不含 `skills/`。

每次改 Dockerfile 或打包流程后必须跑：

```bash
skills/dockercopilot-maintainer/scripts/package-audit.sh
```

## 9. FNOS / UGREEN

- `fn_version` 触发 `FNOS.yml`；
- `ugreen_version` 触发 `UGREEN.yml`；
- FNOS 版本 `config.Version` 含 `FNOS`，禁止通用版自更新覆盖；
- 启动时 Docker 不可达只记录错误，不 `os.Exit`；
- 不需要 `privileged: true`，只挂 `docker.sock`。

详见 `references/fnos.md`。

## 10. 排障

先看：

```bash
docker logs -f <container>
# 或
docker compose logs --no-log-prefix -f
```

再按 `references/troubleshooting.md` 查：

- 启动即退出；
- 页面只有占位；
- 更新/恢复失败与回滚残留；
- 端口/IP/MAC 冲突；
- registry digest 检测；
- 自更新 404 / SHA256；
- MCP 401 / 工具不可见 / 客户端握手失败；
- docker.sock 权限。

## 11. 交付清单

- [ ] `gofmt` 通过
- [ ] `go build ./...` 通过
- [ ] `go vet ./...` 通过
- [ ] `go test ./... -count=1` 通过
- [ ] 必要时 `go test -race ./...` 通过
- [ ] 新增回归测试
- [ ] `git diff --check` 通过
- [ ] 日志为中文，无 `dockerCopilot:` 前缀
- [ ] `package-audit.sh` 通过
- [ ] `/mcp` 工具清单与 `docs/MCP.md` 一致
- [ ] 版本文件已更新
- [ ] 没有提交 `.env`、token
- [ ] 文档/ROADMAP 与行为一致

## 12. 禁止事项

- 不要在日志/错误里输出密钥、token、cookie；
- 不要 `COPY .` 进 Dockerfile；
- 不要把 `skills/` 打进镜像或 Release；
- 不要删除 `force` 恢复的回滚步骤；
- 不要在恢复/更新失败时静默吞错；
- 不要用 `docker system prune -a` 清理用户环境；
- 不要在没有确认时执行删除容器/镜像的破坏性命令。
