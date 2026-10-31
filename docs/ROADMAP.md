# dockerCopilot 改造规划（ROADMAP）

> 本项目自 v2.2.0 起由 DongShull 独立维护（fork 自 onlyLTY/dockerCopilot v2.1.3，上游已停止更新）。
> 本文件记录 v2.1.3 基线体检结论与改造路线。

## 一、基线缺陷清单（v2.1.3）

### P0 · 崩溃/数据级
1. `containerslistlogic.go`：inspect 失败后仍解引用 nil，容器列表接口可被打崩
2. `updatecontainer.go`：更新失败（create/start）时旧容器停留在改名+停止状态，无回滚
3. `updatecontainer.go`：更新成功后不清理旧镜像，磁盘慢性泄漏

### P1 · 功能错误
4. 旧容器改名使用秒级时间戳，同秒二次更新必撞名
5. `checkupdate.go`：digest 比对循环覆盖，"最后一个说了算"，多 RepoDigest 时误报/漏报
6. `image.go`：`strings.Split(":")[1]` 解析 tag，带端口的私有仓库镜像名/tag 错位
7. 停止容器使用 SIGINT，业界惯例 SIGTERM
8. `getRemoteVersion.go`/`updateprogram.go` 硬编码上游 onlyLTY URL（已改可配置 UPDATE_REPO）
9. `decompressTarGz`：tar 条目名不校验，存在 zip-slip 路径穿越
10. `module.ImageUpdateData`：cron 写、HTTP 读共用 map，无锁数据竞争

### P2 · 卫生级
11. ProgressStore 只写不删（内存泄漏），重启丢进度，无任务取消
12. `checkImageInUsed` O(容器×镜像) 双重循环
13. restore 中永假的死代码分支
14. JWT 84 天有效、secretKey 兼作签名密钥（记录在案，暂无计划改动）
15. `Timeout: 1e14`、CORS `*`、`InsecureSkipVerify`（已加 REGISTRY_INSECURE 开关；Timeout 已改为 600000ms，SSE 路由带 `WithSSE()` 走 Accept 头豁免，见 `etc/dockerCopilot.yaml` 注释）

## 二、分期计划

| 期 | 内容 | 状态 |
|---|---|---|
| P0 | 基线整备：URL 可配置、CI（build/vet/test）、本规划入仓 | ✅ v2.2.0 |
| P1 | 修复批：缺陷 1/4/5/6/7/9/10/12/13 | ✅ v2.2.0 |
| P2 | 更新链重构：旧镜像清理、失败回滚、digest 并发检测、compose 标签感知 | ✅ v2.2.0 |
| P3 | 任务系统：进度落盘、TTL 清理、任务列表/取消 API、SSE 推送 | ✅ v2.2.0 |
| P4 | FnOS 适配：去 privileged 要求、HEALTHCHECK、备份目录运行时可配（Settings API） | ✅ v2.2.0 |
| P5 | 增强：webhook 通知、审计日志、定时自动更新白名单 | ✅ v2.2.0 |
| P6 | 稳定性：SSE 心跳、Docker 调用超时、进度文件并发落盘、Compose 导出保真度 | ✅ 待发版 |
| P7 | Agent 化：移除网页前端、保留 HTTP API、内置 MCP（Streamable HTTP）暴露全部能力 | ✅ 待发版 |

### P6 补充说明

- SSE 增加 15 秒心跳和断线重连提示，避免反向代理在任务长时间无进度时断开连接。
- 单次 Docker list/inspect/start/stop/rename/remove 调用增加 60 秒~2 分钟超时；镜像拉取仍由任务上下文控制，避免大镜像被误杀。
- 进度文件落盘与内存快照在同一把锁内完成，修复并发保存时旧快照覆盖新状态的问题。
- Compose 导出补齐 healthcheck、devices、cap_add/cap_drop、security_opt、ulimits、extra_hosts、dns、tmpfs、shm_size、user、group_add、logging、stop_signal、stop_grace_period、sysctls、资源限制等字段；命名卷改用 MountPoint.Name，避免导出宿主机卷路径。
- 恢复 `force=true` 改为“停止并临时改名旧容器 → 创建/启动新容器 → 成功后删除旧容器”；任一步失败都会清理半成品并把旧容器改回原名、恢复运行/暂停状态。
- 备份/更新/恢复统一清洗网络端点：只保留别名、链接、驱动参数和静态 IPAM，清掉运行期 NetworkID/EndpointID/IP/MAC/DNSNames，避免重建时继承失效端点或地址冲突。

### P7 补充说明

- 前端（`go:embed front/*`、`/manager` 静态路由、CI 的前端构建 job）整体移除，`GET /` 改为返回 JSON 状态，同时满足容器 HEALTHCHECK 的 200 要求。
- 保留全部 `/api/*` 路由：脚本、CI 与旧客户端不受影响，MCP 与 API 共用同一套 `internal/utiles` / `internal/logic` 实现。
- 新增 `internal/mcp/`：基于 `mark3labs/mcp-go` 的 Streamable HTTP 服务，挂载 `/mcp`，注册 26 个 `dc_*` 工具，覆盖容器、镜像、备份恢复、进度、设置与自更新。
- `/mcp` 默认鉴权：`MCP_TOKEN` 优先，未设置时回退 `secretKey`；支持 `Authorization: Bearer`、`X-MCP-Token`、`?token=` 三种携带方式，比较使用常量时间算法。`MCP_TOKEN=off` 可显式关闭（仅隔离内网）。
- 破坏性 MCP 操作复用现有审计与通知；长任务复用 `RegisterTask`/`CancelTask`/`FinishTask` 与进度持久化。

## 三、配置项一览（环境变量）

| 变量 | 默认 | 说明 |
|---|---|---|
| `secretKey` | （必填） | 登录密钥，同时用于 JWT 签名，不少于 8 位且非纯数字 |
| `DOCKER_HOST` | unix:///var/run/docker.sock | Docker 守护进程地址 |
| `BACKUP_DIR` | /data/backups | 容器备份目录（优先级：env > Settings API 配置 > 默认） |
| `DelOldContainer` | true | 更新成功后是否删除旧容器（"false" 保留） |
| `CLEANUP_OLD_IMAGE` | true | 更新成功后是否清理不再被引用的旧镜像 |
| `UPDATE_REPO` | DongShull/dockerCopilot | 版本检查与自更新仓库 |
| `UPDATE_BRANCH` | latest | 版本检查分支 |
| `githubProxy` | 空 | GitHub 代理前缀 |
| `REGISTRY_INSECURE` | false | 设为 `true` 时 registry 检测跳过 TLS 校验（默认严格校验） |
| `NOTIFY_WEBHOOK_URL` | 空 | 任务完成通知 webhook（POST JSON） |
| `AUTOUPDATE_CRON` | 空 | 自动更新周期（cron 表达式，空=关闭） |
| `AUTOUPDATE_WHITELIST` | 空 | 自动更新容器白名单（逗号分隔，`*`=全部有更新的容器） |
| `MCP_TOKEN` | 空 | `/mcp` 访问令牌；空则回退 `secretKey`，`off` 显式关闭鉴权 |

## 四、验收标准（v2.2.0）

- `go build ./...` / `go vet ./...` / `go test ./...` 全绿
- 更新一个容器后 `docker images` 不新增悬空镜像
- 模拟更新失败（拉取超时）后旧容器自动回到原名并恢复运行
- 重启进程后任务历史仍可查询；进行中任务可取消
- 50 个镜像的更新检测从串行分钟级进入 10 秒级
- 非 privileged 容器（仅挂载 docker.sock + 数据卷）可跑通全部功能
- `go test ./internal/mcp/` 覆盖工具注册、鉴权与 Streamable HTTP 握手
- MCP 客户端可完成「列容器 → 更新 → 查进度」全流程，`/api/*` 行为不变
