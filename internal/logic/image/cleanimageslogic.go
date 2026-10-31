package image

import (
	"context"
	"strconv"

	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/onlyLTY/dockerCopilot/internal/utiles"

	"github.com/zeromicro/go-zero/core/logx"
)

type CleanImagesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCleanImagesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CleanImagesLogic {
	return &CleanImagesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// CleanImages 批量清理镜像：filter=untagged 删除无 TAG 镜像，
// filter=unused 删除未被任何容器引用的镜像。单个失败不影响其它。
func (l *CleanImagesLogic) CleanImages(req *types.RemoveImagesReq) (resp *types.Resp, err error) {
	resp = &types.Resp{}
	if req.Filter != utiles.CleanupFilterUntagged && req.Filter != utiles.CleanupFilterUnused {
		resp.Code = 400
		resp.Msg = "filter 参数错误，只支持 untagged（无 TAG 镜像）或 unused（未使用镜像）"
		resp.Data = map[string]interface{}{}
		return resp, nil
	}
	removed, failed, err := utiles.RemoveImagesFiltered(l.svcCtx, req.Filter)
	if err != nil {
		resp.Code = 500
		resp.Msg = err.Error()
		resp.Data = map[string]interface{}{}
		return resp, err
	}
	// 已删除/失败清单都进审计，删除操作不可恢复，必须留痕。
	utiles.AuditLog("image.cleanup", req.Filter,
		"removed="+strconv.Itoa(len(removed))+", failed="+strconv.Itoa(len(failed)))
	resp.Code = 200
	if len(failed) > 0 {
		resp.Msg = "清理完成，部分镜像删除失败"
	} else {
		resp.Msg = "success"
	}
	resp.Data = map[string]interface{}{
		"filter":  req.Filter,
		"removed": removed,
		"failed":  failed,
	}
	return resp, nil
}
