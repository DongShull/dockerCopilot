package utiles

import (
	dockerTypes "github.com/docker/docker/api/types"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	backupCompose "github.com/onlyLTY/dockerCopilot/internal/utiles/backup_compose"
	"github.com/zeromicro/go-zero/core/logx"
)

func Backup2Compose(ctx *svc.ServiceContext) (err error) {
	containerList, err := GetContainerList(ctx)
	if err != nil {
		return err
	}
	var containerJSONs []dockerTypes.ContainerJSON
	for _, v := range containerList {
		containerID := v.ID
		inspectedContainer, err := GetContainerInspect(ctx, containerID)
		if err != nil {
			logx.Error("获取容器信息失败" + err.Error())
			return err
		}
		containerJSONs = append(containerJSONs, inspectedContainer)
	}
	err = backupCompose.DockerConfig2ComposeYaml(containerJSONs, BackupDir())
	if err != nil {
		logx.Error("备份失败" + err.Error())
		return err
	}
	// 与 JSON 备份共用保留策略：超出 BACKUP_KEEP（默认 7）的旧 compose 备份自动清理。
	pruneOldBackups(BackupDir(), "backup-*.yaml", BackupKeepCount())
	return nil
}
