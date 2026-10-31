package utiles

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

// 自动更新是全局串行任务：一轮没跑完时下一轮直接跳过，避免重复检查和并发更新。
var autoUpdateMu sync.Mutex

// RunAutoUpdate 依据 AUTOUPDATE_WHITELIST（逗号分隔容器名，* 表示全部）
// 对有可用更新的容器执行自动更新，逐个串行避免 IO 争抢。
func RunAutoUpdate(serviceContext *svc.ServiceContext) {
	if !autoUpdateMu.TryLock() {
		logx.Infof("自动更新：上一轮仍在执行，本轮跳过")
		return
	}
	defer autoUpdateMu.Unlock()

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

	// 与手动更新保持一致：默认删除旧容器，显式 DelOldContainer=false 时保留。
	delOldContainer := os.Getenv("DelOldContainer") != "false"

	for _, c := range containers {
		if len(c.Names) == 0 || c.Image == "" {
			continue
		}
		if c.State != "running" {
			// 自动更新不应该把原本停止的容器启动起来。
			continue
		}
		if c.Labels["com.docker.compose.project"] != "" {
			// Compose 容器交给 compose 管理，避免破坏 compose 状态。
			continue
		}
		name := strings.TrimPrefix(c.Names[0], "/")
		if strings.Contains(strings.ToLower(c.Image), "dockercopilot") {
			// 不自动更新自己，避免更新过程中进程被杀。
			continue
		}
		if !allowAll && !allowed[name] {
			continue
		}
		info, ok := serviceContext.HubImageInfo.Get(c.ImageID)
		if !ok || !info.NeedUpdate {
			continue
		}

		func() {
			taskID := uuid.New().String()
			taskCtx := serviceContext.RegisterTask(taskID)
			notified := false
			defer func() {
				if r := recover(); r != nil {
					msg := fmt.Sprintf("自动更新异常中断: %v", r)
					serviceContext.UpdateProgress(taskID, svc.TaskProgress{
						TaskID:     taskID,
						Percentage: 100,
						Name:       name,
						Message:    "自动更新失败",
						DetailMsg:  msg,
						IsDone:     true,
					})
					if !notified {
						NotifyTaskDone("autoupdate", name, taskID, false, msg)
						AuditLog("container.autoupdate", name, msg)
					}
					logx.Errorf("自动更新容器 %s panic: %v", name, r)
				}
				serviceContext.FinishTask(taskID)
			}()

			logx.Infof("自动更新容器: %s (%s)", name, c.Image)
			updateErr := UpdateContainer(serviceContext, taskCtx, c.ID, name, c.Image, delOldContainer, taskID)

			final, _ := serviceContext.GetProgress(taskID)
			message := final.Message
			if message == "" {
				message = "自动更新已结束"
			}
			detail := final.DetailMsg
			if detail == "" {
				detail = message
			}
			NotifyTaskDone("autoupdate", name, taskID, updateErr == nil, message)
			AuditLog("container.autoupdate", name, detail)
			notified = true
		}()
	}
}

// AutoUpdateConfigured 自动更新是否已配置（供启动日志使用）。
func AutoUpdateConfigured() bool {
	return strings.TrimSpace(os.Getenv("AUTOUPDATE_CRON")) != "" &&
		strings.TrimSpace(os.Getenv("AUTOUPDATE_WHITELIST")) != ""
}
