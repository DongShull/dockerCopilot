package utiles

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"time"
)

// NotifyTaskDone 在任务收尾时向 NOTIFY_WEBHOOK_URL POST 一条 JSON 通知。
// 未配置 webhook 时静默跳过；发送失败不影响业务。
func NotifyTaskDone(kind, name, taskID string, ok bool, message string) {
	url := os.Getenv("NOTIFY_WEBHOOK_URL")
	if url == "" {
		return
	}
	payload := map[string]interface{}{
		"kind":    kind,
		"name":    name,
		"taskID":  taskID,
		"ok":      ok,
		"message": message,
		"time":    time.Now().Format("2006-01-02 15:04:05"),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}
