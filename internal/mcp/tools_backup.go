package mcp

import (
	"context"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/utiles"
)

func registerBackupTools(s *server.MCPServer, ctx *svc.ServiceContext) {
	s.AddTool(mcp.NewTool("dc_backup_containers",
		mcp.WithDescription("备份全部容器配置为 JSON 文件，写入生效的备份目录（BACKUP_DIR > 设置 > /data/backups）；单个容器失败不中止整体备份，失败清单在返回的 failures 中"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		failures, backupErr := utiles.BackupContainer(ctx)
		if backupErr != nil {
			return mcp.NewToolResultError(backupErr.Error()), nil
		}
		utiles.AuditLog("container.backup", "all", "backup completed")
		result := map[string]any{
			"result":   "backup completed",
			"dir":      utiles.EffectiveBackupDir(),
			"failures": failures,
		}
		if len(failures) > 0 {
			result["result"] = "backup completed with failures"
		}
		return jsonResult(result), nil
	})

	s.AddTool(mcp.NewTool("dc_backup_compose",
		mcp.WithDescription("把容器配置导出为 docker-compose YAML 备份"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := utiles.Backup2Compose(ctx); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		utiles.AuditLog("container.backup2compose", "all", "compose backup completed")
		return textResult("compose backup completed, dir=" + utiles.EffectiveBackupDir()), nil
	})

	s.AddTool(mcp.NewTool("dc_list_backups",
		mcp.WithDescription("列出备份目录下的 .json / .yaml 备份文件"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		list, err := utiles.BackupList(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(map[string]any{
			"dir":   utiles.EffectiveBackupDir(),
			"files": list,
		}), nil
	})

	s.AddTool(mcp.NewTool("dc_restore_container",
		mcp.WithDescription("从 JSON 备份恢复容器；force=true 时同名容器会被替换（改名旧容器→创建新容器→成功后删除旧容器，失败自动回滚）"),
		mcp.WithString("filename", mcp.Required(), mcp.Description("备份文件名（仅 .json）")),
		mcp.WithBoolean("start", mcp.Description("是否启动；不传则按备份记录的状态恢复")),
		mcp.WithBoolean("force", mcp.Description("是否替换同名容器，默认 false（同名则跳过）")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		filename, err := mustString(args, "filename")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		fullPath, err := utiles.ResolveBackupPath(filename, ".json")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		var startOverride *bool
		if _, ok := args["start"]; ok {
			v := optionalBool(args, "start", false)
			startOverride = &v
		}
		force := optionalBool(args, "force", false)

		taskID := uuid.New().String()
		taskCtx := ctx.RegisterTask(taskID)
		defer ctx.FinishTask(taskID)

		restoreErr := utiles.RestoreContainer(taskCtx, ctx, filepath.Base(fullPath), taskID, startOverride, force)
		final, _ := ctx.GetProgress(taskID)
		message := final.Message
		if message == "" {
			message = "恢复任务已结束"
		}
		detail := final.DetailMsg
		if detail == "" {
			detail = message
		}
		utiles.NotifyTaskDone("restore", filename, taskID, restoreErr == nil, message)
		utiles.AuditLog("container.restore", filename, detail)
		if restoreErr != nil {
			return mcp.NewToolResultError(restoreErr.Error()), nil
		}
		return jsonResult(final), nil
	})

	s.AddTool(mcp.NewTool("dc_delete_backup",
		mcp.WithDescription("删除一个备份文件（只删备份文件，不动容器）"),
		mcp.WithString("filename", mcp.Required(), mcp.Description("备份文件名")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filename, err := mustString(req.GetArguments(), "filename")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		fullPath, err := utiles.ResolveBackupPath(filename, ".json", ".yaml")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err = os.Remove(fullPath); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		utiles.AuditLog("container.backup.delete", filename, "deleted")
		return textResult("deleted backup: " + filename), nil
	})
}
