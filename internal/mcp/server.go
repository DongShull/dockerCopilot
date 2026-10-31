package mcp

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/server"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
)

// Version 是 MCP server 对外声明的版本，与程序版本保持一致。
const Version = "2.2.0"

// Register 把 dockerCopilot 的全部能力注册为 MCP 工具，并挂载到 /mcp（Streamable HTTP）。
// 与 /api/* 并存：HTTP API 继续给脚本和旧客户端用，MCP 给 Agent 用。
func Register(s *rest.Server, ctx *svc.ServiceContext) {
	mcpServer := NewServer(ctx)
	httpSrv := server.NewStreamableHTTPServer(mcpServer)
	handler := authMiddleware(ctx, httpSrv)

	s.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/mcp", Handler: handler},
		{Method: http.MethodPost, Path: "/mcp", Handler: handler},
		{Method: http.MethodDelete, Path: "/mcp", Handler: handler},
	})

	logx.Infof("MCP 已挂载: /mcp（Streamable HTTP），共 %d 个工具", len(mcpServer.ListTools()))
	if mcpAuthDisabled() {
		logx.Error("MCP 鉴权已通过 MCP_TOKEN=off 显式关闭，请确保 /mcp 不暴露在公网")
	}
}

// NewServer 构建并注册全部 MCP 工具，供 Register 与测试复用。
func NewServer(ctx *svc.ServiceContext) *server.MCPServer {
	mcpServer := server.NewMCPServer(
		"dockerCopilot",
		Version,
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)

	registerCommonTools(mcpServer, ctx)
	registerContainerTools(mcpServer, ctx)
	registerImageTools(mcpServer, ctx)
	registerBackupTools(mcpServer, ctx)
	registerProgressTools(mcpServer, ctx)
	registerSettingsTools(mcpServer, ctx)
	registerVersionTools(mcpServer, ctx)

	return mcpServer
}

// mcpAuthDisabled 允许用 MCP_TOKEN=off 显式关闭鉴权（仅建议在完全隔离的内网使用）。
func mcpAuthDisabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("MCP_TOKEN")))
	return v == "off" || v == "none" || v == "disabled"
}

// expectedToken 优先使用 MCP_TOKEN，未设置时退回 secretKey（与 /api 的 JWT 密钥同源）。
func expectedToken(ctx *svc.ServiceContext) string {
	if t := strings.TrimSpace(os.Getenv("MCP_TOKEN")); t != "" && !mcpAuthDisabled() {
		return t
	}
	return ctx.Config.Auth.AccessSecret
}

// authMiddleware 保护 /mcp：支持 Bearer、X-MCP-Token 和 ?token= 三种携带方式。
// MCP 客户端（Claude Code / Cursor / Cline 等）通常只支持自定义 Header 或 URL 参数，
// 因此三种都接受。
func authMiddleware(ctx *svc.ServiceContext, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if mcpAuthDisabled() {
			next.ServeHTTP(w, r)
			return
		}
		want := expectedToken(ctx)
		if want == "" {
			http.Error(w, `{"code":50000,"msg":"server secretKey 未配置，无法校验 MCP 请求"}`, http.StatusInternalServerError)
			return
		}
		if !tokenMatch(r, want) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":401,"msg":"unauthorized: 需要 Authorization: Bearer <token> 或 X-MCP-Token 或 ?token="}`))
			return
		}
		next.ServeHTTP(w, r)
	}
}

func tokenMatch(r *http.Request, want string) bool {
	candidates := []string{
		strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")),
		strings.TrimSpace(r.Header.Get("X-MCP-Token")),
		strings.TrimSpace(r.URL.Query().Get("token")),
	}
	for _, got := range candidates {
		if got == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1 {
			return true
		}
	}
	return false
}
