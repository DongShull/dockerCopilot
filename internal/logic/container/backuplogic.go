package container

import (
	"context"
	"fmt"
	"github.com/onlyLTY/dockerCopilot/internal/utiles"

	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type BackupLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewBackupLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BackupLogic {
	return &BackupLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *BackupLogic) Backup() (resp *types.Resp, err error) {
	resp = &types.Resp{}
	failures, err := utiles.BackupContainer(l.svcCtx)
	if err != nil {
		resp.Code = 500
		resp.Msg = err.Error()
		resp.Data = map[string]interface{}{"failures": failures}
		return resp, err
	}
	resp.Code = 200
	if len(failures) > 0 {
		resp.Msg = fmt.Sprintf("备份完成，%d 个容器失败", len(failures))
	} else {
		resp.Msg = "success"
	}
	resp.Data = map[string]interface{}{"failures": failures}
	return resp, nil
}
