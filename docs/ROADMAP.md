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
15. `Timeout: 1e14`、CORS `*`、`InsecureSkipVerify`（已加 REGISTRY_INSECURE 开关）

## 二、分期计划

| 期 | 内容 | 状态 |
|---|---|---|
| P0 | 基线整备：URL 可配置、CI（build/vet/test）、本规划入仓 | ✅ v2.2.0 |
| P1 | 修复批：缺陷 1/4/5/6/7/9/10/12/13 | ✅ v2.2.0 |
| P2 | 更新链重构：旧镜像清理、失败回滚、digest 并发检测、compose 标签感知 | ✅ v2.2.0 |
| P3 | 任务系统：进度落盘、TTL 清理、任务列表/取消 API、SSE 推送 | ✅ v2.2.0 |
| P4 | FnOS 适配：去 privileged 要求、HEALTHCHECK、备份目录运行时可配（Settings API） | ✅ v2.2.0 |
| P5 | 增强：webhook 通知、审计日志、定时自动更新白名单 | ✅ v2.2.0 |

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
| `REGISTRY_INSECURE` | true | registry 检测是否跳过 TLS 校验（"false" 严格校验） |
| `NOTIFY_WEBHOOK_URL` | 空 | 任务完成通知 webhook（POST JSON） |
| `AUTOUPDATE_CRON` | 空 | 自动更新周期（cron 表达式，空=关闭） |
| `AUTOUPDATE_WHITELIST` | 空 | 自动更新容器白名单（逗号分隔，`*`=全部有更新的容器） |

## 四、验收标准（v2.2.0）

- `go build ./...` / `go vet ./...` / `go test ./...` 全绿
- 更新一个容器后 `docker images` 不新增悬空镜像
- 模拟更新失败（拉取超时）后旧容器自动回到原名并恢复运行
- 重启进程后任务历史仍可查询；进行中任务可取消
- 50 个镜像的更新检测从串行分钟级进入 10 秒级
- 非 privileged 容器（仅挂载 docker.sock + 数据卷）可跑通全部功能
