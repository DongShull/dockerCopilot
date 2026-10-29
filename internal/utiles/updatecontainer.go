package utiles

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	dockerMsgType "github.com/docker/docker/pkg/jsonmessage"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

// UpdateContainer 执行一次容器更新：拉取新镜像 → 停止并改名旧容器 → 克隆配置创建新容器
// → 启动新容器 → 删除旧容器 → 清理旧镜像。任一步失败都会尝试把旧容器恢复到原名并重新启动。
// ctx 支持取消：任务被取消时在步骤边界退出。
func UpdateContainer(serviceContext *svc.ServiceContext, ctx context.Context, id string, name string, imageNameAndTag string, delOldContainer bool, taskID string) error {
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

	serviceContext.DockerClient.NegotiateAPIVersion(ctx)

	// 先记录旧容器信息：旧镜像 ID 用于更新后的清理，旧容器名用于失败回滚
	inspectedContainer, err := serviceContext.DockerClient.ContainerInspect(ctx, id)
	if err != nil {
		progress(0, "获取容器信息失败", err.Error(), true)
		logx.Errorf("获取容器信息失败: %v", err)
		return err
	}
	oldImageID := inspectedContainer.Image
	oldContainerName := name
	backupName := name + "-" + time.Now().Format("2006-01-02-15-04-05.000000000")

	// rollback 把旧容器恢复到原名并重新启动；reason 写入进度详情
	rollback := func(reason string) {
		bg := context.Background()
		if renameErr := serviceContext.DockerClient.ContainerRename(bg, id, oldContainerName); renameErr != nil {
			logx.Errorf("回滚重命名失败: %v", renameErr)
			progress(100, "更新失败且回滚失败，请手动处理旧容器 "+backupName, reason, true)
			return
		}
		if startErr := serviceContext.DockerClient.ContainerStart(bg, id, container.StartOptions{}); startErr != nil {
			logx.Errorf("回滚启动失败: %v", startErr)
			progress(100, "更新失败，旧容器已改回原名但启动失败", reason+"；启动错误: "+startErr.Error(), true)
			return
		}
		progress(100, "更新失败，已回滚旧容器", reason, true)
	}

	// 1. 拉取新镜像（流式进度 10% → 25%）
	progress(10, "正在拉取新镜像", "正在拉取新镜像", false)
	reader, err := serviceContext.DockerClient.ImagePull(ctx, imageNameAndTag, image.PullOptions{})
	if err != nil {
		progress(100, "拉取镜像失败", err.Error(), true)
		logx.Errorf("Failed to pull image: %s", err)
		return err
	}
	if err = decodePullResp(reader, serviceContext, ctx, taskID); err != nil {
		progress(100, "拉取镜像失败", err.Error(), true)
		logx.Errorf("Failed to pull image: %s", err)
		return err
	}
	if cancelled() {
		return ctx.Err()
	}

	// 2. 停止旧容器
	progress(30, "正在停止容器", "正在停止容器", false)
	stopOptions := container.StopOptions{
		Signal:  signal,
		Timeout: &timeout,
	}
	if err = serviceContext.DockerClient.ContainerStop(ctx, id, stopOptions); err != nil {
		progress(100, "停止容器失败", err.Error(), true)
		return err
	}

	// 3. 重命名旧容器（腾出原名）
	progress(40, "正在重命名旧容器", "正在重命名旧容器", false)
	if err = serviceContext.DockerClient.ContainerRename(ctx, id, backupName); err != nil {
		// 还没改名成功，旧容器仍是原名，重新拉起即可
		rollback("重命名旧容器失败: " + err.Error())
		return err
	}

	// 4. 克隆旧容器配置创建新容器（只替换镜像）
	progress(60, "正在创建新容器", "正在创建新容器", false)
	inspectedContainer.Config.Hostname = ""
	inspectedContainer.Config.Image = imageNameAndTag
	inspectedContainer.Image = imageNameAndTag
	config := inspectedContainer.Config
	hostConfig := inspectedContainer.HostConfig
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: inspectedContainer.NetworkSettings.Networks,
	}
	if _, err = serviceContext.DockerClient.ContainerCreate(ctx, config, hostConfig, networkingConfig, nil, oldContainerName); err != nil {
		rollback("创建新容器失败: " + err.Error())
		return err
	}

	// 5. 启动新容器
	progress(80, "正在启动新容器", "正在启动新容器", false)
	if err = serviceContext.DockerClient.ContainerStart(ctx, oldContainerName, container.StartOptions{}); err != nil {
		// 新容器已创建但起不来：先移除它，再回滚旧容器
		_ = serviceContext.DockerClient.ContainerRemove(context.Background(), oldContainerName, container.RemoveOptions{Force: true})
		rollback("启动新容器失败: " + err.Error())
		return err
	}

	// 6. 删除旧容器（默认保留开关可关闭）
	if delOldContainer {
		if err = serviceContext.DockerClient.ContainerRemove(ctx, id, container.RemoveOptions{}); err != nil {
			// 旧容器删除失败不算更新失败，但需要告知
			logx.Errorf("删除旧容器失败: %v", err)
			progress(90, "更新成功（旧容器删除失败，请手动清理 "+backupName+"）", err.Error(), false)
		}
	}

	// 7. 清理旧镜像：仅当旧容器已删除、且旧镜像不再被任何容器引用时删除
	if delOldContainer && os.Getenv("CLEANUP_OLD_IMAGE") != "false" {
		progress(95, "正在清理旧镜像", "正在清理旧镜像", false)
		if cleanupMsg := cleanupOldImage(serviceContext, oldImageID); cleanupMsg != "" {
			progress(100, "更新成功", cleanupMsg, false)
		}
	}

	progress(100, "更新成功", "更新成功", true)
	return nil
}

// cleanupOldImage 在没有任何容器引用 oldImageID 时删除该镜像。
// 返回补充信息（清理失败原因），失败不影响更新结果。
func cleanupOldImage(serviceContext *svc.ServiceContext, oldImageID string) string {
	if oldImageID == "" {
		return ""
	}
	bg := context.Background()
	containers, err := serviceContext.DockerClient.ContainerList(bg, container.ListOptions{All: true})
	if err != nil {
		logx.Errorf("清理旧镜像前获取容器列表失败: %v", err)
		return ""
	}
	for _, c := range containers {
		if c.ImageID == oldImageID {
			// 仍被引用，不删
			return ""
		}
	}
	if _, err := serviceContext.DockerClient.ImageRemove(bg, oldImageID, image.RemoveOptions{}); err != nil {
		logx.Errorf("清理旧镜像失败（不影响更新结果）: %v", err)
		return "旧镜像清理失败: " + err.Error()
	}
	return "旧镜像已清理"
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
			oldTaskProgress.IsDone = true
			svcCtx.UpdateProgress(taskID, oldTaskProgress)
			logx.Errorf("Failed to decode pull image response: %s", err)
			return fmt.Errorf("拉取镜像失败: %w", err)
		}
		if msg.Error != nil {
			oldTaskProgress.Message = "拉取镜像失败"
			oldTaskProgress.DetailMsg = msg.Error.Error()
			oldTaskProgress.Percentage = 25
			oldTaskProgress.IsDone = true
			svcCtx.UpdateProgress(taskID, oldTaskProgress)
			logx.Errorf("Error: %s", msg.Error)
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
