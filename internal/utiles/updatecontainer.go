package utiles

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	dockerMsgType "github.com/docker/docker/pkg/jsonmessage"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

// 同一个容器只允许一个更新流程，避免手动更新与自动更新并发改名/创建。
var containerUpdateLocks sync.Map

func lockContainerUpdate(id string) func() {
	value, _ := containerUpdateLocks.LoadOrStore(id, &sync.Mutex{})
	mutex := value.(*sync.Mutex)
	mutex.Lock()
	return mutex.Unlock
}

// UpdateContainer 执行一次容器更新：拉取新镜像 → 停止并改名旧容器 → 克隆配置创建新容器
// → 启动新容器 → 删除旧容器 → 清理旧镜像。任一步失败都会尝试把旧容器恢复到原名并重新启动。
// ctx 支持取消：任务被取消时在步骤边界退出。
func UpdateContainer(serviceContext *svc.ServiceContext, ctx context.Context, id string, name string, imageNameAndTag string, delOldContainer bool, taskID string) error {
	unlock := lockContainerUpdate(id)
	defer unlock()

	progress := func(percentage int, message, detailMsg string, isDone bool) {
		serviceContext.UpdateProgress(taskID, svc.TaskProgress{
			TaskID:     taskID,
			Percentage: percentage,
			Name:       name,
			Message:    message,
			DetailMsg:  detailMsg,
			IsDone:     isDone,
		})
	}
	cancelled := func() bool {
		if ctx.Err() != nil {
			progress(100, "任务已取消", "任务已取消", true)
			return true
		}
		return false
	}

	progress(0, "正在连接Docker", "正在连接Docker", false)
	if cancelled() {
		return ctx.Err()
	}

	timeout := 10
	signal := "SIGTERM"

	negotiateCtx, negotiateCancel := dockerTimeout(ctx, dockerNegotiateTimeout)
	serviceContext.DockerClient.NegotiateAPIVersion(negotiateCtx)
	negotiateCancel()

	// 先记录旧容器信息：旧镜像 ID 用于更新后的清理，旧容器名用于失败回滚
	inspectedContainer, err := serviceContext.DockerClient.ContainerInspect(ctx, id)
	if err != nil {
		progress(0, "获取容器信息失败", err.Error(), true)
		logx.Errorf("获取容器信息失败: %v", err)
		return err
	}
	if inspectedContainer.Config == nil {
		err = fmt.Errorf("旧容器配置为空，无法更新")
		progress(0, "获取容器配置失败", err.Error(), true)
		return err
	}
	if inspectedContainer.HostConfig == nil {
		err = fmt.Errorf("旧容器 HostConfig 为空，无法克隆配置")
		progress(0, "获取容器配置失败", err.Error(), true)
		return err
	}
	if strings.TrimSpace(imageNameAndTag) == "" {
		imageNameAndTag = inspectedContainer.Config.Image
	}
	if strings.TrimSpace(imageNameAndTag) == "" {
		err = fmt.Errorf("镜像名称为空")
		progress(0, "更新失败", err.Error(), true)
		return err
	}
	oldImageID := inspectedContainer.Image
	oldContainerName := strings.TrimPrefix(inspectedContainer.Name, "/")
	if oldContainerName == "" {
		oldContainerName = name
	}
	wasRunning := inspectedContainer.State != nil && inspectedContainer.State.Running
	wasPaused := inspectedContainer.State != nil && inspectedContainer.State.Paused
	backupName := oldContainerName + "-" + time.Now().Format("2006-01-02-15-04-05.000000000")
	renamed := false
	newCreated := false
	var warnings []string
	if inspectedContainer.Config.Labels["com.docker.compose.project"] != "" {
		warnings = append(warnings, "该容器由 Compose 管理，本次按容器方式重建；建议优先使用 docker compose pull/up 更新")
	}

	// rollback 只恢复本流程真正改过名的旧容器；原本停止的容器不会被误启动。
	rollback := func(reason string) {
		bg, bgCancel := dockerTimeout(context.Background(), dockerMutationTimeout)
		defer bgCancel()
		// 新容器还占着原名时先移除，否则旧容器改不回去。
		if newCreated {
			if removeErr := serviceContext.DockerClient.ContainerRemove(bg, oldContainerName, container.RemoveOptions{Force: true}); removeErr != nil {
				logx.Errorf("回滚移除新容器失败: %v", removeErr)
			}
			newCreated = false
		}
		if renamed {
			if renameErr := serviceContext.DockerClient.ContainerRename(bg, id, oldContainerName); renameErr != nil {
				logx.Errorf("回滚重命名失败: %v", renameErr)
				progress(100, "更新失败且回滚失败，请手动处理旧容器 "+backupName, reason, true)
				return
			}
			renamed = false
		}
		if wasRunning {
			// 原本是 paused 的容器，先取消暂停再确保运行，最后恢复暂停状态。
			if wasPaused {
				_ = serviceContext.DockerClient.ContainerUnpause(bg, id)
			}
			current, inspectErr := serviceContext.DockerClient.ContainerInspect(bg, id)
			if inspectErr != nil || current.State == nil || !current.State.Running {
				if startErr := serviceContext.DockerClient.ContainerStart(bg, id, container.StartOptions{}); startErr != nil {
					logx.Errorf("回滚启动失败: %v", startErr)
					progress(100, "更新失败，旧容器已改回原名但启动失败", reason+"；启动错误: "+startErr.Error(), true)
					return
				}
			}
			if wasPaused {
				if pauseErr := serviceContext.DockerClient.ContainerPause(bg, id); pauseErr != nil {
					logx.Errorf("回滚恢复暂停状态失败: %v", pauseErr)
					progress(100, "更新失败，旧容器已恢复但暂停状态恢复失败", reason+"；暂停错误: "+pauseErr.Error(), true)
					return
				}
			}
		}
		progress(100, "更新失败，已回滚旧容器", reason, true)
	}

	// 1. 拉取新镜像（流式进度 10% → 25%）
	progress(10, "正在拉取新镜像", "正在拉取新镜像", false)
	reader, err := serviceContext.DockerClient.ImagePull(ctx, imageNameAndTag, image.PullOptions{})
	if err != nil {
		if ctx.Err() != nil {
			progress(100, "任务已取消", "任务已在拉取镜像阶段取消", true)
			return ctx.Err()
		}
		progress(100, "拉取镜像失败", err.Error(), true)
		logx.Errorf("拉取镜像失败: %s", err)
		return err
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			logx.Errorf("关闭镜像拉取流失败: %v", closeErr)
		}
	}()
	if err = decodePullResp(reader, serviceContext, ctx, taskID); err != nil {
		if ctx.Err() != nil {
			// 取消发生在拉取阶段时，错误本质是 context.Canceled，
			// 如实报“任务已取消”而不是误导性的“拉取镜像失败”。
			progress(100, "任务已取消", "任务已在拉取镜像阶段取消", true)
			return ctx.Err()
		}
		progress(100, "拉取镜像失败", err.Error(), true)
		logx.Errorf("拉取镜像失败: %s", err)
		return err
	}
	if cancelled() {
		return ctx.Err()
	}

	// 2. 停止旧容器（原本停止的容器跳过，避免更新后把它误启动）
	progress(30, "正在停止容器", "正在停止容器", false)
	stopOptions := container.StopOptions{
		Signal:  signal,
		Timeout: &timeout,
	}
	if wasRunning {
		// Docker 不允许直接 stop paused 容器，先取消暂停；回滚/新容器启动后会恢复。
		if wasPaused {
			if err = serviceContext.DockerClient.ContainerUnpause(ctx, id); err != nil {
				rollback("取消旧容器暂停失败: " + err.Error())
				return err
			}
		}
		if err = serviceContext.DockerClient.ContainerStop(ctx, id, stopOptions); err != nil {
			rollback("停止容器失败: " + err.Error())
			return err
		}
	}

	// 3. 重命名旧容器（腾出原名）
	progress(40, "正在重命名旧容器", "正在重命名旧容器", false)
	if err = serviceContext.DockerClient.ContainerRename(ctx, id, backupName); err != nil {
		// 还没改名成功，旧容器仍是原名，只恢复它原来的运行状态
		rollback("重命名旧容器失败: " + err.Error())
		return err
	}
	renamed = true

	// 4. 克隆旧容器配置创建新容器（只替换镜像）
	progress(60, "正在创建新容器", "正在创建新容器", false)
	inspectedContainer.Config.Hostname = ""
	inspectedContainer.Config.Image = imageNameAndTag
	inspectedContainer.Image = imageNameAndTag
	config := inspectedContainer.Config
	hostConfig := inspectedContainer.HostConfig
	var endpoints map[string]*network.EndpointSettings
	if inspectedContainer.NetworkSettings != nil {
		endpoints = sanitizeEndpointSettings(inspectedContainer.NetworkSettings.Networks)
	}
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: endpoints,
	}
	if _, err = serviceContext.DockerClient.ContainerCreate(ctx, config, hostConfig, networkingConfig, nil, oldContainerName); err != nil {
		rollback("创建新容器失败: " + err.Error())
		return err
	}
	newCreated = true

	// 5. 启动新容器并等待健康检查；原 paused 容器启动后再暂停。
	if wasRunning {
		progress(80, "正在启动新容器", "正在启动新容器", false)
		if err = serviceContext.DockerClient.ContainerStart(ctx, oldContainerName, container.StartOptions{}); err != nil {
			// 新容器已创建但起不来：rollback 会移除它并恢复旧容器。
			rollback("启动新容器失败: " + err.Error())
			return err
		}
		if !wasPaused {
			progress(85, "正在等待容器健康", "正在等待新容器通过健康检查", false)
			if err = waitForContainerHealthy(ctx, serviceContext, oldContainerName, inspectedContainer.Config.Healthcheck); err != nil {
				rollback("新容器健康检查失败: " + err.Error())
				return err
			}
		}
		if wasPaused {
			if err = serviceContext.DockerClient.ContainerPause(ctx, oldContainerName); err != nil {
				rollback("恢复新容器暂停状态失败: " + err.Error())
				return err
			}
		}
	}

	// 6. 删除旧容器（默认保留开关可关闭）
	if delOldContainer {
		if err = serviceContext.DockerClient.ContainerRemove(ctx, id, container.RemoveOptions{}); err != nil {
			// 旧容器删除失败不算更新失败，但需要告知
			logx.Errorf("删除旧容器失败: %v", err)
			warning := "旧容器删除失败，请手动清理 " + backupName + ": " + err.Error()
			warnings = append(warnings, warning)
			progress(90, "更新成功（旧容器删除失败）", warning, false)
		}
	}

	// 7. 清理旧镜像：仅当旧容器已删除、且旧镜像不再被任何容器引用时删除
	if delOldContainer && os.Getenv("CLEANUP_OLD_IMAGE") != "false" {
		progress(95, "正在清理旧镜像", "正在清理旧镜像", false)
		if cleanupMsg := cleanupOldImage(serviceContext, oldImageID); cleanupMsg != "" {
			warnings = append(warnings, cleanupMsg)
			progress(95, "更新成功（旧镜像未清理）", cleanupMsg, false)
		}
	}

	detail := "更新成功"
	if len(warnings) > 0 {
		detail = strings.Join(warnings, "；\n")
	}
	progress(100, "更新成功", detail, true)
	return nil
}

// waitForContainerHealthy 在容器启动后确认它没有立刻退出。
// 有 HEALTHCHECK 时等待 healthy；没有时只观察 2 秒，避免把“启动成功但马上崩”当成成功。
func waitForContainerHealthy(ctx context.Context, serviceContext *svc.ServiceContext, name string, healthcheck *container.HealthConfig) error {
	checkCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	hasHealthcheck := healthcheck != nil && len(healthcheck.Test) > 0 &&
		!(len(healthcheck.Test) == 1 && strings.EqualFold(healthcheck.Test[0], "NONE"))
	stableDeadline := time.Now().Add(2 * time.Second)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-checkCtx.Done():
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("等待容器健康超时")
		case <-ticker.C:
			inspected, err := serviceContext.DockerClient.ContainerInspect(checkCtx, name)
			if err != nil {
				return fmt.Errorf("检查新容器状态失败: %w", err)
			}
			if inspected.State == nil {
				return fmt.Errorf("新容器状态为空")
			}
			if !inspected.State.Running {
				return fmt.Errorf("新容器已退出，退出码 %d", inspected.State.ExitCode)
			}
			if hasHealthcheck && inspected.State.Health != nil {
				switch strings.ToLower(string(inspected.State.Health.Status)) {
				case "healthy":
					return nil
				case "unhealthy":
					return fmt.Errorf("新容器健康检查结果为 unhealthy")
				}
			} else if !hasHealthcheck && time.Now().After(stableDeadline) {
				return nil
			}
		}
	}
}

// cleanupOldImage 在没有任何容器引用 oldImageID 时删除该镜像。
// 返回补充信息（清理失败原因），失败不影响更新结果。
func cleanupOldImage(serviceContext *svc.ServiceContext, oldImageID string) string {
	if oldImageID == "" {
		return ""
	}
	listCtx, listCancel := dockerTimeout(context.Background(), dockerListTimeout)
	containers, err := serviceContext.DockerClient.ContainerList(listCtx, container.ListOptions{All: true})
	listCancel()
	if err != nil {
		logx.Errorf("清理旧镜像前获取容器列表失败: %v", err)
		return "旧镜像清理检查失败（未确认是否仍被引用）: " + err.Error()
	}
	for _, c := range containers {
		if c.ImageID == oldImageID {
			// 仍被引用，不删
			return ""
		}
	}
	removeCtx, removeCancel := dockerTimeout(context.Background(), dockerMutationTimeout)
	defer removeCancel()
	if _, err := serviceContext.DockerClient.ImageRemove(removeCtx, oldImageID, image.RemoveOptions{}); err != nil {
		logx.Errorf("清理旧镜像失败（不影响更新结果）: %v", err)
		return "旧镜像清理失败: " + err.Error()
	}
	return ""
}

func decodePullResp(reader io.Reader, svcCtx *svc.ServiceContext, ctx context.Context, taskID string) (err error) {
	decoder := json.NewDecoder(reader)
	var oldTaskProgress, result = svcCtx.GetProgress(taskID)
	if !result {
		oldTaskProgress = svc.TaskProgress{
			Percentage: 0,
			Name:       "",
			Message:    "",
			DetailMsg:  "",
			IsDone:     false,
		}
	}
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var msg dockerMsgType.JSONMessage
		if err = decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				return nil
			}
			oldTaskProgress.Message = "拉取镜像失败"
			oldTaskProgress.DetailMsg = err.Error()
			oldTaskProgress.Percentage = 25
			svcCtx.UpdateProgress(taskID, oldTaskProgress)
			logx.Errorf("解析镜像拉取响应失败: %s", err)
			return fmt.Errorf("拉取镜像失败: %w", err)
		}
		if msg.Error != nil {
			oldTaskProgress.Message = "拉取镜像失败"
			oldTaskProgress.DetailMsg = msg.Error.Error()
			oldTaskProgress.Percentage = 25
			svcCtx.UpdateProgress(taskID, oldTaskProgress)
			logx.Errorf("registry 返回错误: %s", msg.Error)
			return fmt.Errorf("拉取镜像失败: %w", msg.Error)
		}
		var formattedMsg string
		if msg.Progress != nil {
			formattedMsg = fmt.Sprintf("进度%s: %s", msg.Status, msg.Progress.String())
		} else {
			formattedMsg = fmt.Sprintf("进度%s", msg.Status)
		}
		oldTaskProgress.DetailMsg = formattedMsg
		oldTaskProgress.Percentage = 25
		svcCtx.UpdateProgress(taskID, oldTaskProgress)
		logx.Infof("拉取镜像进度\t %s: %s\n", msg.Status, msg.Progress)
	}
}
