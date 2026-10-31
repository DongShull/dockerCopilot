# MCP 层速查（`internal/mcp/`）

前端已移除；Agent 走 `/mcp`（Streamable HTTP），脚本走 `/api/*`，两条通道复用同一套
`internal/utiles` / `internal/logic` 实现。

## 1. 端点

| 项 | 值 |
|---|---|
| 路径 | `/mcp` |
| 方法 | `POST`（JSON-RPC）、`GET`（SSE）、`DELETE`（结束会话） |
| 协议版本 | `2024-11-05`（客户端协商） |
| 服务端标识 | `dockerCopilot` + `mcp.Version` |

## 2. 鉴权

- token 取值：`MCP_TOKEN` → 回退 `secretKey`；
- `MCP_TOKEN=off|none|disabled` 关闭鉴权（启动时打 error 告警）；
- 携带方式：`Authorization: Bearer <t>`、`X-MCP-Token: <t>`、`?token=<t>`；
- 校验用 `crypto/subtle.ConstantTimeCompare`，避免时序侧信道。

未授权返回：

```json
{"code":401,"msg":"unauthorized: 需要 Authorization: Bearer <token> 或 X-MCP-Token 或 ?token="}
```

## 3. 工具注册约定

```go
s.AddTool(mcp.NewTool("dc_xxx",
    mcp.WithDescription("中文描述"),
    mcp.WithString("containerId", mcp.Required(), mcp.Description("容器 ID 或名称")),
), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    // 1) 解析参数
    // 2) 调用 internal/utiles 或 internal/logic
    // 3) 破坏性操作写审计
    // 4) 返回 jsonResult/textResult
})
```

约束：

1. 工具名统一 `dc_` 前缀，中文描述；
2. 必填参数必须 `mcp.Required()`；
3. 工具里不做 HTTP 自调用，直接复用 Go 函数；
4. 错误用 `mcp.NewToolResultError(err.Error())` 返回，让客户端看到 `isError: true`；
5. 长任务：

   ```go
   taskID := uuid.New().String()
   taskCtx := ctx.RegisterTask(taskID)
   defer ctx.FinishTask(taskID)
   err := utiles.UpdateContainer(ctx, taskCtx, ..., taskID)
   final, _ := ctx.GetProgress(taskID)
   ```

6. 新增工具必须同步：
   - `internal/mcp/mcp_test.go` 的 `expectedTools`；
   - `docs/MCP.md` 工具清单；
   - `dc_info` 里的 `toolCount`（自动统计，无需手改）。

## 4. 工具清单（27）

| 分组 | 工具 |
|---|---|
| 通用 | `dc_ping` `dc_info` |
| 容器 | `dc_list_containers` `dc_inspect_container` `dc_start_container` `dc_stop_container` `dc_restart_container` `dc_rename_container` `dc_update_container` `dc_autoupdate_run` |
| 镜像 | `dc_list_images` `dc_check_image_update` `dc_remove_image` `dc_clean_images` |
| 备份 | `dc_backup_containers` `dc_backup_compose` `dc_list_backups` `dc_restore_container` `dc_delete_backup` |
| 进度 | `dc_list_progress` `dc_get_progress` `dc_cancel_progress` |
| 设置 | `dc_get_settings` `dc_update_settings` |
| 版本 | `dc_get_version` `dc_check_program_update` `dc_update_program` |

## 5. 测试

```bash
go test ./internal/mcp/ -v
```

覆盖：

- `TestAllToolsRegistered`：工具数量与名字（防漏注册/改名）；
- `TestAuthMiddleware`：无 token / 错误 token / 三种正确携带方式；
- `TestInitializeAndListTools`：真实 Streamable HTTP 握手 + `tools/list`；
- `TestMCPRequiresAuth`：未授权传输层 401。

## 6. 手动联调

```bash
SID=$(curl -s -D - -o /dev/null -X POST http://127.0.0.1:12712/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"curl","version":"1"}}}' \
  | grep -i '^mcp-session-id' | tr -d '\r' | awk '{print $2}')

curl -s -X POST http://127.0.0.1:12712/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "Authorization: Bearer $TOKEN" \
  -H "Mcp-Session-Id: $SID" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"dc_info","arguments":{}}}'
```

## 7. 常见坑

| 现象 | 原因 | 处理 |
|---|---|---|
| 401 | 未带 token / token 与 `secretKey` 不一致 | 检查三种携带方式与 `MCP_TOKEN` |
| `tools/list` 为空 | 传输层未建立会话 | 先 `initialize`，后续请求带 `Mcp-Session-Id` |
| 工具报 `isError` | 业务错误（如 Docker 不可达） | 看 `content[0].text` 的错误文案 |
| 客户端连不上 | 只支持 stdio | 本项目只提供 Streamable HTTP，需客户端支持 http url |
| 工具数量变少 | `registerXxxTools` 未被 `NewServer` 调用 | 检查 `server.go` 的注册顺序 |
