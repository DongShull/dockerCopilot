package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/utiles"
)

func registerImageTools(s *server.MCPServer, ctx *svc.ServiceContext) {
	s.AddTool(mcp.NewTool("dc_list_images",
		mcp.WithDescription("列出本地镜像（含大小、tag、是否被容器占用）"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		list, err := utiles.GetImagesList(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(list), nil
	})

	s.AddTool(mcp.NewTool("dc_check_image_update",
		mcp.WithDescription("立即对所有镜像做一次远端 digest 检查，返回每个镜像 ID 是否有更新（并刷新内部缓存）"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		list, err := utiles.GetImagesList(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		ctx.HubImageInfo.CheckUpdate(list)
		return jsonResult(ctx.HubImageInfo.Snapshot()), nil
	})

	s.AddTool(mcp.NewTool("dc_remove_image",
		mcp.WithDescription("删除镜像；force=true 时强制删除（可能影响仍在引用它的容器）"),
		mcp.WithString("imageId", mcp.Required(), mcp.Description("镜像 ID")),
		mcp.WithBoolean("force", mcp.Description("是否强制删除，默认 false")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		id, err := mustString(args, "imageId")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		force := optionalBool(args, "force", false)
		if err = utiles.RemoveImage(ctx, id, force); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		utiles.AuditLog("image.remove", id, "removed")
		return textResult("removed image: " + id), nil
	})

	s.AddTool(mcp.NewTool("dc_clean_images",
		mcp.WithDescription("批量清理镜像：filter=untagged 删除无 TAG 镜像，filter=unused 删除未被任何容器引用的镜像；逐个尝试，单个失败不影响其它"),
		mcp.WithString("filter", mcp.Required(), mcp.Description("清理过滤器：untagged 或 unused")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filter, err := mustString(req.GetArguments(), "filter")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		removed, failed, cleanErr := utiles.RemoveImagesFiltered(ctx, filter)
		if cleanErr != nil {
			return mcp.NewToolResultError(cleanErr.Error()), nil
		}
		utiles.AuditLog("image.cleanup", filter, "removed and failed lists returned to caller")
		return jsonResult(map[string]any{
			"filter":  filter,
			"removed": removed,
			"failed":  failed,
		}), nil
	})
}
