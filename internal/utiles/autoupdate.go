package utiles

import (
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

// RunAutoUpdate 依据 AUTOUPDATE_WHITELIST（逗号分隔容器名，* 表示全部）
// 对有可用更新的容器执行自动更新，逐个串行避免 IO 争抢。
func RunAutoUpdate(serviceContext *svc.ServiceContext) {
	whitelistRaw := strings.TrimSpace(os.Getenv("AUTOUPDATE_WHITELIST"))
	if whitelistRaw == "" {
		return
	}
	allowAll := whitelistRaw == "*"
	allowed := make(map[string]bool)
	for _, name := range strings.Split(whitelistRaw, ",") {
		if name = strings.TrimSpace(name); name != "" {
			allowed[name] = true
		}
	}

	images, err := GetImagesList(serviceContext)
	if err != nil {
		logx.Errorf("自动更新：获取镜像列表失败: %v", err)
		return
	}
	serviceContext.HubImageInfo.CheckUpdate(images)

	containers, err := GetContainerList(serviceContext)
	if err != nil {
		logx.Errorf("自动更新：获取容器列表失败: %v", err)
		return
	}

	for _, c := range containers {
		if len(c.Names) == 0 || c.Image == "" {
			continue
		}
		name := strings.TrimPrefix(c.Names[0], "/")
		if !allowAll && !allowed[name] {
			continue
		}
		info, ok := serviceContext.HubImageInfo.Get(c.ImageID)
		if !ok || !info.NeedUpdate {
			continue
		}

		taskID := uuid.New().String()
		taskCtx := serviceContext.RegisterTask(taskID)
		logx.Infof("自动更新容器: %s (%s)", name, c.Image)
		updateErr := UpdateContainer(serviceContext, taskCtx, c.ID, name, c.Image, true, taskID)
		serviceContext.FinishTask(taskID)

		final, _ := serviceContext.GetProgress(taskID)
		NotifyTaskDone("autoupdate", name, taskID, updateErr == nil, final.Message)
		AuditLog("container.autoupdate", name, final.Message)
	}
}

// AutoUpdateConfigured 自动更新是否已配置（供启动日志使用）。
func AutoUpdateConfigured() bool {
	return strings.TrimSpace(os.Getenv("AUTOUPDATE_CRON")) != "" &&
		strings.TrimSpace(os.Getenv("AUTOUPDATE_WHITELIST")) != ""
}
