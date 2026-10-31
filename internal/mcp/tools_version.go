package mcp

import (
	"context"
	"errors"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/onlyLTY/dockerCopilot/internal/config"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/utiles"
)

func registerVersionTools(s *server.MCPServer, ctx *svc.ServiceContext) {
	s.AddTool(mcp.NewTool("dc_get_version",
		mcp.WithDescription("获取当前本地版本号与构建时间"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return jsonResult(map[string]string{
			"version":   config.Version,
			"buildDate": config.BuildDate,
		}), nil
	})

	s.AddTool(mcp.NewTool("dc_check_program_update",
		mcp.WithDescription("检查程序是否有新版本（比对远端仓库版本号）；FNOS 版本由专用发布包管理，会返回 fnos=true"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		remote, err := utiles.GetRemoteVersion()
		if errors.Is(err, utiles.ErrFnosManagedUpdate) {
			// FNOS 版本不走通用更新链：明确告知调用方，而不是伪装成“无更新”。
			return jsonResult(map[string]any{
				"localVersion": config.Version,
				"fnos":         true,
				"hasUpdate":    false,
				"hint":         err.Error(),
			}), nil
		}
		if err != nil {
			return mcp.NewToolResultError("获取远程版本失败: " + err.Error()), nil
		}
		return jsonResult(map[string]any{
			"localVersion":  config.Version,
			"remoteVersion": remote,
			"hasUpdate":     remote != config.Version,
		}), nil
	})

	s.AddTool(mcp.NewTool("dc_update_program",
		mcp.WithDescription("更新 dockerCopilot 自身：下载新版本、校验、解压并重启进程（会中断当前服务）"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := utiles.UpdateProgram(ctx); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		utiles.AuditLog("program.update", "self", "program update triggered")
		return textResult("program update triggered, service will restart"), nil
	})
}
