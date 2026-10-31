package utiles

import (
	"context"
	"github.com/docker/docker/api/types/container"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
)

func StopContainer(ctx *svc.ServiceContext, id string) error {
	timeout := 10
	signal := "SIGTERM"
	stopOptions := container.StopOptions{
		Signal:  signal,
		Timeout: &timeout,
	}
	requestCtx, cancel := dockerTimeout(context.Background(), dockerMutationTimeout)
	defer cancel()
	err := ctx.DockerClient.ContainerStop(requestCtx, id, stopOptions)
	if err != nil {
		return err
	}
	return nil
}
