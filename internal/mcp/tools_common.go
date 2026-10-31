package mcp

import (
	"context"
	"os"
	"runtime"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/onlyLTY/dockerCopilot/internal/config"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/utiles"
)

func registerCommonTools(s *server.MCPServer, ctx *svc.ServiceContext) {
	s.AddTool(mcp.NewTool("dc_ping",
		mcp.WithDescription("健康检查：确认 dockerCopilot 服务存活"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return textResult("pong from dockerCopilot"), nil
	})

	s.AddTool(mcp.NewTool("dc_info",
		mcp.WithDescription("获取服务运行信息：版本、构建时间、监听地址、Docker daemon 连通性、备份目录、MCP 鉴权状态"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		info := map[string]any{
			"version":        config.Version,
			"buildDate":      config.BuildDate,
			"host":           ctx.Config.Host,
			"port":           ctx.Config.Port,
			"dockerHost":     os.Getenv("DOCKER_HOST"),
			"backupDir":      utiles.EffectiveBackupDir(),
			"goVersion":      runtime.Version(),
			"autoUpdateCron": os.Getenv("AUTOUPDATE_CRON"),
			"mcpAuthEnabled": !mcpAuthDisabled(),
			"mcpTokenSource": mcpTokenSource(),
			"toolCount":      len(s.ListTools()),
		}

		// 真实探测一次 daemon，避免"客户端对象创建成功"被误判成可用。
		if ctx.DockerClient == nil {
			info["dockerReady"] = false
			info["dockerError"] = "docker client 未初始化"
		} else {
			pingCtx, cancel := context.WithTimeout(ctx2, 5*time.Second)
			pingResp, err := ctx.DockerClient.Ping(pingCtx)
			cancel()
			if err != nil {
				info["dockerReady"] = false
				info["dockerError"] = err.Error()
			} else {
				info["dockerReady"] = true
				info["dockerAPIVersion"] = pingResp.APIVersion
				info["dockerOSType"] = pingResp.OSType
			}
		}
		if ctx.DockerInitErr != nil {
			info["dockerInitErr"] = ctx.DockerInitErr.Error()
		}
		return jsonResult(info), nil
	})
}

// mcpTokenSource 说明 /mcp 当前用的是哪个凭据，便于排障。
func mcpTokenSource() string {
	if mcpAuthDisabled() {
		return "disabled"
	}
	if v := os.Getenv("MCP_TOKEN"); v != "" {
		return "MCP_TOKEN"
	}
	return "secretKey"
}
