package handler

import (
	"encoding/json"
	"net/http"

	"github.com/onlyLTY/dockerCopilot/internal/config"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
)

// webindexHandler 在去前端后作为健康检查与能力说明入口。
// 容器 HEALTHCHECK 会请求 /，必须返回 200。
// 响应体经 json.Marshal 生成，避免配置值（含引号/反斜杠）破坏 JSON 结构；
// 不再暴露备份目录等本机路径，只保留版本与入口信息。
func webindexHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := svcInfoResponse{
			Code: http.StatusOK,
			Msg:  "dockerCopilot API-only mode",
			Data: svcInfoData{
				Version: config.Version,
				MCP:     "/mcp",
				API:     "/api",
			},
		}
		body, err := json.Marshal(resp)
		if err != nil {
			// Marshal 常量结构不会失败，防御性兜底保证健康检查始终 200。
			body = []byte(`{"code":200,"msg":"dockerCopilot API-only mode"}`)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}

type svcInfoResponse struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data svcInfoData `json:"data"`
}

type svcInfoData struct {
	Version string `json:"version"`
	MCP     string `json:"mcp"`
	API     string `json:"api"`
}
