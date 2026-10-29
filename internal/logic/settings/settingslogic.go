package settings

import (
	"context"

	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/onlyLTY/dockerCopilot/internal/utiles"

	"github.com/zeromicro/go-zero/core/logx"
)

type SettingsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSettingsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SettingsLogic {
	return &SettingsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SettingsLogic) GetSettings() (resp *types.Resp, err error) {
	resp = &types.Resp{}
	s := utiles.LoadSettings()
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]interface{}{
		"backupDir":          s.BackupDir,
		"effectiveBackupDir": utiles.EffectiveBackupDir(),
	}
	return resp, nil
}

func (l *SettingsLogic) UpdateSettings(req *types.UpdateSettingsReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	s := utiles.LoadSettings()
	s.BackupDir = req.BackupDir
	if saveErr := utiles.SaveSettings(s); saveErr != nil {
		resp.Code = 500
		resp.Msg = "保存设置失败: " + saveErr.Error()
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	utiles.AuditLog("settings.update", "settings", "backupDir="+req.BackupDir)
	resp.Code = 200
	resp.Msg = "success"
	resp.Data = map[string]interface{}{
		"backupDir":          s.BackupDir,
		"effectiveBackupDir": utiles.EffectiveBackupDir(),
	}
	return resp, nil
}
