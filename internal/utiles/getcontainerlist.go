package utiles

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	MyType "github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
)

func GetContainerList(ctx *svc.ServiceContext) ([]MyType.Container, error) {
	if ctx.DockerClient == nil {
		return nil, fmt.Errorf("Docker 客户端未初始化，请检查 DOCKER_HOST 配置")
	}
	// 获取所有容器（包括停止的容器）
	requestCtx, cancel := dockerTimeout(context.Background(), dockerListTimeout)
	defer cancel()
	dockerContainerList, err := ctx.DockerClient.ContainerList(requestCtx, container.ListOptions{
		All: true, // 设置为true来获取所有容器
	})
	if err != nil {
		logx.Errorf("获取容器列表失败: %v", err)
		return nil, err
	}
	var containerList []MyType.Container
	for _, dockerContainerInfo := range dockerContainerList {
		containerInfo := MyType.Container{
			Container: dockerContainerInfo,
		}
		containerList = append(containerList, containerInfo)
	}
	return containerList, nil
}

func CheckImageUpdate(ctx *svc.ServiceContext, containerListData []MyType.Container) []MyType.Container {
	for i, v := range containerListData {
		if info, ok := ctx.HubImageInfo.Get(v.ImageID); ok && info.NeedUpdate {
			containerListData[i].Update = true
		}
	}
	return containerListData
}
