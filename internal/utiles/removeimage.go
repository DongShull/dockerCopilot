package utiles

import (
	"context"
	"github.com/docker/docker/api/types/image"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
)

func RemoveImage(ctx *svc.ServiceContext, imageID string, force bool) error {
	requestCtx, cancel := dockerTimeout(context.Background(), dockerMutationTimeout)
	defer cancel()
	_, err := ctx.DockerClient.ImageRemove(requestCtx, imageID, image.RemoveOptions{Force: force})
	if err != nil {
		return err
	}
	return nil
}
