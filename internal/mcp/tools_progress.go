package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
)

func registerProgressTools(s *server.MCPServer, ctx *svc.ServiceContext) {
	s.AddTool(mcp.NewTool("dc_list_progress",
		mcp.WithDescription("列出全部任务进度（当前进程内存 + 重启后从磁盘恢复的历史任务）"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return jsonResult(ctx.ListProgress()), nil
	})

	s.AddTool(mcp.NewTool("dc_get_progress",
		mcp.WithDescription("查询单个任务的进度详情"),
		mcp.WithString("taskId", mcp.Required(), mcp.Description("任务 ID")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := mustString(req.GetArguments(), "taskId")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		p, ok := ctx.GetProgress(taskID)
		if !ok {
			return mcp.NewToolResultError("task not found: " + taskID), nil
		}
		return jsonResult(p), nil
	})

	s.AddTool(mcp.NewTool("dc_cancel_progress",
		mcp.WithDescription("取消一个进行中的任务（任务会在下一个步骤边界退出）"),
		mcp.WithString("taskId", mcp.Required(), mcp.Description("任务 ID")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := mustString(req.GetArguments(), "taskId")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if !ctx.CancelTask(taskID) {
			return mcp.NewToolResultError("task not cancellable: " + taskID), nil
		}
		return textResult("cancel requested: " + taskID), nil
	})
}
