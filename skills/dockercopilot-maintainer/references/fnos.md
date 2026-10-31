# FNOS / UGREEN 适配

## 1. 版本区分

| 文件 | 平台 | 用途 |
|---|---|---|
| `version` | 通用 | GitHub Release、latest 镜像 |
| `fn_version` | 飞牛 FnOS | FNOS Release、飞牛专用包 |
| `ugreen_version` | 绿联 UGREEN | UGREEN Release、绿联专用包 |

## 2. FNOS

构建入口：`.github/workflows/FNOS.yml`

- 触发：`fn_version` 变更 / 手动；
- 版本取自 `fn_version`；
- 产物：`dockerCopilot-<arch>.tar.gz` + `.sha256`；
- 二进制名：`dockerCopilot-new`；
- 运行逻辑：

```go
if strings.Contains(config.Version, "FNOS") {
    // 飞牛版本使用独立发布包，不访问通用版 version 文件
}
```

### 注意

- FNOS 版本不能被通用版自更新覆盖；
- `GetRemoteVersion` 遇到 `FNOS` 直接返回本地版本；
- `UpdateProgram` 遇到 `FNOS` 直接报错，要求使用专用发布包；
- 不要删除这些判断。

## 3. UGREEN

构建入口：`.github/workflows/UGREEN.yml`

- 触发：`ugreen_version` 变更 / 手动；
- 版本取自 `ugreen_version`；
- 产物同上。

## 4. 权限与运行

DC 只要求：

- 挂载 `/var/run/docker.sock`；
- 访问 `/data`；
- 端口 `12712`。

不需要 `privileged: true`。

如果 NAS 的 Docker socket 属于 `docker` 组，非 root 运行需要：

```yaml
group_add:
  - <docker-gid>
```

## 5. 开机顺序

NAS 开机时 `docker.sock` 可能晚于 DC 容器就绪。启动逻辑必须：

- Docker 初始化失败只记录错误；
- 不 `os.Exit`；
- 定时任务重试。

不要在启动路径恢复 `log.Fatalf`。

## 6. 常见问题

| 现象 | 检查 |
|---|---|
| FNOS 包被通用版更新覆盖 | `config.Version` 是否含 `FNOS`；`GetRemoteVersion` / `UpdateProgram` 判断是否保留 |
| 容器启动但接口 502 | 检查端口映射与 `secretKey`；`GET /` 应返回 200 JSON |
| 无法管理 Docker | socket 挂载、权限、`DOCKER_HOST` |
| 时区不对 | `TZ=Asia/Shanghai` |
| 图标不显示 | `/data/config/image` 挂载与权限 |
