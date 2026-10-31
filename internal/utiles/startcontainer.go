package utiles

import (
	"context"
	"github.com/docker/docker/api/types/container"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
)

func StartContainer(ctx *svc.ServiceContext, id string) error {
	requestCtx, cancel := dockerTimeout(context.Background(), dockerMutationTimeout)
	defer cancel()
	startOptions := container.StartOptions{}
	err := ctx.DockerClient.ContainerStart(requestCtx, id, startOptions)
	if err != nil {
		return err
	}

	return nil
}
