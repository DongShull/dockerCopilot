package utiles

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	dockerTypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	dockerclient "github.com/docker/docker/client"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

// 恢复会按容器名做冲突判断，两个恢复任务并发时可能都认为“不存在”
// 然后同时创建同名容器；这里串行化整个恢复流程。
var restoreMu sync.Mutex

func restoreShouldStart(entry ContainerBackupEntry, startOverride *bool) bool {
	if startOverride != nil {
		return *startOverride
	}
	if entry.StateKnown {
		return entry.WasRunning
	}
	// 旧备份没有状态字段：保持之前“恢复后启动”的行为。
	return true
}

// restoreOldContainerName 为被替换的旧容器生成临时名。
// 恢复流程先把旧容器改名而不是直接删除，新容器创建/启动失败时才能完整回滚。
func restoreOldContainerName(name string, at time.Time) string {
	return name + "-dc-old-" + at.Format("20060102-150405.000000000")
}

// rollbackRestoredContainer 在 force 恢复失败时清理新容器并把旧容器恢复原名/原状态。
// oldTempName 为空表示旧容器尚未改名；newID 为空表示还没有创建新容器。
func rollbackRestoredContainer(serviceContext *svc.ServiceContext, oldID, oldName, oldTempName, newID string, wasRunning, wasPaused bool) string {
	var problems []string
	bg, cancel := dockerTimeout(context.Background(), dockerMutationTimeout)
	defer cancel()

	// 新容器可能还占着原名，必须先移除才能把旧容器改回去。
	if newID != "" {
		if err := serviceContext.DockerClient.ContainerRemove(bg, newID, container.RemoveOptions{Force: true}); err != nil {
			problems = append(problems, "新容器清理失败: "+err.Error())
		}
	}
	if oldTempName != "" {
		if err := serviceContext.DockerClient.ContainerRename(bg, oldID, oldName); err != nil {
			problems = append(problems, "旧容器改回原名失败: "+err.Error())
		}
	}
	if wasRunning {
		if wasPaused {
			_ = serviceContext.DockerClient.ContainerUnpause(bg, oldID)
		}
		current, inspectErr := serviceContext.DockerClient.ContainerInspect(bg, oldID)
		if inspectErr != nil || current.State == nil || !current.State.Running {
			if startErr := serviceContext.DockerClient.ContainerStart(bg, oldID, container.StartOptions{}); startErr != nil {
				problems = append(problems, "旧容器重新启动失败: "+startErr.Error())
			}
		}
		if wasPaused {
			if pauseErr := serviceContext.DockerClient.ContainerPause(bg, oldID); pauseErr != nil {
				problems = append(problems, "旧容器恢复暂停状态失败: "+pauseErr.Error())
			}
		}
	}
	if len(problems) == 0 {
		return ""
	}
	return "；回滚异常: " + strings.Join(problems, "；")
}

// inspectRestoreTarget 返回同名容器；不存在时返回 nil。
func restoreNetworkingConfig(config *network.NetworkingConfig) *network.NetworkingConfig {
	if config == nil || len(config.EndpointsConfig) == 0 {
		return config
	}
	return &network.NetworkingConfig{
		EndpointsConfig: sanitizeEndpointSettings(config.EndpointsConfig),
	}
}

func inspectRestoreTarget(ctx context.Context, serviceContext *svc.ServiceContext, name string) (*dockerTypes.ContainerJSON, error) {
	existing, err := serviceContext.DockerClient.ContainerInspect(ctx, name)
	if err != nil {
		if dockerclient.IsErrNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &existing, nil
}

// RestoreContainer 从备份 JSON 恢复容器。
// 默认按容器在备份时的运行状态恢复；startOverride 可强制启动/不启动。
// 同名容器已存在时默认跳过（镜像不一致则报错），force=true 时删除后重建。
func RestoreContainer(taskCtx context.Context, serviceContext *svc.ServiceContext, filename string, taskID string, startOverride *bool, force bool) error {
	progress := svc.TaskProgress{
		TaskID:     taskID,
		Percentage: 0,
		Message:    "准备恢复容器",
		Name:       "恢复容器",
		DetailMsg:  "",
		IsDone:     false,
	}
	if serviceContext.DockerClient == nil {
		progress.Message = "恢复失败"
		progress.DetailMsg = "Docker 客户端未初始化，请检查 DOCKER_HOST 配置"
		progress.IsDone = true
		serviceContext.UpdateProgress(taskID, progress)
		return fmt.Errorf("Docker 客户端未初始化，请检查 DOCKER_HOST 配置")
	}
	restoreMu.Lock()
	defer restoreMu.Unlock()
	fullPath, err := ResolveBackupPath(filename, ".json")
	if err != nil {
		logx.Errorf("解析备份文件路径失败: %v", err)
		progress.Message = "非法文件名"
		progress.DetailMsg = err.Error()
		progress.IsDone = true
		serviceContext.UpdateProgress(taskID, progress)
		return err
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		logx.Errorf("读取备份文件失败: %v", err)
		progress.Percentage = 0
		progress.Message = "读取文件失败或者未找到文件。请确认文件名仅由大小写字母、数字和短横线组成"
		progress.DetailMsg = err.Error()
		progress.IsDone = true
		serviceContext.UpdateProgress(taskID, progress)
		return err
	}
	// 新格式在 ContainerCreateConfig 之外带 stateKnown/wasRunning/wasPaused；
	// 旧格式（只有 Name/Config/HostConfig/...）同样能反序列化，状态按未知处理。
	var configList []ContainerBackupEntry
	if err = json.Unmarshal(content, &configList); err != nil {
		logx.Errorf("解析备份 JSON 失败: %v", err)
		progress.Percentage = 0
		progress.Message = "解析文件失败"
		progress.DetailMsg = err.Error()
		progress.IsDone = true
		serviceContext.UpdateProgress(taskID, progress)
		return err
	}
	if len(configList) == 0 {
		progress.Percentage = 100
		progress.Message = "备份文件中没有容器"
		progress.DetailMsg = "备份文件中没有容器"
		progress.IsDone = true
		serviceContext.UpdateProgress(taskID, progress)
		return fmt.Errorf("备份文件中没有容器")
	}

	negotiateCtx, negotiateCancel := dockerTimeout(taskCtx, dockerNegotiateTimeout)
	serviceContext.DockerClient.NegotiateAPIVersion(negotiateCtx)
	negotiateCancel()
	var failures []string
	var completed []string
	for i, entry := range configList {
		if err = taskCtx.Err(); err != nil {
			progress.Percentage = 100
			progress.Message = "任务已取消"
			progress.DetailMsg = "已取消恢复，已完成部分不会回滚"
			progress.IsDone = true
			serviceContext.UpdateProgress(taskID, progress)
			return err
		}
		if entry.Config == nil || strings.TrimSpace(entry.Config.Image) == "" {
			failures = append(failures, fmt.Sprintf("第 %d 个容器缺少镜像配置", i+1))
			continue
		}

		info := "正在恢复第" + strconv.Itoa(i+1) + "个容器"
		progress.Percentage = int(float64(i) / float64(len(configList)) * 90)
		progress.Message = info
		progress.DetailMsg = info
		serviceContext.UpdateProgress(taskID, progress)

		startDesired := restoreShouldStart(entry, startOverride)
		keepPaused := startOverride == nil && entry.StateKnown && entry.WasPaused
		hasName := strings.TrimSpace(entry.Name) != ""
		var replaceExisting *dockerTypes.ContainerJSON

		// 幂等恢复：同名容器已存在时，默认不重复创建。
		if hasName {
			existing, inspectErr := inspectRestoreTarget(taskCtx, serviceContext, entry.Name)
			if inspectErr != nil {
				failures = append(failures, entry.Name+" 检查同名容器失败: "+inspectErr.Error())
				continue
			}
			if existing != nil {
				if force {
					// 拉镜像成功后再停止/改名旧容器；创建或启动失败时还能完整回滚。
					replaceExisting = existing
				} else {
					if existing.Config != nil && existing.Config.Image != entry.Config.Image {
						failures = append(failures, entry.Name+" 已存在且镜像不一致（现有 "+existing.Config.Image+"，备份 "+entry.Config.Image+"），如需覆盖请使用 force=true")
						continue
					}
					if startDesired {
						if existing.State != nil && existing.State.Paused {
							if !keepPaused {
								if unpauseErr := serviceContext.DockerClient.ContainerUnpause(taskCtx, existing.ID); unpauseErr != nil {
									failures = append(failures, entry.Name+" 已存在但取消暂停失败: "+unpauseErr.Error())
									continue
								}
							}
						} else if existing.State == nil || !existing.State.Running {
							if startErr := serviceContext.DockerClient.ContainerStart(taskCtx, existing.ID, container.StartOptions{}); startErr != nil {
								failures = append(failures, entry.Name+" 已存在但启动失败: "+startErr.Error())
								continue
							}
						}
						if keepPaused && (existing.State == nil || !existing.State.Paused) {
							if pauseErr := serviceContext.DockerClient.ContainerPause(taskCtx, existing.ID); pauseErr != nil {
								failures = append(failures, entry.Name+" 已存在但暂停失败: "+pauseErr.Error())
								continue
							}
						}
					}
					completed = append(completed, entry.Name+" 已存在，跳过创建")
					progress.Percentage = int(float64(i+1) / float64(len(configList)) * 90)
					progress.DetailMsg = entry.Name + " 已存在，跳过创建"
					serviceContext.UpdateProgress(taskID, progress)
					continue
				}
			}
		}

		err = func() error {
			reader, pullErr := serviceContext.DockerClient.ImagePull(taskCtx, entry.Config.Image, image.PullOptions{})
			if pullErr != nil {
				return fmt.Errorf("拉取镜像失败: %w", pullErr)
			}
			defer func() {
				if closeErr := reader.Close(); closeErr != nil {
					logx.Errorf("关闭镜像拉取流失败: %v", closeErr)
				}
			}()
			if pullErr = decodePullResp(reader, serviceContext, taskCtx, taskID); pullErr != nil {
				return fmt.Errorf("拉取镜像失败: %w", pullErr)
			}
			return nil
		}()
		if err != nil {
			logx.Errorf("恢复容器 %s 拉取镜像失败: %v", entry.Name, err)
			failures = append(failures, entry.Name+" 拉取镜像失败: "+err.Error())
			continue
		}

		oldID := ""
		oldWasRunning := false
		oldWasPaused := false
		oldTempName := ""
		if replaceExisting != nil {
			if replaceExisting.State == nil {
				failures = append(failures, entry.Name+" 旧容器状态未知，无法安全执行 force 覆盖")
				continue
			}
			oldID = replaceExisting.ID
			oldWasRunning = replaceExisting.State.Running
			oldWasPaused = replaceExisting.State.Paused
			stopTimeout := 10
			if oldWasRunning {
				if oldWasPaused {
					if unpauseErr := serviceContext.DockerClient.ContainerUnpause(taskCtx, replaceExisting.ID); unpauseErr != nil {
						failures = append(failures, entry.Name+" 旧容器取消暂停失败: "+unpauseErr.Error())
						continue
					}
				}
				if stopErr := serviceContext.DockerClient.ContainerStop(taskCtx, replaceExisting.ID, container.StopOptions{Signal: "SIGTERM", Timeout: &stopTimeout}); stopErr != nil {
					rollbackMsg := rollbackRestoredContainer(serviceContext, replaceExisting.ID, entry.Name, "", "", oldWasRunning, oldWasPaused)
					failures = append(failures, entry.Name+" 旧容器停止失败: "+stopErr.Error()+rollbackMsg)
					continue
				}
			}
			oldTempName = restoreOldContainerName(entry.Name, time.Now())
			if renameErr := serviceContext.DockerClient.ContainerRename(taskCtx, replaceExisting.ID, oldTempName); renameErr != nil {
				rollbackMsg := rollbackRestoredContainer(serviceContext, replaceExisting.ID, entry.Name, "", "", oldWasRunning, oldWasPaused)
				failures = append(failures, entry.Name+" 旧容器改名失败: "+renameErr.Error()+rollbackMsg)
				continue
			}
		}

		created, createErr := serviceContext.DockerClient.ContainerCreate(
			taskCtx,
			entry.Config,
			entry.HostConfig,
			restoreNetworkingConfig(entry.NetworkingConfig),
			nil,
			entry.Name,
		)
		if createErr != nil {
			rollbackMsg := rollbackRestoredContainer(serviceContext, oldID, entry.Name, oldTempName, "", oldWasRunning, oldWasPaused)
			logx.Errorf("恢复容器 %s 创建失败: %v", entry.Name, createErr)
			failures = append(failures, entry.Name+" 创建失败: "+createErr.Error()+rollbackMsg)
			continue
		}
		if startDesired {
			if startErr := serviceContext.DockerClient.ContainerStart(taskCtx, created.ID, container.StartOptions{}); startErr != nil {
				rollbackMsg := rollbackRestoredContainer(serviceContext, oldID, entry.Name, oldTempName, created.ID, oldWasRunning, oldWasPaused)
				failures = append(failures, entry.Name+" 启动失败: "+startErr.Error()+rollbackMsg)
				continue
			}
			if keepPaused {
				if pauseErr := serviceContext.DockerClient.ContainerPause(taskCtx, created.ID); pauseErr != nil {
					rollbackMsg := rollbackRestoredContainer(serviceContext, oldID, entry.Name, oldTempName, created.ID, oldWasRunning, oldWasPaused)
					failures = append(failures, entry.Name+" 启动成功但暂停失败: "+pauseErr.Error()+rollbackMsg)
					continue
				}
			}
		}

		// 新容器已经创建并达到目标状态，才真正删除被替换的旧容器。
		result := entry.Name + " 恢复成功"
		if replaceExisting != nil && oldTempName != "" {
			cleanupCtx, cleanupCancel := dockerTimeout(context.Background(), dockerMutationTimeout)
			if removeErr := serviceContext.DockerClient.ContainerRemove(cleanupCtx, replaceExisting.ID, container.RemoveOptions{Force: true}); removeErr != nil {
				result = entry.Name + " 恢复成功，但旧容器清理失败，请手动删除 " + oldTempName + ": " + removeErr.Error()
			}
			cleanupCancel()
		}
		completed = append(completed, result)
		progress.Percentage = int(float64(i+1) / float64(len(configList)) * 90)
		progress.DetailMsg = result
		serviceContext.UpdateProgress(taskID, progress)
	}

	if err = taskCtx.Err(); err != nil {
		progress.Percentage = 100
		progress.Message = "任务已取消"
		progress.DetailMsg = "已取消恢复，已完成部分不会回滚"
		progress.IsDone = true
		serviceContext.UpdateProgress(taskID, progress)
		return err
	}

	progress.Percentage = 100
	details := append([]string{}, completed...)
	details = append(details, failures...)
	progress.DetailMsg = strings.Join(details, "；\n")
	if len(failures) > 0 {
		progress.Message = fmt.Sprintf("恢复完成，%d 个容器失败", len(failures))
		progress.IsDone = true
		serviceContext.UpdateProgress(taskID, progress)
		return fmt.Errorf("%d 个容器恢复失败: %s", len(failures), strings.Join(failures, "；"))
	}
	progress.Message = "恢复完成"
	progress.IsDone = true
	serviceContext.UpdateProgress(taskID, progress)
	return nil
}
