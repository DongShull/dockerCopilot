package utiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type auditEntry struct {
	Time   string `json:"time"`
	Action string `json:"action"`
	Target string `json:"target"`
	Detail string `json:"detail"`
}

var auditMu sync.Mutex

func auditLogPath() string {
	if p := os.Getenv("AUDIT_LOG"); p != "" {
		return p
	}
	return "/data/logs/audit.jsonl"
}

// AuditLog 追加一条审计记录（JSON Lines），失败静默不影响业务。
func AuditLog(action, target, detail string) {
	entry := auditEntry{
		Time:   time.Now().Format("2006-01-02 15:04:05"),
		Action: action,
		Target: target,
		Detail: detail,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	auditMu.Lock()
	defer auditMu.Unlock()
	path := auditLogPath()
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)
	_, _ = f.Write(append(data, '\n'))
}
