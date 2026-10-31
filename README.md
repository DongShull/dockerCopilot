# dockerCopilot

<a href="https://www.gnu.org/licenses/agpl-3.0.en.html">
    <img alt="License: AGPLv3" src="https://shields.io/badge/License-AGPL%20v3-blue.svg">
</a>

一个主打便捷的 Docker 容器管理工具，现已支持所有平台。

> **维护说明**：本项目自 v2.2.0 起由 [DongShull](https://github.com/DongShull) 独立维护（fork 自 onlyLTY/dockerCopilot v2.1.3），
> 适配飞牛 FnOS（fn_version）与绿联 UGREEN 构建，遵循 AGPLv3 协议分发。
> 上游自 2026-07 起不再更新。

# 功能

1. 一键更新容器（失败自动回滚，成功自动清理旧镜像）
2. 指定镜像和 tag 更新
3. 启动、停止、重启容器
4. 重命名容器
5. 批量删除无 TAG 镜像（`DELETE /api/images?filter=untagged` / MCP `dc_clean_images`）
6. 批量删除未使用镜像（`DELETE /api/images?filter=unused` / MCP `dc_clean_images`）
7. 更新进度查看（SSE 实时推送 / 轮询双通道，支持取消，重启不丢历史）
8. 备份容器设置（JSON / docker-compose 双格式；每次备份独立文件，超出 `BACKUP_KEEP` 自动清理）
9. 恢复容器设置
10. 定时自动更新白名单
11. 操作审计日志与 webhook 通知
12. 内置 MCP 服务（Streamable HTTP），Agent 可直接调用全部能力；HTTP API 同时保留
13. 程序自更新（下载 + SHA256 校验；新版连续启动失败自动回退上一版）

## 使用

docker compose 安装（推荐；无需 privileged，仅挂载 docker.sock）

```
services:
  dockercopilot:
    container_name: dockercopilot
    restart: always
    network_mode: bridge
    ports:
      - 12712:12712
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./data:/data
    environment:
      - TZ=Asia/Shanghai
      - DOCKER_HOST=unix:///var/run/docker.sock
      - secretKey=密码，必须大于八位且非纯数字
    # 改成你的 Docker Hub 账号；或使用 GitHub Release 的二进制自行构建
    image: your-dockerhub-user/dockercopilot:latest
```

> 说明：容器内以 root 运行并挂载 `docker.sock` 即可管理 Docker，**不需要** `privileged: true`。
> 如需进一步降权，可为 docker.sock 配置 `group_add` 并使用非 root 用户运行。
>
> 本项目**已移除网页前端**，只提供 HTTP API 与 MCP；建议把端口绑定到回环或内网，
> 例如 `127.0.0.1:12712:12712`，因为 `/mcp` 等价于 Docker 控制面。

## 环境变量

| 变量 | 默认 | 说明 |
|---|---|---|
| `secretKey` | （必填） | 登录密钥（兼作 JWT 签名密钥），必须大于 8 位且非纯数字；缺失会拒绝启动 |
| `DOCKER_HOST` | unix:///var/run/docker.sock | Docker 守护进程地址 |
| `BACKUP_DIR` | /data/backups | 容器备份目录（优先级最高；否则用 Settings 页配置值） |
| `BACKUP_KEEP` | 7 | 每种备份（JSON / compose）保留份数，超出自动清理最旧的 |
| `DelOldContainer` | true | 更新成功后删除旧容器（设为 `false` 保留） |
| `CLEANUP_OLD_IMAGE` | true | 更新成功后清理不再被引用的旧镜像（`false` 关闭） |
| `UPDATE_REPO` | DongShull/dockerCopilot | 版本检查与自更新仓库 |
| `UPDATE_BRANCH` | latest | 版本检查分支 |
| `UPDATE_SKIP_CHECKSUM` | false | 设为 `true` 时跳过自更新 SHA256 校验（仅用于过渡旧版本，不推荐） |
| `githubProxy` | 空 | GitHub 代理前缀，如 `https://ghproxy.example/` |
| `REGISTRY_INSECURE` | false | 设为 `true` 时 registry 检测跳过 TLS 校验（默认严格校验） |
| `PROGRESS_STORE` | /data/config/progress.json | 任务进度持久化文件 |
| `PROGRESS_TTL_HOURS` | 24 | 已完成任务保留小时数，超期自动清理 |
| `NOTIFY_WEBHOOK_URL` | 空 | 任务完成通知 webhook（POST JSON） |
| `AUDIT_LOG` | /data/logs/audit.jsonl | 审计日志（JSON Lines） |
| `AUTOUPDATE_CRON` | 空 | 自动更新周期（cron 表达式，空=关闭），如 `0 4 * * *` |
| `AUTOUPDATE_WHITELIST` | 空 | 自动更新容器白名单（逗号分隔；`*` 表示全部有更新的容器） |
| `MCP_TOKEN` | 空 | `/mcp` 的访问令牌；为空时回退到 `secretKey`，设为 `off` 显式关闭鉴权（仅限隔离内网） |

## 接入方式

### MCP（推荐给 Agent）

`http://<host>:12712/mcp`，Streamable HTTP，27 个 `dc_*` 工具覆盖容器、镜像、备份恢复、
进度、设置与自更新。鉴权支持 `Authorization: Bearer <token>`、`X-MCP-Token` 与 `?token=`。
详见 [docs/MCP.md](docs/MCP.md)。

### HTTP API

- `POST /api/auth` 登录换 JWT（表单编码：`secretKey=<值>`；注意 `/api/auth` 与 update/rename 等接口只接受 form/query 参数，restore、settings 接口收 JSON body）
- `GET /api/containers` / `POST /api/container/:id/{start,stop,restart,rename,update}`
- `GET /api/container/backup` / `GET /api/container/backup2compose` / `POST /api/container/backups/restore`
  - 备份每次生成独立文件（`backup-<时间戳>.json` / `.yaml`），超出 `BACKUP_KEEP` 的旧备份自动清理；响应里的 `failures` 列出本次备份失败的容器
  - restore body：`{"filename":"backup-2026-01-01.json","start":true|false,"force":false}`；`start` 省略时按备份记录的状态恢复；`force=true` 会先停止并临时改名同名容器，新容器创建/启动成功后才删除旧容器，失败会自动回滚
- `GET /api/images` / `DELETE /api/image/:id`
  - `DELETE /api/images?filter=untagged` 批量删除无 TAG 镜像；`?filter=unused` 批量删除未被容器引用的镜像；响应带 `removed` / `failed` 明细
- `GET /api/progress` / `GET /api/progress/:taskid` / `POST /api/progress/:taskid/cancel` / `GET /api/progress/:taskid/events`（SSE，请求需带 `Accept: text/event-stream`）
- `GET /api/settings` / `PUT /api/settings`
- `GET /api/version` / `PUT /api/program`
- `POST /api/icons` / `GET /api/icons` 自定义镜像 logo（`imageName` → 上传图片文件的映射，持久化在 `/data/config`）

## 已知限制

- **私有仓库镜像无法自动检查更新**：程序不持有任何 registry 凭证，私有镜像的 digest 检查会静默跳过（日志可见），`haveUpdate` 恒为 false。
- **FnOS 版不走在线更新链**：`/api/version?type=remote` 与 `dc_check_program_update` 会返回 `fnos=true` 与提示，自更新被明确禁止；请使用 FNOS 专用发布包更新。
- **自更新回滚依赖启动脚本**：`start.sh` 会在换入新版前保留 `dockerCopilot.bak`，新版连续 3 次启动失败（启动后 30 秒内退出）自动回退；重建容器也能回到镜像自带版本。

## 开发环境

go 版本：1.23+（go.mod 指定）。项目不再嵌入前端，`go build ./...` 即可直接构建。

```
go build ./...
go test ./...
```

仓库内附开发维护技能：[`skills/dockercopilot-maintainer/SKILL.md`](skills/dockercopilot-maintainer/SKILL.md)。
该目录仅供开发使用，已通过 `.dockerignore` 排除，不会进入镜像或发布包。

改造规划与缺陷清单见 [docs/ROADMAP.md](docs/ROADMAP.md)。
