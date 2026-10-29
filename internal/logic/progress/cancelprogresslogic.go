package progress

import (
	"context"

	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CancelProgressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCancelProgressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CancelProgressLogic {
	return &CancelProgressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CancelProgressLogic) CancelProgress(req *types.GetProgressReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	progress, exists := l.svcCtx.GetProgress(req.TaskId)
	if !exists {
		resp.Code = 400
		resp.Msg = "taskID 未找到"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	if progress.IsDone {
		resp.Code = 400
		resp.Msg = "任务已结束，无需取消"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	if !l.svcCtx.CancelTask(req.TaskId) {
		resp.Code = 400
		resp.Msg = "任务不可取消（可能已结束）"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	progress.Message = "正在取消..."
	progress.DetailMsg = "已发出取消请求，任务将在当前步骤结束后停止"
	l.svcCtx.UpdateProgress(req.TaskId, progress)
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]interface{}{"taskID": req.TaskId}
	return resp, nil
}
