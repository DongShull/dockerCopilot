package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/utiles"
)

func registerSettingsTools(s *server.MCPServer, ctx *svc.ServiceContext) {
	s.AddTool(mcp.NewTool("dc_get_settings",
		mcp.WithDescription("读取运行时设置（当前配置的备份目录与实际生效的备份目录）"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		s := utiles.LoadSettings()
		return jsonResult(map[string]any{
			"backupDir":          s.BackupDir,
			"effectiveBackupDir": utiles.EffectiveBackupDir(),
		}), nil
	})

	s.AddTool(mcp.NewTool("dc_update_settings",
		mcp.WithDescription("更新运行时设置（目前支持 backupDir，必须是绝对路径且可写）"),
		mcp.WithString("backupDir", mcp.Required(), mcp.Description("备份目录绝对路径")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		dir, err := mustString(req.GetArguments(), "backupDir")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		dir = strings.TrimSpace(dir)
		if !filepath.IsAbs(dir) {
			return mcp.NewToolResultError("备份目录必须是绝对路径"), nil
		}
		if err = os.MkdirAll(dir, 0755); err != nil {
			return mcp.NewToolResultError("无法创建备份目录: " + err.Error()), nil
		}
		testFile, testErr := os.CreateTemp(dir, ".write-test-*")
		if testErr != nil {
			return mcp.NewToolResultError("备份目录不可写: " + testErr.Error()), nil
		}
		testName := testFile.Name()
		_ = testFile.Close()
		_ = os.Remove(testName)

		s := utiles.LoadSettings()
		s.BackupDir = dir
		if err = utiles.SaveSettings(s); err != nil {
			return mcp.NewToolResultError("保存设置失败: " + err.Error()), nil
		}
		utiles.AuditLog("settings.update", "settings", "backupDir="+dir)
		return jsonResult(map[string]any{
			"backupDir":          s.BackupDir,
			"effectiveBackupDir": utiles.EffectiveBackupDir(),
		}), nil
	})
}
