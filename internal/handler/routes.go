// 基于 goctl 生成，包含手工扩展：GET /（webindexHandler）、进度/设置/版本路由、
// SSE 事件流（WithSSE）、批量镜像清理（DELETE /images）。若用 goctl 重新生成，
// 请对照 git 历史补回手工部分。
package handler

import (
	"net/http"

	auth "github.com/onlyLTY/dockerCopilot/internal/handler/auth"
	container "github.com/onlyLTY/dockerCopilot/internal/handler/container"
	icons "github.com/onlyLTY/dockerCopilot/internal/handler/icons"
	image "github.com/onlyLTY/dockerCopilot/internal/handler/image"
	progress "github.com/onlyLTY/dockerCopilot/internal/handler/progress"
	settings "github.com/onlyLTY/dockerCopilot/internal/handler/settings"
	version "github.com/onlyLTY/dockerCopilot/internal/handler/version"
	"github.com/onlyLTY/dockerCopilot/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

func RegisterHandlers(server *rest.Server, serverCtx *svc.ServiceContext) {
	server.AddRoutes(
		[]rest.Route{
			{
				Method:  http.MethodGet,
				Path:    "/",
				Handler: webindexHandler(serverCtx),
			},
		},
	)

	server.AddRoutes(
		[]rest.Route{
			{
				Method:  http.MethodPost,
				Path:    "/auth",
				Handler: auth.LoginHandler(serverCtx),
			},
		},
		rest.WithPrefix("/api"),
	)

	server.AddRoutes(
		[]rest.Route{
			{
				Method:  http.MethodPost,
				Path:    "/container/:id/rename",
				Handler: container.RenameHandler(serverCtx),
			},
			{
				Method:  http.MethodPost,
				Path:    "/container/:id/restart",
				Handler: container.RestartHandler(serverCtx),
			},
			{
				Method:  http.MethodPost,
				Path:    "/container/:id/start",
				Handler: container.StartHandler(serverCtx),
			},
			{
				Method:  http.MethodPost,
				Path:    "/container/:id/stop",
				Handler: container.StopHandler(serverCtx),
			},
			{
				Method:  http.MethodPost,
				Path:    "/container/:id/update",
				Handler: container.UpdateHandler(serverCtx),
			},
			{
				Method:  http.MethodGet,
				Path:    "/container/backup",
				Handler: container.BackupHandler(serverCtx),
			},
			{
				Method:  http.MethodGet,
				Path:    "/container/backup2compose",
				Handler: container.Backup2composeHandler(serverCtx),
			},
			{
				Method:  http.MethodDelete,
				Path:    "/container/backups",
				Handler: container.DelRestoreHandler(serverCtx),
			},
			{
				Method:  http.MethodPost,
				Path:    "/container/backups/restore",
				Handler: container.RestoreHandler(serverCtx),
			},
			{
				Method:  http.MethodGet,
				Path:    "/container/listBackups",
				Handler: container.ListBackupsHandler(serverCtx),
			},
			{
				Method:  http.MethodGet,
				Path:    "/containers",
				Handler: container.ContainersListHandler(serverCtx),
			},
		},
		rest.WithJwt(serverCtx.Config.Auth.AccessSecret),
		rest.WithPrefix("/api"),
	)

	server.AddRoutes(
		[]rest.Route{
			{
				Method:  http.MethodPost,
				Path:    "/",
				Handler: icons.UploadHandler(serverCtx),
			},
			{
				Method:  http.MethodGet,
				Path:    "/",
				Handler: icons.ObtainHandler(serverCtx),
			},
		},
		rest.WithJwt(serverCtx.Config.Auth.AccessSecret),
		rest.WithPrefix("/api/icons"),
	)

	server.AddRoutes(
		[]rest.Route{
			{
				Method:  http.MethodDelete,
				Path:    "/image/:id",
				Handler: image.RemoveHandler(serverCtx),
			},
			{
				Method:  http.MethodGet,
				Path:    "/images",
				Handler: image.ImagesListHandler(serverCtx),
			},
			{
				// 批量清理：?filter=untagged 删除无 TAG 镜像，?filter=unused 删除未使用镜像
				Method:  http.MethodDelete,
				Path:    "/images",
				Handler: image.CleanImagesHandler(serverCtx),
			},
		},
		rest.WithJwt(serverCtx.Config.Auth.AccessSecret),
		rest.WithPrefix("/api"),
	)

	server.AddRoutes(
		[]rest.Route{
			{
				Method:  http.MethodGet,
				Path:    "/progress",
				Handler: progress.ListProgressHandler(serverCtx),
			},
			{
				Method:  http.MethodGet,
				Path:    "/progress/:taskid",
				Handler: progress.GetProgressHandler(serverCtx),
			},
			{
				Method:  http.MethodPost,
				Path:    "/progress/:taskid/cancel",
				Handler: progress.CancelProgressHandler(serverCtx),
			},
		},
		rest.WithJwt(serverCtx.Config.Auth.AccessSecret),
		rest.WithPrefix("/api"),
	)

	// SSE 事件流单独一组：WithSSE 声明为长连接路由，go-zero 对
	// Accept: text/event-stream 的请求不做超时包裹。
	server.AddRoutes(
		[]rest.Route{
			{
				Method:  http.MethodGet,
				Path:    "/progress/:taskid/events",
				Handler: progress.StreamEventsHandler(serverCtx),
			},
		},
		rest.WithJwt(serverCtx.Config.Auth.AccessSecret),
		rest.WithPrefix("/api"),
		rest.WithSSE(),
	)

	server.AddRoutes(
		[]rest.Route{
			{
				Method:  http.MethodGet,
				Path:    "/settings",
				Handler: settings.GetSettingsHandler(serverCtx),
			},
			{
				Method:  http.MethodPut,
				Path:    "/settings",
				Handler: settings.UpdateSettingsHandler(serverCtx),
			},
		},
		rest.WithJwt(serverCtx.Config.Auth.AccessSecret),
		rest.WithPrefix("/api"),
	)

	server.AddRoutes(
		[]rest.Route{
			{
				Method:  http.MethodPut,
				Path:    "/program",
				Handler: version.UpdateProgramHandler(serverCtx),
			},
			{
				Method:  http.MethodGet,
				Path:    "/version",
				Handler: version.VersionHandler(serverCtx),
			},
		},
		rest.WithJwt(serverCtx.Config.Auth.AccessSecret),
		rest.WithPrefix("/api"),
	)
}
