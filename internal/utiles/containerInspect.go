package utiles

import (
	"context"
	"github.com/docker/docker/api/types"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
)

func GetContainerInspect(ctx *svc.ServiceContext, id string) (types.ContainerJSON, error) {
	requestCtx, cancel := dockerTimeout(context.Background(), dockerInspectTimeout)
	defer cancel()
	inspectedContainer, err := ctx.DockerClient.ContainerInspect(requestCtx, id)
	if err != nil {
		return types.ContainerJSON{}, err
	}
	return inspectedContainer, nil
}
