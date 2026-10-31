# 构建与发布

## 1. 版本文件

| 文件 | 用途 |
|---|---|
| `version` | 通用版 / latest 发布版本 |
| `fn_version` | 飞牛 FnOS 专用版本 |
| `ugreen_version` | 绿联 UGREEN 专用版本 |

版本号同时通过 ldflags 注入：

```bash
-X 'github.com/onlyLTY/dockerCopilot/internal/config.Version=<version>'
-X 'github.com/onlyLTY/dockerCopilot/internal/config.BuildDate=<date>'
```

## 2. CI

`.github/workflows/ci.yml`：

- 触发：push / PR 到 `latest`、`dev`；
- 步骤：`go build ./...` → `go vet ./...` → `go test ./... -count=1`（前端步骤已移除）。

本地等价：

```bash
skills/dockercopilot-maintainer/scripts/dev-check.sh
```

## 3. Release 工作流

| 工作流 | 触发 | 版本文件 | 产物 |
|---|---|---|---|
| `latest.yml` | `latest` 分支 | `version` | `dockerCopilot-<arch>.tar.gz` + `.sha256` |
| `FNOS.yml` | `fn_version` 变更 / 手动 | `fn_version` | 同上，飞牛专用 |
| `UGREEN.yml` | `ugreen_version` 变更 / 手动 | `ugreen_version` | 同上，绿联专用 |

产物内是单个 `dockerCopilot-new`，`start.sh` 会在启动时把它移动为 `dockerCopilot`。

## 4. DockerHub 工作流

| 工作流 | 分支 | 镜像 tag |
|---|---|---|
| `docker-build-push-latest.yml` | `latest-dev` | `dockercopilot:latest-dev` |
| `docker-build-push-latest-stable.yml` | `latest`（`version` 变更） | `dockercopilot:latest` |
| `docker-build-push-UGREEN.yml` | `latest-dev` | `dockercopilot:UGREEN-DEV` |
| `docker-build-push-UGREEN-stable.yml` | `latest`（`ugreen_version` 变更） | `dockercopilot:UGREEN` |

需要 Secrets：

- `DOCKERHUB_USERNAME`
- `DOCKERHUB_TOKEN`

fork 也可用；只要配置自己的 Secrets。

## 5. 镜像构建

`.github/workflows/docker-build-push-*.yml` 的流程：

1. 交叉编译 `dist/linux/<arch>/dockerCopilot`；
2. 整理 `dc-back-<arch>/{dist,etc}`；
3. Docker buildx 构建并推送；
4. `docker/Dockerfile` 只 COPY：
   - `dc-back/dist/$TARGETPLATFORM/dockerCopilot`
   - `dc-back/etc/`
   - `start.sh`

## 6. 发布前检查

```bash
skills/dockercopilot-maintainer/scripts/release-check.sh
```

检查项：

1. `git status` 是否干净；
2. `version` 与目标 tag 是否一致；
3. `gofmt`、`go build`、`go vet`、`go test`；
4. 是否误提交 `dist/`、`logs/`、临时文件；
5. `.dockerignore` 是否包含 `skills/`；
6. Dockerfile 是否只 COPY 白名单；
7. 是否包含 `.sha256` 生成逻辑。

## 7. 发布后验证

```bash
# 二进制
tar -tzf dockerCopilot-amd64.tar.gz

# 校验
sha256sum -c dockerCopilot-amd64.tar.gz.sha256

# 镜像内不应出现 skills
docker run --rm --entrypoint sh <image> -c 'ls /app'
```

镜像 `/app` 应只有：

```text
dockerCopilot
etc/
start.sh
```

不应出现 `skills/`、`docs/`、`static/`。

## 8. 常见发布问题

| 问题 | 原因 | 处理 |
|---|---|---|
| Release 404 | `UPDATE_REPO`/`UPDATE_BRANCH` 与资产名不匹配 | 检查 `version`、仓库、资产命名 |
| 自更新 SHA256 失败 | 资产未上传 `.sha256` 或内容不匹配 | 重新生成并上传 |
| DockerHub push 失败 | Secrets 未配置或无权限 | 检查 `DOCKERHUB_USERNAME/TOKEN` |
| 镜像里出现源码 | Dockerfile 被改成 `COPY . .` | 恢复白名单 COPY |
| MCP 端点 401 | 客户端未带 token | 用 `MCP_TOKEN`/`secretKey` 配 Authorization |
| FNOS 版本被通用版覆盖 | 版本检查/更新逻辑未区分 | 保留 `FNOS` 判断 |
