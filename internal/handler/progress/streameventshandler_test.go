package progress

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/rest/pathvar"
)

func TestStreamEventsHandlerSendsTerminalProgress(t *testing.T) {
	t.Setenv("PROGRESS_STORE", filepath.Join(t.TempDir(), "progress.json"))
	svcCtx := &svc.ServiceContext{ProgressStore: make(svc.ProgressStoreType)}
	svcCtx.UpdateProgress("task-1", svc.TaskProgress{
		TaskID:     "task-1",
		Percentage: 100,
		Message:    "done",
		Name:       "demo",
		IsDone:     true,
	})

	req := httptest.NewRequest("GET", "/api/progress/task-1/events", nil)
	req = pathvar.WithVars(req, map[string]string{"taskid": "task-1"})
	rec := httptest.NewRecorder()

	StreamEventsHandler(svcCtx)(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "retry: 3000") {
		t.Fatalf("missing SSE retry hint: %q", body)
	}
	if !strings.Contains(body, `"taskID":"task-1"`) || !strings.Contains(body, `"isDone":true`) {
		t.Fatalf("missing terminal progress payload: %q", body)
	}
}
