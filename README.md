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
5. 删除无 TAG 镜像
6. 删除未使用镜像
7. 更新进度查看（SSE 实时推送 / 轮询双通道，支持取消，重启不丢历史）
8. 备份容器设置（JSON / docker-compose 双格式）
9. 恢复容器设置
10. 定时自动更新白名单
11. 操作审计日志与 webhook 通知

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
      - secretKey=密码，不少于八位且非纯数字
    image: 0nlylty/dockercopilot:latest
```

> 说明：容器内以 root 运行并挂载 `docker.sock` 即可管理 Docker，**不需要** `privileged: true`。
> 如需进一步降权，可为 docker.sock 配置 `group_add` 并使用非 root 用户运行。

## 环境变量

| 变量 | 默认 | 说明 |
|---|---|---|
| `secretKey` | （必填） | 登录密钥（兼作 JWT 签名密钥），不少于 8 位且非纯数字 |
| `DOCKER_HOST` | unix:///var/run/docker.sock | Docker 守护进程地址 |
| `BACKUP_DIR` | /data/backups | 容器备份目录（优先级最高；否则用 Settings 页配置值） |
| `DelOldContainer` | true | 更新成功后删除旧容器（设为 `false` 保留） |
| `CLEANUP_OLD_IMAGE` | true | 更新成功后清理不再被引用的旧镜像（`false` 关闭） |
| `UPDATE_REPO` | DongShull/dockerCopilot | 版本检查与自更新仓库 |
| `UPDATE_BRANCH` | latest | 版本检查分支 |
| `githubProxy` | 空 | GitHub 代理前缀，如 `https://ghproxy.example/` |
| `REGISTRY_INSECURE` | true | registry 检测跳过 TLS 校验（`false` 严格校验） |
| `PROGRESS_STORE` | /data/config/progress.json | 任务进度持久化文件 |
| `PROGRESS_TTL_HOURS` | 24 | 已完成任务保留小时数，超期自动清理 |
| `NOTIFY_WEBHOOK_URL` | 空 | 任务完成通知 webhook（POST JSON） |
| `AUDIT_LOG` | /data/logs/audit.jsonl | 审计日志（JSON Lines） |
| `AUTOUPDATE_CRON` | 空 | 自动更新周期（cron 表达式，空=关闭），如 `0 4 * * *` |
| `AUTOUPDATE_WHITELIST` | 空 | 自动更新容器白名单（逗号分隔；`*` 表示全部有更新的容器） |

## API 摘要

- `POST /api/auth` 登录换 JWT
- `GET /api/containers` / `POST /api/container/:id/{start,stop,restart,rename,update}`
- `GET /api/container/backup` / `GET /api/container/backup2compose` / `POST /api/container/backups/restore`
- `GET /api/images` / `DELETE /api/image/:id`
- `GET /api/progress` / `GET /api/progress/:taskid` / `POST /api/progress/:taskid/cancel` / `GET /api/progress/:taskid/events`（SSE）
- `GET /api/settings` / `PUT /api/settings`
- `GET /api/version` / `PUT /api/program`

## 开发环境

go 版本：1.23+（go.mod 指定）；构建前需在 `front/` 放置前端产物（CI 会自动从
[Docker-Copilot-React](https://github.com/dongshull/Docker-Copilot-React) 拉取构建）。

```
go build ./...
go test ./...
```

改造规划与缺陷清单见 [docs/ROADMAP.md](docs/ROADMAP.md)。
