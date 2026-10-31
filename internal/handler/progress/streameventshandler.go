package progress

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

const (
	progressPollInterval      = 500 * time.Millisecond
	progressHeartbeatInterval = 15 * time.Second
	// progressStreamMaxDuration 单条 SSE 连接的最长时长。到时主动正常关闭，
	// 客户端 EventSource 会按 retry 间隔自动重连；避免异常客户端把连接无限占住。
	progressStreamMaxDuration = 30 * time.Minute
)

// StreamEventsHandler 以 Server-Sent Events 推送任务进度，替代前端高频轮询。
// 任务完成或客户端断开时流结束。无进度变化时定期发送注释行，避免代理
// 因长时间没有字节而断开连接。
func StreamEventsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GetProgressReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		if req.TaskId == "" {
			httpx.ErrorCtx(r.Context(), w, fmt.Errorf("taskID 不能为空"))
			return
		}

		_, ok := w.(http.Flusher)
		if !ok {
			httpx.ErrorCtx(r.Context(), w, fmt.Errorf("streaming unsupported"))
			return
		}

		header := w.Header()
		header.Set("Content-Type", "text/event-stream")
		header.Set("Cache-Control", "no-cache")
		header.Set("Connection", "keep-alive")
		header.Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		flush := func() bool {
			if err := http.NewResponseController(w).Flush(); err != nil {
				return false
			}
			return true
		}
		writeRaw := func(value string) bool {
			if _, err := fmt.Fprint(w, value); err != nil {
				return false
			}
			return flush()
		}

		// EventSource 断线后 3 秒重连；首个进度事件立即发送，不用等第一个 tick。
		if !writeRaw("retry: 3000\n\n") {
			return
		}

		lastPayload := ""
		sendProgress := func() (done bool, ok bool) {
			progress, exists := svcCtx.GetProgress(req.TaskId)
			if !exists {
				writeRaw("event: error\ndata: {\"msg\":\"taskID 未找到\"}\n\n")
				return true, false
			}
			payload, err := json.Marshal(progress)
			if err != nil {
				return false, true
			}
			if string(payload) != lastPayload {
				if !writeRaw("data: " + string(payload) + "\n\n") {
					return true, false
				}
				lastPayload = string(payload)
			}
			if progress.IsDone {
				return true, false
			}
			return false, true
		}

		if done, ok := sendProgress(); done || !ok {
			return
		}

		pollTicker := time.NewTicker(progressPollInterval)
		defer pollTicker.Stop()
		heartbeatTicker := time.NewTicker(progressHeartbeatInterval)
		defer heartbeatTicker.Stop()

		streamDeadline := time.After(progressStreamMaxDuration)
		for {
			select {
			case <-r.Context().Done():
				return
			case <-streamDeadline:
				// 到达单连接最长时长：发注释行告知后正常结束，
				// 客户端 EventSource 按 retry 间隔自动重连继续。
				_ = writeRaw(": stream deadline reached, reconnecting\n\n")
				return
			case <-heartbeatTicker.C:
				if !writeRaw(": keep-alive\n\n") {
					return
				}
			case <-pollTicker.C:
				done, ok := sendProgress()
				if done || !ok {
					return
				}
			}
		}
	}
}
