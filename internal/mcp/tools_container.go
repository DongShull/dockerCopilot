package mcp

import (
	"context"
	"os"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/utiles"
)

func registerContainerTools(s *server.MCPServer, ctx *svc.ServiceContext) {
	s.AddTool(mcp.NewTool("dc_list_containers",
		mcp.WithDescription("列出所有容器（含已停止），附带是否有可用更新、是否由 docker compose 管理"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		list, err := utiles.GetContainerList(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		list = utiles.CheckImageUpdate(ctx, list)
		return jsonResult(list), nil
	})

	s.AddTool(mcp.NewTool("dc_inspect_container",
		mcp.WithDescription("查看单个容器的完整 inspect 明细"),
		mcp.WithString("containerId", mcp.Required(), mcp.Description("容器 ID 或名称")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := mustString(req.GetArguments(), "containerId")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		info, err := utiles.GetContainerInspect(ctx, id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(info), nil
	})

	s.AddTool(mcp.NewTool("dc_start_container",
		mcp.WithDescription("启动容器"),
		mcp.WithString("containerId", mcp.Required(), mcp.Description("容器 ID 或名称")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := mustString(req.GetArguments(), "containerId")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err = utiles.StartContainer(ctx, id); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		utiles.AuditLog("container.start", id, "started")
		return textResult("started: " + id), nil
	})

	s.AddTool(mcp.NewTool("dc_stop_container",
		mcp.WithDescription("停止容器（发送 SIGTERM，10 秒宽限期）"),
		mcp.WithString("containerId", mcp.Required(), mcp.Description("容器 ID 或名称")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := mustString(req.GetArguments(), "containerId")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err = utiles.StopContainer(ctx, id); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		utiles.AuditLog("container.stop", id, "stopped")
		return textResult("stopped: " + id), nil
	})

	s.AddTool(mcp.NewTool("dc_restart_container",
		mcp.WithDescription("重启容器"),
		mcp.WithString("containerId", mcp.Required(), mcp.Description("容器 ID 或名称")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := mustString(req.GetArguments(), "containerId")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err = utiles.RestartContainer(ctx, id); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		utiles.AuditLog("container.restart", id, "restarted")
		return textResult("restarted: " + id), nil
	})

	s.AddTool(mcp.NewTool("dc_rename_container",
		mcp.WithDescription("重命名容器"),
		mcp.WithString("containerId", mcp.Required(), mcp.Description("容器 ID 或名称")),
		mcp.WithString("newName", mcp.Required(), mcp.Description("新容器名")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		id, err := mustString(args, "containerId")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		newName, err := mustString(args, "newName")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err = utiles.RenameContainer(ctx, id, newName); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		utiles.AuditLog("container.rename", id, "renamed to "+newName)
		return textResult("renamed " + id + " -> " + newName), nil
	})

	s.AddTool(mcp.NewTool("dc_update_container",
		mcp.WithDescription("一键更新容器：拉取新镜像 → 停止并改名旧容器 → 克隆配置创建新容器 → 启动并等待健康检查 → 删除旧容器 → 清理旧镜像；任一步失败自动回滚旧容器"),
		mcp.WithString("containerId", mcp.Required(), mcp.Description("容器 ID")),
		mcp.WithString("containerName", mcp.Required(), mcp.Description("容器名（用于进度展示与旧容器命名）")),
		mcp.WithString("image", mcp.Description("镜像名:tag；不传或留空则使用容器当前镜像拉取最新版本")),
		mcp.WithBoolean("delOldContainer", mcp.Description("更新成功后是否删除旧容器，默认读取 DelOldContainer 环境变量（默认 true）")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		id, err := mustString(args, "containerId")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		name, err := mustString(args, "containerName")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		// image 可选：留空时 UpdateContainer 会回退到容器当前镜像。
		image := optionalString(args, "image")
		delOld := optionalBool(args, "delOldContainer", os.Getenv("DelOldContainer") != "false")

		taskID := uuid.New().String()
		taskCtx := ctx.RegisterTask(taskID)
		defer ctx.FinishTask(taskID)

		updateErr := utiles.UpdateContainer(ctx, taskCtx, id, name, image, delOld, taskID)
		final, _ := ctx.GetProgress(taskID)
		message := final.Message
		if message == "" {
			message = "更新任务已结束"
		}
		detail := final.DetailMsg
		if detail == "" {
			detail = message
		}
		utiles.NotifyTaskDone("update", name, taskID, updateErr == nil, message)
		utiles.AuditLog("container.update", name, detail)
		if updateErr != nil {
			return mcp.NewToolResultError(updateErr.Error()), nil
		}
		return jsonResult(final), nil
	})

	s.AddTool(mcp.NewTool("dc_autoupdate_run",
		mcp.WithDescription("手动触发一轮自动更新：按 AUTOUPDATE_WHITELIST 过滤，仅更新运行中、非 compose 管理、非 dockerCopilot 自身的容器；全局串行，上一轮未结束时本轮跳过"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if !utiles.AutoUpdateConfigured() {
			return mcp.NewToolResultError("自动更新未配置：AUTOUPDATE_CRON 或 AUTOUPDATE_WHITELIST 为空"), nil
		}
		utiles.RunAutoUpdate(ctx)
		return textResult("autoupdate finished"), nil
	})
}
