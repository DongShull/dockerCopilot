package version

import (
	"context"
	"errors"

	"github.com/onlyLTY/dockerCopilot/internal/config"

	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/onlyLTY/dockerCopilot/internal/utiles"
	"github.com/zeromicro/go-zero/core/logx"
)

type VersionLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewVersionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *VersionLogic {
	return &VersionLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *VersionLogic) Version(req *types.VersionReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	if req.Type == "local" {
		resp.Code = 200
		resp.Msg = "success"
		resp.Data = map[string]string{
			"version":   config.Version,
			"buildDate": config.BuildDate,
		}
		return resp, nil
	} else if req.Type == "remote" {
		remoteVersion, err := utiles.GetRemoteVersion()
		if errors.Is(err, utiles.ErrFnosManagedUpdate) {
			// FNOS 版本不走通用更新链：明确告知用户，而不是伪装成“无更新”。
			resp.Code = 200
			resp.Msg = err.Error()
			resp.Data = map[string]string{
				"remoteVersion": config.Version,
				"fnos":          "true",
			}
			return resp, nil
		}
		if err != nil {
			// 注意：resp.Code 会被 handler 当作 HTTP 状态码写入，业务码不能超过 999，
			// 否则 net/http 直接丢弃响应体（曾导致错误时返回空 500）。
			resp.Code = 500
			resp.Msg = "获取版本错误" + err.Error()
			resp.Data = map[string]string{
				"remoteVersion": config.Version,
			}
			return resp, nil
		} else if remoteVersion != config.Version {
			resp.Code = 200
			resp.Msg = "程序有更新"
			resp.Data = map[string]string{
				"remoteVersion": remoteVersion,
			}
			return resp, nil
		} else {
			resp.Code = 200
			resp.Msg = "程序无更新"
			resp.Data = map[string]string{
				"remoteVersion": remoteVersion,
			}
			return resp, nil
		}

	} else {
		resp.Code = 400
		resp.Msg = "type 参数错误"
		resp.Data = map[string]string{}
		return resp, nil
	}
}
