package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"github.com/onlyLTY/dockerCopilot/internal/config"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
)

func testContext() *svc.ServiceContext {
	cfg := config.Config{}
	cfg.Auth.AccessSecret = "test-secret-1234"
	return &svc.ServiceContext{Config: cfg}
}

var expectedTools = []string{
	"dc_ping", "dc_info",
	"dc_list_containers", "dc_inspect_container", "dc_start_container", "dc_stop_container",
	"dc_restart_container", "dc_rename_container", "dc_update_container", "dc_autoupdate_run",
	"dc_list_images", "dc_check_image_update", "dc_remove_image", "dc_clean_images",
	"dc_backup_containers", "dc_backup_compose", "dc_list_backups", "dc_restore_container", "dc_delete_backup",
	"dc_list_progress", "dc_get_progress", "dc_cancel_progress",
	"dc_get_settings", "dc_update_settings",
	"dc_get_version", "dc_check_program_update", "dc_update_program",
}

func TestAllToolsRegistered(t *testing.T) {
	s := NewServer(testContext())
	got := s.ListTools()
	for _, name := range expectedTools {
		if _, ok := got[name]; !ok {
			t.Errorf("tool %q not registered", name)
		}
	}
	if len(got) != len(expectedTools) {
		t.Errorf("registered %d tools, want %d", len(got), len(expectedTools))
	}
}

func TestAuthMiddleware(t *testing.T) {
	ctx := testContext()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := authMiddleware(ctx, next)

	cases := []struct {
		name   string
		modify func(*http.Request)
		want   int
	}{
		{"no token", func(r *http.Request) {}, http.StatusUnauthorized},
		{"wrong token", func(r *http.Request) { r.Header.Set("X-MCP-Token", "nope") }, http.StatusUnauthorized},
		{"bearer", func(r *http.Request) { r.Header.Set("Authorization", "Bearer test-secret-1234") }, http.StatusOK},
		{"x-mcp-token", func(r *http.Request) { r.Header.Set("X-MCP-Token", "test-secret-1234") }, http.StatusOK},
		{"query token", func(r *http.Request) {
			q := r.URL.Query()
			q.Set("token", "test-secret-1234")
			r.URL.RawQuery = q.Encode()
		}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			tc.modify(req)
			rec := httptest.NewRecorder()
			h(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

// TestInitializeAndListTools 通过真实 Streamable HTTP 传输验证握手与工具暴露。
func TestInitializeAndListTools(t *testing.T) {
	ctx := testContext()
	mcpServer := NewServer(ctx)
	h := authMiddleware(ctx, server.NewStreamableHTTPServer(mcpServer))
	ts := httptest.NewServer(h)
	defer ts.Close()

	token := "test-secret-1234"

	// 1. initialize
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	resp, header := postJSONRPC(t, ts.URL, token, "", initBody)
	if resp["id"] == nil {
		t.Fatalf("initialize 响应缺少 id: %v", resp)
	}
	sessionID := header.Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatalf("initialize 未返回 Mcp-Session-Id 头: %v", header)
	}

	// 2. notifications/initialized
	postJSONRPC(t, ts.URL, token, sessionID, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)

	// 3. tools/list
	listResp, _ := postJSONRPC(t, ts.URL, token, sessionID, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	result, ok := listResp["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list 响应缺少 result: %v", listResp)
	}
	tools, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list result 缺少 tools: %v", result)
	}
	names := make(map[string]bool, len(tools))
	for _, item := range tools {
		m, _ := item.(map[string]any)
		if n, _ := m["name"].(string); n != "" {
			names[n] = true
		}
	}
	for _, want := range expectedTools {
		if !names[want] {
			t.Errorf("tools/list 未包含 %q", want)
		}
	}
}

// TestMCPRequiresAuth 确认未带 token 时传输层直接 401。
func TestMCPRequiresAuth(t *testing.T) {
	ctx := testContext()
	h := authMiddleware(ctx, server.NewStreamableHTTPServer(NewServer(ctx)))
	ts := httptest.NewServer(h)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

// postJSONRPC 发送一条 JSON-RPC 消息并按需解析 JSON 或 SSE 响应。
func postJSONRPC(t *testing.T, baseURL, token, sessionID, body string) (map[string]any, http.Header) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, raw)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应失败: %v", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, resp.Header
	}
	return parsePayload(t, raw), resp.Header
}

func parsePayload(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	trimmed := bytes.TrimSpace(raw)
	if trimmed[0] == '{' {
		var m map[string]any
		if err := json.Unmarshal(trimmed, &m); err != nil {
			t.Fatalf("解析 JSON 失败: %v (%s)", err, trimmed)
		}
		return m
	}
	// SSE: data: {...}
	scanner := bufio.NewScanner(bytes.NewReader(trimmed))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var m map[string]any
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			continue
		}
		return m
	}
	t.Fatalf("无法从响应中解析 JSON-RPC 消息: %s", raw)
	return nil
}
