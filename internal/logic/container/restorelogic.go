package container

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/onlyLTY/dockerCopilot/internal/utiles"
	"path/filepath"

	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type RestoreLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRestoreLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RestoreLogic {
	return &RestoreLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RestoreLogic) Restore(req *types.ContainerRestoreReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	taskID := uuid.New().String()
	fileName := req.Filename
	fullPath, pathErr := utiles.ResolveBackupPath(fileName, ".json")
	if pathErr != nil {
		resp.Code = 400
		resp.Msg = pathErr.Error()
		resp.Data = map[string]interface{}{}
		return resp, pathErr
	}
	fileName = filepath.Base(fullPath)
	if filepath.Ext(fileName) != ".json" {
		err = fmt.Errorf("目前仅支持config备份恢复")
		resp.Code = 400
		resp.Msg = err.Error()
		resp.Data = map[string]interface{}{}
		return resp, err
	}
	taskCtx := l.svcCtx.RegisterTask(taskID)
	go func() {
		// Catch any panic and log the error
		defer func() {
			if r := recover(); r != nil {
				l.Errorf("恢复容器任务 panic 已恢复: %v", r)
				msg := fmt.Sprintf("恢复任务异常中断: %v", r)
				l.svcCtx.UpdateProgress(taskID, svc.TaskProgress{
					TaskID:     taskID,
					Percentage: 100,
					Name:       "恢复容器",
					Message:    "恢复失败",
					DetailMsg:  msg,
					IsDone:     true,
				})
				utiles.NotifyTaskDone("restore", fileName, taskID, false, msg)
				utiles.AuditLog("container.restore", fileName, msg)
			}
			l.svcCtx.FinishTask(taskID)
		}()
		err := utiles.RestoreContainer(taskCtx, l.svcCtx, fileName, taskID, req.Start, req.Force)
		if err != nil {
			l.Errorf("恢复容器出错: %v", err)
		}
		final, _ := l.svcCtx.GetProgress(taskID)
		message := final.Message
		if message == "" {
			message = "恢复任务已结束"
		}
		detail := final.DetailMsg
		if detail == "" {
			detail = message
		}
		utiles.NotifyTaskDone("restore", fileName, taskID, err == nil, message)
		utiles.AuditLog("container.restore", fileName, detail)
	}()
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]interface{}{
		"taskID": taskID,
	}
	return resp, nil
}
