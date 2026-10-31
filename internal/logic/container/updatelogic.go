package container

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/onlyLTY/dockerCopilot/internal/utiles"
	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateLogic {
	return &UpdateLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateLogic) Update(req *types.ContainerUpdateReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	taskID := uuid.New().String()
	taskCtx := l.svcCtx.RegisterTask(taskID)
	go func() {
		// Catch any panic and log the error
		defer func() {
			if r := recover(); r != nil {
				l.Errorf("更新容器任务 panic 已恢复: %v", r)
				msg := fmt.Sprintf("更新任务异常中断: %v", r)
				l.svcCtx.UpdateProgress(taskID, svc.TaskProgress{
					TaskID:     taskID,
					Percentage: 100,
					Name:       req.ContainerName,
					Message:    "更新失败",
					DetailMsg:  msg,
					IsDone:     true,
				})
				utiles.NotifyTaskDone("update", req.ContainerName, taskID, false, msg)
				utiles.AuditLog("container.update", req.ContainerName, msg)
			}
			l.svcCtx.FinishTask(taskID)
		}()
		imageNameAndTag := req.ImageNameAndTag
		delOldContainer := os.Getenv("DelOldContainer") != "false"
		err := utiles.UpdateContainer(l.svcCtx, taskCtx, req.Id, req.ContainerName, imageNameAndTag, delOldContainer, taskID)
		if err != nil {
			l.Errorf("更新容器出错: %v", err)
		}
		final, _ := l.svcCtx.GetProgress(taskID)
		detail := final.DetailMsg
		if detail == "" {
			detail = final.Message
		}
		utiles.NotifyTaskDone("update", req.ContainerName, taskID, err == nil, final.Message)
		utiles.AuditLog("container.update", req.ContainerName, detail)
	}()
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]string{"taskID": taskID}
	return resp, nil
}
