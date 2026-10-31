package utiles

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	dockerBackend "github.com/docker/docker/api/types/backend"
	"github.com/docker/docker/api/types/network"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

// BackupContainer 把全部容器配置备份为一个 JSON 文件。
// 单个容器 inspect 失败不中止整体备份，失败清单通过返回值带回；
// 全部容器都失败时才返回错误。
func BackupContainer(ctx *svc.ServiceContext) (failures []string, err error) {
	if ctx.DockerClient == nil {
		return nil, fmt.Errorf("Docker 客户端未初始化，请检查 DOCKER_HOST 配置")
	}
	containerList, err := GetContainerList(ctx)
	if err != nil {
		return nil, err
	}
	backupList := make([]ContainerBackupEntry, 0, len(containerList))
	failures = make([]string, 0)
	for i := range containerList {
		containerID := containerList[i].ID
		inspectCtx, cancel := dockerTimeout(context.Background(), dockerInspectTimeout)
		inspectedContainer, err := ctx.DockerClient.ContainerInspect(inspectCtx, containerID)
		cancel()
		if err != nil {
			// 一个坏容器不应该挡住其它容器的日常备份。
			logx.Errorf("容器 %s 获取信息失败，跳过备份: %v", containerID, err)
			failures = append(failures, containerID+": "+err.Error())
			continue
		}
		if inspectedContainer.Config == nil {
			logx.Errorf("容器 %s 配置为空，跳过备份", containerID)
			failures = append(failures, containerID+": 配置为空，无法备份")
			continue
		}
		containerName := strings.TrimPrefix(inspectedContainer.Name, "/")
		if containerName == "" && len(containerList[i].Names) > 0 {
			containerName = strings.TrimPrefix(containerList[i].Names[0], "/")
		}
		if containerName == "" {
			containerName = containerID
			if len(containerName) > 12 {
				containerName = containerName[:12]
			}
			logx.Errorf("容器 %s 名称为空，使用 ID 作为备份名称", containerID)
		}
		inspectedContainer.Config.Hostname = ""
		inspectedContainer.Image = inspectedContainer.Config.Image
		var endpoints map[string]*network.EndpointSettings
		if inspectedContainer.NetworkSettings != nil {
			endpoints = sanitizeEndpointSettings(inspectedContainer.NetworkSettings.Networks)
		}
		networkingConfig := &network.NetworkingConfig{
			EndpointsConfig: endpoints,
		}
		entry := ContainerBackupEntry{
			ContainerCreateConfig: dockerBackend.ContainerCreateConfig{
				Config:           inspectedContainer.Config,
				HostConfig:       inspectedContainer.HostConfig,
				NetworkingConfig: networkingConfig,
				Name:             containerName,
			},
		}
		if inspectedContainer.State != nil {
			entry.StateKnown = true
			entry.WasRunning = inspectedContainer.State.Running
			entry.WasPaused = inspectedContainer.State.Paused
		}
		backupList = append(backupList, entry)
	}
	if len(backupList) == 0 && len(failures) > 0 {
		return failures, fmt.Errorf("全部 %d 个容器备份失败，未生成备份文件", len(failures))
	}
	jsonData, err := json.MarshalIndent(backupList, "", "  ")
	if err != nil {
		logx.Error("数据序列化失败:", err)
		return failures, err
	}
	backupDir := BackupDir() // env BACKUP_DIR > Settings > 默认 /data/backups
	if backupDir == "" {
		backupDir = "/data/backups" // 如果环境变量未设置，使用默认值
	}
	_, err = os.Stat(backupDir)
	if os.IsNotExist(err) {
		err = os.MkdirAll(backupDir, 0755)
		if err != nil {
			logx.Errorf("创建备份目录失败: %v", err)
			return failures, err
		}
	}
	// 文件名带时间戳：同一天多次备份互不覆盖，可回退到任一次；
	// 超出保留份数（BACKUP_KEEP，默认 7）的旧备份自动清理。
	fileName := "backup-" + time.Now().Format("2006-01-02-150405") + ".json"
	fullPath := filepath.Join(backupDir, fileName)
	tmp, err := os.CreateTemp(backupDir, ".backup-*.json.tmp")
	if err != nil {
		logx.Errorf("创建备份临时文件失败: %v", err)
		return failures, err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if err = tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		logx.Errorf("设置备份临时文件权限失败: %v", err)
		return failures, err
	}
	if _, err = tmp.Write(jsonData); err != nil {
		_ = tmp.Close()
		logx.Errorf("写入备份临时文件失败: %v", err)
		return failures, err
	}
	if err = tmp.Close(); err != nil {
		logx.Errorf("关闭备份临时文件失败: %v", err)
		return failures, err
	}
	if err = os.Rename(tmpName, fullPath); err != nil {
		logx.Errorf("替换备份文件失败: %v", err)
		return failures, err
	}
	pruneOldBackups(backupDir, "backup-*.json", BackupKeepCount())
	return failures, nil
}
