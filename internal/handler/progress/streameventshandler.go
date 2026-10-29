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

// StreamEventsHandler 以 Server-Sent Events 推送任务进度，替代前端高频轮询。
// 任务完成或客户端断开时流结束。
func StreamEventsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GetProgressReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		flusher, ok := w.(http.Flusher)
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
		flusher.Flush()

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		lastPayload := ""
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				progress, exists := svcCtx.GetProgress(req.TaskId)
				if !exists {
					fmt.Fprintf(w, "event: error\ndata: {\"msg\":\"taskID 未找到\"}\n\n")
					flusher.Flush()
					return
				}
				payload, err := json.Marshal(progress)
				if err != nil {
					continue
				}
				if string(payload) != lastPayload {
					fmt.Fprintf(w, "data: %s\n\n", payload)
					flusher.Flush()
					lastPayload = string(payload)
				}
				if progress.IsDone {
					return
				}
			}
		}
	}
}
