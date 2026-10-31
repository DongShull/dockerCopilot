package version

import (
	"context"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/onlyLTY/dockerCopilot/internal/utiles"
	"github.com/zeromicro/go-zero/core/logx"
	"os"
	"time"
)

type UpdateProgramLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateProgramLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateProgramLogic {
	return &UpdateProgramLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateProgramLogic) UpdateProgram() (resp *types.Resp, err error) {
	resp = &types.Resp{}
	err = utiles.UpdateProgram(l.svcCtx)
	if err != nil {
		resp.Code = 500
		resp.Msg = err.Error()
		resp.Data = map[string]interface{}{}
		return resp, err
	}
	resp.Code = 200
	resp.Msg = "success"
	// 与 MCP 通道保持一致：自更新成功也要留审计与通知（进程 10 秒后退出，
	// 必须在退出前完成落盘）。
	utiles.AuditLog("program.update", "self", "program update triggered via HTTP API")
	utiles.NotifyTaskDone("program", "self", "", true, "程序自更新完成，服务即将重启换入新版本")
	go func() {
		time.Sleep(10 * time.Second)
		os.Exit(1)
	}()
	resp.Data = map[string]interface{}{}
	return resp, nil
}
